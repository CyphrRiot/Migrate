// Package drives provides drive detection, mounting, and management functionality.
// This module contains utility functions used across the drives package.
package drives

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sync/atomic"
)

// Atomic counters for scanning progress - same pattern as backup operations
var (
	foldersScanned     int64  // total folders scanned so far
	totalFoldersToScan int64  // estimated total folders to scan
	currentFolderName  string // name of folder currently being scanned
)

// getUsedDiskSpace provides backward compatibility wrapper for GetUsedDiskSpace.
func getUsedDiskSpace(path string) (int64, error) {
	return GetUsedDiskSpace(path)
}

// GetUsedDiskSpace calculates used disk space using pure Go syscalls without external commands.
// Returns the actual used bytes on the filesystem containing the specified path.
// Uses syscall.Statfs for accurate filesystem statistics.
func GetUsedDiskSpace(path string) (int64, error) {
	total, free, _, err := GetDiskStats(path)
	if err != nil {
		return 0, err
	}
	return total - free, nil
}

// GetAvailableDiskSpace returns the available (free) space on the filesystem containing the given path.
func GetAvailableDiskSpace(path string) (int64, error) {
	_, _, avail, err := GetDiskStats(path)
	if err != nil {
		return 0, err
	}
	return avail, nil
}

// FormatBytes formats byte counts into human-readable size with proper units and formatting.
// Provides clean output with appropriate decimal places for different size ranges.
//
// Examples:
//
//	FormatBytes(1024) -> "1.0 KB"
//	FormatBytes(1536) -> "1.5 KB"
//	FormatBytes(1048576) -> "1.0 MB"
//	FormatBytes(1073741824) -> "1.0 GB"
//	FormatBytes(999) -> "999 B"
//
// Returns properly formatted string with units for display in UI.
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
		PB = TB * 1024
	)

	if bytes < KB {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < MB {
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	} else if bytes < GB {
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	} else if bytes < TB {
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	} else if bytes < PB {
		return fmt.Sprintf("%.1f TB", float64(bytes)/TB)
	} else {
		return fmt.Sprintf("%.1f PB", float64(bytes)/PB)
	}
}

// CalculateDirectorySize computes total directory size using native Go directory traversal.
// Walks the directory tree and sums individual file sizes with graceful error handling.
// Portable and handles permission errors gracefully without external dependencies.
func CalculateDirectorySize(path string) (int64, error) {
	var totalSize int64

	err := filepath.WalkDir(path, func(filePath string, d os.DirEntry, err error) error {
		if err != nil {
			// Skip errors (permission denied, etc.) but continue
			return nil
		}

		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				totalSize += info.Size()
			}
		}
		return nil
	})

	return totalSize, err
}

// GetHomeDirSize calculates the total size of the current user's home directory.
// Uses the efficient calculateDirectorySize function which prefers du command with Go fallback.
func GetHomeDirSize() (int64, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return 0, err
	}

	// Use du-equivalent Go implementation
	return CalculateDirectorySize(homeDir)
}

// IsImportantSystemFolder determines if a hidden folder should be automatically included.
// Returns true for truly critical system folders that should never be excluded.
func IsImportantSystemFolder(name string) bool {
	switch name {
	case ".ssh": // SSH keys - critical for system access
		return true
	case ".gnupg": // GPG keys - critical for encryption
		return true
	case ".mozilla": // Firefox profiles with saved passwords, critical data
		return true
	default:
		return false
	}
}

// DiscoverHomeFolders analyzes the user's home directory for selective backup operations.
// Scans all directories, calculates sizes, and categorizes them as visible or hidden.
func DiscoverHomeFolders() ([]HomeFolderInfo, error) {
	// DETAILED LOGGING TO DEBUG HANGING
	logFile, _ := os.OpenFile("/tmp/migrate_scan_debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if logFile != nil {
		defer logFile.Close()
		fmt.Fprintf(logFile, "=== SCAN START ===\n")
	}

	// Reset scanning counters
	atomic.StoreInt64(&foldersScanned, 0)
	atomic.StoreInt64(&totalFoldersToScan, 0)
	currentFolderName = ""

	if logFile != nil {
		fmt.Fprintf(logFile, "Step 1: Counters reset\n")
	}

	// Get the original user's home directory, not root's
	homeDir := os.Getenv("SUDO_USER")
	if homeDir != "" {
		if u, err := user.Lookup(homeDir); err == nil {
			homeDir = u.HomeDir
		} else {
			var homeErr error
			homeDir, homeErr = os.UserHomeDir()
			if homeErr != nil {
				if logFile != nil {
					fmt.Fprintf(logFile, "ERROR: Failed to get home dir: %v\n", homeErr)
				}
				return nil, homeErr
			}
		}
	} else {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			if logFile != nil {
				fmt.Fprintf(logFile, "ERROR: Failed to get home dir: %v\n", err)
			}
			return nil, err
		}
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Step 2: Home dir determined: %s\n", homeDir)
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Step 3: About to call os.ReadDir(%s)\n", homeDir)
	}

	entries, err := os.ReadDir(homeDir)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "ERROR: os.ReadDir failed: %v\n", err)
		}
		return nil, err
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Step 4: os.ReadDir succeeded, found %d entries\n", len(entries))
	}

	// Count directories first so we can show progress
	dirCount := int64(0)
	for _, entry := range entries {
		if entry.IsDir() {
			dirCount++
		}
	}
	atomic.StoreInt64(&totalFoldersToScan, dirCount)

	var folders []HomeFolderInfo
	folderIndex := int64(0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		path := filepath.Join(homeDir, name)
		isHidden := name[0] == '.'

		if logFile != nil {
			fmt.Fprintf(logFile, "Step 5: Processing folder %d: %s\n", folderIndex+1, name)
		}

		// Update progress counters
		folderIndex++
		atomic.StoreInt64(&foldersScanned, folderIndex)
		currentFolderName = name

		// Calculate folder size
		size, err := CalculateDirectorySize(path)
		if err != nil {
			size = 0
		}

		if logFile != nil {
			fmt.Fprintf(logFile, "Step 6: Size set to 0 for %s\n", name)
		}

		// Check if folder has subdirectories
		hasSubfolders := false
		if !isHidden && size > 0 {
			if subEntries, err := os.ReadDir(path); err == nil {
				for _, subEntry := range subEntries {
					if subEntry.IsDir() {
						hasSubfolders = true
						break
					}
				}
			}
		}

		folder := HomeFolderInfo{
			Name:          name,
			Path:          path,
			Size:          size,
			IsVisible:     !isHidden,
			Selected:      true,
			AlwaysInclude: IsImportantSystemFolder(name),
			HasSubfolders: hasSubfolders,
			ParentPath:    "",
		}

		if logFile != nil {
			fmt.Fprintf(logFile, "Step 9: Created folder struct for %s\n", name)
		}

		folders = append(folders, folder)

		if logFile != nil {
			fmt.Fprintf(logFile, "Step 10: Added %s to folders list\n", name)
		}
	}

	// Clear progress when done
	currentFolderName = ""

	if logFile != nil {
		fmt.Fprintf(logFile, "Step 11: SCAN COMPLETE - returning %d folders\n", len(folders))
		for i, folder := range folders {
			fmt.Fprintf(logFile, "  Folder %d: %s (size: %d, visible: %v)\n", i, folder.Name, folder.Size, folder.IsVisible)
		}
	}

	return folders, nil
}

// GetScanProgress returns current scanning progress for UI display
func GetScanProgress() (int64, int64, string) {
	return atomic.LoadInt64(&foldersScanned), atomic.LoadInt64(&totalFoldersToScan), currentFolderName
}

// DiscoverSubfolders analyzes subdirectories within a parent folder for granular selection.
func DiscoverSubfolders(parentPath string) ([]HomeFolderInfo, error) {
	entries, err := os.ReadDir(parentPath)
	if err != nil {
		return nil, err
	}

	var subfolders []HomeFolderInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		path := filepath.Join(parentPath, name)
		isHidden := name[0] == '.'

		// Calculate folder size
		size, err := CalculateDirectorySize(path)
		if err != nil || size == 0 {
			continue // Skip empty or inaccessible folders
		}

		subfolder := HomeFolderInfo{
			Name:          name,
			Path:          path,
			Size:          size,
			IsVisible:     !isHidden,
			Selected:      false,
			AlwaysInclude: false,
			HasSubfolders: false,
			ParentPath:    parentPath,
		}

		subfolders = append(subfolders, subfolder)
	}

	return subfolders, nil
}

// DiscoverRestoreFolders analyzes a backup mount point to find available folders for restore.
func DiscoverRestoreFolders(backupMountPoint string) ([]HomeFolderInfo, error) {
	// Check if this is a home backup
	backupInfo := filepath.Join(backupMountPoint, "BACKUP-INFO.txt")
	if _, err := os.Stat(backupInfo); err != nil {
		return nil, fmt.Errorf("backup info not found: %v", err)
	}

	entries, err := os.ReadDir(backupMountPoint)
	if err != nil {
		return nil, err
	}

	var folders []HomeFolderInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Skip backup metadata
		if name == "BACKUP-INFO.txt" || name == "BACKUP-FOLDERS.txt" {
			continue
		}

		path := filepath.Join(backupMountPoint, name)
		isHidden := name[0] == '.'

		// Calculate folder size
		size, err := CalculateDirectorySize(path)
		if err != nil {
			size = 0
		}

		folder := HomeFolderInfo{
			Name:          name,
			Path:          path,
			Size:          size,
			IsVisible:     !isHidden,
			Selected:      true,
			AlwaysInclude: IsImportantSystemFolder(name),
			HasSubfolders: false,
			ParentPath:    "",
		}

		folders = append(folders, folder)
	}

	return folders, nil
}
