package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
)

// Efficient sync directories (based on your working rsync-like implementation)
func syncDirectories(src, dst string, logFile *os.File) error {
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

	// Walk through the source directory efficiently
	err = filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		// Check for cancellation less frequently for better performance
		fileCounter++
		if fileCounter%1000 == 0 && shouldCancelBackup() { // Check every 1000 files instead of 100
			return fmt.Errorf("operation canceled")
		}

		if err != nil {
			if logFile != nil {
				fmt.Fprintf(logFile, "Skip error path %s: %v\n", path, err)
			}
			return nil // Skip errors, don't fail entire backup
		}

		// Skip excluded patterns
		for _, pattern := range ExcludePatterns {
			if strings.Contains(path, strings.TrimSuffix(pattern, "/*")) {
				if d.IsDir() {
					return filepath.SkipDir
				}
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
			// Skip if on a different filesystem (-x option)
			if stat.Dev != srcDev {
				if logFile != nil {
					fmt.Fprintf(logFile, "Skipping different filesystem: %s\n", path)
				}
				return filepath.SkipDir
			}

			// Create the directory if it doesn't exist using MkdirAll for safety
			err = os.MkdirAll(dstPath, fi.Mode())
			if err != nil {
				if logFile != nil {
					fmt.Fprintf(logFile, "Error creating directory %s: %v (continuing)\n", dstPath, err)
				}
				// Continue processing - don't skip the directory contents!
			} else {
				// Set ownership and timestamps only if directory creation succeeded
				os.Lchown(dstPath, int(stat.Uid), int(stat.Gid))
				os.Chtimes(dstPath, fi.ModTime(), fi.ModTime())
				if logFile != nil && strings.Contains(path, "Takeout") {
					fmt.Fprintf(logFile, "Created directory: %s -> %s\n", path, dstPath)
				}
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
			// Count this file in our totals
			atomic.AddInt64(&totalFilesFound, 1)

			// Quick paths for known scenarios
			if _, err := os.Stat(dstPath); os.IsNotExist(err) {
				// Destination doesn't exist - definitely need to copy
				err = copyFileEfficient(path, dstPath)
				if err != nil && logFile != nil {
					fmt.Fprintf(logFile, "Error copying %s: %v\n", path, err)
				} else {
					atomic.AddInt64(&filesCopied, 1)
					if logFile != nil && filesCopied%100 == 0 {
						fmt.Fprintf(logFile, "Copied %s files, skipped %s identical files\n",
							formatNumber(filesCopied), formatNumber(filesSkipped))
					}
				}
				return nil
			}

			// Destination exists - check if identical
			if filesAreIdentical(path, dstPath) {
				atomic.AddInt64(&filesSkipped, 1)
				if logFile != nil && filesSkipped%500 == 0 { // Less frequent logging
					fmt.Fprintf(logFile, "Skipped %s identical files so far...\n", formatNumber(filesSkipped))
				}
				return nil
			}

			// Files are different - copy
			err = copyFileEfficient(path, dstPath)
			if err != nil && logFile != nil {
				fmt.Fprintf(logFile, "Error copying %s: %v\n", path, err)
			} else {
				atomic.AddInt64(&filesCopied, 1)
				if logFile != nil && filesCopied%100 == 0 {
					fmt.Fprintf(logFile, "Copied %s files, skipped %s identical files\n",
						formatNumber(filesCopied), formatNumber(filesSkipped))
				}
			}
			return nil
		}

		// Skip special files
		return nil
	})

	// Mark directory walk as complete
	directoryWalkComplete = true

	// Log final summary
	if logFile != nil {
		fmt.Fprintf(logFile, "Sync completed: copied %s files, skipped %s identical files, processed %s total\n",
			formatNumber(filesCopied), formatNumber(filesSkipped), formatNumber(totalFilesFound))
	}

	return err
}

// Efficient file copying (assumes files are already checked to be different)
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
	bufSize := 64 * 1024 // 64KB buffer
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

// Check if two files are identical using fast checks first, then hash only if needed
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

	// Fast modification time check (much faster than hashing)
	// If times are identical and sizes match, very likely the same file
	if srcInfo.ModTime().Equal(dstInfo.ModTime()) {
		return true // Skip hash check for files with identical mtime + size
	}

	// For small files (≤1MB), do hash comparison
	if srcInfo.Size() <= 1024*1024 {
		return filesHashIdentical(src, dst)
	}

	// For large files, use sampling strategy instead of full hash
	return largFilesIdentical(src, dst, srcInfo.Size())
}

// Hash comparison for small files only
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

// Fast comparison for large files using sampling
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
		0,                // Beginning
		size / 2,         // Middle  
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

// Calculate SHA256 hash of a file
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

// Delete files from backup that no longer exist in source (--delete equivalent)
func deleteExtraFilesFromBackup(sourcePath, backupPath string, logFile *os.File) error {
	if logFile != nil {
		fmt.Fprintf(logFile, "Starting cleanup phase (delete files not in source)\n")
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
		for _, pattern := range ExcludePatterns {
			if strings.Contains(backupFile, strings.TrimSuffix(pattern, "/*")) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Skip special backup files
		if strings.Contains(backupFile, "BACKUP-INFO.txt") {
			return nil
		}

		// Calculate corresponding source file path
		relPath, err := filepath.Rel(backupPath, backupFile)
		if err != nil {
			return nil
		}
		sourceFile := filepath.Join(sourcePath, relPath)

		// If file doesn't exist in source, delete it from backup
		if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
			deletedCount++
			atomic.AddInt64(&filesDeleted, 1) // Track deletion for progress

			if logFile != nil {
				fmt.Fprintf(logFile, "Deleting: %s (not in source)\n", backupFile)
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

// Delete files that exist in target but not in backup (equivalent to rsync --delete)
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

		// Skip special files (BACKUP-INFO.txt, etc.)
		if strings.Contains(targetFile, "BACKUP-INFO.txt") {
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

// Get used disk space using pure Go syscalls (no external commands)
func getUsedDiskSpace(path string) (int64, error) {
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

// Get actual backup size using syscall to get filesystem stats (fast)
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

// Get directory size using du with timeout and excludes
func getDirectorySize(path string) (int64, error) {
	// For backup drives, use df to get actual used space on the filesystem
	// This is much more reliable than du for progress tracking
	return getUsedDiskSpace(path)
}

// Copy directory with limited depth (avoids infinite walking but copies substantial data)
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
	backupStartTime = backupStartTime // Reference these from backup_ops.go
	sourceUsedSpace, _ = getUsedDiskSpace(sourcePath)
	destStartUsedSpace, _ = getUsedDiskSpace(destPath)

	return copyDirectoryWithProgress(sourcePath, destPath, uid, gid)
}
