package handlers

import (
	"migrate/internal/platform"
	"migrate/internal/screens"

	tea "github.com/charmbracelet/bubbletea"
)

// BackupMenuHandler handles backup menu selections
type BackupMenuHandler struct {
	privilege platform.PrivLevel
}

// NewBackupMenuHandler creates a new backup menu handler
func NewBackupMenuHandler(priv platform.PrivLevel) *BackupMenuHandler {
	return &BackupMenuHandler{privilege: priv}
}

// HandleSelection processes a backup menu selection and returns the next state
func (h *BackupMenuHandler) HandleSelection(cursor int) (screen screens.Screen, operation string, choices []string, cmd tea.Cmd) {
	if h.privilege == platform.PrivUser {
		switch cursor {
		case 0: // Home Directory Only
			return screens.ScreenHomeFolderSelect, "home_backup", nil, nil
		case 1: // Backup System Settings
			return screens.ScreenDriveSelect, "settings_backup", nil, nil
		case 2: // Back
			return screens.ScreenMain, "", screens.MainMenuChoicesNonRoot, nil
		}
		return screens.ScreenBackup, "", screens.BackupMenuChoicesNonRoot, nil
	}

	// Root mode
	switch cursor {
	case 0: // Home Directory Only
		return screens.ScreenHomeFolderSelect, "home_backup", nil, nil
	case 1: // Complete System Backup
		return screens.ScreenDriveSelect, "system_backup", nil, nil
	case 2: // Back
		return screens.ScreenMain, "", screens.MainMenuChoices, nil
	}
	return screens.ScreenBackup, "", screens.BackupMenuChoices, nil
}
