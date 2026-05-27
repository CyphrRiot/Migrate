//go:build openbsd

package internal

// DefaultMountPoint returns the default mount point prefix on OpenBSD.
func DefaultMountPoint() string {
	return "/mnt"
}
