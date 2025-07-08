// Package internal provides core utilities and shared functionality for the Migrate backup system.
//
// This package contains common utilities including:
//   - Formatting functions for human-readable display of numbers and byte sizes
//   - Backup operation cancellation management
//   - Logging utilities and path management
//   - Configuration constants and exclude patterns for backup operations
//
// The utilities in this package are designed to be thread-safe where applicable
// and provide consistent formatting across the entire application.
package internal

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
)

// Configuration constants for backup operations
const (
	// DefaultMount is the default mount point prefix for external drives
	DefaultMount = "/run/media"
)

var (
	// ExcludePatterns defines the basic filesystem paths to exclude (legacy - use GetSystemBackupExclusions for system backups).
	// These patterns exclude only the most essential system-managed directories.
	ExcludePatterns = []string{
		"/dev/*",      // Device files
		"/proc/*",     // Process filesystem
		"/sys/*",      // System filesystem
		"/tmp/*",      // Temporary files
		"/var/tmp/*",  // Variable temporary files
		"/lost+found", // Filesystem recovery directory
	}
)

// GetSystemBackupExclusions returns comprehensive exclusion patterns for system backups.
// This excludes all runtime, cache, log, and temporary directories that should not be backed up.
func GetSystemBackupExclusions() []string {
	return []string{
		// Basic system directories
		"/dev/*",
		"/proc/*",
		"/sys/*",
		"/tmp/*",
		"/var/tmp/*",
		"/lost+found",

		// Runtime directories (constantly changing)
		"/run/*",
		"/var/run/*",

		// Log directories (constantly changing, huge, and recoverable)
		"/var/log/*",

		// Cache directories (recoverable and huge)
		"/var/cache/*",
		"/home/*/.cache/*",
		"/root/.cache/*",

		// Lock and PID files
		"/var/lock/*",

		// Trash directories
		"/home/*/.local/share/Trash/*",
		"/root/.local/share/Trash/*",

		// Container and virtualization runtime
		"/var/lib/docker/containers/*",
		"/var/lib/docker/tmp/*",
		"/var/lib/machines/*",
		"/var/lib/portables/*",

		// Package manager cache and temporary files
		"/var/lib/pacman/sync/*",
		"/home/*/.local/share/Steam/*",
		"/home/*/.local/share/flatpak/*",
		"/home/*/.local/share/containers/*",
	}
}

// GetHomeBackupExclusions returns the complete list of exclusion patterns for home directory backups.
// This centralizes all exclusion logic to eliminate copy-paste issues.
func GetHomeBackupExclusions() []string {
	return []string{
		".cache/*",
		".local/share/Trash/*",
		".local/share/Steam/*",
		".cache/yay/*",
		".cache/paru/*",
		".cache/mozilla/*",
		".cache/google-chrome/*",
		".cache/chromium/*",
		".local/share/flatpak/*",
		".local/share/containers/*",
	}
}

// GetBrowserCacheExclusions returns exclusion patterns for browser cache files.
// These files change constantly and should never be verified.
func GetBrowserCacheExclusions() []string {
	return []string{
		"*/.config/BraveSoftware/*/Default/IndexedDB/*",
		"*/.config/BraveSoftware/*/Default/Local Storage/*",
		"*/.config/BraveSoftware/*/Default/GPUCache/*",
		"*/.config/BraveSoftware/*/Default/Sessions/*",
		"*/.config/BraveSoftware/*/Default/Session Storage/*",
		"*/.config/BraveSoftware/*/Default/Local Extension Settings/*",
		"*/.config/BraveSoftware/*/Default/DawnWebGPUCache/*",
		"*/.config/BraveSoftware/*/Default/WebStorage/*",
		"*/.config/BraveSoftware/*/Default/Service Worker/*",
		"*/.config/BraveSoftware/*/Default/blob_storage/*",
		"*/.config/BraveSoftware/*/Default/Application Cache/*",
		"*/.config/BraveSoftware/*/Default/File System/*",
		"*/.config/BraveSoftware/*",
		"*/.config/google-chrome/*/Default/IndexedDB/*",
		"*/.config/google-chrome/*/Default/Local Storage/*",
		"*/.config/google-chrome/*/Default/GPUCache/*",
		"*/.config/chromium/*/Default/IndexedDB/*",
		"*/.config/chromium/*/Default/Local Storage/*",
		"*/.config/chromium/*/Default/GPUCache/*",
		"*/.mozilla/firefox/*/storage/*",
		"*/.mozilla/firefox/*/cache2/*",
		"*/.cache/mozilla/*",
		"*/.cache/google-chrome/*",
		"*/.cache/chromium/*",
		"*/.cache/BraveSoftware/*",
	}
}

// GetSystemCacheExclusions returns exclusion patterns for system cache directories.
// These should be excluded from system backups.
func GetSystemCacheExclusions() []string {
	return []string{
		"/home/*/.cache/*", // User cache directories
		"/root/.cache/*",   // Root cache directory
		"/.cache/*",        // Any other cache directories
		"/var/cache/*",     // System package cache
		"/tmp/*",           // Already in ExcludePatterns but being explicit
	}
}

// GetVerificationExclusions returns the complete exclusion list for verification operations.
// This ensures backup and verification use identical exclusion patterns.
func GetVerificationExclusions(backupType string, selectiveExclusions []string) []string {
	var excludePatterns []string

	switch backupType {
	case "system":
		// System verification: Use comprehensive system exclusions
		excludePatterns = GetSystemBackupExclusions()
		// Add browser cache exclusions
		excludePatterns = append(excludePatterns, GetBrowserCacheExclusions()...)
	case "home":
		// Home verification: Use home exclusions PLUS browser cache exclusions
		excludePatterns = append(GetHomeBackupExclusions(), GetBrowserCacheExclusions()...)
		// Add selective backup exclusions if any
		excludePatterns = append(excludePatterns, selectiveExclusions...)
	default:
		excludePatterns = []string{} // No exclusions for unknown types
	}

	return excludePatterns
}

// backupCancelFlag is a thread-safe cancellation flag for backup operations.
// Use shouldCancelBackup(), CancelBackup(), and resetBackupCancel() to interact with this flag.
var backupCancelFlag int64

// shouldCancelBackup returns true if a backup cancellation has been requested.
// This function is thread-safe and can be called from multiple goroutines.
func shouldCancelBackup() bool {
	return atomic.LoadInt64(&backupCancelFlag) != 0
}

// CancelBackup signals that any running backup operation should be cancelled.
// This function is thread-safe and can be called from any goroutine.
// The cancellation is cooperative - the backup operation must check shouldCancelBackup() periodically.
func CancelBackup() {
	atomic.StoreInt64(&backupCancelFlag, 1)
}

// resetBackupCancel clears the backup cancellation flag, allowing new backup operations to start.
// This is typically called at the beginning of a new backup operation.
func resetBackupCancel() {
	atomic.StoreInt64(&backupCancelFlag, 0)
}

// FormatNumber adds commas to large numbers for readability.
// It accepts int64 values and formats them with thousands separators.
//
// Examples:
//
//	FormatNumber(1234) -> "1,234"
//	FormatNumber(1234567) -> "1,234,567"
//	FormatNumber(999) -> "999" (no comma for numbers < 1000)
func FormatNumber(n int64) string {
	if n < 1000 {
		return strconv.FormatInt(n, 10)
	}

	// Convert to string and add commas
	str := strconv.FormatInt(n, 10)
	var result strings.Builder

	for i, char := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(char)
	}

	return result.String()
}

// getLogFilePath determines the appropriate location for the migrate log file.
// It attempts to create a log in the user's cache directory (~/.cache/migrate/migrate.log)
// and falls back to /tmp/migrate.log if the cache directory cannot be created.
// The function automatically creates the cache directory if it doesn't exist.
func getLogFilePath() string {
	// Try user's home directory first, fall back to /tmp
	if homeDir, err := os.UserHomeDir(); err == nil {
		logDir := filepath.Join(homeDir, ".cache", "migrate")
		if err := os.MkdirAll(logDir, 0755); err == nil {
			return filepath.Join(logDir, "migrate.log")
		}
	}

	// Fall back to /tmp
	return "/tmp/migrate.log"
}

// FormatBytes formats byte counts into human-readable size with proper units and formatting.
// It uses binary units (1024-based) and intelligently chooses precision based on the magnitude.
//
// Examples:
//
//	FormatBytes(1024) -> "1.0 KB"
//	FormatBytes(1536) -> "1.5 KB"
//	FormatBytes(1048576) -> "1.0 MB"
//	FormatBytes(1073741824) -> "1.0 GB"
//	FormatBytes(999) -> "999 B"
//
// The function automatically promotes units (e.g., 1000GB becomes 1.0TB) for readability.
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return strconv.FormatInt(bytes, 10) + " B"
	}

	// Calculate the appropriate unit
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	value := float64(bytes)
	unitIndex := 0

	for value >= unit && unitIndex < len(units)-1 {
		value /= unit
		unitIndex++
	}

	// If we have >= 1000 of current unit, promote to next unit (e.g., 1000GB -> 1.0TB)
	if value >= 1000 && unitIndex < len(units)-1 {
		value /= unit
		unitIndex++
	}

	// Format the number based on its size
	var formatted string
	if value >= 100 {
		// For 100+ units, show whole number with comma separator if > 999
		wholeValue := int(value + 0.5) // Round to nearest integer
		if wholeValue >= 1000 {
			formatted = strconv.Itoa(wholeValue)
			// Add comma thousands separator for readability
			str := formatted
			if len(str) > 3 {
				n := len(str)
				formatted = str[:n-3] + "," + str[n-3:]
			}
		} else {
			formatted = strconv.FormatFloat(float64(wholeValue), 'f', 0, 64)
		}
	} else if value >= 10 {
		// For 10-99.x units, show 1 decimal place
		formatted = strconv.FormatFloat(value, 'f', 1, 64)
	} else {
		// For <10 units, show 2 decimal places
		formatted = strconv.FormatFloat(value, 'f', 2, 64)
	}

	return formatted + " " + units[unitIndex]
}
