// Package drives provides drive detection, mounting, and management functionality.
// This module handles space requirement validation for backup and restore operations.
package drives

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ParseDriveSize converts human-readable size strings to bytes.
// Supports standard units: B, K, M, G, T, P (case-insensitive).
// Examples: "1.5T" -> 1,649,267,441,664 bytes, "500G" -> 537,109,987,328 bytes
func ParseDriveSize(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(sizeStr)
	sizeStr = strings.ReplaceAll(sizeStr, " ", "")
	if len(sizeStr) < 2 {
		return 0, fmt.Errorf("invalid size string: %s", sizeStr)
	}

	// Extract unit (trailing alpha characters) and number part
	// Handles: "1.5TB", "1.5T", "500G", "256.0 GB" (space already stripped)
	unitIdx := len(sizeStr)
	for unitIdx > 0 {
		c := sizeStr[unitIdx-1]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			break
		}
		unitIdx--
	}
	unit := strings.ToUpper(sizeStr[unitIdx:])
	// FormatBytes produces "X.X TB", "X.X GB" — take only the prefix letter
	if len(unit) > 1 {
		unit = unit[:1]
	}
	numberStr := sizeStr[:unitIdx]
	if unit == "" || numberStr == "" {
		return 0, fmt.Errorf("invalid size string: %s", sizeStr)
	}

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

// ValidateMountedBackupSpace validates that a mounted drive has sufficient space for system backup.
// Uses intelligent incremental backup space calculation.
func ValidateMountedBackupSpace(mountPoint string) error {
	// Get used space on internal drive (what we need to backup)
	internalUsedSpace, err := GetUsedDiskSpace("/")
	if err != nil {
		return fmt.Errorf("failed to get internal drive usage: %v", err)
	}

	// Get available space on the mounted external drive
	availableSpace, err := GetAvailableDiskSpace(mountPoint)
	if err != nil {
		return fmt.Errorf("failed to get available space on backup drive: %v", err)
	}

	// Check if there's an existing backup
	existingBackupSize := int64(0)
	hasExistingBackup := false
	if _, err := os.Stat(filepath.Join(mountPoint, "BACKUP-INFO.txt")); err == nil {
		// Existing backup detected - get its size
		existingBackupSize, _ = GetUsedDiskSpace(mountPoint)
		hasExistingBackup = true
	}

	var spaceNeeded int64
	if hasExistingBackup {
		// Incremental backup with delete-first: deletion frees space before copying
		// We only need buffer space for temporary operations during sync
		// The delete phase will free approximately the same space that copy phase uses

		// Conservative buffer: 10GB for temporary files during sync operations
		bufferSpace := int64(10 * 1024 * 1024 * 1024) // 10GB buffer
		spaceNeeded = bufferSpace

		// For very small systems, use 5% of internal space as minimum buffer
		minBuffer := int64(float64(internalUsedSpace) * 0.05) // 5% of system size
		if minBuffer < bufferSpace {
			minBuffer = bufferSpace
		}
		spaceNeeded = minBuffer
	} else {
		// First backup: need full space
		spaceNeeded = internalUsedSpace
	}

	// Check: space_needed <= external_available_space
	if spaceNeeded > availableSpace {
		if hasExistingBackup {
			return fmt.Errorf("⚠️ INSUFFICIENT SPACE for incremental backup\n\nInternal drive used: %s\nExisting backup size: %s\nBuffer space needed: %s\nExternal drive available: %s\n\nIncremental backup uses delete-first to free space before copying.\nYou need at least %s of available space for sync operations.",
				FormatBytes(internalUsedSpace),
				FormatBytes(existingBackupSize),
				FormatBytes(spaceNeeded),
				FormatBytes(availableSpace),
				FormatBytes(spaceNeeded))
		} else {
			return fmt.Errorf("⚠️ INSUFFICIENT SPACE for backup\n\nInternal drive used: %s\nExternal drive available: %s\n\nThe backup drive doesn't have enough free space.\nYou need at least %s of available space.",
				FormatBytes(internalUsedSpace),
				FormatBytes(availableSpace),
				FormatBytes(internalUsedSpace))
		}
	}

	return nil
}

// ValidateMountedHomeBackupSpace validates that a mounted drive has sufficient space for home backup.
// Uses intelligent incremental backup space calculation.
func ValidateMountedHomeBackupSpace(mountPoint string) error {
	// Get home directory size
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}

	homeDirSize, err := CalculateDirectorySize(homeDir)
	if err != nil {
		return fmt.Errorf("failed to calculate home directory size: %v", err)
	}

	// Get available space on the mounted external drive
	availableSpace, err := GetAvailableDiskSpace(mountPoint)
	if err != nil {
		return fmt.Errorf("failed to get available space on backup drive: %v", err)
	}

	// Check if there's an existing backup
	existingBackupSize := int64(0)
	hasExistingBackup := false
	if _, err := os.Stat(filepath.Join(mountPoint, "BACKUP-INFO.txt")); err == nil {
		// Existing backup detected - get its size
		existingBackupSize, _ = GetUsedDiskSpace(mountPoint)
		hasExistingBackup = true
	}

	var spaceNeeded int64
	if hasExistingBackup {
		// Incremental backup: estimate 15% change for home directories (more volatile than system)
		estimatedChangePercent := 0.15 // 15% change estimate
		estimatedChanges := int64(float64(homeDirSize) * estimatedChangePercent)
		spaceNeeded = estimatedChanges

		// Minimum safety buffer of 500MB for small home directories
		minBuffer := int64(500 * 1024 * 1024) // 500MB
		if spaceNeeded < minBuffer {
			spaceNeeded = minBuffer
		}
	} else {
		// First backup: need full space
		spaceNeeded = homeDirSize
	}

	// Check: space_needed <= external_available_space
	if spaceNeeded > availableSpace {
		if hasExistingBackup {
			return fmt.Errorf("⚠️ INSUFFICIENT SPACE for incremental home backup\n\nHome directory size: %s\nExisting backup size: %s\nEstimated changes: %s\nExternal drive available: %s\n\nIncremental backup needs space for changed files only.\nYou need at least %s of available space.",
				FormatBytes(homeDirSize),
				FormatBytes(existingBackupSize),
				FormatBytes(spaceNeeded),
				FormatBytes(availableSpace),
				FormatBytes(spaceNeeded))
		} else {
			return fmt.Errorf("⚠️ INSUFFICIENT SPACE for home backup\n\nHome directory size: %s\nExternal drive available: %s\n\nThe backup drive doesn't have enough free space.\nYou need at least %s of available space.",
				FormatBytes(homeDirSize),
				FormatBytes(availableSpace),
				FormatBytes(homeDirSize))
		}
	}

	return nil
}

// ValidateSelectiveBackupSpaceOnMounted validates space for selective home backup on mounted drive.
// Checks actual available space and accounts for existing backup overlap during sync.
func ValidateSelectiveBackupSpaceOnMounted(homeFolders []HomeFolderInfo, selectedFolders map[string]bool, subfolderCache map[string][]HomeFolderInfo, mountPoint string) error {
	// Calculate total size of selected folders
	totalSelectedSize := int64(0)

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
					for _, subfolder := range subfolders {
						if selectedFolders[subfolder.Path] {
							subfolderTotal += subfolder.Size
						}
					}
					totalSelectedSize += subfolderTotal
				} else {
					// User hasn't drilled down - use parent folder selection
					if selectedFolders[folder.Path] {
						totalSelectedSize += folder.Size
					}
				}
			} else {
				// Simple folder without subfolders
				if selectedFolders[folder.Path] {
					totalSelectedSize += folder.Size
				}
			}
		}
	}

	// Get available space on the mounted external drive
	availableSpace, err := GetAvailableDiskSpace(mountPoint)
	if err != nil {
		return fmt.Errorf("failed to get available space on backup drive: %v", err)
	}

	// Check if there's an existing backup that might need to coexist temporarily
	existingBackupSize := int64(0)
	if _, err := os.Stat(filepath.Join(mountPoint, "BACKUP-INFO.txt")); err == nil {
		// Existing backup detected - get its size
		existingBackupSize, _ = GetUsedDiskSpace(mountPoint)
	}

	// Calculate space needed: new backup + existing backup (temporary overlap during sync)
	spaceNeeded := totalSelectedSize
	if existingBackupSize > 0 {
		// Need space for both old and new backup during sync process
		spaceNeeded = totalSelectedSize + existingBackupSize
	}

	// Check: space_needed <= external_available_space
	if spaceNeeded > availableSpace {
		if existingBackupSize > 0 {
			return fmt.Errorf("⚠️ INSUFFICIENT SPACE for selective home backup\n\nSelected folders size: %s\nExisting backup size: %s\nSpace needed (peak during sync): %s\nExternal drive available: %s\n\nDuring backup, both old and new files exist temporarily.\nYou need at least %s of available space.",
				FormatBytes(totalSelectedSize),
				FormatBytes(existingBackupSize),
				FormatBytes(spaceNeeded),
				FormatBytes(availableSpace),
				FormatBytes(spaceNeeded))
		} else {
			return fmt.Errorf("⚠️ INSUFFICIENT SPACE for selective home backup\n\nSelected folders size: %s\nExternal drive available: %s\n\nThe backup drive doesn't have enough free space.\nYou need at least %s of available space.",
				FormatBytes(totalSelectedSize),
				FormatBytes(availableSpace),
				FormatBytes(totalSelectedSize))
		}
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
func ValidateRestoreSpace(externalDriveSize string, externalMountPoint string, targetPath string) error {
	// Get used space on external drive (backup size)
	externalUsedSpace, err := GetUsedDiskSpace(externalMountPoint)
	if err != nil {
		return fmt.Errorf("failed to get backup drive usage: %v", err)
	}

	// Get total size of target partition (could be / for system restore or /home for home restore)
	if targetPath == "" {
		targetPath = "/"
	}

	// FIXED: Find the actual mount point that contains the target path
	// This ensures we check the correct partition (e.g., /home instead of /)
	actualMountPoint, err := findMountPointForPath(targetPath)
	if err != nil {
		return fmt.Errorf("failed to find mount point for %s: %v", targetPath, err)
	}

	targetTotalSize, _, _, err := GetDiskStats(actualMountPoint)
	if err != nil {
		return fmt.Errorf("failed to get target drive info for %s: %v", actualMountPoint, err)
	}

	// Check: external_used_space <= target_total_size
	if externalUsedSpace > targetTotalSize {
		return fmt.Errorf("⚠️ INSUFFICIENT SPACE for restore\n\nBackup size: %s\nTarget partition (%s) total: %s\n\nThe backup is too large to fit on your target partition.\nYou need at least %s of total drive capacity.",
			FormatBytes(externalUsedSpace),
			actualMountPoint,
			FormatBytes(targetTotalSize),
			FormatBytes(externalUsedSpace))
	}

	return nil
}

// ValidateSelectiveRestoreSpace validates space for selective folder restore.
// Only counts the space needed for the folders the user actually selected to restore.
func ValidateSelectiveRestoreSpace(restoreFolders []HomeFolderInfo, selectedFolders map[string]bool, restoreConfig bool, restoreWindowMgrs bool, targetPath string) error {
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

	// Get total size of target partition (could be / for system restore or /home for home restore)
	if targetPath == "" {
		targetPath = "/"
	}

	// FIXED: Find the actual mount point that contains the target path
	// This ensures we check the correct partition (e.g., /home instead of /)
	actualMountPoint, err := findMountPointForPath(targetPath)
	if err != nil {
		return fmt.Errorf("failed to find mount point for %s: %v", targetPath, err)
	}

	targetTotalSize, _, _, err := GetDiskStats(actualMountPoint)
	if err != nil {
		return fmt.Errorf("failed to get target drive info for %s: %v", actualMountPoint, err)
	}

	// Check: selected_restore_size <= target_total_size
	if totalSelectedSize > targetTotalSize {
		return fmt.Errorf("⚠️ INSUFFICIENT SPACE for restore\n\nSelected items size: %s\nTarget partition (%s) total: %s\n\nThe selected items are too large to fit on your target partition.\nYou need at least %s of total drive capacity.",
			FormatBytes(totalSelectedSize),
			actualMountPoint,
			FormatBytes(targetTotalSize),
			FormatBytes(totalSelectedSize))
	}

	return nil
}

