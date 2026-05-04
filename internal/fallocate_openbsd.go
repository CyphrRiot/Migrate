//go:build openbsd

package internal

// tryFallocate is a no-op on OpenBSD (Fallocate syscall unavailable).
func tryFallocate(fd int, offset, length int64) error {
	return nil
}
