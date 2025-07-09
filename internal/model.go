// Package internal provides the core application model and state management for Migrate's TUI.
//
// This package implements the Bubble Tea model pattern for the interactive terminal user interface.
// The model handles:
//   - Application state management across different screens (main, backup, restore, verify, etc.)
//   - Message handling for user input, system events, and background operations
//   - Screen transitions and navigation logic
//   - Progress tracking for long-running operations (backup, restore, verification)
//   - Drive selection and mounting workflows
//   - Home folder selection for selective backups
//
// The main Model struct contains all UI state and implements the tea.Model interface
// for integration with the Bubble Tea framework.
package internal

import (
	"fmt"
	"io/ioutil"
	"migrate/internal/handlers"
	"migrate/internal/screens"
	"migrate/internal/state"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Model represents the complete application state for the Migrate TUI.
// It implements the tea.Model interface and contains all data needed to
// render screens and handle user interactions.
type Model struct {
	// Screen and navigation state
	screen     screens.Screen // Current active screen
	lastScreen screens.Screen // Previous screen for back navigation
	cursor     int            // Current cursor/selection position
	choices    []string       // Available menu options for current screen

	// Selection and confirmation state
	selected     map[int]struct{} // Multi-select state (legacy, may be unused)
	confirmation string           // Confirmation dialog text

	// Operation state
	progress  float64 // Progress percentage (0.0 to 1.0, or -1 for indeterminate)
	operation string  // Current operation identifier (e.g., "system_backup", "home_restore")
	message   string  // Status or error message to display
	canceling bool    // Flag indicating operation cancellation in progress

	// Display dimensions
	width  int // Terminal width for rendering
	height int // Terminal height for rendering

	// Drive management
	drives        []DriveInfo // List of available external drives
	selectedDrive string      // Currently selected drive path/mount point

	// Animation state
	cylonFrame int // Current frame number for progress bar animation (0-19)

	// Error handling
	errorRequiresManualDismissal bool // True for critical errors needing user acknowledgment

	// Home folder selection state (for selective backups)
	homeFolders     []HomeFolderInfo // Discovered home directory folders
	selectedFolders map[string]bool  // User's folder selections (path -> selected)
	totalBackupSize int64            // Calculated total size of selected content

	// NEW: Navigation state for sub-folder drilling
	currentFolderPath string                      // "" = root, "/Videos" = in Videos submenu
	folderBreadcrumb  []string                    // ["Home", "Videos"] for navigation
	subfolderCache    map[string][]HomeFolderInfo // Cache discovered subfolders

	// Restore options
	restoreConfig     bool // Restore ~/.config directory
	restoreWindowMgrs bool // Restore window managers (Hyprland, GNOME, etc.)
	
	// NEW: Track if user has already been through restore options
	restoreOptionsConfigured bool // True if user has already configured restore options

	// Restore folder selection state (for selective restores)
	restoreFolders         []HomeFolderInfo // Discovered folders from backup
	selectedRestoreFolders map[string]bool  // User's restore folder selections
	totalRestoreSize       int64            // Calculated total size of selected restore content

	// Verification error display
	verificationErrors []string // List of verification errors for display
	errorScrollOffset  int      // Current scroll position in error list
}

// InitialModel creates and returns a new Model instance with default values.
// This sets up the initial application state with the main menu active
// and initializes all required maps and default dimensions.
func InitialModel() Model {
	// Log initial model creation
	if logPath := getLogFilePath(); logPath != "" {
		if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			fmt.Fprintf(logFile, "\n=== MIGRATE DEBUG SESSION STARTED ===\n")
			fmt.Fprintf(logFile, "DEBUG: InitialModel() called, creating new model\n")
			fmt.Fprintf(logFile, "DEBUG: Log file path: %s\n", logPath)
			logFile.Close()
		}
	}
	return Model{
		screen:            screens.ScreenMain,
		choices:           screens.MainMenuChoices,
		selected:          make(map[int]struct{}),
		selectedFolders:   make(map[string]bool),
		subfolderCache:    make(map[string][]HomeFolderInfo), // NEW: Initialize subfolder cache
		restoreConfig:     true,                              // Default to true
		restoreWindowMgrs: true,                              // Default to true
		width:             100,
		height:            30,
	}
}

// Init implements tea.Model.Init() and returns any initial commands.
// Currently returns nil as no initialization commands are needed.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.Update() and handles all incoming messages.
// This is the central message router that processes user input, system events,
// background operation updates, and screen transitions.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Defensive terminal size handling
		m.width = msg.Width
		m.height = msg.Height

		// Ensure minimum usable dimensions
		if m.width < 80 {
			m.width = 80
		}
		if m.height < 24 {
			m.height = 24
		}

		// Cap maximum dimensions for consistent rendering
		if m.width > 200 {
			m.width = 200
		}
		if m.height > 60 {
			m.height = 60
		}

		return m, nil

	case DrivesLoaded:
		// Log when drives are loaded
		if logPath := getLogFilePath(); logPath != "" {
			if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
				fmt.Fprintf(logFile, "DEBUG: DrivesLoaded message received, %d drives found\n", len(msg.drives))
				for i, drive := range msg.drives {
					fmt.Fprintf(logFile, "DEBUG: Drive %d: %s (%s) - %s\n", i, drive.Device, drive.Label, drive.Size)
				}
				logFile.Close()
			}
		}
		m.drives = msg.drives
		m.choices = make([]string, len(m.drives)+1)
		for i, drive := range m.drives {
			m.choices[i] = fmt.Sprintf("💾 %s (%s) - %s", drive.Device, drive.Size, drive.Label)
		}
		m.choices[len(m.drives)] = "⬅️ Back"
		return m, nil

	case HomeFoldersDiscovered:
		if msg.error != nil {
			m.message = fmt.Sprintf("Failed to scan home directory: %v", msg.error)
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return tea.KeyMsg{Type: tea.KeyEsc}
			})
		}

		m.homeFolders = msg.folders
		m.cursor = 0 // Default to "Continue with selection" option

		// Load saved configuration to restore previous selections
		config, err := LoadSelectiveBackupConfig()
		if err != nil {
			// Failed to load config - use defaults (all visible folders selected)
			m.selectedFolders = make(map[string]bool)
			for _, folder := range m.homeFolders {
				if folder.IsVisible {
					m.selectedFolders[folder.Path] = true
				}
			}
			m.message = fmt.Sprintf("Using default selections (config load failed: %v)", err)
		} else {
			// Successfully loaded config - restore previous selections
			m.selectedFolders = make(map[string]bool)

			// Apply saved selections to discovered folders
			for _, folder := range m.homeFolders {
				if folder.IsVisible {
					// Check if we have a saved preference for this folder
					if savedSelection, exists := config.FolderSelections[folder.Path]; exists {
						m.selectedFolders[folder.Path] = savedSelection
					} else {
						// New folder not in saved config - default to selected
						m.selectedFolders[folder.Path] = true
					}
				} else {
					// Hidden folders are always "selected" but not shown in UI
					m.selectedFolders[folder.Path] = true
				}
			}

			// Restore cached subfolders from saved config
			if len(config.SubfolderCache) > 0 {
				m.subfolderCache = ConvertSavedSubfoldersToHomeFolders(config.SubfolderCache)

				// Apply saved subfolder selections
				for parentPath, subfolders := range m.subfolderCache {
					for i := range subfolders {
						if savedSelection, exists := config.FolderSelections[subfolders[i].Path]; exists {
							subfolders[i].Selected = savedSelection
							m.selectedFolders[subfolders[i].Path] = savedSelection
						}
					}
					m.subfolderCache[parentPath] = subfolders
				}
			}

			// Clean up any old selections for folders that no longer exist
			m.selectedFolders = CleanupOldSelections(m.selectedFolders)

			// Restore navigation state if saved (optional - user-friendly feature)
			if config.LastFolderPath != "" {
				m.currentFolderPath = config.LastFolderPath
				m.folderBreadcrumb = config.LastBreadcrumb
			}

			// Only show "Restored" message during restore operations, not backup operations
			if strings.Contains(m.operation, "restore") {
				selectedCount := 0
				for _, selected := range m.selectedFolders {
					if selected {
						selectedCount++
					}
				}
				m.message = fmt.Sprintf("✅ Restored previous folder selections (%d folders)", selectedCount)
			} else {
				// For backup operations, don't show confusing "Restored" message
				m.message = ""
			}
		}

		// Calculate initial total backup size
		m.calculateTotalBackupSize()

		return m, nil

	case SubfoldersDiscovered:
		if msg.error != nil {
			m.message = fmt.Sprintf("Failed to scan subfolder %s: %v", msg.parentPath, msg.error)
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return tea.KeyMsg{Type: tea.KeyEsc}
			})
		}

		// Cache the discovered subfolders for future navigation
		m.subfolderCache[msg.parentPath] = msg.subfolders

		// Update navigation state and switch to subfolder screen
		m.currentFolderPath = msg.parentPath
		m.folderBreadcrumb = []string{"Home", filepath.Base(msg.parentPath)}
		m.screen = screens.ScreenHomeSubfolderSelect
		m.cursor = 0

		return m, nil

	case RestoreFoldersDiscovered:
		if msg.error != nil {
			m.message = fmt.Sprintf("Failed to discover restore folders: %v", msg.error)
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return tea.KeyMsg{Type: tea.KeyEsc}
			})
		}

		// Store discovered folders and initialize selections
		m.restoreFolders = msg.folders
		if m.selectedRestoreFolders == nil {
			m.selectedRestoreFolders = make(map[string]bool)
		}

		// Default: select all folders for restore
		for _, folder := range m.restoreFolders {
			m.selectedRestoreFolders[folder.Path] = true
		}

		m.calculateTotalRestoreSize()
		return m, nil

	case PasswordRequiredMsg:
		// Exit the entire program to handle password, then restart
		return m, tea.Quit

	case passwordInteractionMsg:
		// Remove this - not needed anymore
		return m, nil

	case DriveOperation:
		if strings.Contains(msg.message, "LUKS drive is locked") ||
			strings.Contains(msg.message, "cryptsetup luksOpen") {
			// LUKS error - needs manual dismissal
			m.message = msg.message
			m.errorRequiresManualDismissal = true
			m.lastScreen = m.screen
			m.screen = screens.ScreenError
			return m, nil
		} else if msg.success {
			// Success message - needs manual dismissal
			m.message = msg.message
			m.lastScreen = m.screen
			m.screen = screens.ScreenComplete
			return m, nil
		} else {
			// Regular error message - auto-dismiss after 3 seconds
			m.message = msg.message
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return tea.KeyMsg{Type: tea.KeyEsc}
			})
		}

	case BackupDriveStatus:
		if msg.error != nil {
			// Write debug info
			debugFile := "/tmp/migrate_bds_error"
			ioutil.WriteFile(debugFile, []byte(fmt.Sprintf("BackupDriveStatus error: %v", msg.error)), 0644)
			
			// Check if this is a space requirement error (INSUFFICIENT SPACE) or LUKS error
			errorMsg := msg.error.Error()
			if strings.Contains(errorMsg, "LUKS drive is locked") ||
				strings.Contains(errorMsg, "cryptsetup luksOpen") ||
				strings.Contains(errorMsg, "INSUFFICIENT SPACE") ||
				strings.Contains(errorMsg, "too small") ||
				strings.Contains(errorMsg, "backup") {
				// Critical errors that need manual dismissal
				m.message = errorMsg
				m.errorRequiresManualDismissal = true
				m.lastScreen = m.screen
				m.screen = screens.ScreenError
				return m, nil
			} else {
				// Other errors - auto-dismiss after 3 seconds
				m.message = errorMsg
				return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
					return tea.KeyMsg{Type: tea.KeyEsc}
				})
			}
		} else {
			// Drive successfully mounted, confirm operation
			// Write debug info
			debugFile := "/tmp/migrate_bds_success"
			ioutil.WriteFile(debugFile, []byte(fmt.Sprintf("BackupDriveStatus success: drive=%s mountpoint=%s operation=%s", 
				msg.drivePath, msg.mountPoint, m.operation)), 0644)
			
			// Log to file instead of stdout
			if logPath := getLogFilePath(); logPath != "" {
				if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
					fmt.Fprintf(logFile, "DEBUG: Drive successfully mounted, operation: %s\n", m.operation)
					logFile.Close()
				}
			}
			if strings.Contains(m.operation, "backup") {
				// Backup confirmation
				backupTypeDesc := "ENTIRE SYSTEM"
				sourceSize := ""

				if m.operation == "home_backup" {
					backupTypeDesc = "HOME DIRECTORY"
					if m.totalBackupSize > 0 {
						sourceSize = fmt.Sprintf("Source: %s\n", FormatBytes(m.totalBackupSize))
					}
				} else {
					// For system backup, get used space on root filesystem
					if usedSpace, err := getUsedDiskSpace("/"); err == nil {
						sourceSize = fmt.Sprintf("Source: %s\n", FormatBytes(usedSpace))
					}
				}

				m.confirmation = fmt.Sprintf("Ready to backup %s\n\n%sDestination: %s (%s)\nType: %s\nMounted at: %s\n\nProceed with backup?",
					backupTypeDesc, sourceSize, msg.drivePath, msg.driveSize, msg.driveType, msg.mountPoint)
			} else if strings.Contains(m.operation, "restore") {
				// For restore, first detect backup type
				// Write debug info
				ioutil.WriteFile(debugFile+"_restore", []byte(fmt.Sprintf("Starting backup type detection for restore at: %s", msg.mountPoint)), 0644)
				
				// Log to file instead of stdout
				if logPath := getLogFilePath(); logPath != "" {
					if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
						fmt.Fprintf(logFile, "DEBUG: Starting backup type detection for restore at: %s\n", msg.mountPoint)
						logFile.Close()
					}
				}
				backupType, err := detectBackupType(msg.mountPoint)
				if err != nil {
					// Backup type detection failed - show error
					ioutil.WriteFile(debugFile+"_restore_error", []byte(fmt.Sprintf("Backup type detection failed: %v", err)), 0644)
					
					if logPath := getLogFilePath(); logPath != "" {
						if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
							fmt.Fprintf(logFile, "DEBUG: Backup type detection failed: %v\n", err)
							logFile.Close()
						}
					}
					errorMsg := fmt.Sprintf("❌ Invalid backup drive\n\nThis drive does not contain a valid migrate backup.\n\nError: %v\n\n💡 Make sure you selected the correct drive that contains your backup.", err)
					m.message = errorMsg
					m.errorRequiresManualDismissal = true
					m.lastScreen = m.screen
					m.screen = screens.ScreenError
					return m, nil
				}

				// Log to file instead of stdout
				ioutil.WriteFile(debugFile+"_restore_type", []byte(fmt.Sprintf("Backup type detected: %s", backupType)), 0644)
				
				if logPath := getLogFilePath(); logPath != "" {
					if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
						fmt.Fprintf(logFile, "DEBUG: Backup type detected: %s\n", backupType)
						logFile.Close()
					}
				}
				if backupType == "home" {
					// It's a home backup - check if we need folder selection or can proceed
					ioutil.WriteFile(debugFile+"_restore_home_backup", []byte("Home backup detected, checking restore flow"), 0644)
					
					if logPath := getLogFilePath(); logPath != "" {
						if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
							fmt.Fprintf(logFile, "DEBUG: Home backup detected, restoreOptionsConfigured: %v\n", m.restoreOptionsConfigured)
							logFile.Close()
						}
					}
					
					m.selectedDrive = msg.mountPoint
					
	// Always go to folder selection first for home backups
					ioutil.WriteFile(debugFile+"_restore_folder_selection", []byte("Home backup detected, going to folder selection"), 0644)
					
					if logPath := getLogFilePath(); logPath != "" {
						if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
							fmt.Fprintf(logFile, "DEBUG: Home backup detected, going to folder selection (restoreOptionsConfigured: %v)\n", m.restoreOptionsConfigured)
							logFile.Close()
						}
					}
					
					m.screen = screens.ScreenRestoreFolderSelect
					m.cursor = 0
					// Initialize restore folder state
					m.selectedRestoreFolders = make(map[string]bool)
					m.totalRestoreSize = 0
					// Start discovering folders from backup
					return m, DiscoverRestoreFoldersCmd(msg.mountPoint)
				}

				// System backup detected - proceed with confirmation
				ioutil.WriteFile(debugFile+"_restore_system_backup", []byte("System backup detected, showing confirmation dialog"), 0644)
				
				if logPath := getLogFilePath(); logPath != "" {
					if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
						fmt.Fprintf(logFile, "DEBUG: System backup detected, showing confirmation dialog\n")
						logFile.Close()
					}
				}

				// CRITICAL SPACE CHECK: Verify internal drive has enough space for system restore
				err = checkRestoreSpaceRequirements("", msg.mountPoint)
				if err != nil {
					// Show the space error immediately - don't proceed to confirmation
					m.message = err.Error()
					m.errorRequiresManualDismissal = true
					m.lastScreen = m.screen
					m.screen = screens.ScreenError
					return m, nil
				}

				// Space check passed - proceed with system restore confirmation
				restoreTypeDesc := "ENTIRE SYSTEM"
				if m.operation == "custom_restore" {
					restoreTypeDesc = "CUSTOM PATH"
				}

				m.confirmation = fmt.Sprintf("Ready to restore %s\n\nSource: %s (%s)\nType: %s\nMounted at: %s\n\n⚠️ This will OVERWRITE existing files!\n\nProceed with restore?",
					restoreTypeDesc, msg.drivePath, msg.driveSize, msg.driveType, msg.mountPoint)
			} else if strings.Contains(m.operation, "verify") || m.operation == "auto_verify" {
				// Verification confirmation
				verifyTypeDesc := "AUTO-DETECTED BACKUP"

				m.confirmation = fmt.Sprintf("Ready to verify %s\n\nBackup Source: %s (%s)\nType: %s\nMounted at: %s\n\n🔍 This will auto-detect backup type and compare backup files with your current system\n\nProceed with verification?",
					verifyTypeDesc, msg.drivePath, msg.driveSize, msg.driveType, msg.mountPoint)
			}

			m.selectedDrive = msg.mountPoint // Store mount point for operation
			m.screen = screens.ScreenConfirm
			m.cursor = 0
			
			// Write debug info about confirmation screen
			ioutil.WriteFile(debugFile+"_confirmation", []byte(fmt.Sprintf("Set screen to ScreenConfirm, stored mountPoint: %s", msg.mountPoint)), 0644)
			
			return m, nil
		}

	case ProgressUpdate:
		if msg.Error != nil {
			// Check error type for appropriate handling
			errorMsg := fmt.Sprintf("Error: %v", msg.Error)

			// Check for verification-specific completion (success with warnings/failures)
			if strings.Contains(m.operation, "verify") &&
				(strings.Contains(errorMsg, "VERIFICATION_DETAILED_ERRORS:") ||
					strings.Contains(errorMsg, "verification failed with") ||
					strings.Contains(errorMsg, "errors (threshold:") ||
					strings.Contains(errorMsg, "systematic") ||
					strings.Contains(errorMsg, "integrity issues")) {
				// Verification completed but found issues - show detailed results
				if strings.Contains(errorMsg, "VERIFICATION_DETAILED_ERRORS:") {
					// Copy verification errors to model for detailed display
					m.verificationErrors = GetVerificationErrors()
					m.errorScrollOffset = 0
					m.screen = screens.ScreenVerificationErrors
					m.progress = 0
					m.canceling = false
					return m, nil
				} else {
					// Legacy error handling
					m.message = errorMsg
					m.errorRequiresManualDismissal = true
					m.lastScreen = m.screen
					m.screen = screens.ScreenError
					m.progress = 0
					m.canceling = false
					return m, nil
				}
			}

			// Check for critical system errors that need manual dismissal
			if strings.Contains(errorMsg, "cryptsetup luksOpen") ||
				strings.Contains(errorMsg, "LUKS drive is locked") ||
				strings.Contains(errorMsg, "No such file or directory") ||
				strings.Contains(errorMsg, "permission denied") ||
				strings.Contains(errorMsg, "cannot determine backup type") ||
				strings.Contains(errorMsg, "no valid backup found") ||
				strings.Contains(errorMsg, "error 32") {
				// Critical system error - needs manual dismissal
				m.message = errorMsg
				m.errorRequiresManualDismissal = true
				m.lastScreen = m.screen
				m.screen = screens.ScreenError
				m.progress = 0
				m.canceling = false
				return m, nil
			} else {
				// Regular error - show but continue normal flow
				m.message = errorMsg
				m.progress = 0
				m.canceling = false // Reset canceling state on error
			}
		} else {
			// Only update progress if we're not canceling
			if !m.canceling {
				m.progress = msg.Percentage
				m.message = msg.Message
			}
		}

		if msg.Done || m.canceling {
			// Reset canceling state when operation completes
			wasCanceling := m.canceling
			m.canceling = false

			if wasCanceling {
				// Operation was canceled
				m.message = "Operation canceled by user"
				return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
					return tea.KeyMsg{Type: tea.KeyEsc}
				})
			}

			// Check if this was a backup operation completion
			if strings.Contains(m.operation, "backup") && msg.Error == nil {
				// Backup completed successfully, ask about unmounting
				m.confirmation = "🎉 Backup completed successfully!\n\nDo you want to unmount the backup drive?\n\nNote: Unmounting is recommended for safe removal."
				m.operation = "unmount_backup"
				m.screen = screens.ScreenConfirm
				m.cursor = 1
				return m, nil
			} else if msg.Error == nil {
				// Other operation completed successfully - show completion screen
				m.lastScreen = m.screen
				m.screen = screens.ScreenComplete
				return m, nil
			} else {
				// Operation completed with error
				errorMsg := fmt.Sprintf("Error: %v", msg.Error)

				// Check for verification-specific completion with detected issues
				if strings.Contains(m.operation, "verify") &&
					(strings.Contains(errorMsg, "VERIFICATION_DETAILED_ERRORS:") ||
						strings.Contains(errorMsg, "verification failed with") ||
						strings.Contains(errorMsg, "errors (threshold:") ||
						strings.Contains(errorMsg, "systematic") ||
						strings.Contains(errorMsg, "integrity issues")) {
					// Verification found issues - show detailed error screen
					if strings.Contains(errorMsg, "VERIFICATION_DETAILED_ERRORS:") {
						// Copy verification errors to model for detailed display
						m.verificationErrors = GetVerificationErrors()
						m.errorScrollOffset = 0
						m.screen = screens.ScreenVerificationErrors
						m.progress = 0
						m.canceling = false
						return m, nil
					} else {
						// Legacy error handling
						m.message = errorMsg
						m.errorRequiresManualDismissal = true
						m.lastScreen = m.screen
						m.screen = screens.ScreenError
						return m, nil
					}
				}

				// Check for critical system errors
				if strings.Contains(errorMsg, "cryptsetup luksOpen") ||
					strings.Contains(errorMsg, "LUKS drive is locked") ||
					strings.Contains(errorMsg, "No such file or directory") ||
					strings.Contains(errorMsg, "permission denied") ||
					strings.Contains(errorMsg, "cannot determine backup type") ||
					strings.Contains(errorMsg, "no valid backup found") ||
					strings.Contains(errorMsg, "error 32") {
					// Critical system error - needs manual dismissal
					m.message = errorMsg
					m.errorRequiresManualDismissal = true
					m.lastScreen = m.screen
					m.screen = screens.ScreenError
					return m, nil
				} else {
					// Regular error - auto-dismiss after 3 seconds
					return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
						return tea.KeyMsg{Type: tea.KeyEsc}
					})
				}
			}
		} else {
			// NOT DONE - Schedule next progress update (unless canceling)
			if !m.canceling {
				return m, CheckTUIBackupProgress()
			}
		}

		return m, nil

	case tickMsg:
		// Remove fake progress simulation
		return m, nil

	case state.CylonAnimateMsg:
		// Update cylon animation frame
		m.cylonFrame = (m.cylonFrame + 1) % 20 // 20-frame cycle
		if m.screen == screens.ScreenProgress {
			// Keep animating while on progress screen
			return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
				return state.CylonAnimateMsg{}
			})
		}
		return m, nil

	case tea.KeyMsg:
		// Handle error screen dismissal first
		if m.screen == screens.ScreenError {
			// Any key press dismisses the error screen and returns to main menu
			resetBackupState()
			m.screen = screens.ScreenMain
			m.message = ""
			m.cursor = 0
			m.choices = screens.MainMenuChoices
			m.errorRequiresManualDismissal = false
			m.restoreOptionsConfigured = false // Reset restore flow state
			return m, nil
		}

		// Handle completion screen dismissal
		if m.screen == screens.ScreenComplete {
			// Any key press dismisses the completion screen and returns to main
			resetBackupState()
			m.screen = screens.ScreenMain
			m.message = ""
			m.cursor = 0
			m.choices = screens.MainMenuChoices
			m.restoreOptionsConfigured = false // Reset restore flow state
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			if m.screen == screens.ScreenMain {
				return m, tea.Quit
			}
			// Handle Ctrl+C during progress - set canceling state
			if m.screen == screens.ScreenProgress {
				m.canceling = true
				m.message = "Canceling operation... Please wait for cleanup to complete."
				// Signal the backup operation to cancel
				CancelBackup()
				// Continue to let the progress update handle the cleanup
				return m, nil
			}
			// Go back to main menu from other screens
			m.screen = screens.ScreenMain
			m.cursor = 0
			m.choices = screens.MainMenuChoices
			m.restoreOptionsConfigured = false // Reset restore flow state
			return m, nil

		case "esc":
			if m.screen == screens.ScreenError {
				// Return to main menu from error
				resetBackupState()
				m.screen = screens.ScreenMain
				m.message = ""
				m.cursor = 0
				m.choices = screens.MainMenuChoices
				m.errorRequiresManualDismissal = false
				return m, nil
			} else if m.screen == screens.ScreenHomeSubfolderSelect {
				// NEW: Return to parent folder view from subfolder screen

				m.currentFolderPath = ""
				m.folderBreadcrumb = []string{}
				m.screen = screens.ScreenHomeFolderSelect
				m.cursor = 0
				m.message = "" // Clear any temporary messages
				return m, nil
			} else if m.screen == screens.ScreenRestoreFolderSelect {
				// Return to restore menu from folder selection
				resetBackupState()
				m.screen = screens.ScreenRestore
				m.cursor = 0
				m.choices = screens.RestoreMenuChoices
				m.selectedRestoreFolders = make(map[string]bool)
				m.totalRestoreSize = 0
				return m, nil
			} else if m.screen == screens.ScreenVerificationErrors {
				// NEW: Return to main menu from verification errors screen
				resetBackupState()
				m.screen = screens.ScreenMain
				m.cursor = 0
				m.choices = screens.MainMenuChoices
				m.verificationErrors = []string{} // Clear error list
				m.errorScrollOffset = 0
				return m, nil
			} else if m.screen != screens.ScreenMain {
				// Reset backup state when returning to main menu
				resetBackupState()
				m.screen = screens.ScreenMain
				m.cursor = 0
				m.choices = screens.MainMenuChoices
				m.restoreOptionsConfigured = false // Reset restore flow state
			}
			return m, nil

		case "up", "k":
			if m.screen == screens.ScreenConfirm {
				if m.cursor > 0 {
					m.cursor--
				}
			} else if m.screen == screens.ScreenVerificationErrors {
				// Scroll up in verification errors list
				if m.errorScrollOffset > 0 {
					m.errorScrollOffset--
				}
				return m, nil
			} else if m.screen == screens.ScreenMain {
				// Main menu: wrap around navigation
				if m.cursor > 0 {
					m.cursor--
				} else {
					m.cursor = len(m.choices) - 1 // Wrap to bottom
				}
			} else if m.screen == screens.ScreenHomeFolderSelect {
				// Home folder selection: NEW LAYOUT with controls at top
				// Cursor 0-1: Controls (Continue, Back)
				// Cursor 2+: Folders (non-empty only)
				numControls := 2
				visibleFolders := m.getVisibleFoldersNonEmpty()
				maxCursor := numControls + len(visibleFolders) - 1
				if m.cursor > 0 {
					m.cursor--
				} else {
					m.cursor = maxCursor // Wrap to bottom
				}
			} else if m.screen == screens.ScreenHomeSubfolderSelect {
				// NEW: Subfolder selection navigation
				// Cursor 0-1: Controls (Continue, Back)
				// Cursor 2+: Subfolders (non-empty only)
				numControls := 2
				subfolders := m.getCurrentSubfolders()
				maxCursor := numControls + len(subfolders) - 1
				if m.cursor > 0 {
					m.cursor--
				} else {
					m.cursor = maxCursor // Wrap to bottom
				}
			} else if m.screen == screens.ScreenRestoreFolderSelect {
				// Restore folder selection navigation (FIXED: Separate config and folders)
				numControls := 2       // Continue, Back
				numConfigItems := 2    // Configuration, Window Managers
				visibleFolders := m.getVisibleRestoreFolders()
				maxCursor := numControls + numConfigItems + len(visibleFolders) - 1
				if m.cursor > 0 {
					m.cursor--
				} else {
					m.cursor = maxCursor // Wrap to bottom
				}
			} else if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j":
			if m.screen == screens.ScreenConfirm {
				if m.cursor < 1 {
					m.cursor++
				}
			} else if m.screen == screens.ScreenVerificationErrors {
				// Scroll down in verification errors list
				contentHeight := m.height - 10 // Match the UI calculation
				contentHeight = max(contentHeight, 3)
				// Cap at 12 to match the display limit in renderVerificationErrors
				contentHeight = min(contentHeight, 12)
				maxScrollOffset := len(m.verificationErrors) - contentHeight
				if maxScrollOffset > 0 && m.errorScrollOffset < maxScrollOffset {
					m.errorScrollOffset++
				}
				return m, nil
			} else if m.screen == screens.ScreenMain {
				// Main menu: wrap around navigation
				if m.cursor < len(m.choices)-1 {
					m.cursor++
				} else {
					m.cursor = 0 // Wrap to top
				}
			} else if m.screen == screens.ScreenHomeFolderSelect {
				// Home folder selection: NEW LAYOUT with controls at top
				// Cursor 0-1: Controls (Continue, Back)
				// Cursor 2+: Folders (non-empty only)
				numControls := 2
				visibleFolders := m.getVisibleFoldersNonEmpty()
				maxCursor := numControls + len(visibleFolders) - 1
				if m.cursor < maxCursor {
					m.cursor++
				} else {
					m.cursor = 0 // Wrap to top
				}
			} else if m.screen == screens.ScreenHomeSubfolderSelect {
				// NEW: Subfolder selection navigation
				// Cursor 0-1: Controls (Continue, Back)
				// Cursor 2+: Subfolders (non-empty only)
				numControls := 2
				subfolders := m.getCurrentSubfolders()
				maxCursor := numControls + len(subfolders) - 1
				if m.cursor < maxCursor {
					m.cursor++
				} else {
					m.cursor = 0 // Wrap to top
				}
			} else if m.screen == screens.ScreenRestoreFolderSelect {
				// Restore folder selection navigation (FIXED: Separate config and folders)
				numControls := 2       // Continue, Back
				numConfigItems := 2    // Configuration, Window Managers
				visibleFolders := m.getVisibleRestoreFolders()
				maxCursor := numControls + numConfigItems + len(visibleFolders) - 1
				if m.cursor < maxCursor {
					m.cursor++
				} else {
					m.cursor = 0 // Wrap to top
				}
			} else if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
			return m, nil

		case "enter", " ":
			return m.handleSelection()

		case "a", "A":
			if m.screen == screens.ScreenHomeFolderSelect {
				// Select all visible NON-EMPTY folders
				visibleFolders := m.getVisibleFoldersNonEmpty()
				for _, folder := range visibleFolders {
					m.selectedFolders[folder.Path] = true
				}
				m.calculateTotalBackupSize()
				m.autoSaveSelections() // Auto-save when user selects all
			} else if m.screen == screens.ScreenRestoreFolderSelect {
				// Select all visible folders for restore
				for _, folder := range m.restoreFolders {
					if folder.IsVisible && !folder.AlwaysInclude {
						m.selectedRestoreFolders[folder.Path] = true
					}
				}
				m.calculateTotalRestoreSize()
			}
			return m, nil

		case "n", "N", "x", "X":
			if m.screen == screens.ScreenHomeFolderSelect {
				// Deselect all visible NON-EMPTY folders
				visibleFolders := m.getVisibleFoldersNonEmpty()
				for _, folder := range visibleFolders {
					m.selectedFolders[folder.Path] = false
				}
				m.calculateTotalBackupSize()
				m.autoSaveSelections() // Auto-save when user deselects all
			} else if m.screen == screens.ScreenRestoreFolderSelect {
				// Deselect all visible folders for restore
				for _, folder := range m.restoreFolders {
					if folder.IsVisible && !folder.AlwaysInclude {
						m.selectedRestoreFolders[folder.Path] = false
					}
				}
				m.calculateTotalRestoreSize()
			}
			return m, nil

		case "pgup":
			if m.screen == screens.ScreenVerificationErrors {
				// Page up in verification errors list
				contentHeight := m.height - 10 // Match the UI calculation
				contentHeight = max(contentHeight, 3)
				// Cap at 12 to match the display limit in renderVerificationErrors
				contentHeight = min(contentHeight, 12)
				m.errorScrollOffset -= contentHeight
				if m.errorScrollOffset < 0 {
					m.errorScrollOffset = 0
				}
			}
			return m, nil

		case "pgdown":
			if m.screen == screens.ScreenVerificationErrors {
				// Page down in verification errors list
				contentHeight := m.height - 10 // Match the UI calculation
				contentHeight = max(contentHeight, 3)
				// Cap at 12 to match the display limit in renderVerificationErrors
				contentHeight = min(contentHeight, 12)
				maxScrollOffset := len(m.verificationErrors) - contentHeight
				if maxScrollOffset > 0 {
					m.errorScrollOffset += contentHeight
					if m.errorScrollOffset > maxScrollOffset {
						m.errorScrollOffset = maxScrollOffset
					}
				}
			}
			return m, nil

		case "home":
			if m.screen == screens.ScreenVerificationErrors {
				// Jump to top of error list
				m.errorScrollOffset = 0
			}
			return m, nil

		case "end":
			if m.screen == screens.ScreenVerificationErrors {
				// Jump to bottom of error list
				contentHeight := m.height - 10 // Match the UI calculation
				contentHeight = max(contentHeight, 3)
				// Cap at 12 to match the display limit in renderVerificationErrors
				contentHeight = min(contentHeight, 12)
				maxScrollOffset := len(m.verificationErrors) - contentHeight
				if maxScrollOffset > 0 {
					m.errorScrollOffset = maxScrollOffset
				}
			}
			return m, nil
		}
	}

	return m, nil
}

// handleSelection processes menu selections and handles screen transitions.
// This method implements the navigation logic for all interactive screens,
// managing state changes and initiating background operations as needed.

// handleMainMenuSelection handles selection logic for the main menu screen
// handleMainMenuSelection processes main menu selections and transitions to appropriate screens.
func (m Model) handleMainMenuSelection() (tea.Model, tea.Cmd) {
	// Log main menu selection
	if logPath := getLogFilePath(); logPath != "" {
		if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			fmt.Fprintf(logFile, "DEBUG: handleMainMenuSelection called, cursor: %d\n", m.cursor)
			logFile.Close()
		}
	}
	handler := handlers.NewMainMenuHandler()
	screen, operation, choices, cmd := handler.HandleSelection(m.cursor)

	m.screen = screen
	if operation != "" {
		m.operation = operation
	}
	if choices != nil {
		m.choices = choices
		m.cursor = 0
	}

	// Log the result of main menu selection
	if logPath := getLogFilePath(); logPath != "" {
		if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			fmt.Fprintf(logFile, "DEBUG: Main menu selection result - screen: %v, operation: %s\n", screen, operation)
			logFile.Close()
		}
	}

	if cmd != nil {
		return m, cmd
	}
	return m, nil
}

// handleBackupMenuSelection handles selection logic for the backup menu screen
// handleBackupMenuSelection processes backup menu selections and transitions to appropriate screens.
func (m Model) handleBackupMenuSelection() (tea.Model, tea.Cmd) {
	handler := handlers.NewBackupMenuHandler()
	screen, operation, choices, _ := handler.HandleSelection(m.cursor)

	m.screen = screen
	if operation != "" {
		m.operation = operation
	}
	if choices != nil {
		m.choices = choices
		m.cursor = 0
	}

	// Return the appropriate command based on selection
	switch m.cursor {
	case 0: // Complete System Backup
		return m, LoadDrives()
	case 1: // Home Directory Only
		return m, DiscoverHomeFoldersCmd()
	default:
		return m, nil
	}
}

// handleRestoreMenuSelection handles selection logic for the restore menu screen
func (m Model) handleRestoreMenuSelection() (tea.Model, tea.Cmd) {
	// Log restore menu selection
	if logPath := getLogFilePath(); logPath != "" {
		if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			fmt.Fprintf(logFile, "DEBUG: handleRestoreMenuSelection called, cursor: %d\n", m.cursor)
			logFile.Close()
		}
	}
	
	handler := handlers.NewRestoreMenuHandler()
	screen, operation, choices, _ := handler.HandleSelection(m.cursor)

	m.screen = screen
	if operation != "" {
		m.operation = operation
	}
	if choices != nil {
		m.choices = choices
		m.cursor = 0
	}

	// Log the result of restore menu selection
	if logPath := getLogFilePath(); logPath != "" {
		if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			fmt.Fprintf(logFile, "DEBUG: Restore menu selection result - screen: %v, operation: %s\n", screen, operation)
			logFile.Close()
		}
	}

	// Since we go directly to drive selection now, load drives
	if screen == screens.ScreenDriveSelect {
		return m, LoadDrives()
	}

	return m, nil
}

// handleRestoreFolderSelection handles folder selection for selective restore
func (m Model) handleRestoreFolderSelection() (tea.Model, tea.Cmd) {
	numControls := 2      // Continue, Back
	numConfigItems := 2   // Configuration, Window Managers
	visibleFolders := m.getVisibleRestoreFolders()
	
	if m.cursor == 0 {
		// Continue button - proceed with restore
		if m.totalRestoreSize == 0 && !m.restoreConfig && !m.restoreWindowMgrs {
			m.message = "⚠️ Please select at least one item to restore (folders or configuration)"
			return m, nil
		}

		// CRITICAL SPACE CHECK: Verify internal drive has enough space BEFORE confirmation
		// Use the new selective space checking function that only counts selected items
		err := checkSelectiveRestoreSpaceRequirements(m.restoreFolders, m.selectedRestoreFolders, m.restoreConfig, m.restoreWindowMgrs)
		if err != nil {
			// Show the space error immediately - don't proceed to confirmation
			m.message = err.Error()
			m.errorRequiresManualDismissal = true
			m.lastScreen = m.screen
			m.screen = screens.ScreenError
			return m, nil
		}

		// Space check passed - proceed to confirmation
		// Go directly to confirmation since everything is on one screen now
		restoreTypeDesc := "HOME DIRECTORY"
		
		// Build summary of what will be restored
		var restoreItems []string
		if m.restoreConfig {
			restoreItems = append(restoreItems, "✅ Configuration (~/.config)")
		}
		if m.restoreWindowMgrs {
			restoreItems = append(restoreItems, "✅ Window Managers")
		}
		
		selectedFolders := 0
		for _, folder := range visibleFolders {
			if m.selectedRestoreFolders[folder.Path] {
				selectedFolders++
			}
		}
		if selectedFolders > 0 {
			restoreItems = append(restoreItems, fmt.Sprintf("✅ %d selected folders", selectedFolders))
		}
		
		var itemsList string
		if len(restoreItems) > 0 {
			itemsList = "Items to restore:\n" + strings.Join(restoreItems, "\n") + "\n\n"
		}
		
		// Calculate total size including config estimates
		totalSize := m.totalRestoreSize
		if m.restoreConfig {
			totalSize += 100 * 1024 * 1024 // ~100MB estimate
		}
		if m.restoreWindowMgrs {
			totalSize += 50 * 1024 * 1024 // ~50MB estimate  
		}
		
		m.confirmation = fmt.Sprintf("Ready to restore %s\n\n%sTotal size: %s\nSource: %s\n\n⚠️ This will OVERWRITE existing files!\n\nProceed with restore?",
			restoreTypeDesc, itemsList, FormatBytes(totalSize), m.selectedDrive)
		
		m.screen = screens.ScreenConfirm
		m.cursor = 0
		return m, nil
		
	} else if m.cursor == 1 {
		// Back button - go back to restore menu and clear all restore state
		m.screen = screens.ScreenRestore
		m.cursor = 0
		m.choices = screens.RestoreMenuChoices
		// Clear all restore state to prevent navigation issues
		m.selectedRestoreFolders = make(map[string]bool)
		m.restoreFolders = nil
		m.totalRestoreSize = 0
		m.restoreConfig = false
		m.restoreWindowMgrs = false
		m.restoreOptionsConfigured = false
		m.operation = ""
		m.selectedDrive = ""
		m.message = ""
		return m, nil
		
	} else if m.cursor >= numControls && m.cursor < numControls + numConfigItems {
		// Config item selection
		configIndex := m.cursor - numControls
		if configIndex == 0 {
			// Configuration
			m.restoreConfig = !m.restoreConfig
		} else if configIndex == 1 {
			// Window Managers
			m.restoreWindowMgrs = !m.restoreWindowMgrs
		}
		m.calculateTotalRestoreSize()
		
	} else if m.cursor >= numControls + numConfigItems {
		// Folder selection
		folderIndex := m.cursor - numControls - numConfigItems
		
		if folderIndex >= 0 && folderIndex < len(visibleFolders) {
			folder := visibleFolders[folderIndex]
			if !folder.AlwaysInclude {
				m.selectedRestoreFolders[folder.Path] = !m.selectedRestoreFolders[folder.Path]
				m.calculateTotalRestoreSize()
			}
		}
	}
	
	return m, nil
}

// handleVerifyMenuSelection handles selection logic for the verify menu screen
func (m Model) handleVerifyMenuSelection() (tea.Model, tea.Cmd) {
	switch m.cursor {
	case 0: // Auto-detect backup type and verify
		m.operation = "auto_verify"
		// Go to drive selection for backup source
		m.screen = screens.ScreenDriveSelect
		m.cursor = 0
		return m, LoadDrives()
	case 1: // Back
		m.screen = screens.ScreenMain
		m.choices = screens.MainMenuChoices
		m.cursor = 0
	}
	return m, nil
}

func (m Model) handleSelection() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screens.ScreenMain:
		return m.handleMainMenuSelection()
	case screens.ScreenBackup:
		return m.handleBackupMenuSelection()
	case screens.ScreenRestore:
		return m.handleRestoreMenuSelection()
	case screens.ScreenRestoreFolderSelect:
		return m.handleRestoreFolderSelection()
	case screens.ScreenVerify:
		return m.handleVerifyMenuSelection()
	case screens.ScreenConfirm:
		switch m.cursor {
		case 0: // Yes
			switch m.operation {
			case "unmount_backup":
				// For unmount, don't transition to progress screen - handle the response directly
				return m, PerformBackupUnmount()
			default:
				// Clear all state and transition to progress for other operations
				m.screen = screens.ScreenProgress
				m.progress = 0
				m.message = "Starting operation..."
				m.confirmation = "" // Clear confirmation text

				// Start the actual operation
				switch m.operation {
				case "system_backup":
					// System backup - use universal backup system
					return m, tea.Batch(
						startUniversalBackup(m.operation, m.selectedDrive, nil, nil),
						CheckTUIBackupProgress(),
						tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
							return state.CylonAnimateMsg{}
						}),
					)
				case "home_backup":
					// SAVE SELECTIONS BEFORE BACKUP: Persist user's folder choices
					err := SaveSelectiveBackupConfig(m.selectedFolders, m.subfolderCache, m.currentFolderPath, m.folderBreadcrumb)
					if err != nil {
						// Log error but continue with backup
						m.message = fmt.Sprintf("⚠️ Failed to save folder preferences: %v", err)
					}

					// Home backup - use universal backup system for selective home backup
					return m, tea.Batch(
						startUniversalBackup("selective_home_backup", m.selectedDrive, m.selectedFolders, m.homeFolders),
						CheckTUIBackupProgress(),
						tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
							return state.CylonAnimateMsg{}
						}),
					)
				case "system_restore":
					// Check if we have selected restore folders (means it's a home backup)
					if len(m.selectedRestoreFolders) > 0 {
						return m, startSelectiveRestore(m.selectedDrive, m.selectedRestoreFolders, m.restoreFolders, m.restoreConfig, m.restoreWindowMgrs)
					}
					return m, startRestore(m.selectedDrive, "/", m.restoreConfig, m.restoreWindowMgrs)
				case "custom_restore":
					return m, startRestore(m.selectedDrive, "/tmp/restore", m.restoreConfig, m.restoreWindowMgrs)
				case "system_verify":
					// System verification
					return m, tea.Batch(
						startVerification(m.operation, m.selectedDrive),
						CheckTUIBackupProgress(),
						tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
							return state.CylonAnimateMsg{}
						}),
					)
				case "home_verify":
					// Home directory verification
					return m, tea.Batch(
						startVerification(m.operation, m.selectedDrive),
						CheckTUIBackupProgress(),
						tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
							return state.CylonAnimateMsg{}
						}),
					)
				case "auto_verify":
					// Auto-detection verification
					return m, tea.Batch(
						startVerification(m.operation, m.selectedDrive),
						CheckTUIBackupProgress(),
						tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
							return state.CylonAnimateMsg{}
						}),
					)
				default:
					// Fallback - use universal backup system
					return m, startUniversalBackup(m.operation, m.selectedDrive, nil, nil)
				}
			}
		case 1: // No
			// Store the operation type before clearing
			wasUnmountOp := (m.operation == "unmount_backup")

			// Clear state and return to main menu
			resetBackupState() // Reset all backup state
			m.confirmation = ""
			m.operation = ""
			m.selectedDrive = ""
			m.progress = 0
			m.screen = screens.ScreenMain
			m.choices = screens.MainMenuChoices
			m.cursor = 0
			m.restoreOptionsConfigured = false // Reset restore flow state

			// Set appropriate message
			if wasUnmountOp {
				m.message = "ℹ️  Backup drive left mounted at current location"
			} else {
				m.message = ""
			}
		}
	case screens.ScreenAbout:
		resetBackupState() // Reset state when returning from about screen
		m.screen = screens.ScreenMain
		m.choices = screens.MainMenuChoices
		m.cursor = 0
	case screens.ScreenHomeFolderSelect:
		// NEW LAYOUT: Controls first (0-1), then folders (2+)
		numControls := 2

		if m.cursor < numControls {
			// Handle control selection
			switch m.cursor {
			case 0: // "Continue" option - go to drive selection
				// SAVE SELECTIONS: Persist user's folder choices when they continue
				err := SaveSelectiveBackupConfig(m.selectedFolders, m.subfolderCache, m.currentFolderPath, m.folderBreadcrumb)
				if err != nil {
					m.message = fmt.Sprintf("⚠️ Failed to save preferences: %v", err)
					// Continue anyway after brief display
					return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
						return tea.KeyMsg{Type: tea.KeyEnter} // Retry continue
					})
				}

				m.screen = screens.ScreenDriveSelect
				m.cursor = 0
				return m, LoadDrives()
			case 1: // "Back" option
				m.screen = screens.ScreenBackup
				m.choices = screens.BackupMenuChoices
				m.cursor = 0
			}
		} else {
			// Handle folder selection (cursor >= 2)
			folderIndex := m.cursor - numControls
			visibleFolders := m.getVisibleFoldersNonEmpty()

			if folderIndex < len(visibleFolders) {
				folder := visibleFolders[folderIndex]

				// NEW: Check if this folder has subfolders and can be drilled down
				if folder.HasSubfolders {
					// Check if we already have this folder cached
					if _, exists := m.subfolderCache[folder.Path]; exists {
						// Use cached data - switch directly to subfolder screen
						m.currentFolderPath = folder.Path
						m.folderBreadcrumb = []string{"Home", folder.Name}
						m.screen = screens.ScreenHomeSubfolderSelect
						m.cursor = 0
					} else {
						// Need to discover subfolders first
						m.message = fmt.Sprintf("🔍 Scanning %s...", folder.Name)
						return m, DiscoverSubfoldersCmd(folder.Path)
					}
				} else {
					// No subfolders - use smart toggle selection
					m.toggleParentFolder(folder)

					// Recalculate total backup size
					m.calculateTotalBackupSize()

					// Auto-save after folder toggle
					m.autoSaveSelections()
				}
			}
		}
	case screens.ScreenHomeSubfolderSelect:
		// NEW: Subfolder selection handling
		numControls := 2

		if m.cursor < numControls {
			// Handle control selection
			switch m.cursor {
			case 0: // "Continue" option - go to drive selection
				// SAVE SELECTIONS: Persist user's folder choices when they continue from subfolders
				err := SaveSelectiveBackupConfig(m.selectedFolders, m.subfolderCache, m.currentFolderPath, m.folderBreadcrumb)
				if err != nil {
					m.message = fmt.Sprintf("⚠️ Failed to save preferences: %v", err)
					// Continue anyway after brief display
					return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
						return tea.KeyMsg{Type: tea.KeyEnter} // Retry continue
					})
				}

				m.screen = screens.ScreenDriveSelect
				m.cursor = 0
				return m, LoadDrives()
			case 1: // "Back" option - return to parent folder view
				// Reset navigation state and return to main folder view
				m.currentFolderPath = ""
				m.folderBreadcrumb = []string{}
				m.screen = screens.ScreenHomeFolderSelect
				m.cursor = 0
				// Clear any temporary messages
				m.message = ""
			}
		} else {
			// Handle subfolder selection (cursor >= 2)
			subfolderIndex := m.cursor - numControls
			subfolders := m.getCurrentSubfolders()

			if subfolderIndex < len(subfolders) {
				// Toggle subfolder selection
				subfolder := subfolders[subfolderIndex]
				m.selectedFolders[subfolder.Path] = !m.selectedFolders[subfolder.Path]

				// NEW: Update parent folder selection state based on subfolder changes
				m.updateParentSelectionState(m.currentFolderPath)

				// Recalculate total backup size
				m.calculateTotalBackupSize()

				// Auto-save after subfolder toggle
				m.autoSaveSelections()
			}
		}
	case screens.ScreenDriveSelect:
		// Log drive selection screen handling
		if logPath := getLogFilePath(); logPath != "" {
			if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
				fmt.Fprintf(logFile, "DEBUG: ScreenDriveSelect handling, cursor: %d, total drives: %d, operation: %s\n", m.cursor, len(m.drives), m.operation)
				logFile.Close()
			}
		}
		if m.cursor < len(m.drives) {
			selectedDrive := m.drives[m.cursor]
			m.selectedDrive = selectedDrive.Device

			// Log the selected drive
			if logPath := getLogFilePath(); logPath != "" {
				if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
					fmt.Fprintf(logFile, "DEBUG: Selected drive: %s (%s) - %s\n", selectedDrive.Device, selectedDrive.Label, selectedDrive.Size)
					logFile.Close()
				}
			}

			// IMMEDIATE FEEDBACK: Show mounting message
			m.message = "🔧 Mounting drive and checking space..."

			// Check the operation type
			if strings.Contains(m.operation, "backup") {
				// For backup: mount drive for destination with appropriate space check
				if m.operation == "home_backup" {
					// FIXED: Pass selected folders for accurate space checking
					return m, mountDriveForSelectiveHomeBackup(selectedDrive, m.homeFolders, m.selectedFolders, m.subfolderCache)
				} else {
					return m, mountDriveForBackup(selectedDrive)
				}
			} else if strings.Contains(m.operation, "restore") {
				// For restore: mount drive for source backup
				if logPath := getLogFilePath(); logPath != "" {
					if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
						fmt.Fprintf(logFile, "DEBUG: Starting restore operation for drive: %s (Device: %s, Label: %s)\n", selectedDrive.Device, selectedDrive.Device, selectedDrive.Label)
						logFile.Close()
					}
				}
				return m, mountDriveForRestore(selectedDrive)
			} else if strings.Contains(m.operation, "verify") {
				// For verify: mount drive for source backup (read-only)
				return m, mountDriveForVerification(selectedDrive)
			} else {
				// Fallback: regular mounting
				return m, mountSelectedDrive(selectedDrive)
			}
		} else {
			// Back option
			if strings.Contains(m.operation, "backup") {
				// Go back to backup menu
				m.screen = screens.ScreenBackup
				m.choices = screens.BackupMenuChoices
			} else if strings.Contains(m.operation, "restore") {
				// Go back to restore menu
				m.screen = screens.ScreenRestore
				m.choices = screens.RestoreMenuChoices
			} else if strings.Contains(m.operation, "verify") {
				// Go back to verify menu
				m.screen = screens.ScreenVerify
				m.choices = screens.VerifyMenuChoices
			} else {
				// Go back to main menu
				m.screen = screens.ScreenMain
				m.choices = screens.MainMenuChoices
			}
			m.cursor = 0
		}
	case screens.ScreenRestoreOptions:
		switch m.cursor {
		case 0: // Toggle Restore Configuration
			m.restoreConfig = !m.restoreConfig
			// Update the visual indicator
			if m.restoreConfig {
				m.choices[0] = "☑️ Restore Configuration (~/.config)"
			} else {
				m.choices[0] = "☐ Restore Configuration (~/.config)"
			}
		case 1: // Toggle Restore Window Managers
			m.restoreWindowMgrs = !m.restoreWindowMgrs
			// Update the visual indicator
			if m.restoreWindowMgrs {
				m.choices[1] = "☑️ Restore Window Managers (Hyprland, GNOME, etc.)"
			} else {
				m.choices[1] = "☐ Restore Window Managers (Hyprland, GNOME, etc.)"
			}
		case 2: // Continue
			// Mark that restore options have been configured
			m.restoreOptionsConfigured = true
			// Go to drive selection with the configured options
			m.screen = screens.ScreenDriveSelect
			m.cursor = 0
			return m, LoadDrives()
		case 3: // Back
			m.screen = screens.ScreenRestore
			m.choices = screens.RestoreMenuChoices
			m.cursor = 0
		}
	}
	return m, nil
}

// calculateTotalBackupSize computes the total size of all selected folders for backup.
// This includes both user-selected visible folders and automatically included hidden folders.
// FIXED: Now properly handles hierarchical selections - when subfolders are individually
// selected, uses their specific sizes instead of the parent folder's total size.
// The result is stored in m.totalBackupSize for display and space validation.
func (m *Model) calculateTotalBackupSize() {
	m.totalBackupSize = 0

	// Track which parent folders have been processed to avoid double-counting
	processedParents := make(map[string]bool)

	for _, folder := range m.homeFolders {
		if folder.AlwaysInclude {
			// Hidden folders are always included (dotfiles/dotdirs)
			m.totalBackupSize += folder.Size
		} else if folder.IsVisible {
			// NEW: Handle visible folders with potential subfolders
			if folder.HasSubfolders {
				// Check if any subfolders are cached (user has drilled down)
				if subfolders, exists := m.subfolderCache[folder.Path]; exists {
					// User has drilled down - calculate based on individual subfolder selections
					subfolderTotal := int64(0)
					anySubfolderSelected := false

					for _, subfolder := range subfolders {
						if subfolder.Size > 0 && m.selectedFolders[subfolder.Path] {
							subfolderTotal += subfolder.Size
							anySubfolderSelected = true
						}
					}

					// Only add subfolders if at least one is selected
					if anySubfolderSelected {
						m.totalBackupSize += subfolderTotal
					}
					processedParents[folder.Path] = true
				} else {
					// No subfolders cached - use parent folder selection
					if m.selectedFolders[folder.Path] {
						m.totalBackupSize += folder.Size
					}
					processedParents[folder.Path] = true
				}
			} else {
				// No subfolders - use parent folder selection directly
				if m.selectedFolders[folder.Path] {
					m.totalBackupSize += folder.Size
				}
				processedParents[folder.Path] = true
			}
		}
	}

	// ADDITIONAL: Add any individually selected subfolders whose parents weren't processed
	// This handles edge cases where subfolders might be selected but parent isn't in homeFolders
	for folderPath, isSelected := range m.selectedFolders {
		if !isSelected {
			continue
		}

		// Check if this is a subfolder (has a parent path that was processed)
		parentProcessed := false
		for processedParent := range processedParents {
			if strings.HasPrefix(folderPath, processedParent+"/") {
				parentProcessed = true
				break
			}
		}

		// If no parent was processed, this might be a standalone subfolder selection
		if !parentProcessed {
			// Find the subfolder in cache and add its size
			for _, cachedSubfolders := range m.subfolderCache {
				for _, subfolder := range cachedSubfolders {
					if subfolder.Path == folderPath && subfolder.Size > 0 {
						m.totalBackupSize += subfolder.Size
						break
					}
				}
			}
		}
	}
}

// getVisibleFolders returns a slice of all folders that should be shown to the user.
// This excludes hidden folders (dotfiles/dotdirs) which are handled automatically.
func (m Model) getVisibleFolders() []HomeFolderInfo {
	visibleFolders := make([]HomeFolderInfo, 0)
	for _, folder := range m.homeFolders {
		if folder.IsVisible {
			visibleFolders = append(visibleFolders, folder)
		}
	}
	return visibleFolders
}

// getVisibleFoldersNonEmpty returns only visible folders that contain data.
// This is used for UI display to avoid showing empty folders in the selection interface.
func (m Model) getVisibleFoldersNonEmpty() []HomeFolderInfo {
	visibleFolders := make([]HomeFolderInfo, 0)
	for _, folder := range m.homeFolders {
		if folder.IsVisible && folder.Size > 0 { // Only show non-empty visible folders
			visibleFolders = append(visibleFolders, folder)
		}
	}
	return visibleFolders
}

// getCurrentSubfolders returns the cached subfolders for the current folder path.
// Used for UI navigation and rendering in ScreenHomeSubfolderSelect.
func (m Model) getCurrentSubfolders() []HomeFolderInfo {
	if subfolders, exists := m.subfolderCache[m.currentFolderPath]; exists {
		// Filter to only show non-empty subfolders (like main folders)
		nonEmptySubfolders := make([]HomeFolderInfo, 0)
		for _, subfolder := range subfolders {
			if subfolder.Size > 0 {
				nonEmptySubfolders = append(nonEmptySubfolders, subfolder)
			}
		}
		return nonEmptySubfolders
	}
	return nil
}

// getVisibleRestoreFolders returns only non-empty, visible folders for the restore UI.
func (m Model) getVisibleRestoreFolders() []HomeFolderInfo {
	visibleFolders := []HomeFolderInfo{}
	for _, folder := range m.restoreFolders {
		if folder.Size > 0 && folder.IsVisible {
			visibleFolders = append(visibleFolders, folder)
		}
	}
	return visibleFolders
}

// calculateTotalRestoreSize calculates the total size of selected folders for restore.
func (m *Model) calculateTotalRestoreSize() {
	var total int64
	for _, folder := range m.restoreFolders {
		if folder.AlwaysInclude || m.selectedRestoreFolders[folder.Path] {
			total += folder.Size
		}
	}
	m.totalRestoreSize = total
}

// NEW: Smart selection state management functions

// getFolderSelectionState determines the selection state of a parent folder based on its subfolders.
// Returns: "full" if all subfolders selected, "partial" if some selected, "none" if none selected
func (m Model) getFolderSelectionState(folder HomeFolderInfo) string {
	if !folder.HasSubfolders {
		// No subfolders - return based on direct selection
		if m.selectedFolders[folder.Path] {
			return "full"
		}
		return "none"
	}

	// Check subfolder selection states
	subfolders, exists := m.subfolderCache[folder.Path]
	if !exists {
		// No subfolders cached yet - return based on parent selection
		if m.selectedFolders[folder.Path] {
			return "full"
		}
		return "none"
	}

	// Count selected vs total subfolders
	totalSubfolders := 0
	selectedSubfolders := 0

	for _, subfolder := range subfolders {
		if subfolder.Size > 0 { // Only count non-empty subfolders
			totalSubfolders++
			if m.selectedFolders[subfolder.Path] {
				selectedSubfolders++
			}
		}
	}

	switch selectedSubfolders {
	case 0:
		return "none"
	case totalSubfolders:
		return "full"
	default:
		return "partial"
	}

}

// updateParentSelectionState updates the parent folder's selection based on subfolder changes.
// Called after subfolder selections are modified to maintain consistency.
func (m *Model) updateParentSelectionState(parentFolderPath string) {
	// Find the parent folder in homeFolders
	var parentFolder *HomeFolderInfo
	for i := range m.homeFolders {
		if m.homeFolders[i].Path == parentFolderPath {
			parentFolder = &m.homeFolders[i]
			break
		}
	}

	if parentFolder == nil || !parentFolder.HasSubfolders {
		return
	}

	// Get selection state and update parent accordingly
	state := m.getFolderSelectionState(*parentFolder)
	switch state {
	case "full":
		m.selectedFolders[parentFolderPath] = true
	case "none":
		m.selectedFolders[parentFolderPath] = false
	case "partial":
		// For partial selection, we'll keep parent selected but track it's partial
		// This preserves the user's intent while showing partial state
		m.selectedFolders[parentFolderPath] = true
	}
}

// toggleParentFolder toggles the selection of a parent folder and all its subfolders.
// If toggling on, selects all subfolders. If toggling off, deselects all subfolders.
func (m *Model) toggleParentFolder(folder HomeFolderInfo) {
	currentState := m.selectedFolders[folder.Path]
	newState := !currentState

	// Set parent folder selection
	m.selectedFolders[folder.Path] = newState

	// If folder has subfolders, also set all subfolder selections
	if folder.HasSubfolders {
		if subfolders, exists := m.subfolderCache[folder.Path]; exists {
			for _, subfolder := range subfolders {
				if subfolder.Size > 0 { // Only affect non-empty subfolders
					m.selectedFolders[subfolder.Path] = newState
				}
			}
		}
	}
}

// autoSaveSelections saves the current configuration in the background without blocking the UI.
// This ensures user preferences are preserved even if they exit before completing backup.
func (m *Model) autoSaveSelections() {
	// Save configuration asynchronously - don't block UI for save errors
	go func() {
		SaveSelectiveBackupConfig(m.selectedFolders, m.subfolderCache, m.currentFolderPath, m.folderBreadcrumb)
		// Note: We ignore errors in background saves to avoid disrupting UI flow
		// Critical saves (before backup) still handle errors properly
	}()
}

// This method delegates to specific render functions based on the active screen.
func (m Model) View() string {
	switch m.screen {
	case screens.ScreenMain:
		return m.renderMainMenu()
	case screens.ScreenBackup:
		return m.renderBackupMenu()
	case screens.ScreenRestore:
		return m.renderRestoreMenu()
	case screens.ScreenRestoreOptions:
		return m.renderRestoreOptions()
	case screens.ScreenVerify:
		return m.renderVerifyMenu()
	case screens.ScreenAbout:
		return m.renderAbout()
	case screens.ScreenConfirm:
		return m.renderConfirmation()
	case screens.ScreenProgress:
		return m.renderProgress()
	case screens.ScreenHomeFolderSelect:
		return m.renderHomeFolderSelect()
	case screens.ScreenHomeSubfolderSelect:
		return m.renderHomeSubfolderSelect() // NEW: Subfolder rendering
	case screens.ScreenDriveSelect:
		return m.renderDriveSelect()
	case screens.ScreenError:
		return m.renderError()
	case screens.ScreenComplete:
		return m.renderComplete()
	case screens.ScreenVerificationErrors:
		return m.renderVerificationErrors()
	case screens.ScreenRestoreFolderSelect:
		return m.renderRestoreFolderSelect()
	default:
		return "Unknown screen"
	}
}
