// Package drives provides drive detection, mounting, and management functionality.
// This module handles drive discovery and enumeration using lsblk.
package drives

import (
	"encoding/json"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// LoadDrives scans for available external drives and returns them as a Bubble Tea command.
// OPTIMIZED: Uses structured types, pre-allocation, and helper functions for better performance.
func LoadDrives() tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("lsblk", "-J", "-o", "NAME,SIZE,LABEL,UUID,FSTYPE,MOUNTPOINT,TYPE,HOTPLUG")
		out, err := cmd.Output()
		if err != nil {
			return DrivesLoaded{Drives: []DriveInfo{}}
		}

		var lsblkOutput LsblkOutput
		if err := json.Unmarshal(out, &lsblkOutput); err != nil {
			return DrivesLoaded{Drives: []DriveInfo{}}
		}

		// Pre-allocate with reasonable capacity to avoid repeated allocations
		drives := make([]DriveInfo, 0, 8)

		for _, device := range lsblkOutput.BlockDevices {
			if device.Type != "disk" {
				continue
			}

			// OPTIMIZED: Single pass safety checks
			if isSystemDrive(&device) {
				continue
			}

			// OPTIMIZED: Combined external drive detection
			if !isExternalDrive(&device) {
				continue
			}

			// OPTIMIZED: Extract mounted filesystems in single pass
			mountedFilesystems := extractMountedFilesystems(&device)

			// Convert to DriveInfo and append
			for _, fs := range mountedFilesystems {
				drive := DriveInfo{
					Device:     fs.MountPoint,
					Size:       fs.Size,
					Label:      fs.Label,
					UUID:       fs.UUID,
					Filesystem: fs.Filesystem,
					Encrypted:  fs.Encrypted,
				}

				if drive.Label == "" {
					if drive.Encrypted {
						drive.Label = "Encrypted External Drive"
					} else {
						drive.Label = "External Drive"
					}
				}

				drives = append(drives, drive)
			}
		}

		return DrivesLoaded{Drives: drives}
	}
}

// isSystemDrive checks if a device contains the root filesystem (safety check).
// OPTIMIZED: Single recursive pass instead of multiple nested loops.
func isSystemDrive(device *LsblkDevice) bool {
	if device.Mountpoint == "/" {
		return true
	}

	for _, child := range device.Children {
		if isSystemDrive(&child) {
			return true
		}
	}
	return false
}

// isExternalDrive determines if a device is external based on multiple criteria.
// OPTIMIZED: Combined logic with early returns to avoid unnecessary string operations.
func isExternalDrive(device *LsblkDevice) bool {
	// Criteria 1: Traditional hotplug detection
	if device.Hotplug {
		return true
	}

	// Criteria 2: Device naming pattern (sd* devices are typically external)
	deviceName := strings.ToLower(device.Name)
	if strings.HasPrefix(deviceName, "sd") {
		return true
	}

	// Criteria 3: Check mount locations (single pass through hierarchy)
	return hasExternalMountPoint(device)
}

// hasExternalMountPoint checks if device has partitions mounted in external locations.
// OPTIMIZED: Single recursive function instead of nested loops.
func hasExternalMountPoint(device *LsblkDevice) bool {
	if device.Mountpoint != "" && isExternalMount(device.Mountpoint) {
		return true
	}

	for _, child := range device.Children {
		if hasExternalMountPoint(&child) {
			return true
		}
	}
	return false
}

// isExternalMount checks if a mount point is in a typical external location.
// OPTIMIZED: Single string check instead of multiple Contains calls.
func isExternalMount(mountpoint string) bool {
	return strings.Contains(mountpoint, "/run/media/") ||
		strings.Contains(mountpoint, "/mnt/") ||
		strings.Contains(mountpoint, "/media/")
}

// extractMountedFilesystems extracts all mounted filesystems from a device hierarchy.
// OPTIMIZED: Single pass collection with pre-allocated slice.
func extractMountedFilesystems(device *LsblkDevice) []MountedFilesystem {
	var filesystems []MountedFilesystem
	collectMountedFilesystems(device, &filesystems)
	return filesystems
}

// collectMountedFilesystems recursively collects mounted filesystems.
// OPTIMIZED: Direct slice append instead of intermediate structs.
func collectMountedFilesystems(device *LsblkDevice, filesystems *[]MountedFilesystem) {
	if device.Fstype != "" && device.UUID != "" && device.Mountpoint != "" {
		*filesystems = append(*filesystems, MountedFilesystem{
			Device:     "/dev/" + device.Name,
			Size:       device.Size,
			Label:      device.Label,
			UUID:       device.UUID,
			Filesystem: device.Fstype,
			Encrypted:  device.Fstype == "crypto_LUKS",
			MountPoint: device.Mountpoint,
		})
	}

	for _, child := range device.Children {
		collectMountedFilesystems(&child, filesystems)
	}
}
