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
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// cylonAnimateMsg triggers animation updates for the progress bar cylon effect.
// This message is sent periodically during progress display to create the sweeping animation.
type cylonAnimateMsg struct{}

// screen represents the different UI screens available in the application.
// Each screen has its own rendering logic and input handling behavior.
type screen int

const (
	screenMain                screen = iota // Main menu with primary options
	screenBackup                            // Backup type selection menu
	screenRestore                           // Restore type selection menu
	screenVerify                            // Verification type selection menu
	screenAbout                             // About/help information screen
	screenConfirm                           // Confirmation dialog for operations
	screenProgress                          // Progress display during operations
	screenDriveSelect                       // External drive selection interface
	screenError                             // Error display requiring manual dismissal
	screenComplete                          // Success completion requiring manual dismissal
	screenRestoreOptions                    // Restore configuration options
	screenHomeFolderSelect                  // Home folder selection for backup
	screenHomeSubfolderSelect               // Subfolder selection within home folders
	screenVerificationErrors                // Detailed verification error display
)

// mainMenuChoices defines the main menu options in the correct order
var mainMenuChoices = []string{"🚀 Backup System", "🔍 Verify Backup", "🔄 Restore System", "ℹ️ About", "❌ Exit"}

// Model represents the complete application state for the Migrate TUI.
// It implements the tea.Model interface and contains all data needed to
// render screens and handle user interactions.
type Model struct {
	// Screen and navigation state
	screen     screen   // Current active screen
	lastScreen screen   // Previous screen for back navigation
	cursor     int      // Current cursor/selection position
	choices    []string // Available menu options for current screen

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

	// Verification error display
	verificationErrors []string // List of verification errors for display
	errorScrollOffset  int      // Current scroll position in error list
}

// InitialModel creates and returns a new Model instance with default values.
// This sets up the initial application state with the main menu active
// and initializes all required maps and default dimensions.
func InitialModel() Model {
	return Model{
		screen:            screenMain,
		choices:           mainMenuChoices,
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
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case DrivesLoaded:
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
		m.screen = screenHomeSubfolderSelect
		m.cursor = 0

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
			m.screen = screenError
			return m, nil
		} else if msg.success {
			// Success message - needs manual dismissal
			m.message = msg.message
			m.lastScreen = m.screen
			m.screen = screenComplete
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
				m.screen = screenError
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
			if strings.Contains(m.operation, "backup") {
				// Backup confirmation
				backupTypeDesc := "ENTIRE SYSTEM (1:1)"
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
				// Restore confirmation
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
			m.screen = screenConfirm
			m.cursor = 0
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
					m.screen = screenVerificationErrors
					m.progress = 0
					m.canceling = false
					return m, nil
				} else {
					// Legacy error handling
					m.message = errorMsg
					m.errorRequiresManualDismissal = true
					m.lastScreen = m.screen
					m.screen = screenError
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
				m.screen = screenError
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
				m.screen = screenConfirm
				m.cursor = 1
				return m, nil
			} else if msg.Error == nil {
				// Other operation completed successfully - show completion screen
				m.lastScreen = m.screen
				m.screen = screenComplete
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
						m.screen = screenVerificationErrors
						m.progress = 0
						m.canceling = false
						return m, nil
					} else {
						// Legacy error handling
						m.message = errorMsg
						m.errorRequiresManualDismissal = true
						m.lastScreen = m.screen
						m.screen = screenError
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
					m.screen = screenError
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

	case cylonAnimateMsg:
		// Update cylon animation frame
		m.cylonFrame = (m.cylonFrame + 1) % 20 // 20-frame cycle
		if m.screen == screenProgress {
			// Keep animating while on progress screen
			return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
				return cylonAnimateMsg{}
			})
		}
		return m, nil

	case tea.KeyMsg:
		// Handle error screen dismissal first
		if m.screen == screenError {
			// Any key press dismisses the error screen and returns to main menu
			resetBackupState()
			m.screen = screenMain
			m.message = ""
			m.cursor = 0
			m.choices = mainMenuChoices
			m.errorRequiresManualDismissal = false
			return m, nil
		}

		// Handle completion screen dismissal
		if m.screen == screenComplete {
			// Any key press dismisses the completion screen and returns to main
			resetBackupState()
			m.screen = screenMain
			m.message = ""
			m.cursor = 0
			m.choices = mainMenuChoices
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			if m.screen == screenMain {
				return m, tea.Quit
			}
			// Handle Ctrl+C during progress - set canceling state
			if m.screen == screenProgress {
				m.canceling = true
				m.message = "Canceling operation... Please wait for cleanup to complete."
				// Signal the backup operation to cancel
				CancelBackup()
				// Continue to let the progress update handle the cleanup
				return m, nil
			}
			// Go back to main menu from other screens
			m.screen = screenMain
			m.cursor = 0
			m.choices = mainMenuChoices
			return m, nil

		case "esc":
			if m.screen == screenError {
				// Return to main menu from error
				resetBackupState()
				m.screen = screenMain
				m.message = ""
				m.cursor = 0
				m.choices = mainMenuChoices
				m.errorRequiresManualDismissal = false
				return m, nil
			} else if m.screen == screenHomeSubfolderSelect {
				// NEW: Return to parent folder view from subfolder screen
				m.currentFolderPath = ""
				m.folderBreadcrumb = []string{}
				m.screen = screenHomeFolderSelect
				m.cursor = 0
				m.message = "" // Clear any temporary messages
				return m, nil
			} else if m.screen == screenVerificationErrors {
				// NEW: Return to main menu from verification errors screen
				resetBackupState()
				m.screen = screenMain
				m.cursor = 0
				m.choices = mainMenuChoices
				m.verificationErrors = []string{} // Clear error list
				m.errorScrollOffset = 0
				return m, nil
			} else if m.screen != screenMain {
				// Reset backup state when returning to main menu
				resetBackupState()
				m.screen = screenMain
				m.cursor = 0
				m.choices = mainMenuChoices
			}
			return m, nil

		case "up", "k":
			if m.screen == screenConfirm {
				if m.cursor > 0 {
					m.cursor--
				}
			} else if m.screen == screenVerificationErrors {
				// Scroll up in verification errors list
				if m.errorScrollOffset > 0 {
					m.errorScrollOffset--
				}
			} else if m.screen == screenMain {
				// Main menu: wrap around navigation
				if m.cursor > 0 {
					m.cursor--
				} else {
					m.cursor = len(m.choices) - 1 // Wrap to bottom
				}
			} else if m.screen == screenHomeFolderSelect {
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
			} else if m.screen == screenHomeSubfolderSelect {
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
			} else if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j":
			if m.screen == screenConfirm {
				if m.cursor < 1 {
					m.cursor++
				}
			} else if m.screen == screenVerificationErrors {
				// Scroll down in verification errors list
				contentHeight := m.height - 8 // Compact header + help + padding
				contentHeight = max(contentHeight, 5)
				maxScrollOffset := len(m.verificationErrors) - contentHeight
				if maxScrollOffset > 0 && m.errorScrollOffset < maxScrollOffset {
					m.errorScrollOffset++
				}
			} else if m.screen == screenMain {
				// Main menu: wrap around navigation
				if m.cursor < len(m.choices)-1 {
					m.cursor++
				} else {
					m.cursor = 0 // Wrap to top
				}
			} else if m.screen == screenHomeFolderSelect {
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
			} else if m.screen == screenHomeSubfolderSelect {
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
			} else if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
			return m, nil

		case "enter", " ":
			return m.handleSelection()

		case "a", "A":
			if m.screen == screenHomeFolderSelect {
				// Select all visible NON-EMPTY folders
				visibleFolders := m.getVisibleFoldersNonEmpty()
				for _, folder := range visibleFolders {
					m.selectedFolders[folder.Path] = true
				}
				m.calculateTotalBackupSize()
				m.autoSaveSelections() // Auto-save when user selects all
			}
			return m, nil

		case "n", "N", "x", "X":
			if m.screen == screenHomeFolderSelect {
				// Deselect all visible NON-EMPTY folders
				visibleFolders := m.getVisibleFoldersNonEmpty()
				for _, folder := range visibleFolders {
					m.selectedFolders[folder.Path] = false
				}
				m.calculateTotalBackupSize()
				m.autoSaveSelections() // Auto-save when user deselects all
			}
			return m, nil
		}
	}

	return m, nil
}

// handleSelection processes menu selections and handles screen transitions.
// This method implements the navigation logic for all interactive screens,
// managing state changes and initiating background operations as needed.
func (m Model) handleSelection() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenMain:
		switch m.cursor {
		case 0: // Backup
			m.screen = screenBackup
			m.choices = []string{"📁 Complete System Backup", "🏠 Home Directory Only", "⬅️ Back"}
			m.cursor = 0
		case 1: // Verify Backup
			m.screen = screenVerify
			m.choices = []string{"🔍 Auto-Detect & Verify Backup", "⬅️ Back"}
			m.cursor = 0
		case 2: // Restore
			m.screen = screenRestore
			m.choices = []string{"🔄 Restore to Current System", "📂 Restore to Custom Path", "⬅️ Back"}
			m.cursor = 0
		case 3: // About
			m.screen = screenAbout
		case 4: // Exit
			return m, tea.Quit
		}
	case screenBackup:
		switch m.cursor {
		case 0: // Complete System Backup
			m.operation = "system_backup"
			// Go to drive selection instead of hardcoded drive checking
			m.screen = screenDriveSelect
			m.cursor = 0
			return m, LoadDrives()
		case 1: // Home Directory Only
			m.operation = "home_backup"
			// Go to home folder selection instead of directly to drive selection
			m.screen = screenHomeFolderSelect
			m.cursor = 0
			return m, DiscoverHomeFoldersCmd()
		case 2: // Back
			m.screen = screenMain
			m.choices = mainMenuChoices
			m.cursor = 0
		}
	case screenRestore:
		switch m.cursor {
		case 0: // Restore to Current System
			m.operation = "system_restore"
			// Go to restore options screen first
			m.screen = screenRestoreOptions
			m.cursor = 0
			m.choices = []string{"☑️ Restore Configuration (~/.config)", "☑️ Restore Window Managers (Hyprland, GNOME, etc.)", "✅ Continue", "⬅️ Back"}
		case 1: // Restore to Custom Path
			m.operation = "custom_restore"
			// Go to restore options screen first
			m.screen = screenRestoreOptions
			m.cursor = 0
			m.choices = []string{"☑️ Restore Configuration (~/.config)", "☑️ Restore Window Managers (Hyprland, GNOME, etc.)", "✅ Continue", "⬅️ Back"}
		case 2: // Back
			resetBackupState() // Reset state when going back to main from restore menu
			m.screen = screenMain
			m.choices = mainMenuChoices
			m.cursor = 0
		}
	case screenVerify:
		switch m.cursor {
		case 0: // Auto-detect backup type and verify
			m.operation = "auto_verify"
			// Go to drive selection for backup source
			m.screen = screenDriveSelect
			m.cursor = 0
			return m, LoadDrives()
		case 1: // Back
			m.screen = screenMain
			m.choices = mainMenuChoices
			m.cursor = 0
		}
	case screenConfirm:
		switch m.cursor {
		case 0: // Yes
			switch m.operation {
			case "unmount_backup":
				// For unmount, don't transition to progress screen - handle the response directly
				return m, PerformBackupUnmount()
			default:
				// Clear all state and transition to progress for other operations
				m.screen = screenProgress
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
							return cylonAnimateMsg{}
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
							return cylonAnimateMsg{}
						}),
					)
				case "system_restore":
					return m, startRestore(m.selectedDrive, "/", m.restoreConfig, m.restoreWindowMgrs)
				case "custom_restore":
					return m, startRestore(m.selectedDrive, "/tmp/restore", m.restoreConfig, m.restoreWindowMgrs)
				case "system_verify":
					// System verification
					return m, tea.Batch(
						startVerification(m.operation, m.selectedDrive),
						CheckTUIBackupProgress(),
						tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
							return cylonAnimateMsg{}
						}),
					)
				case "home_verify":
					// Home directory verification
					return m, tea.Batch(
						startVerification(m.operation, m.selectedDrive),
						CheckTUIBackupProgress(),
						tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
							return cylonAnimateMsg{}
						}),
					)
				case "auto_verify":
					// Auto-detection verification
					return m, tea.Batch(
						startVerification(m.operation, m.selectedDrive),
						CheckTUIBackupProgress(),
						tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
							return cylonAnimateMsg{}
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
			m.screen = screenMain
			m.choices = mainMenuChoices
			m.cursor = 0

			// Set appropriate message
			if wasUnmountOp {
				m.message = "ℹ️  Backup drive left mounted at current location"
			} else {
				m.message = ""
			}
		}
	case screenAbout:
		resetBackupState() // Reset state when returning from about screen
		m.screen = screenMain
		m.choices = mainMenuChoices
		m.cursor = 0
	case screenHomeFolderSelect:
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

				m.screen = screenDriveSelect
				m.cursor = 0
				return m, LoadDrives()
			case 1: // "Back" option
				m.screen = screenBackup
				m.choices = []string{"📁 Complete System Backup", "🏠 Home Directory Only", "⬅️ Back"}
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
						m.screen = screenHomeSubfolderSelect
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
	case screenHomeSubfolderSelect:
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

				m.screen = screenDriveSelect
				m.cursor = 0
				return m, LoadDrives()
			case 1: // "Back" option - return to parent folder view
				// Reset navigation state and return to main folder view
				m.currentFolderPath = ""
				m.folderBreadcrumb = []string{}
				m.screen = screenHomeFolderSelect
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
	case screenDriveSelect:
		if m.cursor < len(m.drives) {
			selectedDrive := m.drives[m.cursor]
			m.selectedDrive = selectedDrive.Device

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
				m.screen = screenBackup
				m.choices = []string{"📁 Complete System Backup", "🏠 Home Directory Only", "⬅️ Back"}
			} else if strings.Contains(m.operation, "restore") {
				// Go back to restore menu
				m.screen = screenRestore
				m.choices = []string{"🔄 Restore to Current System", "📂 Restore to Custom Path", "⬅️ Back"}
			} else if strings.Contains(m.operation, "verify") {
				// Go back to verify menu
				m.screen = screenVerify
				m.choices = []string{"🔍 Verify Complete System", "🏠 Verify Home Directory", "⬅️ Back"}
			} else {
				// Go back to main menu
				m.screen = screenMain
				m.choices = mainMenuChoices
			}
			m.cursor = 0
		}
	case screenRestoreOptions:
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
			// Go to drive selection with the configured options
			m.screen = screenDriveSelect
			m.cursor = 0
			return m, LoadDrives()
		case 3: // Back
			m.screen = screenRestore
			m.choices = []string{"🔄 Restore to Current System", "📂 Restore to Custom Path", "⬅️ Back"}
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
// Used for UI navigation and rendering in screenHomeSubfolderSelect.
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
	return []HomeFolderInfo{} // Return empty list if no cache
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
	case screenMain:
		return m.renderMainMenu()
	case screenBackup:
		return m.renderBackupMenu()
	case screenRestore:
		return m.renderRestoreMenu()
	case screenRestoreOptions:
		return m.renderRestoreOptions()
	case screenVerify:
		return m.renderVerifyMenu()
	case screenAbout:
		return m.renderAbout()
	case screenConfirm:
		return m.renderConfirmation()
	case screenProgress:
		return m.renderProgress()
	case screenHomeFolderSelect:
		return m.renderHomeFolderSelect()
	case screenHomeSubfolderSelect:
		return m.renderHomeSubfolderSelect() // NEW: Subfolder rendering
	case screenDriveSelect:
		return m.renderDriveSelect()
	case screenError:
		return m.renderError()
	case screenComplete:
		return m.renderComplete()
	case screenVerificationErrors:
		return m.renderVerificationErrors()
	default:
		return "Unknown screen"
	}
}
