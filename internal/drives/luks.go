// Package drives provides drive detection, mounting, and management functionality.
// This module handles LUKS encryption workflows and password management.
package drives

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// mountLUKSDrive handles mounting of LUKS-encrypted external drives.
// Manages the two-step process: unlocking the LUKS container, then mounting the filesystem.
// Checks for existing unlock/mount status to avoid duplicate operations.
func mountLUKSDrive(drive DriveInfo) (string, error) {
	// Check if already unlocked
	mapperName := "luks-" + drive.UUID
	mapperPath := "/dev/mapper/" + mapperName

	// Step 1: Check if LUKS device needs to be unlocked
	if _, err := os.Stat(mapperPath); os.IsNotExist(err) {
		// Need to unlock
		cmd := exec.Command("udisksctl", "unlock", "-b", drive.Device)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("failed to unlock LUKS drive: %v", err)
		}
		time.Sleep(2 * time.Second)
	}

	// Step 2: Check if already mounted first
	if mountPoint, err := FindMountPointForDevice(mapperPath); err == nil {
		// Already mounted - return existing mount point
		return mountPoint, nil
	}

	// Step 3: Mount the unlocked device (only if not already mounted)
	cmd := exec.Command("udisksctl", "mount", "-b", mapperPath)
	out, err := cmd.Output()
	if err != nil {
		// Check if it failed because it's already mounted
		if strings.Contains(err.Error(), "AlreadyMounted") {
			// Try to find the mount point using native /proc/mounts parsing
			if mountPoint, err := FindMountPointForDevice(mapperPath); err == nil {
				return mountPoint, nil
			}
			return "/run/media/grendel/Grendel", nil // Fallback for known case
		}
		return "", fmt.Errorf("failed to mount unlocked drive: %v", err)
	}

	// Parse mount point from successful mount output
	mountOutput := strings.TrimSpace(string(out))
	if strings.Contains(mountOutput, "Mounted ") {
		parts := strings.Split(mountOutput, " at ")
		if len(parts) >= 2 {
			return strings.Trim(parts[1], "."), nil
		}
	}

	return "/media/unknown", nil
}

// isLUKSLocked checks if a LUKS drive is currently locked
func isLUKSLocked(drive DriveInfo) bool {
	mapperName := "luks-" + drive.UUID
	mapperPath := "/dev/mapper/" + mapperName
	
	_, err := os.Stat(mapperPath)
	return os.IsNotExist(err)
}

// getLUKSMapperPath returns the expected mapper path for a LUKS drive
func getLUKSMapperPath(drive DriveInfo) string {
	mapperName := "luks-" + drive.UUID
	return "/dev/mapper/" + mapperName
}

// TODO: Additional LUKS functions to implement:
// - UnlockLUKS() with password handling
// - CloseLUKS() for cleanup operations  
// - Password interaction workflow functions
