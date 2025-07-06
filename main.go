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
	"migrate/internal"
)

// Singleton instance checking
const lockFilePath = "/tmp/migrate.lock"

// Check if another migrate is running
func checkSingleInstance() error {
	// Check if lock file exists
	if _, err := os.Stat(lockFilePath); err == nil {
		// Lock file exists, check if process is actually running
		lockContent, readErr := os.ReadFile(lockFilePath)
		if readErr == nil {
			pid := strings.TrimSpace(string(lockContent))
			if pid != "" {
				// Check if process is still running
				if err := exec.Command("kill", "-0", pid).Run(); err == nil {
					return fmt.Errorf("another migrate process is already running (PID: %s)", pid)
				}
			}
		}
		// Stale lock file, remove it
		os.Remove(lockFilePath)
	}
	return nil
}

// Create instance lock
func createInstanceLock() error {
	pid := fmt.Sprintf("%d", os.Getpid())
	return os.WriteFile(lockFilePath, []byte(pid), 0644)
}

// Remove instance lock
func removeInstanceLock() {
	os.Remove(lockFilePath)
}

func main() {
	// Check if we need to elevate to root
	if os.Geteuid() != 0 {
		if err := elevateToRoot(); err != nil {
			fmt.Printf("❌ Failed to elevate privileges: %v\n", err)
			os.Exit(1)
		}
		// elevateToRoot() will re-exec this program with sudo, so we never reach here
		return
	}

	// We're now running as root, proceed with normal startup
	runAsRoot()
}

// Elevate to root privileges by re-running with sudo
func elevateToRoot() error {
	// Get the path to the current executable
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	// Check if sudo is available
	if !checkProgramExists("sudo") {
		return fmt.Errorf("sudo is required but not available")
	}

	// Beautiful privilege escalation prompt
	fmt.Println("🔒 Migrate requires administrator privileges")
	fmt.Println("📋 Needed for: drive mounting, LUKS encryption, system backup")
	fmt.Println("🚀 Requesting sudo access...")
	fmt.Println()

	// Re-run this program with sudo, preserving all arguments
	args := append([]string{execPath}, os.Args[1:]...)
	cmd := exec.Command("sudo", args...)
	
	// Connect stdio so user can enter password
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run and replace current process
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("sudo execution failed: %v", err)
	}

	// If we get here, the sudo command completed
	// Exit with the same code as the child process
	if exitError, ok := err.(*exec.ExitError); ok {
		os.Exit(exitError.ExitCode())
	}
	
	// Normal successful exit
	os.Exit(0)
	return nil // Never reached
}

// Main program logic when running as root
func runAsRoot() {
	// Check for another instance
	if err := checkSingleInstance(); err != nil {
		fmt.Println("⚠️  " + err.Error())
		fmt.Println()
		
		// Pretty error display
		fmt.Println("┌─────────────────────────────────────────┐")
		fmt.Println("│        🚫 Migration In Progress         │")
		fmt.Println("├─────────────────────────────────────────┤")
		fmt.Println("│                                         │")
		fmt.Println("│  Another migrate process is already     │")
		fmt.Println("│  running. Please wait for it to         │")
		fmt.Println("│  complete before starting a new one.    │")
		fmt.Println("│                                         │")
		fmt.Println("│  💡 If you're sure no other migrate     │")
		fmt.Println("│     is running, remove the lock file:   │")
		fmt.Println("│     sudo rm /tmp/migrate.lock           │")
		fmt.Println("│                                         │")
		fmt.Println("└─────────────────────────────────────────┘")
		fmt.Println()
		os.Exit(1)
	}

	// Create lock file
	if err := createInstanceLock(); err != nil {
		fmt.Printf("❌ Failed to create instance lock: %v\n", err)
		os.Exit(1)
	}

	// Ensure lock file is removed on exit
	defer removeInstanceLock()

	// Check required system programs (silently)
	if err := checkSystemDependencies(); err != nil {
		fmt.Printf("❌ Dependency check failed: %v\n", err)
		fmt.Println()
		fmt.Println("💡 Install missing dependencies and try again.")
		os.Exit(1)
	}

	// Set up signal handling for clean exit
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		removeInstanceLock() // Clean up on signal
		os.Exit(1)
	}()

	// Always run the beautiful TUI
	m := internal.InitialModel()
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
