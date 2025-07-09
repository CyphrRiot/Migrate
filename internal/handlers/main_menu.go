package handlers

import (
	"migrate/internal/screens"

	tea "github.com/charmbracelet/bubbletea"
)

// MainMenuHandler handles main menu selections and returns the next screen state
type MainMenuHandler struct{}

// NewMainMenuHandler creates a new main menu handler
func NewMainMenuHandler() *MainMenuHandler {
	return &MainMenuHandler{}
}

// HandleSelection processes a main menu selection and returns the next state
func (h *MainMenuHandler) HandleSelection(cursor int) (screen screens.Screen, operation string, choices []string, cmd tea.Cmd) {
	switch cursor {
	case 0: // Backup
		return screens.ScreenBackup, "", screens.BackupMenuChoices, nil
	case 1: // Verify Backup
		return screens.ScreenVerify, "", screens.VerifyMenuChoices, nil
	case 2: // Restore
		return screens.ScreenRestore, "", screens.RestoreMenuChoices, nil
	case 3: // About
		return screens.ScreenAbout, "", nil, nil
	case 4: // Exit
		return screens.ScreenMain, "", nil, tea.Quit
	}
	return screens.ScreenMain, "", screens.MainMenuChoices, nil
}
