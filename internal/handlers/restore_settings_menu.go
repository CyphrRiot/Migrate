package handlers

import (
	"migrate/internal/screens"

	tea "github.com/charmbracelet/bubbletea"
)

// RestoreSettingsMenuHandler handles restore settings menu selections
type RestoreSettingsMenuHandler struct{}

// NewRestoreSettingsMenuHandler creates a new restore settings menu handler
func NewRestoreSettingsMenuHandler() *RestoreSettingsMenuHandler {
	return &RestoreSettingsMenuHandler{}
}

// HandleSelection processes a restore settings menu selection and returns the next state
func (h *RestoreSettingsMenuHandler) HandleSelection(cursor int) (screen screens.Screen, operation string, choices []string, cmd tea.Cmd) {
	switch cursor {
	case 0: // Restore Configuration Files (~/.config)
		// Go directly to drive selection for config restore
		return screens.ScreenDriveSelect, "config_restore", nil, nil
	case 1: // Restore Local Data (~/.local)
		// Go directly to drive selection for local data restore
		return screens.ScreenDriveSelect, "local_restore", nil, nil
	case 2: // Back
		return screens.ScreenMain, "", screens.MainMenuChoices, nil
	}
	return screens.ScreenRestoreSettings, "", screens.RestoreSettingsMenuChoices, nil
}
