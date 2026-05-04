//go:build linux

package drives

import (
	"fmt"
	"syscall"
)

// GetDiskStats returns total, free, and available bytes for the filesystem
// containing the given path. Uses Linux-specific Statfs_t field names.
func GetDiskStats(path string) (total, free, avail int64, err error) {
	var stat syscall.Statfs_t
	if err = syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get filesystem stats for %s: %v", path, err)
	}
	total = int64(stat.Blocks) * int64(stat.Bsize)
	free = int64(stat.Bfree) * int64(stat.Bsize)
	avail = int64(stat.Bavail) * int64(stat.Bsize)
	return total, free, avail, nil
}
