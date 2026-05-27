//go:build openbsd

package drives

import (
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

// LoadDrives scans for available external drives on OpenBSD using Getfsstat.
func LoadDrives() tea.Cmd {
	return func() tea.Msg {
		// First call: get number of mounted filesystems
		buf := make([]syscall.Statfs_t, 0, 64)
		n, err := syscall.Getfsstat(buf, 1) // MNT_WAIT = 1
		if err != nil || n <= 0 {
			return DrivesLoaded{Drives: []DriveInfo{}}
		}

		// Second call: fill the buffer
		buf = make([]syscall.Statfs_t, n)
		_, err = syscall.Getfsstat(buf, 1)
		if err != nil {
			return DrivesLoaded{Drives: []DriveInfo{}}
		}

		drives := make([]DriveInfo, 0, 4)
		for _, stat := range buf {
			mountpoint := fsnToString(stat.F_mntonname[:])

			// Skip root and non-external mounts
			if mountpoint == "/" || !isExternalMount(mountpoint) {
				continue
			}

			// Get size for display
			total, _, _, _ := GetDiskStats(mountpoint)
			sizeStr := FormatBytes(total)

			label := filepath.Base(mountpoint)
			if label == "" {
				label = "External Drive"
			}
			uuid := strings.ReplaceAll(mountpoint, "/", "-")
			if uuid == "" || uuid == "-" {
				uuid = label
			}
			drive := DriveInfo{
				Device:     mountpoint,
				Size:       sizeStr,
				Label:      label,
				UUID:       uuid,
				Filesystem: fsnToString(stat.F_fstypename[:]),
				Encrypted:  false,
			}

			drives = append(drives, drive)
		}

		return DrivesLoaded{Drives: drives}
	}
}

// fsnToString converts an OpenBSD int8 array field to a Go string.
func fsnToString(src []int8) string {
	out := make([]byte, 0, len(src))
	for _, c := range src {
		if c == 0 {
			break
		}
		out = append(out, byte(c))
	}
	return string(out)
}
