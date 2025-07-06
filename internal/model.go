package internal

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Message types
type cylonAnimateMsg struct{}

// Screen types
type screen int

const (
	screenMain screen = iota
	screenBackup
	screenRestore
	screenVerify    // New: verify backup
	screenAbout
	screenConfirm
	screenProgress
	screenDriveSelect
	screenError     // For errors that require manual dismissal
	screenComplete  // For completion messages that require manual dismissal
	screenHomeFolderSelect  // New: selective home folder backup
)

// Model represents the application state
type Model struct {
	screen       screen
	lastScreen   screen
	cursor       int
	choices      []string
	selected     map[int]struct{}
	confirmation string
	progress     float64
	operation    string
	message      string
	width        int
	height       int
	drives       []DriveInfo  // Available drives
	selectedDrive string      // Selected drive path
	cylonFrame   int          // Animation frame for cylon effect
	canceling    bool         // Flag to indicate operation is being canceled
	errorRequiresManualDismissal bool  // Flag for errors that need manual dismissal
	
	// Home folder selection fields
	homeFolders    []HomeFolderInfo  // Available home folders
	selectedFolders map[string]bool   // User selections (by folder path)
	totalBackupSize int64            // Total size of selected folders (including hidden)
}

// InitialModel creates a new model
func InitialModel() Model {
	return Model{
		screen:   screenMain,
		choices:  []string{"🚀 Backup System", "🔄 Restore System", "🔍 Verify Backup", "ℹ️ About", "❌ Exit"},
		selected: make(map[int]struct{}),
		selectedFolders: make(map[string]bool),
		width:    100,
		height:   30,
	}
}

// Initialize
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages
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
		
		// Initialize selected folders - all visible folders selected by default
		m.selectedFolders = make(map[string]bool)
		for _, folder := range m.homeFolders {
			if folder.IsVisible {
				m.selectedFolders[folder.Path] = true
			}
		}
		
		// Calculate initial total backup size
		m.calculateTotalBackupSize()
		
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
				if m.operation == "home_backup" {
					backupTypeDesc = "HOME DIRECTORY"
				}
				
				m.confirmation = fmt.Sprintf("Ready to backup %s\n\nDrive: %s (%s)\nType: %s\nMounted at: %s\n\nProceed with backup?", 
					backupTypeDesc, msg.drivePath, msg.driveSize, msg.driveType, msg.mountPoint)
			} else if strings.Contains(m.operation, "restore") {
				// Restore confirmation
				restoreTypeDesc := "ENTIRE SYSTEM"
				if m.operation == "custom_restore" {
					restoreTypeDesc = "CUSTOM PATH"
				}
				
				m.confirmation = fmt.Sprintf("Ready to restore %s\n\nSource: %s (%s)\nType: %s\nMounted at: %s\n\n⚠️ This will OVERWRITE existing files!\n\nProceed with restore?", 
					restoreTypeDesc, msg.drivePath, msg.driveSize, msg.driveType, msg.mountPoint)
			} else if strings.Contains(m.operation, "verify") {
				// Verification confirmation
				verifyTypeDesc := "ENTIRE SYSTEM"
				if m.operation == "home_verify" {
					verifyTypeDesc = "HOME DIRECTORY"
				}
				
				m.confirmation = fmt.Sprintf("Ready to verify %s backup\n\nBackup Source: %s (%s)\nType: %s\nMounted at: %s\n\n🔍 This will compare backup files with your current system\n\nProceed with verification?", 
					verifyTypeDesc, msg.drivePath, msg.driveSize, msg.driveType, msg.mountPoint)
			}
			
			m.selectedDrive = msg.mountPoint // Store mount point for operation
			m.screen = screenConfirm
			m.cursor = 0
			return m, nil
		}

	case ProgressUpdate:
		if msg.Error != nil {
			// Check if this is a critical error that needs manual dismissal
			errorMsg := fmt.Sprintf("Error: %v", msg.Error)
			if strings.Contains(errorMsg, "verification") || 
			   strings.Contains(errorMsg, "critical") || 
			   strings.Contains(errorMsg, "failed") ||
			   strings.Contains(errorMsg, "error 32") {
				// Critical error - needs manual dismissal
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
				m.confirmation = fmt.Sprintf("🎉 Backup completed successfully!\n\nDo you want to unmount the backup drive?\n\nNote: Unmounting is recommended for safe removal.")
				m.operation = "unmount_backup"
				m.screen = screenConfirm
				m.cursor = 0
				return m, nil
			} else if msg.Error == nil {
				// Other operation completed successfully - show completion screen
				m.lastScreen = m.screen
				m.screen = screenComplete
				return m, nil
			} else {
				// Operation completed with error - check if critical
				errorMsg := fmt.Sprintf("Error: %v", msg.Error)
				if strings.Contains(errorMsg, "verification") || 
				   strings.Contains(errorMsg, "critical") || 
				   strings.Contains(errorMsg, "failed") ||
				   strings.Contains(errorMsg, "error 32") {
					// Critical error - needs manual dismissal
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
			m.choices = []string{"🚀 Backup System", "🔄 Restore System", "🔍 Verify Backup", "ℹ️ About", "❌ Exit"}
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
			m.choices = []string{"🚀 Backup System", "🔄 Restore System", "🔍 Verify Backup", "ℹ️ About", "❌ Exit"}
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
			m.choices = []string{"🚀 Backup System", "🔄 Restore System", "🔍 Verify Backup", "ℹ️ About", "❌ Exit"}
			return m, nil

		case "esc":
			if m.screen == screenError {
				// Return to main menu from error
				resetBackupState()
				m.screen = screenMain
				m.message = ""
				m.cursor = 0
				m.choices = []string{"🚀 Backup System", "🔄 Restore System", "🔍 Verify Backup", "ℹ️ About", "❌ Exit"}
				m.errorRequiresManualDismissal = false
				return m, nil
			} else if m.screen != screenMain {
				// Reset backup state when returning to main menu
				resetBackupState()
				m.screen = screenMain
				m.cursor = 0
				m.choices = []string{"🚀 Backup System", "🔄 Restore System", "🔍 Verify Backup", "ℹ️ About", "❌ Exit"}
			}
			return m, nil

		case "up", "k":
			if m.screen == screenConfirm {
				if m.cursor > 0 {
					m.cursor--
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
			} else if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j":
			if m.screen == screenConfirm {
				if m.cursor < 1 {
					m.cursor++
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
			}
			return m, nil
		}
	}

	return m, nil
}

// Handle menu selection
func (m Model) handleSelection() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenMain:
		switch m.cursor {
		case 0: // Backup
			m.screen = screenBackup
			m.choices = []string{"📁 Complete System Backup", "🏠 Home Directory Only", "⬅️ Back"}
			m.cursor = 0
		case 1: // Restore
			m.screen = screenRestore
			m.choices = []string{"🔄 Restore to Current System", "📂 Restore to Custom Path", "⬅️ Back"}
			m.cursor = 0
		case 2: // Verify Backup
			m.screen = screenVerify
			m.choices = []string{"🔍 Verify Complete System", "🏠 Verify Home Directory", "⬅️ Back"}
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
			m.choices = []string{"🚀 Backup System", "🔄 Restore System", "🔍 Verify Backup", "ℹ️ About", "❌ Exit"}
			m.cursor = 0
		}
	case screenRestore:
		switch m.cursor {
		case 0: // Restore to Current System  
			m.operation = "system_restore"
			// Go to drive selection like backup does
			m.screen = screenDriveSelect
			m.cursor = 0
			return m, LoadDrives()
		case 1: // Restore to Custom Path
			m.operation = "custom_restore"
			// Go to drive selection for source backup
			m.screen = screenDriveSelect  
			m.cursor = 0
			return m, LoadDrives()
		case 2: // Back
			resetBackupState() // Reset state when going back to main from restore menu
			m.screen = screenMain
			m.choices = []string{"🚀 Backup System", "🔄 Restore System", "🔍 Verify Backup", "ℹ️ About", "❌ Exit"}
			m.cursor = 0
		}
	case screenVerify:
		switch m.cursor {
		case 0: // Verify Complete System
			m.operation = "system_verify"
			// Go to drive selection for backup source
			m.screen = screenDriveSelect
			m.cursor = 0
			return m, LoadDrives()
		case 1: // Verify Home Directory
			m.operation = "home_verify"
			// Go to drive selection for backup source
			m.screen = screenDriveSelect
			m.cursor = 0
			return m, LoadDrives()
		case 2: // Back
			m.screen = screenMain
			m.choices = []string{"🚀 Backup System", "🔄 Restore System", "🔍 Verify Backup", "ℹ️ About", "❌ Exit"}
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
					// System backup - use regular backup
					return m, tea.Batch(
						startActualBackup(m.operation, m.selectedDrive),
						CheckTUIBackupProgress(),
						tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
							return cylonAnimateMsg{}
						}),
					)
				case "home_backup":
					// CRITICAL FIX: Use selective backup for home directory
					return m, tea.Batch(
						startSelectiveHomeBackup(m.selectedDrive, m.homeFolders, m.selectedFolders),
						CheckTUIBackupProgress(),
						tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
							return cylonAnimateMsg{}
						}),
					)
				case "system_restore":
					return m, startRestore(m.selectedDrive, "/")
				case "custom_restore":
					// TODO: Ask for custom destination path
					m.message = "Custom restore path selection not implemented yet"
					return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
						return tea.KeyMsg{Type: tea.KeyEsc}
					})
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
				default:
					return m, startActualBackup(m.operation, m.selectedDrive)
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
			m.choices = []string{"🚀 Backup System", "🔄 Restore System", "🔍 Verify Backup", "ℹ️ About", "❌ Exit"}
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
		m.choices = []string{"🚀 Backup System", "🔄 Restore System", "🔍 Verify Backup", "ℹ️ About", "❌ Exit"}
		m.cursor = 0
	case screenHomeFolderSelect:
		// NEW LAYOUT: Controls first (0-1), then folders (2+)
		numControls := 2
		
		if m.cursor < numControls {
			// Handle control selection
			switch m.cursor {
			case 0: // "Continue" option - go to drive selection
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
				// Toggle folder selection
				folder := visibleFolders[folderIndex]
				m.selectedFolders[folder.Path] = !m.selectedFolders[folder.Path]
				
				// Recalculate total backup size
				m.calculateTotalBackupSize()
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
					return m, mountDriveForSelectiveHomeBackup(selectedDrive, m.homeFolders, m.selectedFolders)
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
				m.choices = []string{"🚀 Backup System", "🔄 Restore System", "🔍 Verify Backup", "ℹ️ About", "❌ Exit"}
			}
			m.cursor = 0
		}
	}
	return m, nil
}

// Calculate total backup size for selected folders (including all hidden folders)
func (m *Model) calculateTotalBackupSize() {
	m.totalBackupSize = 0
	
	for _, folder := range m.homeFolders {
		if folder.AlwaysInclude {
			// Hidden folders are always included
			m.totalBackupSize += folder.Size
		} else if m.selectedFolders[folder.Path] {
			// Visible folders only if selected
			m.totalBackupSize += folder.Size
		}
	}
}

// Get visible folders (helper for navigation)
func (m Model) getVisibleFolders() []HomeFolderInfo {
	visibleFolders := make([]HomeFolderInfo, 0)
	for _, folder := range m.homeFolders {
		if folder.IsVisible {
			visibleFolders = append(visibleFolders, folder)
		}
	}
	return visibleFolders
}

// Get visible non-empty folders (helper for navigation and UI)
func (m Model) getVisibleFoldersNonEmpty() []HomeFolderInfo {
	visibleFolders := make([]HomeFolderInfo, 0)
	for _, folder := range m.homeFolders {
		if folder.IsVisible && folder.Size > 0 { // Only show non-empty visible folders
			visibleFolders = append(visibleFolders, folder)
		}
	}
	return visibleFolders
}

// View renders the UI
func (m Model) View() string {
	switch m.screen {
	case screenMain:
		return m.renderMainMenu()
	case screenBackup:
		return m.renderBackupMenu()
	case screenRestore:
		return m.renderRestoreMenu()
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
	case screenDriveSelect:
		return m.renderDriveSelect()
	case screenError:
		return m.renderError()
	case screenComplete:
		return m.renderComplete()
	default:
		return "Unknown screen"
	}
}
