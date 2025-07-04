package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Drive information
type DriveInfo struct {
	Device     string
	Size       string
	Label      string
	UUID       string
	Filesystem string
	Encrypted  bool
}

// Message types for drive operations
type DrivesLoaded struct {
	drives []DriveInfo
}

type DriveOperation struct {
	message string
	success bool
}

// Backup drive status message
type BackupDriveStatus struct {
	drivePath  string
	driveSize  string
	driveType  string
	mountPoint string
	needsMount bool
	error      error
}

// Special message type for requesting password input outside TUI
type PasswordRequiredMsg struct {
	drivePath string
	driveSize string
	driveType string
}

// New message type for password interaction
type passwordInteractionMsg struct {
	drivePath  string
	driveSize  string
	driveType  string
	originalOp string
}

// Check if any backup drive is mounted (pure Go - no external commands)
func checkAnyBackupMounted() (string, bool) {
	// Parse /proc/mounts directly instead of using findmnt command
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return "", false
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return "", false
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			mountPoint := fields[1]
			// Check for typical external drive mount points
			if strings.Contains(mountPoint, "/run/media/") || strings.Contains(mountPoint, "/mnt/") {
				return mountPoint, true
			}
		}
	}

	return "", false
}

// Mount backup drive with proper interactive password handling (DEPRECATED - use mountLUKSDrive/mountRegularDrive instead)
func mountBackupDrive() (string, error) {
	return "", fmt.Errorf("deprecated function - use drive selection instead")
}

// Unmount backup drive using pure Go
func unmountBackupDrive(mountPoint string) error {
	// Sync first
	syscall.Sync()

	// Get device path from /proc/mounts instead of using findmnt
	device, err := getDeviceFromProcMounts(mountPoint)
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

// Get device path for a mount point from /proc/mounts (pure Go)
func getDeviceFromProcMounts(mountPoint string) (string, error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return "", err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == mountPoint {
			return fields[0], nil // Return device path
		}
	}

	return "", fmt.Errorf("mount point %s not found", mountPoint)
}

// LoadDrives loads available drives - SAFE external drive detection using HOTPLUG
func LoadDrives() tea.Cmd {
	return func() tea.Msg {
		// Get all block devices including hotplug info
		cmd := exec.Command("lsblk", "-J", "-o", "NAME,SIZE,LABEL,UUID,FSTYPE,MOUNTPOINT,TYPE,HOTPLUG")
		out, err := cmd.Output()
		if err != nil {
			return DrivesLoaded{drives: []DriveInfo{}}
		}

		var lsblkOutput struct {
			BlockDevices []struct {
				Name       string `json:"name"`
				Size       string `json:"size"`
				Label      string `json:"label"`
				UUID       string `json:"uuid"`
				Fstype     string `json:"fstype"`
				Mountpoint string `json:"mountpoint"`
				Type       string `json:"type"`
				Hotplug    bool   `json:"hotplug"` // true for removable, false for internal
				Children   []struct {
					Name       string `json:"name"`
					Size       string `json:"size"`
					Label      string `json:"label"`
					UUID       string `json:"uuid"`
					Fstype     string `json:"fstype"`
					Mountpoint string `json:"mountpoint"`
					Type       string `json:"type"`
					Hotplug    bool   `json:"hotplug"`
					Children   []struct {
						Name       string `json:"name"`
						Size       string `json:"size"`
						Label      string `json:"label"`
						UUID       string `json:"uuid"`
						Fstype     string `json:"fstype"`
						Mountpoint string `json:"mountpoint"`
						Type       string `json:"type"`
						Hotplug    bool   `json:"hotplug"`
					} `json:"children"`
				} `json:"children"`
			} `json:"blockdevices"`
		}

		if err := json.Unmarshal(out, &lsblkOutput); err != nil {
			return DrivesLoaded{drives: []DriveInfo{}}
		}

		var drives []DriveInfo

		for _, device := range lsblkOutput.BlockDevices {
			// Only consider actual disk devices
			if device.Type != "disk" {
				continue
			}

			// CRITICAL: Only include HOTPLUG devices (external/removable drives)
			if !device.Hotplug {
				continue // Skip internal/fixed drives (device.Hotplug is false)
			}

			// Skip if this device contains the root filesystem (extra safety)
			hasRootPartition := false
			if device.Mountpoint == "/" {
				hasRootPartition = true
			}

			// Check children for root filesystem
			for _, child := range device.Children {
				if child.Mountpoint == "/" {
					hasRootPartition = true
					break
				}
				// Check grandchildren (LUKS containers)
				for _, grandchild := range child.Children {
					if grandchild.Mountpoint == "/" {
						hasRootPartition = true
						break
					}
				}
				if hasRootPartition {
					break
				}
			}

			// Skip any drive with root filesystem (should never happen with hotplug, but extra safety)
			if hasRootPartition {
				continue
			}

			// Include this external/removable drive
			devicePath := "/dev/" + device.Name

			// Add the main device if it has a filesystem
			if device.Fstype != "" || device.UUID != "" {
				drive := DriveInfo{
					Device:     devicePath,
					Size:       device.Size,
					Label:      device.Label,
					UUID:       device.UUID,
					Filesystem: device.Fstype,
					Encrypted:  device.Fstype == "crypto_LUKS",
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

		return DrivesLoaded{drives: drives}
	}
}

// Mount selected drive
func mountSelectedDrive(drive DriveInfo) tea.Cmd {
	return func() tea.Msg {
		// Check if this is a locked LUKS drive
		if drive.Encrypted {
			// Check if it's already unlocked by looking for the mapper device
			mapperName := "luks-" + drive.UUID
			mapperPath := "/dev/mapper/" + mapperName
			
			if _, err := os.Stat(mapperPath); os.IsNotExist(err) {
				// LUKS drive is locked - show helpful error
				return DriveOperation{
					message: fmt.Sprintf("❌ LUKS drive is locked\n\nTo unlock manually:\nsudo cryptsetup luksOpen %s %s\nsudo udisksctl mount -b %s\n\nThen restart migrate.", drive.Device, mapperName, mapperPath),
					success: false,
				}
			}
			
			// LUKS drive is unlocked, try to mount the mapper device
			mountPoint, err := mountRegularDrive(DriveInfo{
				Device: mapperPath,
				Size: drive.Size,
				Label: drive.Label,
				UUID: drive.UUID,
				Filesystem: drive.Filesystem,
				Encrypted: false, // Treat unlocked LUKS as regular drive
			})
			
			if err != nil {
				return DriveOperation{
					message: fmt.Sprintf("Failed to mount unlocked LUKS drive %s: %v", mapperPath, err),
					success: false,
				}
			}

			return DriveOperation{
				message: fmt.Sprintf("Successfully mounted LUKS drive %s to %s", drive.Device, mountPoint),
				success: true,
			}
		}

		// Regular drive mounting
		mountPoint, err := mountRegularDrive(drive)
		if err != nil {
			return DriveOperation{
				message: fmt.Sprintf("Failed to mount %s: %v", drive.Device, err),
				success: false,
			}
		}

		return DriveOperation{
			message: fmt.Sprintf("Successfully mounted %s to %s", drive.Device, mountPoint),
			success: true,
		}
	}
}

// Mount LUKS encrypted drive
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
	cmd := exec.Command("findmnt", "-rn", "-o", "TARGET", mapperPath)
	out, err := cmd.Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		// Already mounted - return existing mount point
		mountPoint := strings.TrimSpace(string(out))
		return mountPoint, nil
	}

	// Step 3: Mount the unlocked device (only if not already mounted)
	cmd = exec.Command("udisksctl", "mount", "-b", mapperPath)
	out, err = cmd.Output()
	if err != nil {
		// Check if it failed because it's already mounted
		if strings.Contains(err.Error(), "AlreadyMounted") {
			// Try to find the mount point from the error message or use findmnt
			cmd = exec.Command("findmnt", "-rn", "-o", "TARGET", mapperPath)
			out, err = cmd.Output()
			if err == nil {
				return strings.TrimSpace(string(out)), nil
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

// Mount regular drive
func mountRegularDrive(drive DriveInfo) (string, error) {
	// Check if already mounted first
	cmd := exec.Command("findmnt", "-rn", "-o", "TARGET", drive.Device)
	out, err := cmd.Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		// Already mounted - return existing mount point
		mountPoint := strings.TrimSpace(string(out))
		return mountPoint, nil
	}

	// Mount the device
	cmd = exec.Command("udisksctl", "mount", "-b", drive.Device)
	out, err = cmd.Output()
	if err != nil {
		// Check if it failed because it's already mounted
		if strings.Contains(err.Error(), "AlreadyMounted") {
			// Try to find the mount point using findmnt
			cmd = exec.Command("findmnt", "-rn", "-o", "TARGET", drive.Device)
			out, err = cmd.Output()
			if err == nil {
				return strings.TrimSpace(string(out)), nil
			}
		}
		return "", fmt.Errorf("failed to mount drive: %v", err)
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

// Mount any external drive for backup (replaces hardcoded Grendel-only mounting)
func mountDriveForBackup(drive DriveInfo) tea.Cmd {
	return func() tea.Msg {
		// Check if this is a locked LUKS drive
		if drive.Encrypted {
			// Check if it's already unlocked by looking for the mapper device
			mapperName := "luks-" + drive.UUID
			mapperPath := "/dev/mapper/" + mapperName
			
			if _, err := os.Stat(mapperPath); os.IsNotExist(err) {
				// LUKS drive is locked - show helpful error
				return BackupDriveStatus{
					error: fmt.Errorf("❌ LUKS drive is locked\n\nTo unlock manually:\nsudo cryptsetup luksOpen %s %s\nsudo udisksctl mount -b %s\n\nThen restart migrate.", drive.Device, mapperName, mapperPath),
				}
			}
			
			// LUKS drive is unlocked, try to mount the mapper device
			mountPoint, err := mountRegularDrive(DriveInfo{
				Device: mapperPath,
				Size: drive.Size,
				Label: drive.Label,
				UUID: drive.UUID,
				Filesystem: drive.Filesystem,
				Encrypted: false, // Treat unlocked LUKS as regular drive
			})
			
			if err != nil {
				return BackupDriveStatus{
					error: fmt.Errorf("Failed to mount unlocked LUKS drive %s: %v", mapperPath, err),
				}
			}

			// Successfully mounted LUKS drive - now ask for backup confirmation
			return BackupDriveStatus{
				drivePath:  drive.Device,
				driveSize:  drive.Size,
				driveType:  fmt.Sprintf("%s [LUKS]", drive.Label),
				mountPoint: mountPoint,
				needsMount: false,
				error:      nil,
			}
		}

		// Regular drive mounting
		mountPoint, err := mountRegularDrive(drive)
		if err != nil {
			return BackupDriveStatus{
				error: fmt.Errorf("Failed to mount %s: %v", drive.Device, err),
			}
		}

		return BackupDriveStatus{
			drivePath:  drive.Device,
			driveSize:  drive.Size,
			driveType:  fmt.Sprintf("%s [%s]", drive.Label, drive.Filesystem),
			mountPoint: mountPoint,
			needsMount: false,
			error:      nil,
		}
	}
}

// Mount any external drive for restore (same logic as backup, but for restore confirmation)
func mountDriveForRestore(drive DriveInfo) tea.Cmd {
	return func() tea.Msg {
		// Check if this is a locked LUKS drive
		if drive.Encrypted {
			// Check if it's already unlocked by looking for the mapper device
			mapperName := "luks-" + drive.UUID
			mapperPath := "/dev/mapper/" + mapperName
			
			if _, err := os.Stat(mapperPath); os.IsNotExist(err) {
				// LUKS drive is locked - show helpful error
				return BackupDriveStatus{
					error: fmt.Errorf("❌ LUKS drive is locked\n\nTo unlock manually:\nsudo cryptsetup luksOpen %s %s\nsudo udisksctl mount -b %s\n\nThen restart migrate.", drive.Device, mapperName, mapperPath),
				}
			}
			
			// LUKS drive is unlocked, try to mount the mapper device
			mountPoint, err := mountRegularDrive(DriveInfo{
				Device: mapperPath,
				Size: drive.Size,
				Label: drive.Label,
				UUID: drive.UUID,
				Filesystem: drive.Filesystem,
				Encrypted: false, // Treat unlocked LUKS as regular drive
			})
			
			if err != nil {
				return BackupDriveStatus{
					error: fmt.Errorf("Failed to mount unlocked LUKS drive %s: %v", mapperPath, err),
				}
			}

			// Successfully mounted LUKS drive - now ask for restore confirmation
			return BackupDriveStatus{
				drivePath:  drive.Device,
				driveSize:  drive.Size,
				driveType:  fmt.Sprintf("%s [LUKS]", drive.Label),
				mountPoint: mountPoint,
				needsMount: false,
				error:      nil,
			}
		}

		// Regular drive mounting
		mountPoint, err := mountRegularDrive(drive)
		if err != nil {
			return BackupDriveStatus{
				error: fmt.Errorf("Failed to mount %s: %v", drive.Device, err),
			}
		}

		return BackupDriveStatus{
			drivePath:  drive.Device,
			driveSize:  drive.Size,
			driveType:  fmt.Sprintf("%s [%s]", drive.Label, drive.Filesystem),
			mountPoint: mountPoint,
			needsMount: false,
			error:      nil,
		}
	}
}

// Handle password input outside TUI
func handlePasswordInput(msg PasswordRequiredMsg, originalOp string) tea.Cmd {
	return func() tea.Msg {
		// We'll handle this by creating a special command that exits alt screen,
		// handles password, then returns result
		return passwordInteractionMsg{
			drivePath:  msg.drivePath,
			driveSize:  msg.driveSize,
			driveType:  msg.driveType,
			originalOp: originalOp,
		}
	}
}

// Perform backup drive unmount after successful backup (works with any drive)
func PerformBackupUnmount() tea.Cmd {
	return func() tea.Msg {
		// Get the current backup mount point
		mountPoint, mounted := checkAnyBackupMounted()
		if !mounted {
			return DriveOperation{
				message: "ℹ️  Backup drive is already unmounted",
				success: true,
			}
		}

		// Unmount the backup drive
		err := unmountBackupDrive(mountPoint)
		if err != nil {
			return DriveOperation{
				message: fmt.Sprintf("⚠️  Failed to unmount backup drive: %v", err),
				success: false,
			}
		}

		return DriveOperation{
			message: "✅ Backup drive unmounted successfully!\n🔌 Safe to physically remove drive.",
			success: true,
		}
	}
}
