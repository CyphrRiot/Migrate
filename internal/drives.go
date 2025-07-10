// Package internal provides drive management functionality through a facade pattern.
// This file serves as a backward compatibility layer that delegates to the modular drives package.
package internal

import (
	"fmt"
	"os"

	"migrate/internal/drives"

	tea "github.com/charmbracelet/bubbletea"
)

// Type aliases for backward compatibility
type DriveInfo = drives.DriveInfo
type DrivesLoaded = drives.DrivesLoaded
type HomeFolderInfo = drives.HomeFolderInfo

// Legacy message types for backward compatibility
type DriveOperation struct {
	message string
	success bool
}

type BackupDriveStatus struct {
	drivePath  string
	driveSize  string
	driveType  string
	mountPoint string
	needsMount bool
	error      error
}

type PasswordRequiredMsg struct {
	drivePath string
	driveSize string
	driveType string
}

type passwordInteractionMsg struct {
	drivePath  string
	driveSize  string
	driveType  string
	originalOp string
}

type HomeFoldersDiscovered struct {
	folders []HomeFolderInfo
	error   error
}

type SubfoldersDiscovered struct {
	parentPath string
	subfolders []HomeFolderInfo
	error      error
}

type RestoreFoldersDiscovered struct {
	folders []HomeFolderInfo
	error   error
}

// Drive detection functions - delegate to optimized modules
func LoadDrives() tea.Cmd {
	return drives.LoadDrives()
}

func checkAnyBackupMounted() (string, bool) {
	return drives.CheckAnyBackupMounted()
}

func unmountBackupDrive(mountPoint string) error {
	return drives.UnmountBackupDrive(mountPoint)
}

// Space validation functions - delegate to optimized modules
func checkBackupSpaceRequirements(externalDriveSize string) error {
	return drives.ValidateBackupSpace(externalDriveSize)
}

func CheckSelectiveHomeBackupSpaceRequirements(homeFolders []HomeFolderInfo, selectedFolders map[string]bool, subfolderCache map[string][]HomeFolderInfo, externalDriveSize string) error {
	return drives.ValidateSelectiveBackupSpace(homeFolders, selectedFolders, subfolderCache, externalDriveSize)
}

func CheckHomeBackupSpaceRequirements(externalDriveSize string) error {
	return drives.ValidateHomeBackupSpace(externalDriveSize)
}

func checkRestoreSpaceRequirements(externalDriveSize string, externalMountPoint string) error {
	return drives.ValidateRestoreSpace(externalDriveSize, externalMountPoint)
}

func checkSelectiveRestoreSpaceRequirements(restoreFolders []HomeFolderInfo, selectedFolders map[string]bool, restoreConfig bool, restoreWindowMgrs bool) error {
	return drives.ValidateSelectiveRestoreSpace(restoreFolders, selectedFolders, restoreConfig, restoreWindowMgrs)
}

// Folder discovery functions - delegate to optimized modules
func DiscoverHomeFoldersCmd() tea.Cmd {
	return func() tea.Msg {
		folders, err := drives.DiscoverHomeFolders()
		return HomeFoldersDiscovered{
			folders: folders,
			error:   err,
		}
	}
}

func DiscoverSubfoldersCmd(parentPath string) tea.Cmd {
	return func() tea.Msg {
		subfolders, err := drives.DiscoverSubfolders(parentPath)
		return SubfoldersDiscovered{
			parentPath: parentPath,
			subfolders: subfolders,
			error:      err,
		}
	}
}

func DiscoverRestoreFoldersCmd(backupPath string) tea.Cmd {
	return func() tea.Msg {
		folders, err := drives.DiscoverRestoreFolders(backupPath)
		return RestoreFoldersDiscovered{
			folders: folders,
			error:   err,
		}
	}
}

// Mount operation functions - delegate to optimized modules with compatibility layer
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
					message: fmt.Sprintf("🔒 Drive is encrypted and locked. Please unlock it first using:\n\nsudo cryptsetup open %s %s", drive.Device, mapperName),
					success: false,
				}
			}
		}

		// Try to mount the drive
		mountPoint, err := drives.MountRegularDrive(drive)
		if err != nil {
			return DriveOperation{
				message: fmt.Sprintf("❌ Failed to mount drive: %v", err),
				success: false,
			}
		}

		return DriveOperation{
			message: fmt.Sprintf("✅ Drive mounted successfully at %s", mountPoint),
			success: true,
		}
	}
}

func mountDriveForBackup(drive DriveInfo) tea.Cmd {
	return mountDriveForOperation(drive, "system_backup")
}

func mountDriveForSelectiveHomeBackup(drive DriveInfo, homeFolders []HomeFolderInfo, selectedFolders map[string]bool, subfolderCache map[string][]HomeFolderInfo) tea.Cmd {
	return func() tea.Msg {
		// Check space requirements first
		err := CheckSelectiveHomeBackupSpaceRequirements(homeFolders, selectedFolders, subfolderCache, drive.Size)
		if err != nil {
			return BackupDriveStatus{error: err}
		}

		// Proceed with mounting
		return mountDriveForOperation(drive, "selective_home_backup")()
	}
}

func mountDriveForHomeBackup(drive DriveInfo) tea.Cmd {
	return mountDriveForOperation(drive, "home_backup")
}

func mountDriveForOperation(drive DriveInfo, operationType string) tea.Cmd {
	return func() tea.Msg {
		// Check space requirements based on operation type
		var err error
		switch operationType {
		case "system_backup":
			err = checkBackupSpaceRequirements(drive.Size)
		case "home_backup":
			err = CheckHomeBackupSpaceRequirements(drive.Size)
		}

		if err != nil {
			return BackupDriveStatus{error: err}
		}

		// Handle encrypted drives
		if drive.Encrypted {
			mapperName := "luks-" + drive.UUID
			mapperPath := "/dev/mapper/" + mapperName

			if _, err := os.Stat(mapperPath); os.IsNotExist(err) {
				return PasswordRequiredMsg{
					drivePath: drive.Device,
					driveSize: drive.Size,
					driveType: "LUKS",
				}
			}
		}

		// Mount the drive
		mountPoint, err := drives.MountRegularDrive(drive)
		if err != nil {
			return BackupDriveStatus{error: err}
		}

		return BackupDriveStatus{
			drivePath:  drive.Device,
			driveSize:  drive.Size,
			driveType:  drive.Filesystem,
			mountPoint: mountPoint,
			needsMount: false,
		}
	}
}

func mountDriveForRestore(drive DriveInfo) tea.Cmd {
	return func() tea.Msg {
		// Handle encrypted drives
		if drive.Encrypted {
			mapperName := "luks-" + drive.UUID
			mapperPath := "/dev/mapper/" + mapperName

			if _, err := os.Stat(mapperPath); os.IsNotExist(err) {
				return PasswordRequiredMsg{
					drivePath: drive.Device,
					driveSize: drive.Size,
					driveType: "LUKS",
				}
			}
		}

		// Mount the drive
		mountPoint, err := drives.MountRegularDrive(drive)
		if err != nil {
			return BackupDriveStatus{error: err}
		}

		return BackupDriveStatus{
			drivePath:  drive.Device,
			driveSize:  drive.Size,
			driveType:  drive.Filesystem,
			mountPoint: mountPoint,
			needsMount: false,
		}
	}
}

func mountDriveForVerification(drive DriveInfo) tea.Cmd {
	return mountDriveForRestore(drive) // Same logic as restore
}

func handlePasswordInput(msg PasswordRequiredMsg, originalOp string) tea.Cmd {
	return func() tea.Msg {
		return passwordInteractionMsg{
			drivePath:  msg.drivePath,
			driveSize:  msg.driveSize,
			driveType:  msg.driveType,
			originalOp: originalOp,
		}
	}
}

func PerformBackupUnmount() tea.Cmd {
	return func() tea.Msg {
		mountPoint, mounted := checkAnyBackupMounted()
		if !mounted {
			return DriveOperation{
				message: "ℹ️  Backup drive is already unmounted",
				success: true,
			}
		}

		if err := unmountBackupDrive(mountPoint); err != nil {
			return DriveOperation{
				message: fmt.Sprintf("❌ Failed to unmount drive: %v", err),
				success: false,
			}
		}

		return DriveOperation{
			message: "✅ Backup drive unmounted successfully",
			success: true,
		}
	}
}

// Deprecated functions
func mountBackupDrive() (string, error) {
	return "", fmt.Errorf("deprecated function - use drive selection instead")
}
