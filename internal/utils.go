package internal

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
)

// Configuration constants  
const (
	DefaultMount = "/run/media"
)

var (
	// No more hardcoded labels - any external drive can be used for backup
	ExcludePatterns = []string{
		"/dev/*",
		"/proc/*",
		"/sys/*",
		"/tmp/*",
		"/var/tmp/*",
		"/lost+found",
	}
)

// Global cancellation flag for backup operations
var backupCancelFlag int64

// Check if backup should be canceled
func shouldCancelBackup() bool {
	return atomic.LoadInt64(&backupCancelFlag) != 0
}

// Set backup cancellation flag
func CancelBackup() {
	atomic.StoreInt64(&backupCancelFlag, 1)
}

// Reset backup cancellation flag
func resetBackupCancel() {
	atomic.StoreInt64(&backupCancelFlag, 0)
}

// FormatNumber adds commas to large numbers for readability
func FormatNumber(n int) string {
	str := strconv.Itoa(n)
	if len(str) <= 3 {
		return str
	}

	// Add commas from right to left
	var result strings.Builder
	for i, digit := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(digit)
	}
	return result.String()
}

// Get log file path in appropriate location
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

// Format bytes into human readable size with proper units and formatting
func formatBytes(bytes int64) string {
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

// Format numbers with comma separators for readability
func formatNumber(n int64) string {
	if n < 1000 {
		return strconv.FormatInt(n, 10)
	}

	// Convert to string and add commas
	str := strconv.FormatInt(n, 10)
	result := ""

	for i, char := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(char)
	}

	return result
}
