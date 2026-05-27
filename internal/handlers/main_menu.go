package handlers

import (
	"migrate/internal/platform"
	"migrate/internal/screens"

	tea "github.com/charmbracelet/bubbletea"
)

// MainMenuHandler handles main menu selections and returns the next screen state
type MainMenuHandler struct {
	privilege platform.PrivLevel
}

// NewMainMenuHandler creates a new main menu handler
func NewMainMenuHandler(priv platform.PrivLevel) *MainMenuHandler {
	return &MainMenuHandler{privilege: priv}
}

// HandleSelection processes a main menu selection and returns the next state
func (h *MainMenuHandler) HandleSelection(cursor int) (screen screens.Screen, operation string, choices []string, cmd tea.Cmd) {
	if h.privilege == platform.PrivUser {
		switch cursor {
		case 0: // Backup
			return screens.ScreenBackup, "", screens.BackupMenuChoicesNonRoot, nil
		case 1: // Backup System Settings
			return screens.ScreenDriveSelect, "settings_backup", nil, nil
		case 2: // Verify Backup
			return screens.ScreenVerify, "", screens.VerifyMenuChoices, nil
		case 3: // Restore
			return screens.ScreenRestore, "", screens.RestoreMenuChoices, nil
		case 4: // About
			return screens.ScreenAbout, "", nil, nil
		case 5: // Exit
			return screens.ScreenMain, "", nil, tea.Quit
		}
		return screens.ScreenMain, "", screens.MainMenuChoicesNonRoot, nil
	}

	// Root mode
	switch cursor {
	case 0: // Backup
		return screens.ScreenBackup, "", screens.BackupMenuChoices, nil
	case 1: // Verify Backup
		return screens.ScreenVerify, "", screens.VerifyMenuChoices, nil
	case 2: // Restore
		return screens.ScreenRestore, "", screens.RestoreMenuChoices, nil
	case 3: // Restore Settings
		return screens.ScreenRestoreSettings, "", screens.RestoreSettingsMenuChoices, nil
	case 4: // About
		return screens.ScreenAbout, "", nil, nil
	case 5: // Exit
		return screens.ScreenMain, "", nil, tea.Quit
	}
	return screens.ScreenMain, "", screens.MainMenuChoices, nil
}
