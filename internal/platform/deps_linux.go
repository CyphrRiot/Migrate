//go:build linux

package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

// CheckDependencies validates that all required Linux system programs are available.
func CheckDependencies() error {
	if err := CheckRequirements(); err != nil {
		return err
	}

	requiredPrograms := []struct {
		name     string
		purpose  string
		critical bool
	}{
		{"lsblk", "drive detection and information", true},
		{"udisksctl", "drive mounting and unmounting", true},
		{"umount", "drive unmounting", true},
		{"cryptsetup", "LUKS encryption/decryption", true},
	}

	var missing []string
	var warnings []string

	for _, prog := range requiredPrograms {
		if _, err := exec.LookPath(prog.name); err != nil {
			if prog.critical {
				missing = append(missing, fmt.Sprintf("%s (%s)", prog.name, prog.purpose))
			} else {
				warnings = append(warnings, fmt.Sprintf("%s (%s)", prog.name, prog.purpose))
			}
		}
	}

	if len(warnings) > 0 {
		fmt.Println("⚠️  Optional programs missing (functionality may be limited):")
		for _, w := range warnings {
			fmt.Printf("   • %s\n", w)
		}
		fmt.Println()
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing critical programs:\n%s\n\n🔧 Installation commands:\n%s",
			formatMissingList(missing),
			getInstallCommands(missing))
	}

	return nil
}

func formatMissingList(missing []string) string {
	result := ""
	for _, prog := range missing {
		result += fmt.Sprintf("   • %s\n", prog)
	}
	return result
}

func getInstallCommands(missing []string) string {
	commands := []string{}

	needsLsblk := false
	needsUdisks := false
	needsCryptsetup := false
	needsUtil := false

	for _, prog := range missing {
		if strings.Contains(prog, "lsblk") {
			needsLsblk = true
		}
		if strings.Contains(prog, "udisksctl") {
			needsUdisks = true
		}
		if strings.Contains(prog, "cryptsetup") {
			needsCryptsetup = true
		}
		if strings.Contains(prog, "umount") {
			needsUtil = true
		}
	}

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
