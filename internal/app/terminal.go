// Package app provides application-level bootstrap logic for Migrate.
package app

import (
	"os"

	"golang.org/x/term"
)

// GetTerminalSize returns the current terminal width and height
// with reasonable fallbacks for when detection fails.
func GetTerminalSize() (int, int) {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80, 24
	}
	if width < 60 {
		width = 60
	}
	if height < 20 {
		height = 20
	}
	return width, height
}
