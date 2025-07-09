package handlers

import (
	"migrate/internal/screens"

	tea "github.com/charmbracelet/bubbletea"
)

// BackupMenuHandler handles backup menu selections
type BackupMenuHandler struct{}

// NewBackupMenuHandler creates a new backup menu handler
func NewBackupMenuHandler() *BackupMenuHandler {
	return &BackupMenuHandler{}
}

// HandleSelection processes a backup menu selection and returns the next state
func (h *BackupMenuHandler) HandleSelection(cursor int) (screen screens.Screen, operation string, choices []string, cmd tea.Cmd) {
	switch cursor {
	case 0: // Complete System Backup
		// LoadDrives is imported from drives.go
		return screens.ScreenDriveSelect, "system_backup", nil, nil
	case 1: // Home Directory Only
		// DiscoverHomeFoldersCmd is imported from drives.go
		return screens.ScreenHomeFolderSelect, "home_backup", nil, nil
	case 2: // Back
		return screens.ScreenMain, "", screens.MainMenuChoices, nil
	}
	return screens.ScreenBackup, "", screens.BackupMenuChoices, nil
}
