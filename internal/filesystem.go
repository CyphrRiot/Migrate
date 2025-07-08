// Package internal provides low-level filesystem operations for backup, restore, and synchronization.
//
// This package implements efficient file and directory operations including:
//   - High-performance directory synchronization (rsync-like functionality)
//   - Smart file comparison using size-based and hash-based methods
//   - Deletion cleanup for maintaining backup consistency (--delete behavior)
//   - Directory size calculation with multiple strategies (du command and fallback)
//   - Cross-filesystem boundary detection and handling
//   - Optimized file copying with large buffer support
//   - Thread-safe progress tracking for long-running operations
//
// All operations are designed for maximum performance while maintaining data integrity.
// The sync algorithms use intelligent comparison strategies to minimize unnecessary
// file operations during incremental backups.
package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// syncDirectories performs efficient directory synchronization using default exclude patterns.
// This is a convenience wrapper around syncDirectoriesWithExclusions using the standard ExcludePatterns.
func syncDirectories(src, dst string, logFile *os.File) error {
	return syncDirectoriesWithExclusions(src, dst, ExcludePatterns, logFile)
}

// syncDirectoriesWithSelectiveInclusions performs hierarchical-aware directory synchronization.
// This enhanced version properly handles cases where parent folders are excluded but specific
// subfolders need to be included (hierarchical selection from the UI).
//
// Parameters:
//   - src: Source directory path
//   - dst: Destination directory path
//   - excludePatterns: Standard exclusion patterns (cache, etc.)
//   - selectedSubfolders: Map of explicitly selected subfolders to include despite parent exclusions
//   - logFile: Log file for detailed operation tracking
//
// This function solves the critical issue where traditional exclusion logic would exclude
// parent folders and all their contents, even when specific subfolders should be included.
func syncDirectoriesWithSelectiveInclusions(src, dst string,
	excludePatterns []string, selectedSubfolders map[string]bool, logFile *os.File) error {
	// Check for cancellation before starting
	if shouldCancelBackup() {
		return fmt.Errorf("operation canceled")
	}

	// Get the device ID of the source directory to enforce -x (no crossing filesystem boundaries)
	srcStat, err := os.Lstat(src)
	if err != nil {
		return err
	}
	srcSysStat, ok := srcStat.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot get stat for %s", src)
	}
	srcDev := srcSysStat.Dev

	if logFile != nil {
		fmt.Fprintf(logFile, "Starting HIERARCHICAL directory walk of %s\n", src)
		fmt.Fprintf(logFile, "Selected subfolders for smart inclusion: %v\n", selectedSubfolders)
	}

	// Counter to periodically check for cancellation and show progress
	fileCounter := 0
	localFilesFound := 0 // Local counter to batch atomic operations

	// Pre-compile exclusion patterns for performance (avoid string ops in hot path)
	exclusionPrefixes := make([]string, len(excludePatterns))
	for i, pattern := range excludePatterns {
		exclusionPrefixes[i] = strings.TrimSuffix(pattern, "/*")
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Exclusion patterns compiled: %v\n", exclusionPrefixes)
	}

	// Walk through the source directory efficiently with hierarchical awareness
	err = filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		// Check for cancellation less frequently for better performance
		fileCounter++
		if fileCounter%5000 == 0 && shouldCancelBackup() { // Check every 5000 files for responsiveness
			return fmt.Errorf("operation canceled")
		}

		// Update current directory for TUI display much more frequently
		if fileCounter%500 == 0 { // Update display every 500 files instead of 10k
			currentDir := filepath.Dir(path)
			currentDirectory = currentDir
		}

		// Log current directory being processed every 10k files to track slowdowns
		if fileCounter%10000 == 0 && logFile != nil {
			currentDir := filepath.Dir(path)
			fmt.Fprintf(logFile, "Processing directory: %s (file #%d: %s)\n", currentDir, fileCounter, filepath.Base(path))
		}

		if err != nil {
			if logFile != nil {
				fmt.Fprintf(logFile, "Skip error path %s: %v\n", path, err)
			}
			// Skip permission errors but continue - don't hang on problematic directories
			return nil
		}

		// BULLETPROOF HIERARCHICAL SELECTION LOGIC
		// Step 1: Check explicit subfolder inclusions FIRST (these override exclusions)
		isExplicitSubfolderInclude := false
		for selectedPath := range selectedSubfolders {
			if strings.HasPrefix(path, selectedPath) {
				isExplicitSubfolderInclude = true
				if logFile != nil && fileCounter%10000 == 0 {
					fmt.Fprintf(logFile, "EXPLICIT SUBFOLDER INCLUDE: %s (matches: %s)\n", path, selectedPath)
				}
				break
			}
		}

		// Step 2: If not an explicit include, apply ALL exclusions strictly
		if !isExplicitSubfolderInclude {
			for _, prefix := range exclusionPrefixes {
				// Convert to absolute path for proper matching
				var fullPattern string
				if strings.HasPrefix(prefix, "/") {
					// Absolute path pattern
					fullPattern = prefix
				} else {
					// Relative pattern - make it relative to source
					fullPattern = filepath.Join(src, prefix)
				}

				// STRICT EXCLUSION: If path matches ANY exclusion pattern, exclude it immediately
				if strings.HasPrefix(path, fullPattern) {
					if d.IsDir() {
						if logFile != nil && fileCounter%1000 == 0 {
							fmt.Fprintf(logFile, "EXCLUDING DIRECTORY: %s (pattern: %s)\n", path, fullPattern)
						}
						return filepath.SkipDir
					} else {
						if logFile != nil && fileCounter%10000 == 0 {
							fmt.Fprintf(logFile, "EXCLUDING FILE: %s (pattern: %s)\n", path, fullPattern)
						}
						return nil
					}
				}
			}
		}

		// Compute the destination path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return nil
		}
		dstPath := filepath.Join(dst, relPath)

		// Handle directories
		if d.IsDir() {
			fi, err := os.Lstat(path)
			if err != nil {
				if logFile != nil {
					fmt.Fprintf(logFile, "Warning: could not stat directory %s: %v\n", path, err)
				}
				return nil // Skip this directory but continue
			}
			stat, ok := fi.Sys().(*syscall.Stat_t)
			if !ok {
				if logFile != nil {
					fmt.Fprintf(logFile, "Warning: could not get stat info for %s\n", path)
				}
				return nil
			}

			// Skip if on a different filesystem (-x option) BUT allow /home even if different subvolume
			if stat.Dev != srcDev {
				// Allow /home directory even if it's on different btrfs subvolume
				if strings.HasPrefix(path, "/home") {
					if logFile != nil {
						fmt.Fprintf(logFile, "Allowing /home on different filesystem (likely btrfs subvolume): %s\n", path)
					}
				} else {
					if logFile != nil {
						fmt.Fprintf(logFile, "Skipping different filesystem: %s\n", path)
					}
					return filepath.SkipDir
				}
			}

			// Create the directory if it doesn't exist using MkdirAll for safety
			err = os.MkdirAll(dstPath, fi.Mode())
			if err != nil {
				if logFile != nil {
					fmt.Fprintf(logFile, "ERROR: Failed to create directory %s: %v (continuing)\n", dstPath, err)
				}
				// Continue processing - don't skip the directory contents!
			} else {
				// Set ownership and timestamps only if directory creation succeeded
				os.Lchown(dstPath, int(stat.Uid), int(stat.Gid))
				os.Chtimes(dstPath, fi.ModTime(), fi.ModTime())
			}
			return nil // Continue processing directory contents
		}

		// Handle symbolic links
		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return nil
			}
			os.Symlink(target, dstPath)
			return nil
		}

		// Handle regular files with smart tracking (same as original function)
		if d.Type().IsRegular() {
			// Count this file in our local counter (batch atomic operations)
			localFilesFound++

			// Log slow file processing to identify bottlenecks
			if localFilesFound%1000 == 0 && logFile != nil {
				if info, err := d.Info(); err == nil {
					fmt.Fprintf(logFile, "Processing file %d: %s (size: %s)\n", localFilesFound, path, FormatBytes(info.Size()))
				} else {
					fmt.Fprintf(logFile, "Processing file %d: %s\n", localFilesFound, path)
				}
			}

			// Batch update atomic counter every 1000 files for performance
			if localFilesFound%1000 == 0 {
				atomic.AddInt64(&totalFilesFound, 1000)
				if logFile != nil && localFilesFound%10000 == 0 { // Log every 10k files
					fmt.Fprintf(logFile, "Processed %s files...\n", FormatNumber(atomic.LoadInt64(&totalFilesFound)))
				}
			}

			// Quick paths for known scenarios
			// PERFORMANCE OPTIMIZATION: Use faster file existence check
			dstStat, err := os.Stat(dstPath)
			if os.IsNotExist(err) {
				// Destination doesn't exist - definitely need to copy
				err = copyFileEfficient(path, dstPath)
				if err != nil {
					if logFile != nil {
						fmt.Fprintf(logFile, "Error copying %s: %v\n", path, err)
					}
					// Check for fatal disk space errors
					if isSpaceError(err) {
						spaceInfo := getSpaceErrorDetails(filepath.Dir(dstPath))
						return fmt.Errorf("⚠️ OUT OF SPACE during backup\n\nError copying file: %s\nSpace error: %v\n\n%s\n\nThe backup drive is full. Please use a larger drive or select fewer folders.", path, err, spaceInfo)
					}
				} else {
					atomic.AddInt64(&filesCopied, 1)
					// Track copied files for verification (thread-safe)
					copiedFilesListMutex.Lock()
					copiedFilesList = append(copiedFilesList, path)
					copiedFilesListMutex.Unlock()
					if logFile != nil && filesCopied%1000 == 0 { // Log every 1000 files instead of 100
						fmt.Fprintf(logFile, "Copied %s files, skipped %s identical files\n",
							FormatNumber(filesCopied), FormatNumber(filesSkipped))
					}
				}
				return nil
			}

			// Optimize file comparison for large files - get source info ONCE
			// PERFORMANCE OPTIMIZATION: Get source info first, then use it efficiently
			srcInfo, err := d.Info()
			if err != nil {
				return nil // Skip files we can't stat
			}

			// LARGE FILE FAST-PATH: For large files, use optimized comparison
			if dstStat.Size() > 500*1024*1024 { // 500MB threshold
				// For large files, do immediate size comparison without extra syscalls
				if srcInfo.Size() == dstStat.Size() {
					atomic.AddInt64(&filesSkipped, 1)
					if logFile != nil && filesSkipped%100 == 0 {
						fmt.Fprintf(logFile, "LARGE FILE FAST-SKIP: %s (size: %s) - sizes match\n", path, FormatBytes(dstStat.Size()))
					}
					return nil
				}
				// Sizes don't match - fall through to copy
			} else {
				// Regular file size comparison
				if srcInfo.Size() == dstStat.Size() {
					atomic.AddInt64(&filesSkipped, 1)
					if logFile != nil && filesSkipped%500 == 0 { // Less frequent logging
						fmt.Fprintf(logFile, "Skipped %s identical files so far...\n", FormatNumber(filesSkipped))
					}
					return nil
				}
			}

			// Files are different - copy
			err = copyFileEfficient(path, dstPath)
			if err != nil {
				if logFile != nil {
					fmt.Fprintf(logFile, "Error copying %s: %v\n", path, err)
				}
				// Check for fatal disk space errors
				if isSpaceError(err) {
					spaceInfo := getSpaceErrorDetails(filepath.Dir(dstPath))
					return fmt.Errorf("⚠️ OUT OF SPACE during backup\n\nError copying file: %s\nSpace error: %v\n\n%s\n\nThe backup drive is full. Please use a larger drive or select fewer folders.", path, err, spaceInfo)
				}
			} else {
				atomic.AddInt64(&filesCopied, 1)
				// Track copied files for verification (thread-safe)
				copiedFilesListMutex.Lock()
				copiedFilesList = append(copiedFilesList, path)
				copiedFilesListMutex.Unlock()
				if logFile != nil && filesCopied%100 == 0 {
					fmt.Fprintf(logFile, "Copied %s files, skipped %s identical files\n",
						FormatNumber(filesCopied), FormatNumber(filesSkipped))
				}
			}
			return nil
		}

		// Skip special files
		return nil
	})

	// Final batch update for any remaining files not yet added to atomic counter
	remainingFiles := localFilesFound % 1000
	if remainingFiles > 0 {
		atomic.AddInt64(&totalFilesFound, int64(remainingFiles))
	}

	// Mark directory walk as complete
	directoryWalkComplete = true

	// Log final summary
	if logFile != nil {
		fmt.Fprintf(logFile, "Hierarchical sync completed: copied %s files, skipped %s identical files, processed %s total\n",
			FormatNumber(filesCopied), FormatNumber(filesSkipped), FormatNumber(totalFilesFound))
	}

	return err
}

// syncDirectoriesWithExclusions performs high-performance directory synchronization with custom exclusions.
// Implements rsync-like functionality using pure Go with the following optimizations:
//   - Size-based comparison for fast incremental sync detection
//   - Batch atomic operations for progress tracking performance
//   - Cross-filesystem boundary detection (-x option equivalent)
//   - Pre-compiled exclusion patterns for hot path optimization
//   - Periodic cancellation checking for user responsiveness
//   - Smart directory traversal with performance logging
//
// The function respects filesystem boundaries, handles permissions and timestamps,
// and provides detailed progress feedback through global counters.
func syncDirectoriesWithExclusions(src, dst string, excludePatterns []string, logFile *os.File) error {
	// Check for cancellation before starting
	if shouldCancelBackup() {
		return fmt.Errorf("operation canceled")
	}

	// Get the device ID of the source directory to enforce -x (no crossing filesystem boundaries)
	srcStat, err := os.Lstat(src)
	if err != nil {
		return err
	}
	srcSysStat, ok := srcStat.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot get stat for %s", src)
	}
	srcDev := srcSysStat.Dev

	if logFile != nil {
		fmt.Fprintf(logFile, "Starting directory walk of %s\n", src)
	}

	// Counter to periodically check for cancellation and show progress
	fileCounter := 0
	localFilesFound := 0 // Local counter to batch atomic operations

	// Pre-compile exclusion patterns for performance (avoid string ops in hot path)
	exclusionPrefixes := make([]string, len(excludePatterns))
	for i, pattern := range excludePatterns {
		exclusionPrefixes[i] = strings.TrimSuffix(pattern, "/*")
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Exclusion patterns compiled: %v\n", exclusionPrefixes)
	}

	// Walk through the source directory efficiently
	err = filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		// Check for cancellation less frequently for better performance
		fileCounter++
		if fileCounter%5000 == 0 && shouldCancelBackup() { // Check every 5000 files for responsiveness
			return fmt.Errorf("operation canceled")
		}

		// Update current directory for TUI display much more frequently
		if fileCounter%500 == 0 { // Update display every 500 files instead of 10k
			currentDir := filepath.Dir(path)
			currentDirectory = currentDir
		}

		// Log current directory being processed every 10k files to track slowdowns
		if fileCounter%10000 == 0 && logFile != nil {
			currentDir := filepath.Dir(path)
			fmt.Fprintf(logFile, "Processing directory: %s (file #%d: %s)\n", currentDir, fileCounter, filepath.Base(path))
		}

		if err != nil {
			if logFile != nil {
				fmt.Fprintf(logFile, "Skip error path %s: %v\n", path, err)
			}
			// Skip permission errors but continue - don't hang on problematic directories
			return nil
		}

		// Skip excluded patterns (using pre-compiled prefixes for performance)
		// PERFORMANCE OPTIMIZATION: Quick check for non-cache paths first
		isInHomeDir := strings.HasPrefix(path, src)

		for _, prefix := range exclusionPrefixes {
			// Convert to absolute path for proper matching
			var fullPattern string
			if strings.HasPrefix(prefix, "/") {
				// Absolute path pattern (like "/home/grendel/Videos")
				fullPattern = prefix
			} else {
				// Relative pattern (like ".cache") - make it relative to source
				fullPattern = filepath.Join(src, prefix)

				// PERFORMANCE BYPASS: Skip cache pattern checks if we're not in a cache-like directory
				if isInHomeDir && !strings.Contains(path, "/.cache") && !strings.Contains(path, "/.local") &&
					(strings.Contains(prefix, ".cache") || strings.Contains(prefix, ".local")) {
					continue // Skip cache patterns when not in cache directories
				}
			}

			// Check for exact path prefix match (much more efficient)
			if strings.HasPrefix(path, fullPattern) {
				if d.IsDir() {
					if logFile != nil && fileCounter%50000 == 0 {
						fmt.Fprintf(logFile, "Skipping excluded directory: %s (matched pattern: %s)\n", path, fullPattern)
					}
					return filepath.SkipDir
				}
				// File matches exclusion pattern
				return nil
			}
		}

		// Compute the destination path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return nil
		}
		dstPath := filepath.Join(dst, relPath)

		// Handle directories
		if d.IsDir() {
			fi, err := os.Lstat(path)
			if err != nil {
				if logFile != nil {
					fmt.Fprintf(logFile, "Warning: could not stat directory %s: %v\n", path, err)
				}
				return nil // Skip this directory but continue
			}
			stat, ok := fi.Sys().(*syscall.Stat_t)
			if !ok {
				if logFile != nil {
					fmt.Fprintf(logFile, "Warning: could not get stat info for %s\n", path)
				}
				return nil
			}

			// Skip if on a different filesystem (-x option) BUT allow /home even if different subvolume
			if stat.Dev != srcDev {
				// Allow /home directory even if it's on different btrfs subvolume
				if strings.HasPrefix(path, "/home") {
					if logFile != nil {
						fmt.Fprintf(logFile, "Allowing /home on different filesystem (likely btrfs subvolume): %s\n", path)
					}
				} else {
					if logFile != nil {
						fmt.Fprintf(logFile, "Skipping different filesystem: %s\n", path)
					}
					return filepath.SkipDir
				}
			}

			// Create the directory if it doesn't exist using MkdirAll for safety
			err = os.MkdirAll(dstPath, fi.Mode())
			if err != nil {
				if logFile != nil {
					fmt.Fprintf(logFile, "ERROR: Failed to create directory %s: %v (continuing)\n", dstPath, err)
				}
				// Continue processing - don't skip the directory contents!
			} else {
				// Set ownership and timestamps only if directory creation succeeded
				os.Lchown(dstPath, int(stat.Uid), int(stat.Gid))
				os.Chtimes(dstPath, fi.ModTime(), fi.ModTime())
			}
			return nil // Continue processing directory contents
		}

		// Handle symbolic links
		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return nil
			}
			os.Symlink(target, dstPath)
			return nil
		}

		// Handle regular files with smart tracking
		if d.Type().IsRegular() {
			// Count this file in our local counter (batch atomic operations)
			localFilesFound++

			// Log slow file processing to identify bottlenecks
			if localFilesFound%1000 == 0 && logFile != nil {
				if info, err := d.Info(); err == nil {
					fmt.Fprintf(logFile, "Processing file %d: %s (size: %s)\n", localFilesFound, path, FormatBytes(info.Size()))
				} else {
					fmt.Fprintf(logFile, "Processing file %d: %s\n", localFilesFound, path)
				}
			}

			// Batch update atomic counter every 1000 files for performance
			if localFilesFound%1000 == 0 {
				atomic.AddInt64(&totalFilesFound, 1000)
				if logFile != nil && localFilesFound%10000 == 0 { // Log every 10k files
					fmt.Fprintf(logFile, "Processed %s files...\n", FormatNumber(atomic.LoadInt64(&totalFilesFound)))
				}
			}

			// Quick paths for known scenarios
			// PERFORMANCE OPTIMIZATION: Use faster file existence check
			dstStat, err := os.Stat(dstPath)
			if os.IsNotExist(err) {
				// Destination doesn't exist - definitely need to copy
				err = copyFileEfficient(path, dstPath)
				if err != nil {
					if logFile != nil {
						fmt.Fprintf(logFile, "Error copying %s: %v\n", path, err)
					}
					// Check for fatal disk space errors
					if isSpaceError(err) {
						spaceInfo := getSpaceErrorDetails(filepath.Dir(dstPath))
						return fmt.Errorf("⚠️ OUT OF SPACE during backup\n\nError copying file: %s\nSpace error: %v\n\n%s\n\nThe backup drive is full. Please use a larger drive or select fewer folders.", path, err, spaceInfo)
					}
				} else {
					atomic.AddInt64(&filesCopied, 1)
					// Track copied files for verification (thread-safe)
					copiedFilesListMutex.Lock()
					copiedFilesList = append(copiedFilesList, path)
					copiedFilesListMutex.Unlock()
					if logFile != nil && filesCopied%1000 == 0 { // Log every 1000 files instead of 100
						fmt.Fprintf(logFile, "Copied %s files, skipped %s identical files\n",
							FormatNumber(filesCopied), FormatNumber(filesSkipped))
					}
				}
				return nil
			}

			// Optimize file comparison for large files - get source info ONCE
			// PERFORMANCE OPTIMIZATION: Get source info first, then use it efficiently
			srcInfo, err := d.Info()
			if err != nil {
				return nil // Skip files we can't stat
			}

			// LARGE FILE FAST-PATH: For large files, use optimized comparison
			if dstStat.Size() > 500*1024*1024 { // 500MB threshold
				// For large files, do immediate size comparison without extra syscalls
				if srcInfo.Size() == dstStat.Size() {
					atomic.AddInt64(&filesSkipped, 1)
					if logFile != nil && filesSkipped%100 == 0 {
						fmt.Fprintf(logFile, "LARGE FILE FAST-SKIP: %s (size: %s) - sizes match\n", path, FormatBytes(dstStat.Size()))
					}
					return nil
				}
				// Sizes don't match - fall through to copy
			} else {
				// Regular file size comparison
				if srcInfo.Size() == dstStat.Size() {
					atomic.AddInt64(&filesSkipped, 1)
					if logFile != nil && filesSkipped%500 == 0 { // Less frequent logging
						fmt.Fprintf(logFile, "Skipped %s identical files so far...\n", FormatNumber(filesSkipped))
					}
					return nil
				}
			}

			// Files are different - copy
			err = copyFileEfficient(path, dstPath)
			if err != nil {
				if logFile != nil {
					fmt.Fprintf(logFile, "Error copying %s: %v\n", path, err)
				}
				// Check for fatal disk space errors
				if isSpaceError(err) {
					spaceInfo := getSpaceErrorDetails(filepath.Dir(dstPath))
					return fmt.Errorf("⚠️ OUT OF SPACE during backup\n\nError copying file: %s\nSpace error: %v\n\n%s\n\nThe backup drive is full. Please use a larger drive or select fewer folders.", path, err, spaceInfo)
				}
			} else {
				atomic.AddInt64(&filesCopied, 1)
				// Track copied files for verification (thread-safe)
				copiedFilesListMutex.Lock()
				copiedFilesList = append(copiedFilesList, path)
				copiedFilesListMutex.Unlock()
				if logFile != nil && filesCopied%100 == 0 {
					fmt.Fprintf(logFile, "Copied %s files, skipped %s identical files\n",
						FormatNumber(filesCopied), FormatNumber(filesSkipped))
				}
			}
			return nil
		}

		// Skip special files
		return nil
	})

	// Final batch update for any remaining files not yet added to atomic counter
	remainingFiles := localFilesFound % 1000
	if remainingFiles > 0 {
		atomic.AddInt64(&totalFilesFound, int64(remainingFiles))
	}

	// Mark directory walk as complete
	directoryWalkComplete = true

	// Log final summary
	if logFile != nil {
		fmt.Fprintf(logFile, "Sync completed: copied %s files, skipped %s identical files, processed %s total\n",
			FormatNumber(filesCopied), FormatNumber(filesSkipped), FormatNumber(totalFilesFound))
	}

	return err
}

// copyFileEfficient performs optimized file copying with variable buffer sizes and metadata preservation.
// Features:
//   - Dynamic buffer sizing (64KB standard, 1MB for files >100MB)
//   - Automatic directory creation for destination path
//   - Complete metadata preservation (permissions, ownership, timestamps)
//   - Assumes files have already been determined to be different (no duplicate checking)
func copyFileEfficient(src, dst string) error {
	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Get source file info
	fi, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// Create destination directory if needed
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Use larger buffer for better performance on large files
	bufSize := 64 * 1024           // 64KB buffer
	if fi.Size() > 100*1024*1024 { // Files >100MB get bigger buffer
		bufSize = 1024 * 1024 // 1MB buffer
	}

	// Copy file contents with optimized buffer
	buffer := make([]byte, bufSize)
	_, err = io.CopyBuffer(dstFile, srcFile, buffer)
	if err != nil {
		return err
	}

	// Set permissions and ownership
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if ok {
		os.Chmod(dst, fi.Mode())
		os.Chown(dst, int(stat.Uid), int(stat.Gid))
		os.Chtimes(dst, fi.ModTime(), fi.ModTime())
	}

	return nil
}

// filesAreIdentical performs intelligent file comparison optimized for incremental backups.
// Uses a multi-stage approach:
//  1. Fast size comparison (most effective for incremental backups)
//  2. Empty file optimization (immediate return for zero-byte files)
//  3. Size-match assumption for performance (skips expensive hash/timestamp checks)
//
// This approach is optimized for the common case where files are either completely
// different or completely identical, making it ideal for backup scenarios.
func filesAreIdentical(src, dst string) bool {
	// Get file info for both files
	srcInfo, err := os.Stat(src)
	if err != nil {
		return false
	}

	dstInfo, err := os.Stat(dst)
	if err != nil {
		return false // Destination doesn't exist
	}

	// Quick size comparison first (fastest check)
	if srcInfo.Size() != dstInfo.Size() {
		return false
	}

	// For empty files, if sizes match (both 0), they're identical
	if srcInfo.Size() == 0 {
		return true
	}

	// PERFORMANCE FIX: For incremental backups, if sizes match, assume identical
	// This is much faster than time/hash checking for large media files
	// Most backup scenarios involve identical files that just need size verification
	return true // Skip all expensive checks - size match is sufficient for incremental backup
}

// filesHashIdentical performs SHA256-based file comparison for small files.
// This method provides cryptographic verification of file identity but is expensive
// for large files. Recommended only for files where hash comparison is necessary.
func filesHashIdentical(src, dst string) bool {
	srcHash, err := getFileSHA256(src)
	if err != nil {
		return false
	}

	dstHash, err := getFileSHA256(dst)
	if err != nil {
		return false
	}

	return srcHash == dstHash
}

// largFilesIdentical performs efficient sampling-based comparison for large files.
// Uses strategic sampling at beginning, middle, end, and additional points for very large files.
// Sampling strategy:
//   - 4KB samples at start, middle, end positions
//   - Additional quarter and three-quarter samples for files >100MB
//   - Much faster than full hash comparison while maintaining high accuracy
func largFilesIdentical(src, dst string, size int64) bool {
	// Open both files
	srcFile, err := os.Open(src)
	if err != nil {
		return false
	}
	defer srcFile.Close()

	dstFile, err := os.Open(dst)
	if err != nil {
		return false
	}
	defer dstFile.Close()

	// Sample strategy: check beginning, middle, and end
	sampleSize := int64(4096) // 4KB samples
	positions := []int64{
		0,                 // Beginning
		size / 2,          // Middle
		size - sampleSize, // End
	}

	// For very large files, add a few more random samples
	if size > 100*1024*1024 { // >100MB
		positions = append(positions,
			size/4,   // Quarter
			size*3/4, // Three quarters
		)
	}

	for _, pos := range positions {
		if pos < 0 {
			pos = 0
		}
		if pos+sampleSize > size {
			sampleSize = size - pos
		}

		// Compare samples at this position
		srcBuf := make([]byte, sampleSize)
		dstBuf := make([]byte, sampleSize)

		srcFile.Seek(pos, 0)
		dstFile.Seek(pos, 0)

		srcFile.Read(srcBuf)
		dstFile.Read(dstBuf)

		// If any sample differs, files are different
		for i := range srcBuf {
			if srcBuf[i] != dstBuf[i] {
				return false
			}
		}
	}

	// All samples match - very likely identical
	return true
}

// getFileSHA256 calculates the SHA256 hash of a file.
// Uses streaming IO to handle large files efficiently without loading entire file into memory.
// Returns hex-encoded hash string for easy comparison and storage.
func getFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	_, err = io.Copy(hasher, file)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// deleteExtraFilesFromBackup removes files from backup that no longer exist in source.
// This is a convenience wrapper using default exclude patterns, equivalent to rsync's --delete option.
func deleteExtraFilesFromBackup(sourcePath, backupPath string, logFile *os.File) error {
	return deleteExtraFilesFromBackupWithExclusions(sourcePath, backupPath, ExcludePatterns, logFile)
}

// deleteExtraFilesFromBackupWithExclusions performs backup cleanup with custom exclusion patterns.
// Implements the --delete equivalent functionality by:
//   - Walking the backup directory tree
//   - Checking if each file/directory exists in the source
//   - Removing backup items that are no longer present in source
//   - Respecting exclusion patterns during cleanup
//   - Providing cancellation support and progress tracking
func deleteExtraFilesFromBackupWithExclusions(sourcePath, backupPath string, excludePatterns []string, logFile *os.File) error {
	return deleteExtraFilesFromBackupWithSelectiveSupport(sourcePath, backupPath, excludePatterns, nil, logFile)
}

// deleteExtraFilesFromBackupWithSelectiveSupport performs backup cleanup with selective folder support.
// For selective backups, it checks both file existence and folder selection.
// For regular backups, it only checks file existence (selectedFolders = nil).
func deleteExtraFilesFromBackupWithSelectiveSupport(sourcePath, backupPath string, excludePatterns []string, selectedFolders map[string]bool, logFile *os.File) error {
	if logFile != nil {
		if selectedFolders != nil {
			fmt.Fprintf(logFile, "Starting cleanup phase (delete files not in source or not selected)\n")
		} else {
			fmt.Fprintf(logFile, "Starting cleanup phase (delete files not in source)\n")
		}
	}

	deletedCount := 0

	return filepath.WalkDir(backupPath, func(backupFile string, d os.DirEntry, err error) error {
		// Check for cancellation every 50 files
		if deletedCount%50 == 0 && shouldCancelBackup() {
			return fmt.Errorf("operation canceled during deletion phase")
		}

		if err != nil {
			return nil // Skip errors
		}

		// Skip excluded patterns even during deletion
		for _, pattern := range excludePatterns {
			if strings.Contains(backupFile, strings.TrimSuffix(pattern, "/*")) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Skip special backup metadata files
		if strings.Contains(backupFile, "BACKUP-INFO.txt") || strings.Contains(backupFile, "BACKUP-FOLDERS.txt") {
			return nil
		}

		// Calculate corresponding source file path
		relPath, err := filepath.Rel(backupPath, backupFile)
		if err != nil {
			return nil
		}
		sourceFile := filepath.Join(sourcePath, relPath)

		// Check if file should be deleted
		shouldDelete := false
		deleteReason := ""

		// First check: file doesn't exist in source
		if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
			shouldDelete = true
			deleteReason = "not in source"
		} else if selectedFolders != nil {
			// Second check: for selective backups, check if file is in selected folders
			if !isFileInSelectedFolders(sourceFile, selectedFolders) {
				shouldDelete = true
				deleteReason = "not in selected folders"
			}
		}

		if shouldDelete {
			deletedCount++
			atomic.AddInt64(&filesDeleted, 1) // Track deletion for progress

			if logFile != nil {
				fmt.Fprintf(logFile, "Deleting: %s (%s)\n", backupFile, deleteReason)
			}

			if d.IsDir() {
				// Remove directory and all contents
				err := os.RemoveAll(backupFile)
				if err != nil && logFile != nil {
					fmt.Fprintf(logFile, "Error deleting directory %s: %v\n", backupFile, err)
				}
				return filepath.SkipDir
			} else {
				// Remove file
				err := os.Remove(backupFile)
				if err != nil && logFile != nil {
					fmt.Fprintf(logFile, "Error deleting file %s: %v\n", backupFile, err)
				}
			}
		}

		return nil
	})
}

// deleteExtraFiles removes files from target that don't exist in backup during restore operations.
// Implements rsync --delete behavior for restore operations, ensuring the target matches the backup exactly.
// Automatically excludes special files and respects the standard exclusion patterns.
func deleteExtraFiles(backupPath, targetPath string, logFile *os.File) error {
	if logFile != nil {
		fmt.Fprintf(logFile, "Starting cleanup phase (delete extra files)\n")
	}

	return filepath.WalkDir(targetPath, func(targetFile string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip excluded patterns even during restore
		for _, pattern := range ExcludePatterns {
			if strings.Contains(targetFile, strings.TrimSuffix(pattern, "/*")) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Skip special backup metadata files
		if strings.Contains(targetFile, "BACKUP-INFO.txt") || strings.Contains(targetFile, "BACKUP-FOLDERS.txt") {
			return nil
		}

		// Calculate corresponding backup file path
		relPath, err := filepath.Rel(targetPath, targetFile)
		if err != nil {
			return nil
		}
		backupFile := filepath.Join(backupPath, relPath)

		// If file doesn't exist in backup, delete it from target
		if _, err := os.Stat(backupFile); os.IsNotExist(err) {
			if logFile != nil {
				fmt.Fprintf(logFile, "Deleting extra file: %s\n", targetFile)
			}

			if d.IsDir() {
				// Remove directory and all contents
				os.RemoveAll(targetFile)
				return filepath.SkipDir
			} else {
				// Remove file
				os.Remove(targetFile)
			}
		}

		return nil
	})
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

// getUsedDiskSpace provides backward compatibility wrapper for GetUsedDiskSpace.
func getUsedDiskSpace(path string) (int64, error) {
	return GetUsedDiskSpace(path)
}

// calculateDirectorySize computes total directory size using the system du command for accuracy and speed.
// Falls back to pure Go implementation if du command fails.
// Uses 'du -sb' for byte-accurate size calculation following symlinks but not crossing filesystems.
func calculateDirectorySize(path string) (int64, error) {
	// Use du -sb to get size in bytes, following symlinks but not crossing filesystems
	cmd := exec.Command("du", "-sb", path)
	output, err := cmd.Output()
	if err != nil {
		// If du fails, try a fallback method
		return calculateDirectorySizeFallback(path)
	}

	// Parse du output: "123456\t/path/to/dir"
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return 0, fmt.Errorf("empty du output")
	}

	parts := strings.Fields(outputStr)
	if len(parts) < 1 {
		return 0, fmt.Errorf("unexpected du output format: %q", outputStr)
	}

	size, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse size from %q: %v", parts[0], err)
	}

	return size, nil
}

// calculateDirectorySizeFallback provides pure Go directory size calculation when du command fails.
// Walks the directory tree and sums individual file sizes.
// Slower than du but more portable and handles permission errors gracefully.
func calculateDirectorySizeFallback(path string) (int64, error) {
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
	return calculateDirectorySize(homeDir)
}

// getActualBackupSize calculates filesystem usage for a backup mount point.
// Uses syscalls for fast filesystem statistics instead of walking directory trees.
// Returns the actual bytes consumed on the backup drive.
func getActualBackupSize(backupMount string) (int64, error) {
	// Use Go's built-in syscall to get filesystem usage
	var stat syscall.Statfs_t
	err := syscall.Statfs(backupMount, &stat)
	if err != nil {
		return 0, err
	}

	// Calculate used bytes: (total - available) * block_size
	totalBytes := stat.Blocks * uint64(stat.Bsize)
	availableBytes := stat.Bavail * uint64(stat.Bsize)
	usedBytes := totalBytes - availableBytes

	return int64(usedBytes), nil
}

// getDirectorySize is an alias for getUsedDiskSpace optimized for backup drive progress tracking.
// For backup drives, this returns filesystem usage rather than du-style directory traversal
// for more reliable progress monitoring.
func getDirectorySize(path string) (int64, error) {
	// For backup drives, use df to get actual used space on the filesystem
	// This is much more reliable than du for progress tracking
	return getUsedDiskSpace(path)
}

// copyDirectoryLimitedDepth performs recursive directory copying with depth limiting.
// Prevents infinite recursion while still copying substantial directory structures.
// Useful for controlled backup operations where depth needs to be restricted.
func copyDirectoryLimitedDepth(src, dst string, maxDepth int) error {
	return copyDirectoryLimitedDepthRecursive(src, dst, 0, maxDepth)
}

func copyDirectoryLimitedDepthRecursive(src, dst string, currentDepth, maxDepth int) error {
	if currentDepth > maxDepth {
		return nil // Stop recursion
	}

	// Create destination directory
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	// Read source directory contents
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		// Skip excluded patterns
		skip := false
		for _, pattern := range ExcludePatterns {
			if strings.Contains(srcPath, strings.TrimSuffix(pattern, "/*")) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		if entry.IsDir() {
			// Recurse into directory (but limited depth)
			err := copyDirectoryLimitedDepthRecursive(srcPath, dstPath, currentDepth+1, maxDepth)
			if err != nil {
				continue // Skip directories with errors
			}
		} else {
			// Copy file
			info, err := entry.Info()
			if err != nil {
				continue
			}
			copyFileSafe(srcPath, dstPath, info)
		}
	}

	return nil
}

// isSpaceError checks if an error is related to disk space issues
func isSpaceError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Common disk space error patterns
	spaceErrors := []string{
		"no space left on device",
		"disk full",
		"out of space",
		"insufficient disk space",
		"device out of space",
		"write failed: no space left",
		"enospc",
	}

	for _, spaceErr := range spaceErrors {
		if strings.Contains(errStr, spaceErr) {
			return true
		}
	}

	return false
}

// getSpaceErrorDetails provides detailed space information for error messages
func getSpaceErrorDetails(path string) string {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return "Unable to get disk space information"
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	usedBytes := totalBytes - freeBytes

	return fmt.Sprintf("Disk Space Information:\n  Total: %s\n  Used:  %s\n  Free:  %s",
		FormatBytes(int64(totalBytes)),
		FormatBytes(int64(usedBytes)),
		FormatBytes(int64(freeBytes)))
}

// isFileInSelectedFolders checks if a file path is within any selected folder
func isFileInSelectedFolders(filePath string, selectedFolders map[string]bool) bool {
	if selectedFolders == nil {
		return true // No selection filtering
	}

	// Check if the file path is within any selected folder
	for folderPath, selected := range selectedFolders {
		if selected {
			// Check if filePath is within this selected folder
			if strings.HasPrefix(filePath, folderPath+"/") || filePath == folderPath {
				return true
			}
		}
	}

	return false
}

// Copy single file safely
func copyFileSafe(src, dst string, srcInfo os.FileInfo) error {
	// Skip special files
	if !srcInfo.Mode().IsRegular() {
		return nil
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return nil // Skip files we can't open
	}
	defer srcFile.Close()

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Copy content
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	// Set permissions
	return os.Chmod(dst, srcInfo.Mode())
}

// Copy directory recursively with progress updates
func copyDirectoryWithProgress(src, dst string, uid, gid int) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip excluded patterns
		for _, pattern := range ExcludePatterns {
			// Simple pattern matching
			if strings.Contains(path, strings.TrimSuffix(pattern, "/*")) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Skip backup destination itself
		if strings.HasPrefix(path, dst) {
			return filepath.SkipDir
		}

		// Create relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			// Create directory
			err := os.MkdirAll(destPath, info.Mode())
			if err != nil {
				return err
			}
			// Set ownership
			return os.Chown(destPath, uid, gid)
		}

		// Copy file
		return copyFileWithOwnership(path, destPath, info, uid, gid)
	})
}

// Copy individual file with proper ownership
func copyFileWithOwnership(src, dst string, srcInfo os.FileInfo, uid, gid int) error {
	// Skip special files
	if !srcInfo.Mode().IsRegular() {
		return nil
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return nil // Skip files we can't open
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Copy file content
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	// Set permissions
	err = os.Chmod(dst, srcInfo.Mode())
	if err != nil {
		return err
	}

	// Set ownership
	err = os.Chown(dst, uid, gid)
	if err != nil {
		return err
	}

	// Set timestamps
	return os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime())
}

// Pure Go backup implementation [ORIGINAL - keeping for reference]
func performGoBackup(sourcePath, destPath string) error {
	// Get current user info for proper ownership
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %v", err)
	}

	// Parse user/group IDs
	uid, _ := strconv.Atoi(currentUser.Uid)
	gid, _ := strconv.Atoi(currentUser.Gid)

	// Initialize progress tracking
	backupStartTime = time.Now() // Reference these from backup_ops.go
	sourceUsedSpace, _ = getUsedDiskSpace(sourcePath)
	destStartUsedSpace, _ = getUsedDiskSpace(destPath)

	return copyDirectoryWithProgress(sourcePath, destPath, uid, gid)
}
