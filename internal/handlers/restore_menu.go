package handlers

import (
	"migrate/internal/screens"

	tea "github.com/charmbracelet/bubbletea"
)

// RestoreMenuHandler handles restore menu selections
type RestoreMenuHandler struct{}

// NewRestoreMenuHandler creates a new restore menu handler
func NewRestoreMenuHandler() *RestoreMenuHandler {
	return &RestoreMenuHandler{}
}

// HandleSelection processes a restore menu selection and returns the next state
func (h *RestoreMenuHandler) HandleSelection(cursor int) (screen screens.Screen, operation string, choices []string, cmd tea.Cmd) {
	switch cursor {
	case 0: // Restore to Current System
		// Go directly to drive selection - config options are now on the unified selection screen
		return screens.ScreenDriveSelect, "system_restore", nil, nil
	case 1: // Restore to Custom Path
		// Go directly to drive selection - config options are now on the unified selection screen
		return screens.ScreenDriveSelect, "custom_restore", nil, nil
	case 2: // Back
		return screens.ScreenMain, "", screens.MainMenuChoices, nil
	}
	return screens.ScreenRestore, "", screens.RestoreMenuChoices, nil
}
