package drives

import "strings"

// isExternalMount checks if a mount point is in a typical external location.
func isExternalMount(mountpoint string) bool {
	return strings.Contains(mountpoint, "/run/media/") ||
		strings.Contains(mountpoint, "/mnt/") ||
		strings.Contains(mountpoint, "/media/")
}
