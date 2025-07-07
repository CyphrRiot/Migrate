// Package internal provides drive detection, mounting, and management functionality for Migrate.
//
// This package handles all aspects of external drive interaction including:
//   - Drive discovery and enumeration using lsblk
//   - LUKS encrypted drive support and unlocking workflows
//   - Space requirement validation for backup and restore operations
//   - Mount and unmount operations with proper cleanup
//   - Home directory structure analysis for selective backups
//   - Safety checks to prevent mounting system drives
//
// The drive system supports both regular and LUKS-encrypted external drives,
// with automatic detection of mount points and proper handling of various
// filesystem types. All operations are designed to be safe and prevent
// accidental damage to the system drive.
package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// HomeFolderInfo represents metadata about a directory within the user's home folder.
// Used for selective backup operations to track folder size, visibility, and user selections.
type HomeFolderInfo struct {
	Name          string // Directory name (e.g., "Documents", ".config")
	Path          string // Full absolute path to the directory
	Size          int64  // Total size in bytes (calculated recursively)
	IsVisible     bool   // false for dotfiles/dotdirs (hidden folders)
	Selected      bool   // Current user selection state for backup inclusion
	AlwaysInclude bool   // true for dotdirs (automatically included, non-selectable)
}

// DriveInfo contains metadata about an external drive available for backup operations.
// Represents both the physical device and its current mount status.
type DriveInfo struct {
	Device     string // Mount point path (e.g., "/run/media/user/drive") or device path
	Size       string // Human-readable size string (e.g., "1.5T", "500G")
	Label      string // Volume label or friendly name
	UUID       string // Filesystem UUID for identification
	Filesystem string // Filesystem type (e.g., "ext4", "ntfs", "crypto_LUKS")
	Encrypted  bool   // true if this is an encrypted drive (LUKS)
}

// DrivesLoaded is a Bubble Tea message containing the results of drive enumeration.
type DrivesLoaded struct {
	drives []DriveInfo // List of discovered external drives
}

// DriveOperation reports the result of a drive mount/unmount operation.
type DriveOperation struct {
	message string // Human-readable status or error message
	success bool   // true if the operation completed successfully
}

// BackupDriveStatus provides detailed information about drive mounting for backup operations.
// Contains all necessary data for the UI to display confirmation dialogs and proceed with operations.
type BackupDriveStatus struct {
	drivePath  string // Original drive identifier
	driveSize  string // Human-readable size
	driveType  string // Descriptive type string (e.g., "External Drive [ext4]", "Encrypted [LUKS]")
	mountPoint string // Current mount point path
	needsMount bool   // true if drive still needs mounting (typically false after mounting)
	error      error  // Non-nil if mounting failed or space check failed
}

// PasswordRequiredMsg signals that LUKS password input is needed outside the TUI.
// This message type is used to coordinate password entry workflows.
type PasswordRequiredMsg struct {
	drivePath string // Path to the encrypted drive
	driveSize string // Human-readable size for display
	driveType string // Drive type description
}

// passwordInteractionMsg handles password interaction workflows for LUKS drives.
// Used internally for coordinating password entry and subsequent operations.
type passwordInteractionMsg struct {
	drivePath  string // Path to the encrypted drive
	driveSize  string // Human-readable size for display  
	driveType  string // Drive type description
	originalOp string // The original operation that required password input
}

// checkAnyBackupMounted scans for mounted external drives using pure Go (no external commands).
// Returns the mount point and true if an external backup drive is currently mounted.
// Parses /proc/mounts directly for efficiency and reliability.
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

// mountBackupDrive is deprecated in favor of the new drive selection system.
// Use LoadDrives() and mountDriveForBackup() instead for proper drive selection.
func mountBackupDrive() (string, error) {
	return "", fmt.Errorf("deprecated function - use drive selection instead")
}

// unmountBackupDrive safely unmounts and cleans up an external backup drive.
// Handles both regular drives and LUKS encrypted drives with proper cleanup.
// Performs filesystem sync before unmounting for data safety.
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

// getDeviceFromProcMounts finds the device path for a given mount point using pure Go.
// Parses /proc/mounts to avoid external command dependencies.
// Returns the device path (e.g., "/dev/sdb1") for the specified mount point.
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

// parseDriveSize converts human-readable size strings to bytes.
// Supports standard units: B, K, M, G, T, P (case-insensitive).
// Examples: "1.5T" -> 1,649,267,441,664 bytes, "500G" -> 537,109,987,328 bytes
func parseDriveSize(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(sizeStr)
	if len(sizeStr) < 2 {
		return 0, fmt.Errorf("invalid size string: %s", sizeStr)
	}
	
	// Get the unit (last character)
	unit := strings.ToUpper(sizeStr[len(sizeStr)-1:])
	numberStr := sizeStr[:len(sizeStr)-1]
	
	// Parse the number part
	var number float64
	var err error
	if _, err = fmt.Sscanf(numberStr, "%f", &number); err != nil {
		return 0, fmt.Errorf("invalid number in size: %s", numberStr)
	}
	
	// Convert based on unit
	var multiplier int64
	switch unit {
	case "B":
		multiplier = 1
	case "K":
		multiplier = 1024
	case "M":
		multiplier = 1024 * 1024
	case "G":
		multiplier = 1024 * 1024 * 1024
	case "T":
		multiplier = 1024 * 1024 * 1024 * 1024
	case "P":
		multiplier = 1024 * 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown size unit: %s", unit)
	}
	
	return int64(number * float64(multiplier)), nil
}

// checkBackupSpaceRequirements validates that an external drive has sufficient space for system backup.
// Compares the used space on the root filesystem against the total capacity of the external drive.
// Returns an error with detailed space information if the drive is too small.
func checkBackupSpaceRequirements(externalDriveSize string) error {
	// Get used space on internal drive (what we need to backup)
	internalUsedSpace, err := getUsedDiskSpace("/")
	if err != nil {
		return fmt.Errorf("failed to get internal drive usage: %v", err)
	}
	
	// Parse external drive total size
	externalTotalSize, err := parseDriveSize(externalDriveSize)
	if err != nil {
		return fmt.Errorf("failed to parse external drive size: %v", err)
	}
	
	// Check: internal_used_space <= external_total_size
	if internalUsedSpace > externalTotalSize {
		return fmt.Errorf("⚠️ INSUFFICIENT SPACE for backup\n\nInternal drive used: %s\nExternal drive total: %s\n\nThe external drive is too small to hold your backup.\nYou need at least %s of total drive capacity.",
			FormatBytes(internalUsedSpace),
			FormatBytes(externalTotalSize), 
			FormatBytes(internalUsedSpace))
	}
	
	return nil
}

// Check if external drive has enough space for selective home backup
func CheckSelectiveHomeBackupSpaceRequirements(selectedFolders []HomeFolderInfo, externalDriveSize string) error {
	// Calculate total size of selected folders
	var totalSelectedSize int64
	for _, folder := range selectedFolders {
		if folder.AlwaysInclude {
			// Hidden folders are always included
			totalSelectedSize += folder.Size
		} else if folder.Selected {
			// Visible folders only if selected
			totalSelectedSize += folder.Size
		}
	}
	
	// Parse external drive total size
	externalTotalSize, err := parseDriveSize(externalDriveSize)
	if err != nil {
		return fmt.Errorf("failed to parse external drive size: %v", err)
	}
	
	// Check: selected_folders_size <= external_total_size
	if totalSelectedSize > externalTotalSize {
		return fmt.Errorf("⚠️ INSUFFICIENT SPACE for selective home backup\n\nSelected folders size: %s\nExternal drive total: %s\n\nThe external drive is too small to hold your selected folders.\nYou need at least %s of total drive capacity.",
			FormatBytes(totalSelectedSize),
			FormatBytes(externalTotalSize), 
			FormatBytes(totalSelectedSize))
	}
	
	return nil
}

// Check if external drive has enough space for home backup
func CheckHomeBackupSpaceRequirements(externalDriveSize string) error {
	// Get actual home directory size instead of full internal drive
	homeDirSize, err := GetHomeDirSize()
	if err != nil {
		return fmt.Errorf("failed to calculate home directory size: %v", err)
	}
	
	// Parse external drive total size
	externalTotalSize, err := parseDriveSize(externalDriveSize)
	if err != nil {
		return fmt.Errorf("failed to parse external drive size: %v", err)
	}
	
	// Check: home_directory_size <= external_total_size
	if homeDirSize > externalTotalSize {
		return fmt.Errorf("⚠️ INSUFFICIENT SPACE for home backup\n\nHome directory size: %s\nExternal drive total: %s\n\nThe external drive is too small to hold your home directory.\nYou need at least %s of total drive capacity.",
			FormatBytes(homeDirSize),
			FormatBytes(externalTotalSize), 
			FormatBytes(homeDirSize))
	}
	
	return nil
}

// Check if internal drive has enough space for restore  
func checkRestoreSpaceRequirements(externalDriveSize string, externalMountPoint string) error {
	// Get used space on external drive (backup size)
	externalUsedSpace, err := getUsedDiskSpace(externalMountPoint)
	if err != nil {
		return fmt.Errorf("failed to get backup drive usage: %v", err)
	}
	
	// Get total size of internal drive
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return fmt.Errorf("failed to get internal drive info: %v", err)
	}
	
	internalTotalSize := int64(stat.Blocks) * int64(stat.Bsize)
	
	// Check: external_used_space <= internal_total_size  
	if externalUsedSpace > internalTotalSize {
		return fmt.Errorf("⚠️ INSUFFICIENT SPACE for restore\n\nBackup size: %s\nInternal drive total: %s\n\nThe backup is too large to fit on your internal drive.\nYou need at least %s of total drive capacity.",
			FormatBytes(externalUsedSpace),
			FormatBytes(internalTotalSize),
			FormatBytes(externalUsedSpace))
	}
	
	return nil
}
// LoadDrives scans for available external drives and returns them as a Bubble Tea command.
// Uses lsblk to enumerate drives with safety checks to prevent listing system drives.
// Supports multiple detection criteria: hotplug flag, device naming, and mount location analysis.
// Only returns drives that are currently mounted and accessible.
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

			// Check if this device contains the root filesystem (safety check)
			hasRootPartition := false
			if device.Mountpoint == "/" {
				hasRootPartition = true
			}

			// Check all levels for root filesystem
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

			// CRITICAL SAFETY: Skip any drive with root filesystem
			if hasRootPartition {
				continue
			}

			// NEW IMPROVED LOGIC: Include external drives based on multiple criteria
			devicePath := "/dev/" + device.Name
			isExternalDrive := false
			
			// Criteria 1: Traditional hotplug detection (USB drives, etc.)
			if device.Hotplug {
				isExternalDrive = true
			}
			
			// Criteria 2: Check if it's a device that's likely external based on naming
			// Many modern external drives don't report hotplug=true properly
			deviceName := strings.ToLower(device.Name)
			if strings.HasPrefix(deviceName, "sd") {  // SATA/USB drives usually start with sd*
				isExternalDrive = true
			}
			
			// Criteria 3: Check if any partitions are mounted in typical external locations
			for _, child := range device.Children {
				if child.Mountpoint != "" {
					mount := child.Mountpoint
					if strings.Contains(mount, "/run/media/") || 
					   strings.Contains(mount, "/mnt/") ||
					   strings.Contains(mount, "/media/") {
						isExternalDrive = true
						break
					}
				}
				// Check LUKS children too
				for _, grandchild := range child.Children {
					if grandchild.Mountpoint != "" {
						mount := grandchild.Mountpoint
						if strings.Contains(mount, "/run/media/") || 
						   strings.Contains(mount, "/mnt/") ||
						   strings.Contains(mount, "/media/") {
							isExternalDrive = true
							break
						}
					}
				}
				if isExternalDrive {
					break
				}
			}

			// Skip if this doesn't look like an external drive
			if !isExternalDrive {
				continue
			}

			// Look for mounted filesystems in the device or its partitions
			var mountedFilesystems []struct{
				Device     string
				Size       string
				Label      string
				UUID       string
				Filesystem string
				Encrypted  bool
				MountPoint string
			}

			// Check main device for mounted filesystem
			if device.Fstype != "" && device.UUID != "" && device.Mountpoint != "" {
				mountedFilesystems = append(mountedFilesystems, struct{
					Device     string
					Size       string
					Label      string
					UUID       string
					Filesystem string
					Encrypted  bool
					MountPoint string
				}{
					Device:     devicePath,
					Size:       device.Size,
					Label:      device.Label,
					UUID:       device.UUID,
					Filesystem: device.Fstype,
					Encrypted:  device.Fstype == "crypto_LUKS",
					MountPoint: device.Mountpoint,
				})
			}

			// Check partitions for mounted filesystems
			for _, child := range device.Children {
				if child.Fstype != "" && child.UUID != "" && child.Mountpoint != "" {
					childPath := "/dev/" + child.Name
					mountedFilesystems = append(mountedFilesystems, struct{
						Device     string
						Size       string
						Label      string
						UUID       string
						Filesystem string
						Encrypted  bool
						MountPoint string
					}{
						Device:     childPath,
						Size:       child.Size,
						Label:      child.Label,
						UUID:       child.UUID,
						Filesystem: child.Fstype,
						Encrypted:  child.Fstype == "crypto_LUKS",
						MountPoint: child.Mountpoint,
					})
				}

				// Check LUKS children for mounted filesystems
				for _, grandchild := range child.Children {
					if grandchild.Fstype != "" && grandchild.UUID != "" && grandchild.Mountpoint != "" {
						grandchildPath := "/dev/" + grandchild.Name
						mountedFilesystems = append(mountedFilesystems, struct{
							Device     string
							Size       string
							Label      string
							UUID       string
							Filesystem string
							Encrypted  bool
							MountPoint string
						}{
							Device:     grandchildPath,
							Size:       grandchild.Size,
							Label:      grandchild.Label,
							UUID:       grandchild.UUID,
							Filesystem: grandchild.Fstype,
							Encrypted:  false, // LUKS children are already unlocked
							MountPoint: grandchild.Mountpoint,
						})
					}
				}
			}

			// Add only mounted filesystems as drives (like the old working logic)
			for _, filesystem := range mountedFilesystems {
				drive := DriveInfo{
					Device:     filesystem.MountPoint, // USE MOUNT POINT, NOT DEVICE PATH!
					Size:       filesystem.Size,
					Label:      filesystem.Label,
					UUID:       filesystem.UUID,
					Filesystem: filesystem.Filesystem,
					Encrypted:  filesystem.Encrypted,
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

// mountRegularDrive handles mounting of standard (non-encrypted) external drives.
// Checks for existing mount status and uses udisksctl for safe mounting.
// Returns the mount point path on success.
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
	return mountDriveForOperation(drive, "system_backup")
}

// Mount any external drive for selective home backup with accurate space checking
func mountDriveForSelectiveHomeBackup(drive DriveInfo, homeFolders []HomeFolderInfo, selectedFolders map[string]bool) tea.Cmd {
	return func() tea.Msg {
		// Create a list with correct selection state for space calculation
		foldersWithSelections := make([]HomeFolderInfo, len(homeFolders)) 
		copy(foldersWithSelections, homeFolders)
		
		// Update the Selected field based on current user selections
		for i := range foldersWithSelections {
			if foldersWithSelections[i].IsVisible {
				// For visible folders, use user selection
				foldersWithSelections[i].Selected = selectedFolders[foldersWithSelections[i].Path]
			} else {
				// Hidden folders are always included
				foldersWithSelections[i].Selected = true
			}
		}
		
		// FIRST: Check if external drive has sufficient space for SELECTED folders only
		err := CheckSelectiveHomeBackupSpaceRequirements(foldersWithSelections, drive.Size)
		if err != nil {
			return BackupDriveStatus{
				error: err,
			}
		}
		
		// Space is sufficient, proceed with mounting (SKIP redundant space check)
		// Call the mount operation directly without re-checking space
		return tea.Cmd(func() tea.Msg {
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
		})()  // Execute the command immediately
	}
}

// Mount any external drive for home backup
func mountDriveForHomeBackup(drive DriveInfo) tea.Cmd {
	return mountDriveForOperation(drive, "home_backup")
}

// Mount external drive for specific operation type
func mountDriveForOperation(drive DriveInfo, operationType string) tea.Cmd {
	return func() tea.Msg {
		// IMMEDIATE FEEDBACK: Show mounting message first
		// Note: This will be followed by another message with the result
		
		// FIRST: Check if external drive has sufficient space before mounting
		var err error
		if operationType == "home_backup" {
			// For home backup, we need to pass the current model state somehow
			// For now, use the full home dir size (will fix this separately)
			err = CheckHomeBackupSpaceRequirements(drive.Size)
		} else {
			err = checkBackupSpaceRequirements(drive.Size)
		}
		
		if err != nil {
			return BackupDriveStatus{
				error: err,
			}
		}
		
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
			
			// LUKS drive mounted, now check space requirements for restore
			if err := checkRestoreSpaceRequirements(drive.Size, mountPoint); err != nil {
				return BackupDriveStatus{
					error: err,
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
		
		// AFTER mounting: Check space requirements for restore
		if err := checkRestoreSpaceRequirements(drive.Size, mountPoint); err != nil {
			return BackupDriveStatus{
				error: err,
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

// Mount any external drive for verification (read-only access to backup source)
func mountDriveForVerification(drive DriveInfo) tea.Cmd {
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

			// Successfully mounted LUKS drive - now ask for verification confirmation
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

		// No space checking needed for verification - it's read-only
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

// discoverHomeFolders analyzes the user's home directory for selective backup operations.
// Scans all directories, calculates sizes, and categorizes them as visible or hidden.
// Logs details to the application log file for debugging purposes.
// Returns a sorted list with visible folders first, then hidden folders alphabetically.
func discoverHomeFolders() ([]HomeFolderInfo, error) {
	// Get the original user's home directory, not root's
	homeDir := os.Getenv("SUDO_USER")
	if homeDir != "" {
		homeDir = "/home/" + homeDir
	} else {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
	}
	
	entries, err := os.ReadDir(homeDir)
	if err != nil {
		return nil, err
	}
	
	// Open log file for debug output
	logPath := getLogFilePath()
	logFile, logErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if logErr == nil {
		defer logFile.Close()
		fmt.Fprintf(logFile, "\n=== HOME FOLDER DISCOVERY DEBUG ===\n")
		fmt.Fprintf(logFile, "Scanning home directory: %s\n", homeDir)
	}
	
	var folders []HomeFolderInfo
	
	// Process directories sequentially (simpler and more reliable)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue // Skip files, only process directories
		}
		
		name := entry.Name()
		path := filepath.Join(homeDir, name)
		isHidden := strings.HasPrefix(name, ".")
		
		// Calculate folder size - with better error handling
		size, err := calculateDirectorySize(path)
		if err != nil {
			// Log error to file, not stdout!
			if logFile != nil {
				fmt.Fprintf(logFile, "❌ ERROR calculating size for %s: %v\n", path, err)
			}
			size = 0
		} else {
			// Log success to file, not stdout!
			if logFile != nil {
				fmt.Fprintf(logFile, "✅ SUCCESS: %s = %d bytes (%.2f GB)\n", name, size, float64(size)/1024/1024/1024)
			}
		}
		
		folder := HomeFolderInfo{
			Name:          name,
			Path:          path,
			Size:          size,
			IsVisible:     !isHidden,
			Selected:      true,  // Default: all selected
			AlwaysInclude: isHidden, // Dotdirs always included
		}
		
		folders = append(folders, folder)
	}
	
	if logFile != nil {
		fmt.Fprintf(logFile, "=== FOLDER DISCOVERY COMPLETE ===\n")
	}
	
	// Sort: visible folders first, then by name
	sort.Slice(folders, func(i, j int) bool {
		if folders[i].IsVisible != folders[j].IsVisible {
			return folders[i].IsVisible // visible first
		}
		return folders[i].Name < folders[j].Name
	})
	
	return folders, nil
}

// HomeFoldersDiscovered is a Bubble Tea message containing home directory analysis results.
type HomeFoldersDiscovered struct {
	folders []HomeFolderInfo // Discovered directories with size and metadata
	error   error            // Non-nil if directory scanning failed
}

// DiscoverHomeFoldersCmd creates a Bubble Tea command to asynchronously analyze the home directory.
// Returns a HomeFoldersDiscovered message when the analysis completes.
func DiscoverHomeFoldersCmd() tea.Cmd {
	return func() tea.Msg {
		folders, err := discoverHomeFolders()
		return HomeFoldersDiscovered{
			folders: folders,
			error:   err,
		}
	}
}
