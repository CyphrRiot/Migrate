// Package drives provides drive detection, mounting, and management functionality.
// This module contains utility functions used across the drives package.
package drives

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// getUsedDiskSpace provides backward compatibility wrapper for GetUsedDiskSpace.
func getUsedDiskSpace(path string) (int64, error) {
	return GetUsedDiskSpace(path)
}

// GetUsedDiskSpace calculates used disk space using pure Go syscalls without external commands.
// Returns the actual used bytes on the filesystem containing the specified path.
// Uses syscall.Statfs for accurate filesystem statistics.
func GetUsedDiskSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, fmt.Errorf("failed to get filesystem stats for %s: %v", path, err)
	}

	// Calculate used space: total - free
	// stat.Blocks = total blocks
	// stat.Bfree = free blocks (including reserved for root)
	// stat.Bsize = block size
	totalBytes := int64(stat.Blocks) * int64(stat.Bsize)
	freeBytes := int64(stat.Bfree) * int64(stat.Bsize)
	usedBytes := totalBytes - freeBytes

	return usedBytes, nil
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
	// Get the original user's home directory, not root's
	homeDir := os.Getenv("SUDO_USER")
	if homeDir != "" {
		homeDir = "/home/" + homeDir
	} else {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
	}

	entries, err := os.ReadDir(homeDir)
	if err != nil {
		return nil, err
	}

	var folders []HomeFolderInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		path := filepath.Join(homeDir, name)
		isHidden := name[0] == '.'

		// Calculate folder size
		size, err := CalculateDirectorySize(path)
		if err != nil {
			size = 0
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

		folders = append(folders, folder)
	}

	return folders, nil
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
