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
)

const (
	appName    = "Migrate"
	appVersion = "1.0.0"
)

func main() {
	// Simple root check
	if os.Geteuid() != 0 {
		fmt.Println("❌ YOU MUST RUN AS ROOT")
		fmt.Println("Run: sudo migrate")
		os.Exit(1)
	}

	fmt.Println("🚀 Starting Migrate v1.0.0 - Pure Go Backup & Restore Tool")
	fmt.Println("🔍 Checking system dependencies...")

	// Check required system programs
	if err := checkSystemDependencies(); err != nil {
		fmt.Printf("❌ Dependency check failed: %v\n", err)
		fmt.Println()
		fmt.Println("💡 Install missing dependencies and try again.")
		os.Exit(1)
	}

	fmt.Println("✅ All dependencies available!")
	fmt.Println()

	// Set up signal handling for clean exit
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		os.Exit(1)
	}()

	// Always run the beautiful TUI
	m := initialModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

// Check if we already have sudo access
func hasSudoAccess() bool {
	cmd := exec.Command("sudo", "-n", "true")
	err := cmd.Run()
	return err == nil
}

// Check all required system dependencies
func checkSystemDependencies() error {
	// Required programs for core functionality
	requiredPrograms := []struct{
		name string
		purpose string  
		critical bool
	}{
		// Drive detection and information
		{"lsblk", "drive detection and information", true},
		
		// Drive mounting/unmounting
		{"udisksctl", "drive mounting and unmounting", true},
		{"umount", "drive unmounting", true},
		
		// LUKS encryption support
		{"cryptsetup", "LUKS encryption/decryption", true},
		
		// System information
		{"uname", "system information for backup metadata", true},
		{"hostname", "hostname for backup metadata", false},
		
		// Sudo access validation
		{"sudo", "privilege escalation", true},
	}

	missing := []string{}
	warnings := []string{}

	for _, prog := range requiredPrograms {
		if !checkProgramExists(prog.name) {
			if prog.critical {
				missing = append(missing, fmt.Sprintf("%s (%s)", prog.name, prog.purpose))
			} else {
				warnings = append(warnings, fmt.Sprintf("%s (%s)", prog.name, prog.purpose))
			}
		}
	}

	// Show warnings for non-critical missing programs
	if len(warnings) > 0 {
		fmt.Println("⚠️  Optional programs missing (functionality may be limited):")
		for _, warning := range warnings {
			fmt.Printf("   • %s\n", warning)
		}
		fmt.Println()
	}

	// Check for critical missing programs  
	if len(missing) > 0 {
		return fmt.Errorf("missing critical programs:\n%s\n\n🔧 Installation commands:\n%s", 
			formatMissingList(missing), 
			getInstallCommands(missing))
	}

	// Additional checks
	if err := checkSpecialRequirements(); err != nil {
		return err
	}

	return nil
}

// Check if a program exists in PATH
func checkProgramExists(program string) bool {
	_, err := exec.LookPath(program)
	return err == nil
}

// Format the missing programs list
func formatMissingList(missing []string) string {
	result := ""
	for _, prog := range missing {
		result += fmt.Sprintf("   • %s\n", prog)
	}
	return result
}

// Get installation commands for missing programs
func getInstallCommands(missing []string) string {
	commands := []string{}
	
	needsLsblk := false
	needsUdisks := false
	needsCryptsetup := false
	needsUtil := false

	for _, prog := range missing {
		if contains(prog, "lsblk") {
			needsLsblk = true
		}
		if contains(prog, "udisksctl") {
			needsUdisks = true  
		}
		if contains(prog, "cryptsetup") {
			needsCryptsetup = true
		}
		if contains(prog, "uname") || contains(prog, "umount") {
			needsUtil = true
		}
	}

	// Debian/Ubuntu
	debianPkgs := []string{}
	if needsLsblk || needsUtil {
		debianPkgs = append(debianPkgs, "util-linux")
	}
	if needsUdisks {
		debianPkgs = append(debianPkgs, "udisks2")
	}
	if needsCryptsetup {
		debianPkgs = append(debianPkgs, "cryptsetup")
	}

	// Arch Linux  
	archPkgs := []string{}
	if needsLsblk || needsUtil {
		archPkgs = append(archPkgs, "util-linux")
	}
	if needsUdisks {
		archPkgs = append(archPkgs, "udisks2")
	}
	if needsCryptsetup {
		archPkgs = append(archPkgs, "cryptsetup")
	}

	if len(debianPkgs) > 0 {
		commands = append(commands, fmt.Sprintf("   Debian/Ubuntu: sudo apt install %s", strings.Join(debianPkgs, " ")))
	}
	if len(archPkgs) > 0 {
		commands = append(commands, fmt.Sprintf("   Arch Linux:    sudo pacman -S %s", strings.Join(archPkgs, " ")))
	}
	
	return strings.Join(commands, "\n")
}

// Check special requirements beyond just program existence
func checkSpecialRequirements() error {
	// Check if we can actually use sudo
	if !hasSudoAccess() {
		return fmt.Errorf("sudo access required but not available\n" +
			"💡 Run 'sudo -v' to authenticate or add your user to sudoers")
	}

	// Check if /proc/mounts is accessible (should always be, but let's be sure)
	if _, err := os.Stat("/proc/mounts"); err != nil {
		return fmt.Errorf("/proc/mounts not accessible - this is unusual and may indicate a problem")
	}

	// Check if /sys/block exists (used for device detection)
	if _, err := os.Stat("/sys/block"); err != nil {
		return fmt.Errorf("/sys/block not accessible - device detection may fail")
	}

	return nil
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
