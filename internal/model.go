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
	screenAbout
	screenConfirm
	screenProgress
	screenDriveSelect
	screenError  // For errors that require manual dismissal
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
}

// InitialModel creates a new model
func InitialModel() Model {
	return Model{
		screen:   screenMain,
		choices:  []string{"🚀 Backup System", "🔄 Restore System", "💾 Mount External Drive", "ℹ️ About", "❌ Exit"},
		selected: make(map[int]struct{}),
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
		} else {
			// Regular message
			m.message = msg.message
			if msg.success {
				// Show success message then return to main
				return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
					return tea.KeyMsg{Type: tea.KeyEsc}
				})
			}
			return m, nil
		}

	case BackupDriveStatus:
		if msg.error != nil {
			// Check if this is a LUKS error that needs manual dismissal
			errorMsg := msg.error.Error()
			if strings.Contains(errorMsg, "LUKS drive is locked") || 
			   strings.Contains(errorMsg, "cryptsetup luksOpen") {
				// LUKS error - needs manual dismissal
				m.message = errorMsg
				m.errorRequiresManualDismissal = true
				m.lastScreen = m.screen
				m.screen = screenError
				return m, nil
			} else {
				// Regular error - auto-dismiss after 3 seconds
				m.message = errorMsg
				return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
					return tea.KeyMsg{Type: tea.KeyEsc}
				})
			}
		} else {
			// Drive successfully mounted, confirm backup
			backupTypeDesc := "ENTIRE SYSTEM (1:1)"
			if m.operation == "home_backup" {
				backupTypeDesc = "HOME DIRECTORY"
			}
			
			m.confirmation = fmt.Sprintf("Ready to backup %s\n\nDrive: %s (%s)\nType: %s\nMounted at: %s\n\nProceed with backup?", 
				backupTypeDesc, msg.drivePath, msg.driveSize, msg.driveType, msg.mountPoint)
			m.selectedDrive = msg.mountPoint // Store mount point for backup
			m.screen = screenConfirm
			m.cursor = 0
			return m, nil
		}

	case ProgressUpdate:
		if msg.Error != nil {
			m.message = fmt.Sprintf("Error: %v", msg.Error)
			m.progress = 0
			m.canceling = false // Reset canceling state on error
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
			} else {
				// Other operation completed, show result for a moment then return to main
				return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
					return tea.KeyMsg{Type: tea.KeyEsc}
				})
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
			// Any key press dismisses the error screen
			m.screen = m.lastScreen
			m.message = ""
			m.errorRequiresManualDismissal = false
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
			m.choices = []string{"🚀 Backup System", "🔄 Restore System", "💾 Mount External Drive", "ℹ️ About", "❌ Exit"}
			return m, nil

		case "esc":
			if m.screen == screenError {
				// Return to previous screen from error
				m.screen = m.lastScreen
				m.message = ""
				m.errorRequiresManualDismissal = false
				return m, nil
			} else if m.screen != screenMain {
				// Reset backup state when returning to main menu
				resetBackupState()
				m.screen = screenMain
				m.cursor = 0
				m.choices = []string{"🚀 Backup System", "🔄 Restore System", "💾 Mount External Drive", "ℹ️ About", "❌ Exit"}
			}
			return m, nil

		case "up", "k":
			if m.screen == screenConfirm {
				if m.cursor > 0 {
					m.cursor--
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
			} else if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
			return m, nil

		case "enter", " ":
			return m.handleSelection()
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
		case 2: // Mount External Drive
			m.screen = screenDriveSelect
			m.cursor = 0
			// Load available drives
			return m, LoadDrives()
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
			// Go to drive selection instead of hardcoded drive checking
			m.screen = screenDriveSelect
			m.cursor = 0
			return m, LoadDrives()
		case 2: // Back
			m.screen = screenMain
			m.choices = []string{"🚀 Backup System", "🔄 Restore System", "💾 Mount External Drive", "ℹ️ About", "❌ Exit"}
			m.cursor = 0
		}
	case screenRestore:
		switch m.cursor {
		case 0: // Restore to Current System
			m.confirmation = "This will OVERWRITE your current system with the backup. Continue?"
			m.operation = "system_restore"
			m.screen = screenConfirm
		case 1: // Restore to Custom Path
			m.message = "Custom path restore not implemented yet"
			// TODO: Implement custom path selection
		case 2: // Back
			resetBackupState() // Reset state when going back to main from restore menu
			m.screen = screenMain
			m.choices = []string{"🚀 Backup System", "🔄 Restore System", "💾 Mount External Drive", "ℹ️ About", "❌ Exit"}
			m.cursor = 0
		}
	case screenConfirm:
		switch m.cursor {
		case 0: // Yes
			// Clear all state and transition to progress
			m.screen = screenProgress
			m.progress = 0
			m.message = "Starting operation..."
			m.confirmation = "" // Clear confirmation text
			
			// Start the actual operation
			switch m.operation {
			case "mount_for_backup":
				// This is no longer used - we handle backup confirmation differently now
				return m, startActualBackup(m.operation, m.selectedDrive)
			case "proceed_backup":
				// Start the actual backup with progress checking
				return m, tea.Batch(
					startActualBackup(m.operation, m.selectedDrive),
					CheckTUIBackupProgress(),
				)
			case "system_backup", "home_backup":
				// Start backup with progress monitoring AND cylon animation
				return m, tea.Batch(
					startActualBackup(m.operation, m.selectedDrive),
					CheckTUIBackupProgress(),
					tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
						return cylonAnimateMsg{}
					}),
				)
			case "unmount_backup":
				// Unmount the backup drive
				return m, PerformBackupUnmount()
			case "system_restore":
				return m, startRestore("/")
			default:
				return m, startActualBackup(m.operation, m.selectedDrive)
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
			m.choices = []string{"🚀 Backup System", "🔄 Restore System", "💾 Mount External Drive", "ℹ️ About", "❌ Exit"}
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
		m.choices = []string{"🚀 Backup System", "🔄 Restore System", "💾 Mount External Drive", "ℹ️ About", "❌ Exit"}
		m.cursor = 0
	case screenDriveSelect:
		if m.cursor < len(m.drives) {
			selectedDrive := m.drives[m.cursor]
			m.selectedDrive = selectedDrive.Device
			
			// Check if this is for backup or just mounting
			if strings.Contains(m.operation, "backup") {
				// For backup: mount drive first, then confirm backup
				return m, mountDriveForBackup(selectedDrive)
			} else {
				// For regular mounting: just mount the drive
				return m, mountSelectedDrive(selectedDrive)
			}
		} else {
			// Back option
			if strings.Contains(m.operation, "backup") {
				// Go back to backup menu
				m.screen = screenBackup
				m.choices = []string{"📁 Complete System Backup", "🏠 Home Directory Only", "⬅️ Back"}
			} else {
				// Go back to main menu
				m.screen = screenMain
				m.choices = []string{"🚀 Backup System", "🔄 Restore System", "💾 Mount External Drive", "ℹ️ About", "❌ Exit"}
			}
			m.cursor = 0
		}
	}
	return m, nil
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
	case screenAbout:
		return m.renderAbout()
	case screenConfirm:
		return m.renderConfirmation()
	case screenProgress:
		return m.renderProgress()
	case screenDriveSelect:
		return m.renderDriveSelect()
	case screenError:
		return m.renderError()
	default:
		return "Unknown screen"
	}
}
