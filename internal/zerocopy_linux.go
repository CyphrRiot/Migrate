//go:build linux

package internal

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// tryCopyFileRange attempts zero-copy via copy_file_range.
// Returns (bytesCopied, fallback).
// If fallback is true, caller should try sendfile or buffered copy.
func tryCopyFileRange(srcFD, dstFD int, size int64) (int64, bool) {
	var copied int64
	for copied < size {
		remaining := size - copied
		chunk := remaining
		if chunk > 1<<30 {
			chunk = 1 << 30
		}

		n, err := unix.CopyFileRange(srcFD, nil, dstFD, nil, int(chunk), 0)
		if n > 0 {
			copied += int64(n)
		}
		if err == nil {
			if n == 0 {
				break
			}
			continue
		}

		if errno, ok := err.(syscall.Errno); ok {
			switch errno {
			case syscall.EINTR, syscall.EAGAIN:
				continue
			case syscall.EINVAL, syscall.ENOSYS, syscall.EXDEV, syscall.EOVERFLOW:
				return 0, true
			default:
				return 0, true
			}
		}
		return 0, true
	}
	return copied, false
}

// trySendfile attempts zero-copy via sendfile.
// Returns (bytesCopied, fallbackToBuffered).
func trySendfile(srcFD, dstFD int, size int64) (int64, bool) {
	var off int64
	var copied int64
	for copied < size {
		remaining := size - copied
		count := remaining
		if count > 1<<30 {
			count = 1 << 30
		}

		n, err := syscall.Sendfile(dstFD, srcFD, &off, int(count))
		if n > 0 {
			copied += int64(n)
		}
		if err == nil {
			if n == 0 {
				break
			}
			continue
		}

		if errno, ok := err.(syscall.Errno); ok {
			switch errno {
			case syscall.EINTR, syscall.EAGAIN:
				continue
			case syscall.EINVAL, syscall.ENOSYS, syscall.EXDEV, syscall.EOVERFLOW:
				return copied, true
			default:
				return copied, true
			}
		}
		return copied, true
	}
	return copied, false
}
