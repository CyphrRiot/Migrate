//go:build openbsd

package platform

// CheckDependencies validates required system programs on OpenBSD.
// OpenBSD drive detection and mount/unmount operations use syscalls
// (Getfsstat, Mount, Unmount) rather than external binaries.
// The only tools we might need are standard mount/umount for edge cases.
func CheckDependencies() error {
	return nil
}
