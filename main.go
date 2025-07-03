package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	appName    = "Migrate"
	appVersion = "1.0.0"
)

func main() {
	// Simple root check
	if os.Geteuid() != 0 {
		fmt.Println("❌ YOU MUST RUN AS ROOT")
		fmt.Println("Run: sudo ./bin/migrate")
		os.Exit(1)
	}

	if len(os.Args) > 1 {
		// Handle command line arguments for backwards compatibility
		switch os.Args[1] {
		case "backup":
			runBackup()
		case "restore":
			target := "/"
			if len(os.Args) > 2 {
				target = os.Args[2]
			}
			runRestore(target)
		case "unlock-luks":
			// Hidden command to unlock LUKS drive
			unlockLUKSAndRestart()
			return
		case "help", "--help", "-h":
			showCLIHelp()
		default:
			fmt.Printf("Unknown command: %s\n", os.Args[1])
			showCLIHelp()
			os.Exit(1)
		}
		return
	}

	// Check if LUKS drive needs unlocking
	if needsLUKSUnlock() {
		fmt.Println("🔒 LUKS Password Required")
		fmt.Println("=========================")
		fmt.Println("Your backup drive is encrypted and needs to be unlocked.")
		fmt.Println("Please enter your LUKS password when prompted...")
		fmt.Println()
		
		// Try to unlock the drive
		if unlockLUKSDrive() {
			fmt.Println("\n✅ Drive unlocked successfully!")
			fmt.Println("Starting migration tool...")
			fmt.Println()
		} else {
			fmt.Println("\n❌ Failed to unlock drive. Please try again.")
			os.Exit(1)
		}
	}

	// Set up signal handling to cleanup backup process on exit
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cleanupBackupProcess()
		os.Exit(1)
	}()

	// Run the beautiful TUI
	m := initialModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	
	if _, err := p.Run(); err != nil {
		cleanupBackupProcess() // Cleanup on any exit
		log.Fatal(err)
	}
	
	// Cleanup on normal exit
	cleanupBackupProcess()
}

// Check if we already have sudo access
func hasSudoAccess() bool {
	cmd := exec.Command("sudo", "-n", "true")
	err := cmd.Run()
	return err == nil
}

func showCLIHelp() {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true)

	fmt.Printf("%s\n", style.Render("Migrate - Beautiful Backup & Restore Tool"))
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  migrate                 - Launch interactive TUI")
	fmt.Println("  migrate backup          - Run backup in CLI mode")
	fmt.Println("  migrate restore [path]  - Run restore in CLI mode")
	fmt.Println("  migrate help            - Show this help")
}

func runBackup() {
	fmt.Println("Running backup in CLI mode...")
	
	// Check if any backup drive is mounted (works with any external drive)
	mountPoint, mounted := checkAnyBackupMounted()
	if !mounted {
		fmt.Println("❌ No external backup drive found or mounted!")
		fmt.Println("Please mount your backup drive first with:")
		fmt.Println("  ./migrate")
		fmt.Println("Then select 'Mount External Drive'")
		os.Exit(1)
	}
	
	fmt.Printf("✅ Found backup drive at: %s\n", mountPoint)
	
	// Run the actual backup
	config := BackupConfig{
		SourcePath:      "/",
		DestinationPath: mountPoint,
		ExcludePatterns: ExcludePatterns,
		BackupType:      "Complete System",
	}
	
	// Execute backup synchronously
	result := startBackup(config)()
	
	// Handle result
	if progressUpdate, ok := result.(ProgressUpdate); ok {
		if progressUpdate.Error != nil {
			fmt.Printf("❌ Backup failed: %v\n", progressUpdate.Error)
			os.Exit(1)
		} else if progressUpdate.Done {
			fmt.Println("✅ Backup completed successfully!")
		}
	}
}

func runRestore(target string) {
	fmt.Printf("Running restore to %s in CLI mode...\n", target)
	// TODO: Implement CLI restore
}

// Check if LUKS drive needs unlocking
func needsLUKSUnlock() bool {
	// Check if backup drive exists
	cmd := exec.Command("lsblk", "-o", "UUID")
	out, err := cmd.Output()
	if err != nil || !strings.Contains(string(out), "d251bd57-1925-4290-b109-21e7b8b8bab8") {
		return false // No backup drive found
	}

	// Check if LUKS device already exists (unlocked)
	mapperPath := "/dev/mapper/luks-d251bd57-1925-4290-b109-21e7b8b8bab8"
	if _, err := os.Stat(mapperPath); err == nil {
		return false // Already unlocked
	}

	return true // Needs unlocking
}

// Unlock LUKS drive
func unlockLUKSDrive() bool {
	driveBy := "/dev/disk/by-uuid/d251bd57-1925-4290-b109-21e7b8b8bab8"
	cmd := exec.Command("udisksctl", "unlock", "-b", driveBy)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	err := cmd.Run()
	return err == nil
}

// Unlock LUKS and restart (unused for now)
func unlockLUKSAndRestart() {
	// This could be used for a restart approach if needed
}
