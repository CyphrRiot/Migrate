//go:build openbsd

package drives

import (
	"fmt"
	"syscall"
)

// GetDiskStats returns total, free, and available bytes for the filesystem
// containing the given path. Uses OpenBSD-specific Statfs_t field names.
func GetDiskStats(path string) (total, free, avail int64, err error) {
	var stat syscall.Statfs_t
	if err = syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get filesystem stats for %s: %v", path, err)
	}
	total = int64(stat.F_blocks) * int64(stat.F_bsize)
	free = int64(stat.F_bfree) * int64(stat.F_bsize)
	avail = int64(stat.F_bavail) * int64(stat.F_bsize)
	return total, free, avail, nil
}
