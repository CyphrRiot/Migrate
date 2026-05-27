//go:build openbsd

// Package drives provides drive detection, mounting, and management functionality.
// This module handles mount/unmount operations for external drives.
package drives

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// UnmountBackupDrive safely unmounts an external backup drive.
// Performs filesystem sync before unmounting for data safety.
func UnmountBackupDrive(mountPoint string) error {
	syscall.Sync()

	if err := syscall.Unmount(mountPoint, 0); err != nil {
		return fmt.Errorf("failed to unmount: %v", err)
	}

	// Clean up the mount point directory we created
	if strings.HasPrefix(mountPoint, "/mnt/migrate-mount-") {
		_ = os.Remove(mountPoint)
	}

	return nil
}

// MountRegularDrive handles mounting of standard (non-encrypted) external drives.
func MountRegularDrive(drive DriveInfo) (string, error) {
	devicePath, mountPoint, alreadyMounted := resolveDriveDevice(drive.Device)

	if alreadyMounted {
		return mountPoint, nil
	}

	return mountDevice(devicePath, drive.Filesystem)
}

// resolveDriveDevice determines the actual device path and current mount status.
func resolveDriveDevice(device string) (devicePath, mountPoint string, alreadyMounted bool) {
	if strings.HasPrefix(device, "/") && !strings.HasPrefix(device, "/dev/") {
		if _, err := os.Stat(device); err == nil {
			return "", device, true
		}
		if dev, err := GetDeviceFromProcMounts(device); err == nil {
			return dev, device, true
		}
		return "", "", false
	}

	if mountPt, err := FindMountPointForDevice(device); err == nil {
		return device, mountPt, true
	}

	return device, "", false
}

// mountDevice performs the actual mounting operation via the OpenBSD mount(2) syscall.
func mountDevice(devicePath, fsType string) (string, error) {
	fstype := fsType
	if fstype == "" {
		fstype = "msdos"
	}

	base := filepath.Base(devicePath)
	mountPoint := filepath.Join("/mnt", fmt.Sprintf("migrate-mount-%s", base))

	// Remove any stale directory from a previous attempt
	_ = os.Remove(mountPoint)
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return "", fmt.Errorf("failed to create mount point: %v", err)
	}

	fs, err := syscall.BytePtrFromString(fstype)
	if err != nil {
		return "", err
	}
	mdir, err := syscall.BytePtrFromString(mountPoint)
	if err != nil {
		return "", err
	}
	data, err := syscall.BytePtrFromString(devicePath)
	if err != nil {
		return "", err
	}

	_, _, errno := syscall.Syscall6(
		syscall.SYS_MOUNT,
		uintptr(unsafe.Pointer(fs)),
		uintptr(unsafe.Pointer(mdir)),
		0,
		uintptr(unsafe.Pointer(data)),
		0, 0,
	)
	if errno != 0 {
		_ = os.Remove(mountPoint)
		return "", fmt.Errorf("failed to mount %s: %v", devicePath, errno)
	}

	return mountPoint, nil
}

// GetMountHint returns a platform-specific hint for manually mounting a drive.
func GetMountHint() string {
	return "Mount your drive with: doas mount /dev/sdXi /mnt"
}
