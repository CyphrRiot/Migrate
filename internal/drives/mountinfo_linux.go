//go:build linux

package drives

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// findMountPointForPath finds the mount point that contains the given path.
func findMountPointForPath(targetPath string) (string, error) {
	cleanPath, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for %s: %v", targetPath, err)
	}

	file, err := os.Open("/proc/mounts")
	if err != nil {
		return "", fmt.Errorf("failed to open /proc/mounts: %v", err)
	}
	defer file.Close()

	var bestMatch string
	var bestMatchLen int

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			mountPoint := fields[1]
			if strings.HasPrefix(cleanPath, mountPoint) && len(mountPoint) > bestMatchLen {
				bestMatch = mountPoint
				bestMatchLen = len(mountPoint)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading /proc/mounts: %v", err)
	}

	if bestMatch == "" {
		return "/", nil
	}
	return bestMatch, nil
}

// CheckAnyBackupMounted scans for mounted external drives.
// Returns the mount point and true if an external backup drive is currently mounted.
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
			if strings.HasPrefix(mountPoint, "/run/media/") ||
				strings.HasPrefix(mountPoint, "/mnt/") {
				return mountPoint, true
			}
		}
	}
	return "", false
}

// FindMountPointForDevice returns the mount point for a given device path.
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
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("device not found in /proc/mounts")
}

// GetDeviceFromProcMounts finds the device path for a given mount point by parsing /proc/mounts.
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
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("mount point %s not found", mountPoint)
}
