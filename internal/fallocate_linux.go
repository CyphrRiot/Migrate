//go:build linux

package internal

import "syscall"

// tryFallocate attempts to preallocate space for a file on Linux.
func tryFallocate(fd int, offset, length int64) error {
	return syscall.Fallocate(fd, 0, offset, length)
}
