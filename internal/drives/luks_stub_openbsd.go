//go:build openbsd

// Package drives provides drive detection, mounting, and management functionality.
// On OpenBSD, LUKS encryption is not supported; these are no-op stubs.
package drives

import (
	"fmt"
)

// mountLUKSDrive is not supported on OpenBSD.
func mountLUKSDrive(drive DriveInfo) (string, error) {
	return "", fmt.Errorf("LUKS encryption is not supported on OpenBSD")
}

// isLUKSLocked always returns false on OpenBSD.
func isLUKSLocked(drive DriveInfo) bool {
	return false
}

// getLUKSMapperPath returns an empty string on OpenBSD.
func getLUKSMapperPath(drive DriveInfo) string {
	return ""
}

// GetMapperPath returns an empty string on OpenBSD.
func GetMapperPath(mapperName string) string {
	return ""
}
