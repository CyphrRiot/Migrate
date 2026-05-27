// Package app provides application-level bootstrap logic for Migrate.
package app

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"migrate/internal"
	"migrate/internal/platform"

	tea "github.com/charmbracelet/bubbletea"
)

// Run initializes and starts the Migrate TUI application.
// It handles singleton locking, dependency checks, signal handling,
// and Bubble Tea program execution.
func Run(perm platform.PrivLevel) {
	if err := CheckSingleInstance(); err != nil {
		fmt.Println("⚠️  " + err.Error())
		fmt.Println()
		errorBox := CreateResponsiveErrorBox(
			"🚫 Migration In Progress",
			"Another migrate process is already running. Please wait for it to complete before starting a new one.",
			[]string{
				"💡 If you're sure no other migrate is running, remove the lock file:",
				"   sudo rm " + lockFilePath,
			},
		)
		fmt.Println(errorBox)
		fmt.Println()
		os.Exit(1)
	}

	if err := CreateInstanceLock(); err != nil {
		fmt.Printf("❌ Failed to create instance lock: %v\n", err)
		os.Exit(1)
	}
	defer RemoveInstanceLock()

	if err := platform.CheckDependencies(); err != nil {
		fmt.Printf("❌ Dependency check failed: %v\n", err)
		fmt.Println()
		fmt.Println("💡 Install missing dependencies and try again.")
		os.Exit(1)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		RemoveInstanceLock()
		os.Exit(1)
	}()

	termWidth, termHeight := GetTerminalSize()
	m := internal.InitialModel(perm)
	m.SetInitialDimensions(termWidth, termHeight)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
