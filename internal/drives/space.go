// Package drives provides drive detection, mounting, and management functionality.
// This module handles space requirement validation for backup and restore operations.
package drives

import (
	"fmt"
	"strings"
	"syscall"
)

// ParseDriveSize converts human-readable size strings to bytes.
// Supports standard units: B, K, M, G, T, P (case-insensitive).
// Examples: "1.5T" -> 1,649,267,441,664 bytes, "500G" -> 537,109,987,328 bytes
func ParseDriveSize(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(sizeStr)
	if len(sizeStr) < 2 {
		return 0, fmt.Errorf("invalid size string: %s", sizeStr)
	}

	// Get the unit (last character)
	unit := strings.ToUpper(sizeStr[len(sizeStr)-1:])
	numberStr := sizeStr[:len(sizeStr)-1]

	// Parse the number part
	var number float64
	var err error
	if _, err = fmt.Sscanf(numberStr, "%f", &number); err != nil {
		return 0, fmt.Errorf("invalid number in size: %s", numberStr)
	}

	// Convert based on unit
	var multiplier int64
	switch unit {
	case "B":
		multiplier = 1
	case "K":
		multiplier = 1024
	case "M":
		multiplier = 1024 * 1024
	case "G":
		multiplier = 1024 * 1024 * 1024
	case "T":
		multiplier = 1024 * 1024 * 1024 * 1024
	case "P":
		multiplier = 1024 * 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown size unit: %s", unit)
	}

	return int64(number * float64(multiplier)), nil
}

// ValidateBackupSpace validates that an external drive has sufficient space for system backup.
// Compares the used space on the root filesystem against the total capacity of the external drive.
// Returns an error with detailed space information if the drive is too small.
func ValidateBackupSpace(externalDriveSize string) error {
	// Get used space on internal drive (what we need to backup)
	internalUsedSpace, err := GetUsedDiskSpace("/")
	if err != nil {
		return fmt.Errorf("failed to get internal drive usage: %v", err)
	}

	// Parse external drive total size
	externalTotalSize, err := ParseDriveSize(externalDriveSize)
	if err != nil {
		return fmt.Errorf("failed to parse external drive size: %v", err)
	}

	// Check: internal_used_space <= external_total_size
	if internalUsedSpace > externalTotalSize {
		return fmt.Errorf("⚠️ INSUFFICIENT SPACE for backup\n\nInternal drive used: %s\nExternal drive total: %s\n\nThe external drive is too small to hold your backup.\nYou need at least %s of total drive capacity.",
			FormatBytes(internalUsedSpace),
			FormatBytes(externalTotalSize),
			FormatBytes(internalUsedSpace))
	}

	return nil
}

// ValidateSelectiveBackupSpace validates space for selective home backup.
// FIXED: Now properly handles hierarchical folder selections from the UI selection map.
func ValidateSelectiveBackupSpace(homeFolders []HomeFolderInfo, selectedFolders map[string]bool, subfolderCache map[string][]HomeFolderInfo, externalDriveSize string) error {
	// Use the same logic as calculateTotalBackupSize() for consistency
	var totalSelectedSize int64
	processedParents := make(map[string]bool)

	for _, folder := range homeFolders {
		if folder.AlwaysInclude {
			// Hidden folders are always included (dotfiles/dotdirs)
			totalSelectedSize += folder.Size
		} else if folder.IsVisible {
			// Handle visible folders with potential subfolders
			if folder.HasSubfolders {
				// Check if any subfolders are cached (user has drilled down)
				if subfolders, exists := subfolderCache[folder.Path]; exists {
					// User has drilled down - calculate based on individual subfolder selections
					subfolderTotal := int64(0)
					anySubfolderSelected := false

					for _, subfolder := range subfolders {
						if subfolder.Size > 0 && selectedFolders[subfolder.Path] {
							subfolderTotal += subfolder.Size
							anySubfolderSelected = true
						}
					}

					// Only add subfolders if at least one is selected
					if anySubfolderSelected {
						totalSelectedSize += subfolderTotal
					}
					processedParents[folder.Path] = true
				} else {
					// No subfolders cached - use parent folder selection
					if selectedFolders[folder.Path] {
						totalSelectedSize += folder.Size
					}
					processedParents[folder.Path] = true
				}
			} else {
				// No subfolders - use parent folder selection directly
				if selectedFolders[folder.Path] {
					totalSelectedSize += folder.Size
				}
				processedParents[folder.Path] = true
			}
		}
	}

	// Additional: Add any individually selected subfolders whose parents weren't processed
	for folderPath, isSelected := range selectedFolders {
		if !isSelected {
			continue
		}

		// Check if this is a subfolder (has a parent path that was processed)
		parentProcessed := false
		for processedParent := range processedParents {
			if strings.HasPrefix(folderPath, processedParent+"/") {
				parentProcessed = true
				break
			}
		}

		// If no parent was processed, this might be a standalone subfolder selection
		if !parentProcessed {
			// Find the subfolder in cache and add its size
			for _, cachedSubfolders := range subfolderCache {
				for _, subfolder := range cachedSubfolders {
					if subfolder.Path == folderPath && subfolder.Size > 0 {
						totalSelectedSize += subfolder.Size
						break
					}
				}
			}
		}
	}

	// Parse external drive total size
	externalTotalSize, err := ParseDriveSize(externalDriveSize)
	if err != nil {
		return fmt.Errorf("failed to parse external drive size: %v", err)
	}

	// Check: selected_folders_size <= external_total_size
	if totalSelectedSize > externalTotalSize {
		return fmt.Errorf("⚠️ INSUFFICIENT SPACE for selective home backup\n\nSelected folders size: %s\nExternal drive total: %s\n\nThe external drive is too small to hold your selected folders.\nYou need at least %s of total drive capacity.",
			FormatBytes(totalSelectedSize),
			FormatBytes(externalTotalSize),
			FormatBytes(totalSelectedSize))
	}

	return nil
}

// ValidateHomeBackupSpace validates space for home backup.
func ValidateHomeBackupSpace(externalDriveSize string) error {
	// Get actual home directory size instead of full internal drive
	homeDirSize, err := GetHomeDirSize()
	if err != nil {
		return fmt.Errorf("failed to calculate home directory size: %v", err)
	}

	// Parse external drive total size
	externalTotalSize, err := ParseDriveSize(externalDriveSize)
	if err != nil {
		return fmt.Errorf("failed to parse external drive size: %v", err)
	}

	// Check: home_directory_size <= external_total_size
	if homeDirSize > externalTotalSize {
		return fmt.Errorf("⚠️ INSUFFICIENT SPACE for home backup\n\nHome directory size: %s\nExternal drive total: %s\n\nThe external drive is too small to hold your home directory.\nYou need at least %s of total drive capacity.",
			FormatBytes(homeDirSize),
			FormatBytes(externalTotalSize),
			FormatBytes(homeDirSize))
	}

	return nil
}

// ValidateRestoreSpace validates space for restore operations.
func ValidateRestoreSpace(externalDriveSize string, externalMountPoint string) error {
	// Get used space on external drive (backup size)
	externalUsedSpace, err := GetUsedDiskSpace(externalMountPoint)
	if err != nil {
		return fmt.Errorf("failed to get backup drive usage: %v", err)
	}

	// Get total size of internal drive
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return fmt.Errorf("failed to get internal drive info: %v", err)
	}

	internalTotalSize := int64(stat.Blocks) * int64(stat.Bsize)

	// Check: external_used_space <= internal_total_size
	if externalUsedSpace > internalTotalSize {
		return fmt.Errorf("⚠️ INSUFFICIENT SPACE for restore\n\nBackup size: %s\nInternal drive total: %s\n\nThe backup is too large to fit on your internal drive.\nYou need at least %s of total drive capacity.",
			FormatBytes(externalUsedSpace),
			FormatBytes(internalTotalSize),
			FormatBytes(externalUsedSpace))
	}

	return nil
}

// ValidateSelectiveRestoreSpace validates space for selective folder restore.
// Only counts the space needed for the folders the user actually selected to restore.
func ValidateSelectiveRestoreSpace(restoreFolders []HomeFolderInfo, selectedFolders map[string]bool, restoreConfig bool, restoreWindowMgrs bool) error {
	// Calculate space required for SELECTED items only
	var totalSelectedSize int64

	// Add selected folders
	for _, folder := range restoreFolders {
		if folder.AlwaysInclude || selectedFolders[folder.Path] {
			totalSelectedSize += folder.Size
		}
	}

	// Add estimates for configuration options
	if restoreConfig {
		totalSelectedSize += 100 * 1024 * 1024 // ~100MB estimate for .config
	}
	if restoreWindowMgrs {
		totalSelectedSize += 50 * 1024 * 1024 // ~50MB estimate for window managers
	}

	// Get total size of internal drive
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return fmt.Errorf("failed to get internal drive info: %v", err)
	}

	internalTotalSize := int64(stat.Blocks) * int64(stat.Bsize)

	// Check: selected_restore_size <= internal_total_size
	if totalSelectedSize > internalTotalSize {
		return fmt.Errorf("⚠️ INSUFFICIENT SPACE for restore\n\nSelected items size: %s\nInternal drive total: %s\n\nThe selected items are too large to fit on your internal drive.\nYou need at least %s of total drive capacity.",
			FormatBytes(totalSelectedSize),
			FormatBytes(internalTotalSize),
			FormatBytes(totalSelectedSize))
	}

	return nil
}
