//go:build openbsd

package internal

// tryCopyFileRange is a no-op on OpenBSD (copy_file_range unavailable).
func tryCopyFileRange(srcFD, dstFD int, size int64) (int64, bool) {
	return 0, true
}

// trySendfile is a no-op on OpenBSD (sendfile unavailable).
func trySendfile(srcFD, dstFD int, size int64) (int64, bool) {
	return 0, true
}
