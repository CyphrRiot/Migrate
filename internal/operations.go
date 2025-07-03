package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

// Global completion tracking for TUI
var tuiBackupCompleted = false
var tuiBackupError error

// Global variables to track backup progress and timing  
var backupStartTime time.Time
var sourceUsedSpace int64      // Source drive used space (fixed)
var destStartUsedSpace int64   // Destination used space when backup started (fixed)
var progressCallCounter int    // Simple counter to prove function is being called
var totalFilesProcessed int64  // New: track files processed
var totalFilesEstimate int64   // New: estimated total files
var syncPhaseComplete bool     // New: track if sync phase is done
var deletionPhaseActive bool   // New: track deletion phase

// SMART PROGRESS TRACKING - Based on actual work done
var filesSkipped int64         // Files skipped (identical)
var filesCopied int64          // Files actually copied  
var filesDeleted int64         // Files deleted in cleanup
var totalFilesFound int64      // Total files discovered during walk
var directoryWalkComplete bool // Directory enumeration finished

// Start backup operation - TUI ONLY (Pure Go)
func startBackup(config BackupConfig) tea.Cmd {
	return func() tea.Msg {
		// Always run in TUI mode with pure Go implementation
		go runBackupSilently(config)
		return ProgressUpdate{Percentage: -1, Message: "Starting backup...", Done: false}
	}
}

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

	// Mark sync phase as complete
	syncPhaseComplete = true

	// Phase 2: Delete files that exist in backup but not in source (--delete behavior)
	if logFile != nil {
		fmt.Fprintf(logFile, "Starting deletion phase (removing files not in source)\n")
	}

	// Mark deletion phase as active
	deletionPhaseActive = true

	err = deleteExtraFilesFromBackup(sourcePath, destPath, logFile)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "ERROR during deletion: %v\n", err)
		}
		return fmt.Errorf("deletion phase failed: %v", err)
	}

	// Mark deletion phase as complete
	deletionPhaseActive = false

	if logFile != nil {
		fmt.Fprintf(logFile, "Pure Go backup completed successfully\n")
	}
	return nil
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

// Calculate actual backup progress - SMART FILE-BASED TRACKING
func calculateRealProgress() (float64, string) {
	// Initialize on first run
	if backupStartTime.IsZero() {
		backupStartTime = time.Now()

		// Reset all counters
		filesSkipped = 0
		filesCopied = 0
		filesDeleted = 0
		totalFilesFound = 0
		directoryWalkComplete = false
		syncPhaseComplete = false
		deletionPhaseActive = false
	}

	// SMART PROGRESS CALCULATION BASED ON ACTUAL WORK
	var progress float64
	var message string

	if deletionPhaseActive {
		// Deletion phase: base progress on files processed vs estimated cleanup
		if totalFilesFound > 0 {
			// 85-100% range for deletion phase
			baseProgress := 0.85
			deletionProgress := float64(filesDeleted) / float64(totalFilesFound/10) // Assume ~10% need deletion
			if deletionProgress > 1.0 {
				deletionProgress = 1.0
			}
			progress = baseProgress + (0.15 * deletionProgress)
		} else {
			progress = 0.92 // Default deletion progress
		}

		message = fmt.Sprintf("Deleting removed files (%s files cleaned up)", formatNumber(filesDeleted))

	} else if syncPhaseComplete {
		// Sync complete, starting deletion
		progress = 0.85
		message = "Preparing deletion phase..."

	} else if directoryWalkComplete {
		// Directory walk done, sync in progress
		if totalFilesFound > 0 {
			filesProcessed := filesSkipped + filesCopied
			syncProgress := float64(filesProcessed) / float64(totalFilesFound)
			if syncProgress > 1.0 {
				syncProgress = 1.0
			}
			progress = syncProgress * 0.85 // Reserve 15% for deletion
		} else {
			progress = 0.50 // Fallback if no file count
		}

		// Show meaningful sync status
		if filesCopied > 0 {
			message = fmt.Sprintf("Syncing files (%s copied, %s skipped of %s total)",
				formatNumber(filesCopied), formatNumber(filesSkipped), formatNumber(totalFilesFound))
		} else if filesSkipped > 1000 {
			message = fmt.Sprintf("Skipping identical files (%s of %s processed)",
				formatNumber(filesSkipped), formatNumber(totalFilesFound))
		} else {
			message = fmt.Sprintf("Processing files (%s of %s)",
				formatNumber(filesSkipped+filesCopied), formatNumber(totalFilesFound))
		}

	} else {
		// Still discovering files
		elapsed := time.Since(backupStartTime)
		if elapsed.Seconds() < 30 {
			progress = 0.05
			message = "Scanning filesystem..."
		} else {
			// Base early progress on files found so far
			estimatedTotal := totalFilesFound * 2 // Rough estimate
			if estimatedTotal > 0 {
				progress = float64(filesSkipped+filesCopied) / float64(estimatedTotal) * 0.85
				if progress > 0.20 {
					progress = 0.20 // Cap early progress
				}
			} else {
				progress = 0.10
			}
			message = fmt.Sprintf("Scanning and processing (%s files found so far)", formatNumber(totalFilesFound))
		}
	}

	// Never exceed 100%
	if progress > 1.0 {
		progress = 1.0
	}

	return progress, message
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

// Simulate progress for demo purposes
func simulateProgress(operation string) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
