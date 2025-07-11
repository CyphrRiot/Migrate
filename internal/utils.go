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
	"migrate/internal/drives"
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

		// Git directories (version control)
		".git/*",
		"*/.git/*",
		"/home/*/.git/*",
		"/root/.git/*",

		// Signal app cache directories
		"/home/*/.config/Signal/blob_storage/*",
		"/home/*/.config/Signal/drafts.noindex/*",
		"/home/*/.config/Signal/attachments.noindex/*",
		"/home/*/.config/Signal/logs/*",

		// Go language server cache
		"/home/*/.cache/go-build/*",
		"/home/*/.cache/gopls/*",
		"/home/*/.cache/golangci-lint/*",

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
		".git/*",
		"*/.git/*",
		// Signal app cache
		".config/Signal/blob_storage/*",
		".config/Signal/drafts.noindex/*",
		".config/Signal/attachments.noindex/*",
		".config/Signal/logs/*",
		// Go language server cache
		".cache/go-build/*",
		".cache/gopls/*",
		".cache/golangci-lint/*",
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
		// Hash-named cache files (common pattern)
		"*/*-a",
		"*/*-d",
		"*/*-*-a",
		"*/*-*-d",
		// Go language server cache files
		"*/*-diagnostics",
		"*/*-export",
		"*/*-methodsets",
		"*/*-tests",
		"*/*-xrefs",
		"*/*-typerefs",
		"*/*-cas",
		// Signal app cache patterns
		"*/.config/Signal/blob_storage/*",
		"*/.config/Signal/drafts.noindex/*",
		"*/.config/Signal/attachments.noindex/*",
		"*/.config/Signal/logs/*",
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
		// Hash-named cache files (common in various cache directories)
		"*/*-a",
		"*/*-d",
		"*/*-*-a",
		"*/*-*-d",
		// Go language server cache files
		"*/*-diagnostics",
		"*/*-export",
		"*/*-methodsets",
		"*/*-tests",
		"*/*-xrefs",
		"*/*-typerefs",
		"*/*-cas",
		// Signal app cache patterns
		"/home/*/.config/Signal/blob_storage/*",
		"/home/*/.config/Signal/drafts.noindex/*",
		"/home/*/.config/Signal/attachments.noindex/*",
		"/home/*/.config/Signal/logs/*",
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
		// Add additional runtime exclusions that cause verification false positives
		excludePatterns = append(excludePatterns, []string{
			"/run/*",
			"/var/run/*",
			"/var/log/*",
			"/root/.cache/*",
			"/root/.config/*",
			"/root/.ssh/*",
			"/root/go/*",
			"/srv/*",
			"/var/lib/machines/*",
			"/var/lib/portables/*",
			"/var/lib/docker/containers/*",
			"/var/lib/docker/tmp/*",
			"/var/lib/systemd/*",
			"/var/cache/*",
			"/home/*/.cache/*",
			"/home/*/.local/share/Trash/*",
			"/home/*/.local/share/Steam/*",
			"/home/*/.local/share/flatpak/*",
			"/home/*/.local/share/containers/*",
			// Signal app cache directories
			"/home/*/.config/Signal/blob_storage/*",
			"/home/*/.config/Signal/drafts.noindex/*",
			"/home/*/.config/Signal/attachments.noindex/*",
			"/home/*/.config/Signal/logs/*",
			// Go language server cache
			"/home/*/.cache/go-build/*",
			"/home/*/.cache/gopls/*",
			"/home/*/.cache/golangci-lint/*",
		}...)
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

// GetSelectiveRestoreExclusions returns exclusion patterns for selective restore operations.
// This respects user choices about restoring .config and .local directories.
func GetSelectiveRestoreExclusions(restoreConfig, restoreWindowMgrs bool) []string {
	// Start with basic system exclusions that should always be excluded
	excludePatterns := []string{
		"/dev/*",      // Device files
		"/proc/*",     // Process filesystem
		"/sys/*",      // System filesystem
		"/tmp/*",      // Temporary files
		"/var/tmp/*",  // Variable temporary files
		"/lost+found", // Filesystem recovery directory
	}

	// Add COMPREHENSIVE cache and dangerous directory exclusions (always safe to exclude)
	excludePatterns = append(excludePatterns, []string{
		// Cache directories (CRITICAL - should never be restored)
		".cache/*",
		".local/share/Trash/*",
		".local/share/Steam/*",
		".local/share/flatpak/*",
		".local/share/containers/*",

		// Browser cache and temporary data (CRITICAL)
		".cache/mozilla/*",
		".cache/google-chrome/*",
		".cache/chromium/*",
		".cache/BraveSoftware/*",
		".mozilla/firefox/*/storage/*",
		".mozilla/firefox/*/cache2/*",
		".config/BraveSoftware/*/Default/IndexedDB/*",
		".config/BraveSoftware/*/Default/Local Storage/*",
		".config/BraveSoftware/*/Default/GPUCache/*",
		".config/google-chrome/*/Default/IndexedDB/*",
		".config/google-chrome/*/Default/Local Storage/*",
		".config/google-chrome/*/Default/GPUCache/*",
		".config/chromium/*/Default/IndexedDB/*",
		".config/chromium/*/Default/Local Storage/*",
		".config/chromium/*/Default/GPUCache/*",

		// Git directories (version control)
		".git/*",
		"*/.git/*",

		// Signal app cache directories
		".config/Signal/blob_storage/*",
		".config/Signal/drafts.noindex/*",
		".config/Signal/attachments.noindex/*",
		".config/Signal/logs/*",

		// Go language server cache
		".cache/go-build/*",
		".cache/gopls/*",
		".cache/golangci-lint/*",

		// Package manager cache
		".cache/yay/*",
		".cache/paru/*",

		// Security-sensitive directories (CRITICAL - private keys)
		".ssh/id_*",                  // SSH private keys
		".ssh/known_hosts",           // SSH known hosts (changes frequently)
		".gnupg/private-keys-v1.d/*", // GPG private keys
		".gnupg/pubring.kbx~",        // GPG temporary files
		".gnupg/trustdb.gpg",         // GPG trust database

		// Runtime and lock files
		".dbus/session-bus/*",
		".pulse-cookie",
		".Xauthority",
		".ICEauthority",

		// Application-specific dangerous directories
		".docker/config.json", // Docker auth
		".kube/config",        // Kubernetes config
		".aws/credentials",    // AWS credentials
		".gcp/credentials",    // GCP credentials

		// Build and development cache
		"node_modules/*",
		".npm/*",
		".yarn/*",
		".cargo/registry/*",
		".cargo/git/*",
		".rustup/*",
		".go/pkg/*",
		".gradle/*",
		".m2/repository/*",

		// IDE and editor cache
		".vscode/logs/*",
		".vscode/CachedExtensions/*",
		".vscode/CachedData/*",
		".idea/system/*",
		".idea/logs/*",

		// Game directories with large cache
		".steam/steam/appcache/*",
		".steam/steam/logs/*",
		".steam/steam/dumps/*",
		".local/share/Steam/logs/*",
		".local/share/Steam/appcache/*",
		".local/share/Steam/dumps/*",

		// Wine directories (can be problematic)
		".wine/drive_c/windows/Temp/*",
		".wine/drive_c/users/*/Temp/*",
		".wine/drive_c/users/*/AppData/Local/Temp/*",

		// Virtualization
		".local/share/gnome-boxes/images/*",
		"VirtualBox VMs/*",
		".config/VirtualBox/VBoxSVC.log*",

		// Backup and sync conflicts
		"*conflicted copy*",
		"*.tmp",
		"*.temp",
		"*.lock",
		"*.pid",
		"*.swp",
		"*.swo",
		"*~",
		".DS_Store",
		"Thumbs.db",
	}...)

	// If user chose NOT to restore .config, add .config exclusions
	if !restoreConfig {
		excludePatterns = append(excludePatterns, []string{
			".config/*",
			"*/.config/*",
		}...)
	}

	// If user chose NOT to restore window managers, add .local exclusions
	if !restoreWindowMgrs {
		excludePatterns = append(excludePatterns, []string{
			".local/*",
			"*/.local/*",
		}...)
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
// When running with sudo, it still uses the original user's home directory.
func getLogFilePath() string {
	// When running with sudo, use the original user's home directory
	var homeDir string
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		// Running with sudo - use original user's home directory
		homeDir = "/home/" + sudoUser
	} else {
		// Not running with sudo - use current user's home directory
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return "/tmp/migrate.log"
		}
	}

	logDir := filepath.Join(homeDir, ".cache", "migrate")
	if err := os.MkdirAll(logDir, 0755); err == nil {
		return filepath.Join(logDir, "migrate.log")
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
// FormatBytes formats byte counts into human-readable size with proper units.
// Use optimized version from drives package
func FormatBytes(bytes int64) string {
	return drives.FormatBytes(bytes)
}
