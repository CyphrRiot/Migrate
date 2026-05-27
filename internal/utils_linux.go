//go:build linux

package internal

// DefaultMountPoint returns the default mount point prefix on Linux.
func DefaultMountPoint() string {
	return "/run/media"
}
