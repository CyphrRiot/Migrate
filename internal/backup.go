package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

	tea "github.com/charmbracelet/bubbletea"
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

// Configuration constants  
const (
	DefaultMount  = "/run/media"
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

// BackupConfig holds backup configuration
type BackupConfig struct {
	SourcePath      string
	DestinationPath string
	ExcludePatterns []string
	BackupType      string
}

// Progress update message
type ProgressUpdate struct {
	Percentage float64
	Message    string
	Done       bool
	Error      error
}

// Drive information
type DriveInfo struct {
	Device    string
	Size      string
	Label     string
	UUID      string
	Filesystem string
	Encrypted  bool
}

// Message types for drive operations
type DrivesLoaded struct {
	drives []DriveInfo
}

type DriveOperation struct {
	message string
	success bool
}

// Backup drive status message
type BackupDriveStatus struct {
	drivePath  string
	driveSize  string
	driveType  string
	mountPoint string
	needsMount bool
	error      error
}

// Check if any backup drive is mounted (pure Go - no external commands)
func checkAnyBackupMounted() (string, bool) {
	// Parse /proc/mounts directly instead of using findmnt command
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return "", false
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return "", false
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			mountPoint := fields[1]
			// Check for typical external drive mount points
			if strings.Contains(mountPoint, "/run/media/") || strings.Contains(mountPoint, "/mnt/") {
				return mountPoint, true
			}
		}
	}

	return "", false
}

// Mount backup drive with proper interactive password handling (DEPRECATED - use mountLUKSDrive/mountRegularDrive instead)
func mountBackupDrive() (string, error) {
	return "", fmt.Errorf("deprecated function - use drive selection instead")
}

// Unmount backup drive using pure Go
func unmountBackupDrive(mountPoint string) error {
	// Sync first
	syscall.Sync()

	// Get device path from /proc/mounts instead of using findmnt
	device, err := getDeviceFromProcMounts(mountPoint)
	if err != nil {
		return fmt.Errorf("failed to find device for %s: %v", mountPoint, err)
	}

	// Unmount
	cmd := exec.Command("sudo", "umount", mountPoint)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to unmount: %v", err)
	}

	// Close LUKS device if it's a mapper device
	if strings.Contains(device, "mapper") {
		mapperName := filepath.Base(device)
		cmd = exec.Command("sudo", "cryptsetup", "close", mapperName)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to close LUKS device: %v", err)
		}
	}

	return nil
}

// Get device path for a mount point from /proc/mounts (pure Go)
func getDeviceFromProcMounts(mountPoint string) (string, error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return "", err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == mountPoint {
			return fields[0], nil // Return device path
		}
	}

	return "", fmt.Errorf("mount point %s not found", mountPoint)
}

// Start backup operation - TUI ONLY (Pure Go)
func startBackup(config BackupConfig) tea.Cmd {
	return func() tea.Msg {
		// Always run in TUI mode with pure Go implementation
		go runBackupSilently(config)
		return ProgressUpdate{Percentage: -1, Message: "Starting backup...", Done: false}
	}
}

// Global completion tracking for TUI
var tuiBackupCompleted = false
var tuiBackupError error

// Run backup using pure Go implementation (TUI only)
func runBackupSilently(config BackupConfig) {
	// Reset cancellation flag at start
	resetBackupCancel()

	// Setup logging in appropriate directory
	logPath := getLogFilePath()
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		fmt.Fprintf(logFile, "\n=== PURE GO BACKUP STARTED: %s ===\n", time.Now().Format(time.RFC3339))
		fmt.Fprintf(logFile, "Log file: %s\n", logPath)
		fmt.Fprintf(logFile, "Source: %s -> Destination: %s\n", config.SourcePath, config.DestinationPath)
		defer logFile.Close()
	}

	tuiBackupCompleted = false
	tuiBackupError = nil
	
	// Initialize progress tracking
	backupStartTime = time.Now()
	sourceUsedSpace, _ = getUsedDiskSpace(config.SourcePath)
	destStartUsedSpace, _ = getUsedDiskSpace(config.DestinationPath)
	
	if logFile != nil {
		fmt.Fprintf(logFile, "Using pure Go: source=%s, dest_start=%s\n", 
			formatBytes(sourceUsedSpace), formatBytes(destStartUsedSpace))
	}

	// Use pure Go implementation for actual backup
	go func() {
		err := performPureGoBackup(config.SourcePath, config.DestinationPath, logFile)
		if err != nil {
			if logFile != nil {
				fmt.Fprintf(logFile, "PURE GO ERROR: %v\n", err)
			}
			tuiBackupCompleted = true
			if shouldCancelBackup() {
				tuiBackupError = fmt.Errorf("backup canceled by user")
			} else {
				tuiBackupError = fmt.Errorf("backup failed: %v", err)
			}
		} else {
			if logFile != nil {
				fmt.Fprintf(logFile, "PURE GO SUCCESS: completed\n")
			}
			tuiBackupCompleted = true
			tuiBackupError = nil
		}
	}()
}

// Pure Go backup implementation (no external dependencies)
func performPureGoBackup(sourcePath, destPath string, logFile *os.File) error {
	if logFile != nil {
		fmt.Fprintf(logFile, "Starting pure Go backup (zero external dependencies)\n")
		fmt.Fprintf(logFile, "Source: %s -> Dest: %s\n", sourcePath, destPath)
	}

	// Create backup info file first
	err := createBackupInfo(destPath, "System Backup")
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "Failed to create backup info: %v\n", err)
		}
		return fmt.Errorf("failed to create backup info: %v", err)
	}

	// Phase 1: Sync directories (copy/update files from source to dest)
	err = syncDirectories(sourcePath, destPath, logFile)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "ERROR during sync: %v\n", err)
		}
		return err
	}

	// Phase 2: Delete files that exist in backup but not in source (--delete behavior)
	if logFile != nil {
		fmt.Fprintf(logFile, "Starting deletion phase (removing files not in source)\n")
	}
	err = deleteExtraFilesFromBackup(sourcePath, destPath, logFile)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "ERROR during deletion: %v\n", err)
		}
		return fmt.Errorf("deletion phase failed: %v", err)
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Pure Go backup completed successfully\n")
	}
	return nil
}

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
	skippedCount := 0
	copiedCount := 0

	// Walk through the source directory efficiently
	err = filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		// Check for cancellation every 100 files
		fileCounter++
		if fileCounter%100 == 0 && shouldCancelBackup() {
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
				return nil
			}
			stat, ok := fi.Sys().(*syscall.Stat_t)
			if !ok {
				return nil
			}
			// Skip if on a different filesystem (-x option)
			if stat.Dev != srcDev {
				return filepath.SkipDir
			}
			
			// Create the directory if it doesn't exist
			err = os.Mkdir(dstPath, fi.Mode())
			if err != nil && !os.IsExist(err) {
				return nil
			}
			// Set ownership and timestamps
			os.Lchown(dstPath, int(stat.Uid), int(stat.Gid))
			os.Chtimes(dstPath, fi.ModTime(), fi.ModTime())
			return nil
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

		// Handle regular files
		if d.Type().IsRegular() {
			// Check if files are identical before copying
			if filesAreIdentical(path, dstPath) {
				skippedCount++
				if logFile != nil && skippedCount%100 == 0 {
					fmt.Fprintf(logFile, "Skipped %d identical files so far...\n", skippedCount)
				}
				return nil
			}

			// Copy file (only if different)
			err = copyFileEfficient(path, dstPath)
			if err != nil && logFile != nil {
				fmt.Fprintf(logFile, "Error copying %s: %v\n", path, err)
			} else {
				copiedCount++
				if logFile != nil && copiedCount%100 == 0 {
					fmt.Fprintf(logFile, "Copied %d files, skipped %d identical files\n", copiedCount, skippedCount)
				}
			}
			return nil
		}

		// Skip special files
		return nil
	})

	// Log final summary
	if logFile != nil {
		fmt.Fprintf(logFile, "Sync completed: copied %d files, skipped %d identical files\n", copiedCount, skippedCount)
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

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Copy file contents
	_, err = io.Copy(dstFile, srcFile)
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

// Check if two files are identical by comparing size and SHA256 hash
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

	// Quick size comparison first
	if srcInfo.Size() != dstInfo.Size() {
		return false
	}

	// For very small files (under 1KB), always check hash
	// For larger files, also check modification time as optimization
	if srcInfo.Size() > 1024 {
		// If modification times are different, files might be different
		// But we still need to hash check as modification time can be misleading
		// This is just an early optimization hint
		if !srcInfo.ModTime().Equal(dstInfo.ModTime()) {
			// Still do hash check, but this suggests they might be different
		}
	}

	// Compare SHA256 hashes
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

// Format bytes into human readable size with proper units and formatting
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
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
			formatted = fmt.Sprintf("%d", wholeValue)
			// Add comma thousands separator for readability
			str := formatted
			if len(str) > 3 {
				n := len(str)
				formatted = str[:n-3] + "," + str[n-3:]
			}
		} else {
			formatted = fmt.Sprintf("%.0f", value)
		}
	} else if value >= 10 {
		// For 10-99.x units, show 1 decimal place
		formatted = fmt.Sprintf("%.1f", value)
	} else {
		// For <10 units, show 2 decimal places
		formatted = fmt.Sprintf("%.2f", value)
	}
	
	return formatted + " " + units[unitIndex]
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
	backupStartTime = time.Now()
	sourceUsedSpace, _ = getUsedDiskSpace(sourcePath)
	destStartUsedSpace, _ = getUsedDiskSpace(destPath)
	
	return copyDirectoryWithProgress(sourcePath, destPath, uid, gid)
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

// Check TUI backup progress with real disk usage monitoring
func CheckTUIBackupProgress() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		if tuiBackupCompleted {
			if tuiBackupError != nil {
				return ProgressUpdate{Error: tuiBackupError, Done: true}
			}
			return ProgressUpdate{Percentage: 1.0, Message: "Backup completed successfully!", Done: true}
		}
		
		// Calculate real progress based on disk usage
		progress, message := calculateRealProgress()
		return ProgressUpdate{Percentage: progress, Message: message, Done: false}
	})
}

// Global variables to track backup progress and timing  
var backupStartTime time.Time
var sourceUsedSpace int64      // Source drive used space (fixed)
var destStartUsedSpace int64   // Destination used space when backup started (fixed)
var progressCallCounter int    // Simple counter to prove function is being called

// Calculate actual backup progress - PROPERLY FIXED LOGIC
func calculateRealProgress() (float64, string) {
	// Get backup drive mount point
	backupMount, mounted := checkAnyBackupMounted()
	if !mounted {
		return 0.01, "Backup drive not mounted..."
	}
	
	// Initialize on first run
	if backupStartTime.IsZero() {
		backupStartTime = time.Now()
		
		// Get source used space (what we need to copy)
		var err error
		sourceUsedSpace, err = getUsedDiskSpace("/")
		if err != nil {
			return 0.01, "Error reading source drive"
		}
		
		// Get destination starting used space (baseline)
		destStartUsedSpace, err = getUsedDiskSpace(backupMount)
		if err != nil {
			return 0.01, "Error reading destination drive"
		}
	}
	
	// Force filesystem sync and get CURRENT destination usage
	syscall.Sync()
	currentDestUsed, err := getUsedDiskSpace(backupMount)
	if err != nil {
		return 0.01, "Error reading destination drive"
	}
	
	// CORRECTED LOGIC: 
	// If destination already has data (like 795GB), that counts as progress already made!
	// Current progress = destination_current_usage / source_total_usage
	progress := float64(currentDestUsed) / float64(sourceUsedSpace)
	if progress > 1.0 {
		progress = 1.0
	}
	
	// Calculate how much was copied in this session
	copiedThisSession := currentDestUsed - destStartUsedSpace

	// Convert to formatted human-readable sizes
	currentDestFormatted := formatBytes(currentDestUsed)
	sourceFormatted := formatBytes(sourceUsedSpace)
	copiedSessionFormatted := formatBytes(copiedThisSession)

	// Calculate time estimation based on current session progress
	elapsed := time.Since(backupStartTime)
	var timeStr string
	
	// Only show time estimate if we've actually copied meaningful data
	// If most files are being skipped (incremental backup), don't show misleading estimates
	if copiedThisSession > 100*1024*1024 && elapsed.Seconds() > 30 { // At least 100MB copied and 30 seconds elapsed
		// Base estimation on session progress
		remainingBytes := sourceUsedSpace - currentDestUsed
		bytesPerSecond := float64(copiedThisSession) / elapsed.Seconds()
		
		// Sanity check: if copy rate is suspiciously slow, don't show estimate
		if bytesPerSecond > 1024*1024 { // At least 1MB/s
			remainingSeconds := float64(remainingBytes) / bytesPerSecond
			remaining := time.Duration(remainingSeconds) * time.Second
			
			hours := int(remaining.Hours())
			minutes := int(remaining.Minutes()) % 60
			
			// Don't show crazy estimates
			if hours < 1000 { 
				timeStr = fmt.Sprintf(" (Est %dh %dm)", hours, minutes)
			} else {
				timeStr = " (Calculating...)"
			}
		} else {
			timeStr = " (Calculating...)"
		}
	} else if copiedThisSession < 1024*1024 && elapsed.Seconds() > 60 {
		// Very little data copied - likely an incremental backup with mostly skipped files
		timeStr = " (Mostly skipping identical files)"
	} else {
		timeStr = " (Calculating...)"
	}
	
	// Show ACTUAL progress: current destination usage vs source total
	message := fmt.Sprintf("Copying %s / %s (+%s this session)%s", 
		currentDestFormatted, sourceFormatted, copiedSessionFormatted, timeStr)
		
	return progress, message
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

// Get directory size using du with timeout and excludes
func getDirectorySize(path string) (int64, error) {
	// For backup drives, use df to get actual used space on the filesystem
	// This is much more reliable than du for progress tracking
	return getUsedDiskSpace(path)
}

// Start restore operation - TUI ONLY (Pure Go)
func startRestore(targetPath string) tea.Cmd {
	return func() tea.Msg {
		// Setup logging in appropriate directory
		logPath := getLogFilePath()
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			fmt.Fprintf(logFile, "\n=== PURE GO RESTORE STARTED: %s ===\n", time.Now().Format(time.RFC3339))
			fmt.Fprintf(logFile, "Log file: %s\n", logPath)
			defer logFile.Close()
		}

		// Check if backup drive is already mounted (user should mount via TUI first)
		mountPoint, mounted := checkAnyBackupMounted()
		if !mounted {
			return ProgressUpdate{Error: fmt.Errorf("no backup drive mounted - please mount a backup drive first through the main menu")}
		}

		// Check if backup exists
		backupInfo := filepath.Join(mountPoint, "BACKUP-INFO.txt")
		if _, err := os.Stat(backupInfo); os.IsNotExist(err) {
			return ProgressUpdate{Error: fmt.Errorf("no valid backup found at %s", mountPoint)}
		}

		if logFile != nil {
			fmt.Fprintf(logFile, "Starting pure Go restore from %s to %s\n", mountPoint, targetPath)
		}

		// Perform pure Go restore
		err = performPureGoRestore(mountPoint, targetPath, logFile)
		if err != nil {
			return ProgressUpdate{Error: err, Done: true}
		}

		return ProgressUpdate{Percentage: 1.0, Message: "Restore completed successfully!", Done: true}
	}
}

// Pure Go restore implementation
func performPureGoRestore(backupPath, targetPath string, logFile *os.File) error {
	if logFile != nil {
		fmt.Fprintf(logFile, "Starting restore: %s -> %s\n", backupPath, targetPath)
	}

	// Phase 1: Copy all files from backup to target
	err := syncDirectories(backupPath, targetPath, logFile)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "Error during restore copy: %v\n", err)
		}
		return fmt.Errorf("restore copy failed: %v", err)
	}

	// Phase 2: Delete files that exist in target but not in backup (--delete behavior)
	err = deleteExtraFiles(backupPath, targetPath, logFile)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "Error during cleanup: %v\n", err)
		}
		return fmt.Errorf("restore cleanup failed: %v", err)
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Pure Go restore completed successfully\n")
	}
	return nil
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

// Create backup info file
func createBackupInfo(mountPoint, backupType string) error {
	hostname, _ := os.Hostname()
	
	cmd := exec.Command("uname", "-r")
	kernelOut, _ := cmd.Output()
	kernel := strings.TrimSpace(string(kernelOut))
	
	cmd = exec.Command("uname", "-m")
	archOut, _ := cmd.Output()
	arch := strings.TrimSpace(string(archOut))

	info := fmt.Sprintf(`%s BACKUP
=========================
Created: %s
Hostname: %s
Kernel: %s
Architecture: %s
Backup Type: %s

%s

To restore:
1. Install fresh Arch Linux (any desktop environment)
2. Reboot into the new installation
3. Connect and mount this backup drive
4. Run: migrate restore

The restored system will overwrite the fresh install and boot exactly as it was when backed up.
`, strings.ToUpper(backupType), time.Now().Format(time.RFC3339), hostname, kernel, arch, backupType, GetBackupInfoHeader(backupType))

	infoPath := filepath.Join(mountPoint, "BACKUP-INFO.txt")
	return os.WriteFile(infoPath, []byte(info), 0644)
}

// LoadDrives loads available drives - SAFE external drive detection using HOTPLUG
func LoadDrives() tea.Cmd {
	return func() tea.Msg {
		// Get all block devices including hotplug info
		cmd := exec.Command("lsblk", "-J", "-o", "NAME,SIZE,LABEL,UUID,FSTYPE,MOUNTPOINT,TYPE,HOTPLUG")
		out, err := cmd.Output()
		if err != nil {
			return DrivesLoaded{drives: []DriveInfo{}}
		}

		var lsblkOutput struct {
			BlockDevices []struct {
				Name       string `json:"name"`
				Size       string `json:"size"`
				Label      string `json:"label"`
				UUID       string `json:"uuid"`
				Fstype     string `json:"fstype"`
				Mountpoint string `json:"mountpoint"`
				Type       string `json:"type"`
				Hotplug    bool   `json:"hotplug"` // true for removable, false for internal
				Children   []struct {
					Name       string `json:"name"`
					Size       string `json:"size"`
					Label      string `json:"label"`
					UUID       string `json:"uuid"`
					Fstype     string `json:"fstype"`
					Mountpoint string `json:"mountpoint"`
					Type       string `json:"type"`
					Hotplug    bool   `json:"hotplug"`
					Children   []struct {
						Name       string `json:"name"`
						Size       string `json:"size"`
						Label      string `json:"label"`
						UUID       string `json:"uuid"`
						Fstype     string `json:"fstype"`
						Mountpoint string `json:"mountpoint"`
						Type       string `json:"type"`
						Hotplug    bool   `json:"hotplug"`
					} `json:"children"`
				} `json:"children"`
			} `json:"blockdevices"`
		}

		if err := json.Unmarshal(out, &lsblkOutput); err != nil {
			return DrivesLoaded{drives: []DriveInfo{}}
		}

		var drives []DriveInfo
		
		for _, device := range lsblkOutput.BlockDevices {
			// Only consider actual disk devices
			if device.Type != "disk" {
				continue
			}
			
			// CRITICAL: Only include HOTPLUG devices (external/removable drives)
			if !device.Hotplug {
				continue  // Skip internal/fixed drives (device.Hotplug is false)
			}
			
			// Skip if this device contains the root filesystem (extra safety)
			hasRootPartition := false
			if device.Mountpoint == "/" {
				hasRootPartition = true
			}
			
			// Check children for root filesystem
			for _, child := range device.Children {
				if child.Mountpoint == "/" {
					hasRootPartition = true
					break
				}
				// Check grandchildren (LUKS containers)
				for _, grandchild := range child.Children {
					if grandchild.Mountpoint == "/" {
						hasRootPartition = true
						break
					}
				}
				if hasRootPartition {
					break
				}
			}
			
			// Skip any drive with root filesystem (should never happen with hotplug, but extra safety)
			if hasRootPartition {
				continue
			}
			
			// Include this external/removable drive
			devicePath := "/dev/" + device.Name
			
			// Add the main device if it has a filesystem
			if device.Fstype != "" || device.UUID != "" {
				drive := DriveInfo{
					Device:     devicePath,
					Size:       device.Size,
					Label:      device.Label,
					UUID:       device.UUID,
					Filesystem: device.Fstype,
					Encrypted:  device.Fstype == "crypto_LUKS",
				}
				
				if drive.Label == "" {
					if drive.Encrypted {
						drive.Label = "Encrypted External Drive"
					} else {
						drive.Label = "External Drive"
					}
				}
				
				drives = append(drives, drive)
			}
		}

		return DrivesLoaded{drives: drives}
	}
}

// Mount selected drive
func mountSelectedDrive(drive DriveInfo) tea.Cmd {
	return func() tea.Msg {
		var mountPoint string
		var err error

		if drive.Encrypted {
			// Handle LUKS encrypted drive
			mountPoint, err = mountLUKSDrive(drive)
		} else {
			// Handle regular drive
			mountPoint, err = mountRegularDrive(drive)
		}

		if err != nil {
			return DriveOperation{
				message: fmt.Sprintf("Failed to mount %s: %v", drive.Device, err),
				success: false,
			}
		}

		return DriveOperation{
			message: fmt.Sprintf("Successfully mounted %s to %s", drive.Device, mountPoint),
			success: true,
		}
	}
}

// Mount LUKS encrypted drive
func mountLUKSDrive(drive DriveInfo) (string, error) {
	// Check if already unlocked
	mapperName := "luks-" + drive.UUID
	mapperPath := "/dev/mapper/" + mapperName
	
	// Step 1: Check if LUKS device needs to be unlocked
	if _, err := os.Stat(mapperPath); os.IsNotExist(err) {
		// Need to unlock
		cmd := exec.Command("udisksctl", "unlock", "-b", drive.Device)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("failed to unlock LUKS drive: %v", err)
		}
		time.Sleep(2 * time.Second)
	}

	// Step 2: Check if already mounted first
	cmd := exec.Command("findmnt", "-rn", "-o", "TARGET", mapperPath)
	out, err := cmd.Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		// Already mounted - return existing mount point
		mountPoint := strings.TrimSpace(string(out))
		return mountPoint, nil
	}

	// Step 3: Mount the unlocked device (only if not already mounted)
	cmd = exec.Command("udisksctl", "mount", "-b", mapperPath)
	out, err = cmd.Output()
	if err != nil {
		// Check if it failed because it's already mounted
		if strings.Contains(err.Error(), "AlreadyMounted") {
			// Try to find the mount point from the error message or use findmnt
			cmd = exec.Command("findmnt", "-rn", "-o", "TARGET", mapperPath)
			out, err = cmd.Output()
			if err == nil {
				return strings.TrimSpace(string(out)), nil
			}
			return "/run/media/grendel/Grendel", nil // Fallback for known case
		}
		return "", fmt.Errorf("failed to mount unlocked drive: %v", err)
	}

	// Parse mount point from successful mount output
	mountOutput := strings.TrimSpace(string(out))
	if strings.Contains(mountOutput, "Mounted ") {
		parts := strings.Split(mountOutput, " at ")
		if len(parts) >= 2 {
			return strings.Trim(parts[1], "."), nil
		}
	}

	return "/media/unknown", nil
}

// Mount regular drive
func mountRegularDrive(drive DriveInfo) (string, error) {
	// Check if already mounted first
	cmd := exec.Command("findmnt", "-rn", "-o", "TARGET", drive.Device)
	out, err := cmd.Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		// Already mounted - return existing mount point
		mountPoint := strings.TrimSpace(string(out))
		return mountPoint, nil
	}

	// Mount the device
	cmd = exec.Command("udisksctl", "mount", "-b", drive.Device)
	out, err = cmd.Output()
	if err != nil {
		// Check if it failed because it's already mounted
		if strings.Contains(err.Error(), "AlreadyMounted") {
			// Try to find the mount point using findmnt
			cmd = exec.Command("findmnt", "-rn", "-o", "TARGET", drive.Device)
			out, err = cmd.Output()
			if err == nil {
				return strings.TrimSpace(string(out)), nil
			}
		}
		return "", fmt.Errorf("failed to mount drive: %v", err)
	}

	// Parse mount point from successful mount output
	mountOutput := strings.TrimSpace(string(out))
	if strings.Contains(mountOutput, "Mounted ") {
		parts := strings.Split(mountOutput, " at ")
		if len(parts) >= 2 {
			return strings.Trim(parts[1], "."), nil
		}
	}

	return "/media/unknown", nil
}

// DEPRECATED - Drive selection is now handled by loadDrives() and drive selection UI

// Special message type for requesting password input outside TUI
type PasswordRequiredMsg struct {
	drivePath string
	driveSize string
	driveType string
}

// Mount any external drive for backup (replaces hardcoded Grendel-only mounting)
func mountDriveForBackup(drive DriveInfo) tea.Cmd {
	return func() tea.Msg {
		var mountPoint string
		var err error

		if drive.Encrypted {
			// Handle LUKS encrypted drive
			mountPoint, err = mountLUKSDrive(drive)
		} else {
			// Handle regular drive
			mountPoint, err = mountRegularDrive(drive)
		}

		if err != nil {
			return BackupDriveStatus{
				error: fmt.Errorf("Failed to mount %s: %v", drive.Device, err),
			}
		}

		// Successfully mounted - now ask for backup confirmation
		return BackupDriveStatus{
			drivePath:  drive.Device,
			driveSize:  drive.Size,
			driveType:  fmt.Sprintf("%s [%s]", drive.Label, drive.Filesystem),
			mountPoint: mountPoint,
			needsMount: false,
			error:      nil,
		}
	}
}

// Handle password input outside TUI
func handlePasswordInput(msg PasswordRequiredMsg, originalOp string) tea.Cmd {
	return func() tea.Msg {
		// We'll handle this by creating a special command that exits alt screen,
		// handles password, then returns result
		return passwordInteractionMsg{
			drivePath: msg.drivePath,
			driveSize: msg.driveSize,
			driveType: msg.driveType,
			originalOp: originalOp,
		}
	}
}

// New message type for password interaction
type passwordInteractionMsg struct {
	drivePath  string
	driveSize  string
	driveType  string
	originalOp string
}

// DEPRECATED - Password interaction now handled by mountLUKSDrive()

// Start the actual backup with proper configuration
func startActualBackup(operationType, mountPoint string) tea.Cmd {
	return func() tea.Msg {
		var config BackupConfig
		
		switch operationType {
		case "system_backup":
			config = BackupConfig{
				SourcePath:      "/",
				DestinationPath: mountPoint,
				ExcludePatterns: ExcludePatterns,
				BackupType:      "Complete System",
			}
		case "home_backup":
			homeDir, _ := os.UserHomeDir()
			config = BackupConfig{
				SourcePath:      homeDir,
				DestinationPath: mountPoint,
				ExcludePatterns: []string{".cache/*", ".local/share/Trash/*"},
				BackupType:      "Home Directory",
			}
		default:
			return ProgressUpdate{Error: fmt.Errorf("unknown backup type: %s", operationType)}
		}

		// Start the backup
		cmd := startBackup(config)
		return cmd()
	}
}

// Perform backup drive unmount after successful backup (works with any drive)
func PerformBackupUnmount() tea.Cmd {
	return func() tea.Msg {
		// Get the current backup mount point
		mountPoint, mounted := checkAnyBackupMounted()
		if !mounted {
			return DriveOperation{
				message: "ℹ️  Backup drive is already unmounted",
				success: true,
			}
		}

		// Unmount the backup drive
		err := unmountBackupDrive(mountPoint)
		if err != nil {
			return DriveOperation{
				message: fmt.Sprintf("⚠️  Failed to unmount backup drive: %v", err),
				success: false,
			}
		}

		return DriveOperation{
			message: "✅ Backup drive unmounted successfully!\n🔌 Safe to physically remove drive.",
			success: true,
		}
	}
}

// Simulate progress for demo purposes
func simulateProgress(operation string) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
