//go:build openbsd

package drives

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
)

const mntWait = 1 // MNT_WAIT

func int8SliceToString(buf []int8) string {
	var n int
	for n = 0; n < len(buf) && buf[n] != 0; n++ {
	}
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = byte(buf[i])
	}
	return string(b)
}

func getAllMountInfo() ([]syscall.Statfs_t, error) {
	var buf []syscall.Statfs_t
	n, err := syscall.Getfsstat(buf, mntWait)
	if err != nil {
		return nil, err
	}
	buf = make([]syscall.Statfs_t, n)
	n2, err := syscall.Getfsstat(buf, mntWait)
	if err != nil {
		return nil, err
	}
	return buf[:n2], nil
}

func mountPointToPath(m []int8) string {
	return int8SliceToString(m)
}

// findMountPointForPath finds the mount point that contains the given path.
func findMountPointForPath(targetPath string) (string, error) {
	cleanPath, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for %s: %v", targetPath, err)
	}

	entries, err := getAllMountInfo()
	if err != nil {
		return "", fmt.Errorf("failed to get mount info: %v", err)
	}

	var bestMatch string
	var bestMatchLen int

	for _, ent := range entries {
		mountPoint := mountPointToPath(ent.F_mntonname[:])
		if mountPoint == "" {
			continue
		}
		if strings.HasPrefix(cleanPath, mountPoint) && len(mountPoint) > bestMatchLen {
			bestMatch = mountPoint
			bestMatchLen = len(mountPoint)
		}
	}

	if bestMatch == "" {
		return "/", nil
	}
	return bestMatch, nil
}

// CheckAnyBackupMounted scans for mounted external drives.
func CheckAnyBackupMounted() (string, bool) {
	entries, err := getAllMountInfo()
	if err != nil {
		return "", false
	}

	for _, ent := range entries {
		mountPoint := mountPointToPath(ent.F_mntonname[:])
		if strings.HasPrefix(mountPoint, "/mnt/") {
			return mountPoint, true
		}
	}
	return "", false
}

// FindMountPointForDevice returns the mount point for a given device path.
func FindMountPointForDevice(device string) (string, error) {
	entries, err := getAllMountInfo()
	if err != nil {
		return "", err
	}

	for _, ent := range entries {
		mntFrom := mountPointToPath(ent.F_mntfromname[:])
		if mntFrom == device {
			return mountPointToPath(ent.F_mntonname[:]), nil
		}
	}
	return "", fmt.Errorf("device not found")
}

// GetDeviceFromProcMounts finds the device path for a given mount point.
func GetDeviceFromProcMounts(mountPoint string) (string, error) {
	entries, err := getAllMountInfo()
	if err != nil {
		return "", err
	}

	for _, ent := range entries {
		mnt := mountPointToPath(ent.F_mntonname[:])
		if mnt == mountPoint {
			return mountPointToPath(ent.F_mntfromname[:]), nil
		}
	}
	return "", fmt.Errorf("mount point %s not found", mountPoint)
}
