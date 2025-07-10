// Package drives provides drive detection, mounting, and management functionality.
// This module handles mount/unmount operations for external drives.
package drives

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// CheckAnyBackupMounted scans for mounted external drives using pure Go (no external commands).
// Returns the mount point and true if an external backup drive is currently mounted.
// OPTIMIZED: Uses buffered scanning instead of reading entire file into memory.
func CheckAnyBackupMounted() (string, bool) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return "", false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			mountPoint := fields[1]
			// OPTIMIZED: Check prefix first (faster than Contains)
			if strings.HasPrefix(mountPoint, "/run/media/") ||
				strings.HasPrefix(mountPoint, "/mnt/") {
				return mountPoint, true
			}
		}
	}

	return "", false
}

// UnmountBackupDrive safely unmounts and cleans up an external backup drive.
// Handles both regular drives and LUKS encrypted drives with proper cleanup.
// Performs filesystem sync before unmounting for data safety.
func UnmountBackupDrive(mountPoint string) error {
	// Sync first
	syscall.Sync()

	// Get device path from /proc/mounts instead of using findmnt
	device, err := GetDeviceFromProcMounts(mountPoint)
	if err != nil {
		return fmt.Errorf("failed to find device for %s: %v", mountPoint, err)
	}

	// Unmount
	cmd := exec.Command("sudo", "umount", mountPoint)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to unmount: %v", err)
	}

	// Close LUKS device if it's a mapper device
	if strings.Contains(device, "mapper") {
		mapperName := filepath.Base(device)
		cmd = exec.Command("sudo", "cryptsetup", "close", mapperName)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to close LUKS device: %v", err)
		}
	}

	return nil
}

// FindMountPointForDevice replaces findmnt with native /proc/mounts parsing
// Returns the mount point for a given device path
func FindMountPointForDevice(device string) (string, error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == device {
			return fields[1], nil // Return mount point
		}
	}

	return "", fmt.Errorf("device not found in /proc/mounts")
}

// GetDeviceFromProcMounts finds the device path for a given mount point by parsing /proc/mounts.
// OPTIMIZED: Uses buffered scanning and early termination for better performance.
func GetDeviceFromProcMounts(mountPoint string) (string, error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[1] == mountPoint {
			return fields[0], nil // Return device path
		}
	}

	return "", fmt.Errorf("mount point %s not found", mountPoint)
}

// MountRegularDrive handles mounting of standard (non-encrypted) external drives.
// OPTIMIZED: Cleaner logic flow with early returns and reduced string operations.
func MountRegularDrive(drive DriveInfo) (string, error) {
	// OPTIMIZED: Determine device path and mount status in single check
	devicePath, mountPoint, alreadyMounted := resolveDriveDevice(drive.Device)

	if alreadyMounted {
		return mountPoint, nil
	}

	// OPTIMIZED: Mount device with streamlined error handling
	return mountDevice(devicePath)
}

// resolveDriveDevice determines the actual device path and current mount status.
// OPTIMIZED: Single function to handle all device resolution logic.
func resolveDriveDevice(device string) (devicePath, mountPoint string, alreadyMounted bool) {
	// Check if device is already a mount point
	if strings.HasPrefix(device, "/") && !strings.HasPrefix(device, "/dev/") {
		// Verify mount point exists
		if _, err := os.Stat(device); err == nil {
			return "", device, true
		}

		// Find actual device from mount point
		if dev, err := GetDeviceFromProcMounts(device); err == nil {
			return dev, device, true
		}
		return "", "", false
	}

	// Device path provided - check if already mounted
	if mountPt, err := FindMountPointForDevice(device); err == nil {
		return device, mountPt, true
	}

	return device, "", false
}

// mountDevice performs the actual mounting operation.
// OPTIMIZED: Focused function with cleaner output parsing.
func mountDevice(devicePath string) (string, error) {
	cmd := exec.Command("udisksctl", "mount", "-b", devicePath)
	out, err := cmd.Output()
	if err != nil {
		// Handle already mounted case
		if strings.Contains(err.Error(), "AlreadyMounted") {
			if mountPoint, err := FindMountPointForDevice(devicePath); err == nil {
				return mountPoint, nil
			}
		}
		return "", fmt.Errorf("failed to mount drive: %v", err)
	}

	// OPTIMIZED: Parse mount point with single string operation
	mountOutput := strings.TrimSpace(string(out))
	if idx := strings.Index(mountOutput, " at "); idx != -1 {
		mountPoint := strings.TrimSuffix(mountOutput[idx+4:], ".")
		return mountPoint, nil
	}

	return "/media/unknown", nil
}
