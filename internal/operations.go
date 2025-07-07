// Package internal provides the core backup, restore, and verification operations for Migrate.
//
// This package implements the primary business logic including:
//   - Pure Go backup operations with no external dependencies (no rsync)
//   - Smart restore operations with automatic backup type detection
//   - Backup verification with sampling and integrity checking
//   - Real-time progress tracking and cancellation support
//   - Selective home directory backup with folder exclusion
//   - Configuration management for different backup types
//
// The operations are designed to be thread-safe and provide detailed progress
// feedback through Bubble Tea commands. All operations support graceful cancellation
// and comprehensive logging for debugging purposes.
package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// BackupConfig contains all configuration parameters for a backup operation.
// Supports both full system backups and selective home directory backups.
type BackupConfig struct {
	SourcePath         string           // Root directory to backup (e.g., "/", "/home/user")
	DestinationPath    string           // Target directory for backup (mount point)
	ExcludePatterns    []string         // Glob patterns for files/directories to exclude
	BackupType         string           // Human-readable backup type ("Complete System", "Home Directory")
	IsSelectiveBackup  bool             // true for user-selected folder backups
	SelectedFolders    map[string]bool  // folder path -> selected state (for selective backups)
	HomeFolders        []HomeFolderInfo // metadata about home folders (for selective backups)
	SelectedSubfolders map[string]bool  // explicitly selected subfolders for smart inclusion (hierarchical support)
}

// BackupFolderList contains folder selection information from selective home backups.
// This structure is read from BACKUP-FOLDERS.txt during verification to determine
// which folders should and shouldn't exist in the backup.
type BackupFolderList struct {
	IncludedFolders []string // Folders that were backed up (verification should check these)
	ExcludedFolders []string // Folders that were skipped (verification should ignore these)
}

// VerificationConfig controls how backup verification is performed.
// Provides options for sampling, timeouts, and parallel processing.
type VerificationConfig struct {
	SampleRate      float64  // Fraction of unchanged files to verify (0.0-1.0, default 0.01 = 1%)
	TimeoutMinutes  int      // Maximum time to spend on verification (default 5)
	ParallelWorkers int      // Number of concurrent verification workers (default 4)
	CriticalFiles   []string // System files that must always be verified
}

// DefaultVerificationConfig provides sensible defaults for backup verification.
// Balances thoroughness with performance for typical backup sizes.
var DefaultVerificationConfig = VerificationConfig{
	SampleRate:      0.01, // 1% random sampling of unchanged files
	TimeoutMinutes:  5,    // 5 minute timeout
	ParallelWorkers: 4,    // 4 concurrent workers
	CriticalFiles: []string{
		"/etc/fstab",
		"/etc/passwd",
		"/etc/shadow",
		"/etc/group",
		"/boot/grub/grub.cfg",
		"/boot/loader/loader.conf",
		"/etc/systemd/system",
	},
}

// EnableVerification controls whether backup verification is performed.
// Currently disabled by default for debugging purposes.
var EnableVerification = false

// VerificationResult contains the results and statistics from a verification operation.
type VerificationResult struct {
	Success         bool          // true if verification passed without critical errors
	FilesVerified   int64         // Total number of files that were verified
	NewFilesChecked int64         // Number of newly copied files verified
	SampledFiles    int64         // Number of files verified through random sampling
	CriticalFiles   int64         // Number of critical system files verified
	ErrorCount      int           // Count of non-critical errors encountered
	Warnings        []string      // List of warning messages
	Duration        time.Duration // Total time spent on verification
}

// ProgressUpdate is a Bubble Tea message that reports operation progress.
// Used for backup, restore, and verification operations.
type ProgressUpdate struct {
	Percentage float64 // Progress from 0.0 to 1.0, or -1 for indeterminate
	Message    string  // Human-readable status message
	Done       bool    // true when operation is complete
	Error      error   // Non-nil if operation failed
}

// Global state variables for TUI operation tracking.
// These variables coordinate between background operations and the UI.
var (
	// TUI coordination variables
	tuiBackupCompleted  bool  // true when background operation completes
	tuiBackupError      error // non-nil if operation failed
	tuiBackupCancelling bool  // true when cancellation is in progress

	// Timing and baseline measurements
	backupStartTime     time.Time // when the current operation started
	sourceUsedSpace     int64     // source drive used space (fixed at start)
	destStartUsedSpace  int64     // destination used space when backup started
	progressCallCounter int       // simple counter for progress function calls
	totalFilesProcessed int64     // cumulative files processed
	totalFilesEstimate  int64     // estimated total files to process

	// Phase tracking flags
	syncPhaseComplete        bool // true when main sync phase is done
	deletionPhaseActive      bool // true during deletion phase
	directoryWalkComplete    bool // true when initial directory enumeration is done
	verificationPhaseActive  bool // true during verification phase
	isStandaloneVerification bool // true for standalone verification (not part of backup)

	// File operation counters
	filesSkipped    int64 // files skipped (identical between source and destination)
	filesCopied     int64 // files actually copied/updated
	filesDeleted    int64 // files deleted during cleanup phase
	totalFilesFound int64 // total files discovered during directory walk

	// Verification tracking
	totalFilesVerified   int64      // counter for verification progress
	copiedFilesList      []string   // list of files that were actually copied (for verification)
	copiedFilesListMutex sync.Mutex // protect copiedFilesList for thread safety
	verificationErrors   []string   // non-critical errors during verification

	// Enhanced progress tracking
	currentDirectory    string // current directory being scanned (for display)
	lastProgressMessage string // last progress message (to avoid spam)
)

// resetBackupState clears all global progress tracking variables.
// Must be called before starting any new operation to ensure clean state.
func resetBackupState() {
	// Reset timing
	backupStartTime = time.Time{}

	// Reset counters
	filesSkipped = 0
	filesCopied = 0
	filesDeleted = 0
	totalFilesFound = 0
	progressCallCounter = 0
	totalFilesProcessed = 0
	totalFilesEstimate = 0

	// Reset phase tracking
	directoryWalkComplete = false
	syncPhaseComplete = false
	deletionPhaseActive = false
	verificationPhaseActive = false
	isStandaloneVerification = false

	// Reset verification tracking
	totalFilesVerified = 0
	copiedFilesList = []string{}
	verificationErrors = []string{}

	// Reset enhanced progress tracking
	currentDirectory = ""
	lastProgressMessage = ""

	// Reset TUI state
	tuiBackupCompleted = false
	tuiBackupError = nil
	tuiBackupCancelling = false
}

// startBackup creates a Bubble Tea command to initiate a backup operation.
// Resets all state, starts the operation in a background goroutine, and returns
// an initial progress message. The operation runs using pure Go (no external dependencies).
func startBackup(config BackupConfig) tea.Cmd {
	return func() tea.Msg {
		// Reset all backup state before starting
		resetBackupState()

		// Always run in TUI mode with pure Go implementation
		go runBackupSilently(config)
		return ProgressUpdate{Percentage: -1, Message: "Starting backup...", Done: false}
	}
}

// runBackupSilently performs the actual backup operation in the background.
// Handles logging, progress tracking, and error reporting. Runs in pure Go
// without external dependencies like rsync.
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
	tuiBackupCancelling = false

	// Initialize progress tracking
	backupStartTime = time.Now()
	sourceUsedSpace, _ = getUsedDiskSpace(config.SourcePath)
	destStartUsedSpace, _ = getUsedDiskSpace(config.DestinationPath)

	if logFile != nil {
		fmt.Fprintf(logFile, "Using pure Go: source=%s, dest_start=%s\n",
			FormatBytes(sourceUsedSpace), FormatBytes(destStartUsedSpace))
	}

	// Use pure Go implementation for actual backup
	if logFile != nil {
		fmt.Fprintf(logFile, "About to start performPureGoBackup SYNCHRONOUSLY\n")
	}

	// Run synchronously instead of in goroutine to fix execution bug
	err = performPureGoBackup(config, logFile)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "PURE GO ERROR: %v\n", err)
		}
		tuiBackupCompleted = true
		if shouldCancelBackup() {
			tuiBackupCancelling = false // Cancellation complete
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
}

// performPureGoBackup executes the three-phase backup process using only pure Go.
// Phase 1: Sync files from source to destination (with selective exclusions)
// Phase 2: Delete files that exist in destination but not source (--delete behavior)
// Phase 3: Verify backup integrity (if enabled)
// All phases support cancellation and provide detailed progress tracking.
func performPureGoBackup(config BackupConfig, logFile *os.File) error {
	if logFile != nil {
		fmt.Fprintf(logFile, "Starting pure Go backup (zero external dependencies)\n")
		fmt.Fprintf(logFile, "Source: %s -> Dest: %s\n", config.SourcePath, config.DestinationPath)
	}

	// Create backup info file first with CORRECT backup type
	err := createBackupInfo(config.DestinationPath, config.BackupType) // Use actual backup type, not hardcoded
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "Failed to create backup info: %v\n", err)
		}
		return fmt.Errorf("failed to create backup info: %v", err)
	}

	// For selective backups, create folder selection metadata for verification
	if config.IsSelectiveBackup {
		err := createBackupFolderList(config.DestinationPath, config.SelectedFolders, logFile)
		if err != nil {
			if logFile != nil {
				fmt.Fprintf(logFile, "Failed to create backup folder list: %v\n", err)
			}
			return fmt.Errorf("failed to create backup folder list: %v", err)
		}
	}

	// SELECTIVE BACKUP: Handle folder-specific backup with HIERARCHICAL LOGIC
	if config.IsSelectiveBackup {
		if logFile != nil {
			fmt.Fprintf(logFile, "=== SELECTIVE BACKUP MODE ===\n")
			fmt.Fprintf(logFile, "Selected folders: %+v\n", config.SelectedFolders)
			fmt.Fprintf(logFile, "HomeFolders metadata: %d folders\n", len(config.HomeFolders))
			fmt.Fprintf(logFile, "DEBUG: Folder selection details:\n")
			for folderPath, isSelected := range config.SelectedFolders {
				fmt.Fprintf(logFile, "  Folder: '%s' -> Selected: %t\n", folderPath, isSelected)
			}
		}

		// BULLETPROOF HIERARCHICAL EXCLUSION LOGIC
		// Build exclusion patterns that respect subfolder selections
		enhancedExcludes := make([]string, len(config.ExcludePatterns))
		copy(enhancedExcludes, config.ExcludePatterns)

		// SIMPLE APPROACH: Add every deselected folder to exclusions
		// No complex logic - if it's not selected, exclude it
		for folderPath, isSelected := range config.SelectedFolders {
			if !isSelected {
				enhancedExcludes = append(enhancedExcludes, folderPath)
				if logFile != nil {
					fmt.Fprintf(logFile, "EXCLUDING deselected folder: %s\n", folderPath)
				}
			}
		}

		// SIMPLE INCLUSION: Only put EXPLICITLY selected individual subfolders
		// Don't include parent folders at all - they'll be handled by exclusions
		selectedSubfolders := make(map[string]bool)
		for folderPath, isSelected := range config.SelectedFolders {
			if isSelected {
				// Check if this is actually a subfolder (contains "/" after home dir)
				if strings.Count(folderPath, "/") > 3 { // /home/user/parent/subfolder = 4 slashes
					selectedSubfolders[folderPath] = true
					if logFile != nil {
						fmt.Fprintf(logFile, "SELECTED subfolder for inclusion: %s\n", folderPath)
					}
				} else {
					if logFile != nil {
						fmt.Fprintf(logFile, "SELECTED root folder (no special inclusion needed): %s\n", folderPath)
					}
				}
			}
		}

		// Update config with enhanced exclusions
		config.ExcludePatterns = enhancedExcludes
		if logFile != nil {
			fmt.Fprintf(logFile, "Enhanced exclusion patterns: %v\n", enhancedExcludes)
			fmt.Fprintf(logFile, "Selected subfolders for inclusion: %v\n", selectedSubfolders)
		}
	}

	// REGULAR BACKUP: Sync entire source directory with smart hierarchical support
	if config.IsSelectiveBackup {
		err = syncDirectoriesWithSelectiveInclusions(config.SourcePath, config.DestinationPath, config.ExcludePatterns, config.SelectedSubfolders, logFile)
	} else {
		err = syncDirectoriesWithExclusions(config.SourcePath, config.DestinationPath, config.ExcludePatterns, logFile)
	}
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

	err = deleteExtraFilesFromBackupWithExclusions(config.SourcePath, config.DestinationPath, config.ExcludePatterns, logFile)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "ERROR during deletion: %v\n", err)
		}
		return fmt.Errorf("deletion phase failed: %v", err)
	}

	// Mark deletion phase as complete
	deletionPhaseActive = false

	// Phase 3: Verification phase
	if logFile != nil {
		fmt.Fprintf(logFile, "Starting backup verification phase\n")
	}

	// Skip verification if disabled
	if !EnableVerification {
		if logFile != nil {
			fmt.Fprintf(logFile, "Verification skipped (disabled for debugging)\n")
		}
	} else {
		err = performBackupVerification(config.SourcePath, config.DestinationPath, config.ExcludePatterns, logFile)
		if err != nil {
			if logFile != nil {
				fmt.Fprintf(logFile, "ERROR during verification: %v\n", err)
			}
			return fmt.Errorf("verification phase failed: %v", err)
		}
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Pure Go backup completed successfully with verification\n")
	}
	return nil
}

// CheckTUIBackupProgress creates a Bubble Tea command that periodically checks operation progress.
// Handles cancellation detection, completion status, and real-time progress calculation.
// Returns ProgressUpdate messages with current status for the UI.
func CheckTUIBackupProgress() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		// Check if cancellation was requested - switch to cancelling state
		if shouldCancelBackup() && !tuiBackupCancelling && !tuiBackupCompleted {
			tuiBackupCancelling = true
			return ProgressUpdate{Percentage: -1, Message: "Cancelling backup...", Done: false}
		}

		// If we're in cancelling state, keep showing that message until actually complete
		if tuiBackupCancelling && !tuiBackupCompleted {
			return ProgressUpdate{Percentage: -1, Message: "Cancelling backup...", Done: false}
		}

		if tuiBackupCompleted {
			if tuiBackupError != nil {
				return ProgressUpdate{Error: tuiBackupError, Done: true}
			}

			// Use context-appropriate success message
			var successMessage string
			if isStandaloneVerification {
				successMessage = "Verification completed successfully!"
			} else {
				successMessage = "Backup completed successfully!"
			}

			return ProgressUpdate{Percentage: 1.0, Message: successMessage, Done: true}
		}

		// Only show regular progress if not cancelling
		if !tuiBackupCancelling {
			// Calculate real progress based on disk usage
			progress, message := calculateRealProgress()
			return ProgressUpdate{Percentage: progress, Message: message, Done: false}
		}

		// Default fallback (shouldn't reach here)
		return ProgressUpdate{Percentage: -1, Message: "Processing...", Done: false}
	})
}

// calculateRealProgress computes current operation progress using smart file-based tracking.
// Uses different algorithms depending on the current phase:
// - Directory scanning: 0-1% based on files discovered
// - File sync: 1-95% based on files processed vs. total found
// - Deletion: 95-99% based on cleanup progress
// - Verification: 95-100% (backup) or 0-100% (standalone)
// Returns progress (0.0-1.0) and a descriptive status message.
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

	if verificationPhaseActive {
		if isStandaloneVerification {
			// Standalone verification: Time-based progress to ensure smooth progression
			elapsed := time.Since(backupStartTime)

			// Target verification to take 8-12 seconds for good UX
			targetDuration := 10.0 // 10 seconds
			timeProgress := elapsed.Seconds() / targetDuration

			// Combine time progress with file progress
			var fileProgress float64
			if totalFilesVerified == 0 {
				fileProgress = 0.0
			} else if totalFilesVerified < 10 {
				fileProgress = 0.3 // Up to 30% for first 10 files (critical files)
			} else if totalFilesVerified < 100 {
				fileProgress = 0.3 + (float64(totalFilesVerified-10)/90.0)*0.6 // 30% to 90% for sampling
			} else {
				fileProgress = 0.9 // Cap file progress at 90%
			}

			// Use the minimum of time and file progress for realistic progression
			progress = timeProgress*0.7 + fileProgress*0.3 // Weighted toward time
			if progress > 0.99 {
				progress = 0.99 // Cap at 99% until complete
			}

			// Progressive messages based on stage
			if totalFilesVerified == 0 {
				message = "🔍 Initializing verification..."
			} else if totalFilesVerified < 10 {
				message = fmt.Sprintf("🔍 Checking critical files • %d verified", totalFilesVerified)
			} else if totalFilesVerified < 50 {
				message = fmt.Sprintf("🔍 Sampling backup files • %d verified", totalFilesVerified)
			} else {
				message = fmt.Sprintf("🔍 Completing verification • %d files verified", totalFilesVerified)
			}
		} else {
			// Backup verification phase: 95-100% range (part of backup process)
			if totalFilesVerified > 0 && len(copiedFilesList) > 0 {
				// Thread-safe access to copiedFilesList length
				copiedFilesListMutex.Lock()
				copiedFilesCount := len(copiedFilesList)
				copiedFilesListMutex.Unlock()

				// Progress based on files verified vs files that need verification
				estimatedVerificationFiles := int64(copiedFilesCount) + int64(float64(filesSkipped)*DefaultVerificationConfig.SampleRate) + int64(len(DefaultVerificationConfig.CriticalFiles))
				verificationProgress := float64(totalFilesVerified) / float64(estimatedVerificationFiles)
				if verificationProgress > 1.0 {
					verificationProgress = 1.0
				}
				progress = 0.95 + (verificationProgress * 0.05) // 95% to 100%
			} else {
				progress = 0.95 // Starting backup verification
			}

			message = fmt.Sprintf("Verifying backup integrity • %s files verified", FormatNumber(totalFilesVerified))
		}

	} else if deletionPhaseActive {
		// Deletion phase: 95-99% range (after sync completion)
		if totalFilesFound > 0 {
			baseProgress := 0.95
			deletionProgress := float64(filesDeleted) / float64(totalFilesFound/10) // Assume ~10% need deletion
			if deletionProgress > 1.0 {
				deletionProgress = 1.0
			}
			progress = baseProgress + (0.04 * deletionProgress) // 95% to 99%
		} else {
			progress = 0.97 // Default deletion progress
		}

		message = fmt.Sprintf("Deleting removed files (%s files cleaned up)", FormatNumber(filesDeleted))

	} else if syncPhaseComplete {
		// Sync complete, starting deletion
		progress = 0.95
		message = "Preparing deletion phase..."

	} else {
		// Use file-based progress throughout (no arbitrary time estimates)
		if totalFilesFound > 0 {
			filesProcessed := filesSkipped + filesCopied

			if directoryWalkComplete {
				// Directory walk done - file progress from 1% to 95%
				syncProgress := float64(filesProcessed) / float64(totalFilesFound)
				if syncProgress > 1.0 {
					syncProgress = 1.0
				}
				progress = 0.01 + (syncProgress * 0.94) // 1% to 95% range (94% span)

				// Show sync status
				if filesCopied > 0 {
					message = fmt.Sprintf("📁 Syncing files • %s copied, %s skipped • %s total",
						FormatNumber(filesCopied), FormatNumber(filesSkipped), FormatNumber(totalFilesFound))
				} else if filesSkipped > 1000 {
					message = fmt.Sprintf("⚡ Comparing files • %s identical • %s total processed",
						FormatNumber(filesSkipped), FormatNumber(totalFilesFound))
				} else {
					message = fmt.Sprintf("🔄 Processing files • %s of %s analyzed",
						FormatNumber(filesProcessed), FormatNumber(totalFilesFound))
				}

				// Add current directory info if available - FIXED WIDTH with truncation
				if currentDirectory != "" {
					// Clean up the path - remove /home/grendel prefix and show just the relative folder
					displayPath := currentDirectory
					displayPath = strings.TrimPrefix(displayPath, "/home/grendel/")
					if displayPath == "" {
						displayPath = "~"
					}
					// Truncate if too long
					if len(displayPath) > 57 {
						displayPath = displayPath[:57] + "..."
					}
					message = fmt.Sprintf("%s\n📁 %s", message, displayPath)
				}
			} else {
				// Directory walk still in progress - scanning only (0% to 1%)
				elapsed := time.Since(backupStartTime)

				// Scanning phase: 0% to 1% only
				if elapsed.Seconds() < 10 {
					progress = 0.001 + (float64(totalFilesFound)/500000)*0.009 // 0.1% to 1% based on files found
					if progress > 0.01 {
						progress = 0.01 // Cap at 1%
					}
				} else {
					// After 10 seconds of scanning, approach 1% gradually
					progress = 0.005 + (elapsed.Seconds()-10)/300*0.005 // 0.5% to 1% over 5 minutes
					if progress > 0.01 {
						progress = 0.01 // Still cap at 1%
					}
				}

				// Calculate discovery rate
				fileDiscoveryRate := 0.0
				if elapsed.Seconds() > 1.0 {
					fileDiscoveryRate = float64(totalFilesFound) / elapsed.Seconds()
				}

				if fileDiscoveryRate > 0 {
					message = fmt.Sprintf("🔍 Scanning filesystem • %s files found • %s files/sec",
						FormatNumber(totalFilesFound), FormatNumber(int64(fileDiscoveryRate)))
				} else {
					message = fmt.Sprintf("🔍 Scanning filesystem • %s files found", FormatNumber(totalFilesFound))
				}

				// Add current directory info if available - FIXED WIDTH with truncation
				if currentDirectory != "" {
					// Clean up the path - remove /home/grendel prefix and show just the relative folder
					displayPath := currentDirectory
					displayPath = strings.TrimPrefix(displayPath, "/home/grendel/")
					if displayPath == "" {
						displayPath = "~"
					}
					// Truncate if too long
					if len(displayPath) > 57 {
						displayPath = displayPath[:57] + "..."
					}
					message = fmt.Sprintf("%s\n📁 %s", message, displayPath)
				}
			}
		} else {
			// Very beginning - no files found yet (0%)
			progress = 0.0
			message = "Initializing filesystem scan..."
		}
	}

	// Never exceed 100%
	if progress > 1.0 {
		progress = 1.0
	}

	return progress, message
}

// startRestore creates a Bubble Tea command for restore operations.
// Automatically detects backup type (system/home) and determines the appropriate
// target path. Handles both full system restores and custom path restores.
func startRestore(sourcePath, targetPath string, restoreConfig, restoreWindowMgrs bool) tea.Cmd {
	return func() tea.Msg {
		// Setup logging in appropriate directory
		logPath := getLogFilePath()
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			fmt.Fprintf(logFile, "\n=== SMART RESTORE STARTED: %s ===\n", time.Now().Format(time.RFC3339))
			fmt.Fprintf(logFile, "Log file: %s\n", logPath)
			defer logFile.Close()
		}

		// Check if valid backup exists
		backupInfo := filepath.Join(sourcePath, "BACKUP-INFO.txt")
		if _, err := os.Stat(backupInfo); os.IsNotExist(err) {
			return ProgressUpdate{Error: fmt.Errorf("no valid backup found at %s", sourcePath)}
		}

		// CRITICAL: Detect backup type for safety
		backupType, err := detectBackupType(sourcePath)
		if err != nil {
			return ProgressUpdate{Error: fmt.Errorf("cannot determine backup type: %v", err)}
		}

		// SMART TARGETING: Auto-determine restore destination based on backup type
		var actualTargetPath string
		var operationDesc string

		switch backupType {
		case "system":
			if targetPath == "/" {
				// System backup → system restore (safe)
				actualTargetPath = "/"
				operationDesc = "SYSTEM RESTORE (Complete System)"
			} else {
				// System backup → custom path (dangerous but allowed with warning)
				actualTargetPath = targetPath
				operationDesc = fmt.Sprintf("CUSTOM RESTORE (System backup to %s)", targetPath)
				if logFile != nil {
					fmt.Fprintf(logFile, "WARNING: Restoring system backup to custom path: %s\n", targetPath)
				}
			}

		case "home":
			if targetPath == "/" {
				// Home backup → auto-target home directory
				username := getCurrentUser()
				actualTargetPath = "/home/" + username
				operationDesc = fmt.Sprintf("HOME RESTORE (Home backup to /home/%s)", username)
				if logFile != nil {
					fmt.Fprintf(logFile, "Auto-targeting home backup to /home/%s\n", username)
				}
			} else {
				// Home backup → custom path (user specified)
				actualTargetPath = targetPath
				operationDesc = fmt.Sprintf("CUSTOM RESTORE (Home backup to %s)", targetPath)
			}

		default:
			return ProgressUpdate{Error: fmt.Errorf("unknown backup type: %s", backupType)}
		}

		if logFile != nil {
			fmt.Fprintf(logFile, "Backup type detected: %s\n", backupType)
			fmt.Fprintf(logFile, "Restore target: %s\n", actualTargetPath)
			fmt.Fprintf(logFile, "Operation: %s\n", operationDesc)
			fmt.Fprintf(logFile, "Starting restore from %s to %s\n", sourcePath, actualTargetPath)
		}

		// Perform the actual restore with options
		err = performPureGoRestore(sourcePath, actualTargetPath, restoreConfig, restoreWindowMgrs, logFile)
		if err != nil {
			return ProgressUpdate{Error: fmt.Errorf("restore failed: %v", err)}
		}

		return ProgressUpdate{Percentage: 1.0, Message: fmt.Sprintf("%s completed successfully!", operationDesc), Done: true}
	}
}

// performPureGoRestore executes a two-phase restore process using pure Go.
// Phase 1: Copy all files from backup to target location
// Phase 2: Delete files that exist in target but not in backup (--delete behavior)
// Provides comprehensive logging and error handling.
func performPureGoRestore(backupPath, targetPath string, restoreConfig, restoreWindowMgrs bool, logFile *os.File) error {
	if logFile != nil {
		fmt.Fprintf(logFile, "Starting restore: %s -> %s\n", backupPath, targetPath)
		fmt.Fprintf(logFile, "Restore config: %v, Restore window managers: %v\n", restoreConfig, restoreWindowMgrs)
	}

	// Phase 1: Copy files from backup to target with selective restore
	err := syncDirectoriesWithOptions(backupPath, targetPath, restoreConfig, restoreWindowMgrs, logFile)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "Error during restore copy: %v\n", err)
		}
		return err
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

// detectBackupType analyzes a backup directory to determine its type.
// Checks BACKUP-INFO.txt first, then falls back to directory structure analysis.
// Returns "system", "home", or "unknown" with an error if type cannot be determined.
func detectBackupType(backupPath string) (string, error) {
	infoPath := filepath.Join(backupPath, "BACKUP-INFO.txt")
	content, err := os.ReadFile(infoPath)
	if err != nil {
		return "", fmt.Errorf("failed to read backup info: %v", err)
	}

	contentStr := string(content)
	if strings.Contains(contentStr, "Backup Type: Complete System") {
		return "system", nil
	} else if strings.Contains(contentStr, "Backup Type: Home Directory") {
		return "home", nil
	}

	// Fallback: try to detect from folder structure
	if _, err := os.Stat(filepath.Join(backupPath, "etc")); err == nil {
		// Has /etc directory - likely system backup
		return "system", nil
	} else if _, err := os.Stat(filepath.Join(backupPath, ".config")); err == nil {
		// Has .config directory - likely home backup
		return "home", nil
	}

	return "unknown", fmt.Errorf("cannot determine backup type from %s", backupPath)
}

// getCurrentUser returns the username, handling sudo context properly.
// When running under sudo, returns the original user (SUDO_USER).
// Falls back to USER environment variable, then "unknown".
func getCurrentUser() string {
	// If running under sudo, get the original user
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		return sudoUser
	}

	// Otherwise get current user
	if user := os.Getenv("USER"); user != "" {
		return user
	}

	// Fallback
	return "unknown"
}

// syncDirectoriesWithOptions copies files from source to destination with selective restore options.
// This is similar to syncDirectories but allows filtering based on restore preferences.
func syncDirectoriesWithOptions(sourcePath, destPath string, restoreConfig, restoreWindowMgrs bool, logFile *os.File) error {
	if logFile != nil {
		fmt.Fprintf(logFile, "Starting selective restore: config=%v, windowMgrs=%v\n", restoreConfig, restoreWindowMgrs)
	}

	// Build exclusion patterns based on user choices
	var excludePatterns []string

	// If user doesn't want to restore configuration, exclude .config directories
	if !restoreConfig {
		excludePatterns = append(excludePatterns,
			".config/*",
			"*/.config/*",
		)
		if logFile != nil {
			fmt.Fprintf(logFile, "Excluding configuration directories (.config)\n")
		}
	}

	// If user doesn't want to restore window managers, exclude WM-specific directories
	if !restoreWindowMgrs {
		excludePatterns = append(excludePatterns,
			".config/hypr/*",
			".config/Hyprland/*",
			".config/sway/*",
			".config/i3/*",
			".config/awesome/*",
			".config/dwm/*",
			".config/bspwm/*",
			".config/qtile/*",
			".config/xmonad/*",
			".local/share/gnome-shell/*",
			".local/share/plasma/*",
			".config/plasma*",
			".config/kde*",
			".config/gnome*",
			".local/share/applications/*",
			".config/autostart/*",
		)
		if logFile != nil {
			fmt.Fprintf(logFile, "Excluding window manager directories\n")
		}
	}

	// Use the filesystem package sync function with our exclusion patterns
	return syncDirectoriesWithExclusions(sourcePath, destPath, excludePatterns, logFile)
}

// createBackupInfo generates the BACKUP-INFO.txt file for a backup.
// Includes system information, timestamps, backup type, and restore instructions.
// This file is used for backup type detection and user reference.
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

// createBackupFolderList generates BACKUP-FOLDERS.txt for selective home backups.
// This file contains the list of included and excluded folders so verification
// knows which folders should and shouldn't exist in the backup.
func createBackupFolderList(mountPoint string, selectedFolders map[string]bool, logFile *os.File) error {
	var content strings.Builder

	content.WriteString("SELECTIVE HOME BACKUP FOLDER LIST\n")
	content.WriteString("=====================================\n")
	content.WriteString(fmt.Sprintf("Created: %s\n\n", time.Now().Format(time.RFC3339)))

	// List included folders
	content.WriteString("INCLUDED FOLDERS (backed up):\n")
	includedCount := 0
	for folderPath, isSelected := range selectedFolders {
		if isSelected {
			content.WriteString(fmt.Sprintf("  ✅ %s\n", folderPath))
			includedCount++
		}
	}

	// List excluded folders
	content.WriteString("\nEXCLUDED FOLDERS (not backed up):\n")
	excludedCount := 0
	for folderPath, isSelected := range selectedFolders {
		if !isSelected {
			content.WriteString(fmt.Sprintf("  ❌ %s\n", folderPath))
			excludedCount++
		}
	}

	content.WriteString(fmt.Sprintf("\nSUMMARY: %d folders included, %d folders excluded\n", includedCount, excludedCount))
	content.WriteString("\nNOTE: Verification will only check included folders.\n")
	content.WriteString("Excluded folders are intentionally missing from backup.\n")

	folderListPath := filepath.Join(mountPoint, "BACKUP-FOLDERS.txt")
	err := os.WriteFile(folderListPath, []byte(content.String()), 0644)

	if logFile != nil && err == nil {
		fmt.Fprintf(logFile, "Created backup folder list: %d included, %d excluded\n", includedCount, excludedCount)
	}

	return err
}

// loadBackupFolderList reads BACKUP-FOLDERS.txt from a selective home backup.
// Returns the list of included and excluded folders for verification purposes.
// Returns an error if the file doesn't exist (indicating a full backup, not selective).
func loadBackupFolderList(backupPath string, logFile *os.File) (*BackupFolderList, error) {
	folderListPath := filepath.Join(backupPath, "BACKUP-FOLDERS.txt")

	content, err := os.ReadFile(folderListPath)
	if err != nil {
		// File doesn't exist - this is a full backup, not selective
		return nil, fmt.Errorf("no backup folder list found (full backup)")
	}

	lines := strings.Split(string(content), "\n")

	var result BackupFolderList
	var inIncluded bool
	var inExcluded bool

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "INCLUDED FOLDERS") {
			inIncluded = true
			inExcluded = false
			continue
		}
		if strings.Contains(line, "EXCLUDED FOLDERS") {
			inIncluded = false
			inExcluded = true
			continue
		}
		if strings.Contains(line, "SUMMARY:") {
			break // End of folder lists
		}

		// Parse folder entries
		if inIncluded && strings.HasPrefix(line, "✅ ") {
			folder := strings.TrimPrefix(line, "✅ ")
			result.IncludedFolders = append(result.IncludedFolders, folder)
		} else if inExcluded && strings.HasPrefix(line, "❌ ") {
			folder := strings.TrimPrefix(line, "❌ ")
			result.ExcludedFolders = append(result.ExcludedFolders, folder)
		}
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Loaded folder list: %d included, %d excluded\n",
			len(result.IncludedFolders), len(result.ExcludedFolders))
	}

	return &result, nil
}

// createBackupConfig creates the appropriate BackupConfig for any backup operation type.
// This is the unified configuration builder that handles system, home, and selective backups.
//
// Parameters:
//   - operationType: "system_backup", "home_backup", or "selective_home_backup"
//   - mountPoint: Destination backup directory
//   - selectedFolders: For selective backups only (can be nil for others)
//   - homeFolders: For selective backups only (can be nil for others)
//
// Returns a properly configured BackupConfig struct for the specified operation type.
func createBackupConfig(operationType, mountPoint string, selectedFolders map[string]bool, homeFolders []HomeFolderInfo) (BackupConfig, error) {
	var config BackupConfig

	switch operationType {
	case "system_backup":
		config = BackupConfig{
			SourcePath:        "/",
			DestinationPath:   mountPoint,
			ExcludePatterns:   ExcludePatterns, // System exclusions only
			BackupType:        "Complete System",
			IsSelectiveBackup: false,
			SelectedFolders:   nil,
			HomeFolders:       nil,
		}

	case "home_backup":
		// Handle SUDO_USER properly - get the actual user's home directory
		var homeDir string
		if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
			homeDir = "/home/" + sudoUser
		} else {
			homeDir, _ = os.UserHomeDir()
		}

		config = BackupConfig{
			SourcePath:        homeDir,
			DestinationPath:   mountPoint,
			ExcludePatterns:   []string{".cache/*", ".local/share/Trash/*"}, // Home exclusions
			BackupType:        "Home Directory",
			IsSelectiveBackup: false,
			SelectedFolders:   nil,
			HomeFolders:       nil,
		}

	case "selective_home_backup":
		// Handle SUDO_USER properly - get the actual user's home directory
		var homeDir string
		if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
			homeDir = "/home/" + sudoUser
		} else {
			homeDir, _ = os.UserHomeDir()
		}

		config = BackupConfig{
			SourcePath:      homeDir,
			DestinationPath: mountPoint,
			ExcludePatterns: []string{
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
			},
			BackupType:        "Home Directory",
			IsSelectiveBackup: true,
			SelectedFolders:   selectedFolders,
			HomeFolders:       homeFolders,
		}

	default:
		return BackupConfig{}, fmt.Errorf("unknown backup operation type: %s", operationType)
	}

	return config, nil
}

// startUniversalBackup is the unified entry point for ALL backup operations.
// This single function handles system, home, and selective home backups through
// the same code path, ensuring consistent behavior and progress tracking.
//
// Parameters:
//   - operationType: "system_backup", "home_backup", or "selective_home_backup"
//   - mountPoint: Destination backup directory
//   - selectedFolders: For selective backups (pass nil for system/home backups)
//   - homeFolders: For selective backups (pass nil for system/home backups)
//
// Returns a Bubble Tea command that starts the backup operation with proper
// progress tracking, cancellation support, and error handling.
func startUniversalBackup(operationType, mountPoint string, selectedFolders map[string]bool, homeFolders []HomeFolderInfo) tea.Cmd {
	return func() tea.Msg {
		// Create the appropriate configuration for this backup type
		config, err := createBackupConfig(operationType, mountPoint, selectedFolders, homeFolders)
		if err != nil {
			return ProgressUpdate{Error: err, Done: true}
		}

		// Use the unified backup system
		cmd := startBackup(config)
		return cmd()
	}
}

// Start verification operation
func startVerification(operationType, mountPoint string) tea.Cmd {
	return func() tea.Msg {
		// Reset all backup state and set up for standalone verification
		resetBackupState()
		isStandaloneVerification = true

		// Start verification in background like backup operations
		go runVerificationSilently(operationType, mountPoint)
		return ProgressUpdate{Percentage: -1, Message: "Starting verification...", Done: false}
	}
}

// Run verification using the same pattern as backup operations
func runVerificationSilently(operationType, mountPoint string) {
	// Reset cancellation flag at start
	resetBackupCancel()

	// Setup logging in appropriate directory
	logPath := getLogFilePath()
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		fmt.Fprintf(logFile, "\n=== VERIFICATION STARTED: %s ===\n", time.Now().Format(time.RFC3339))
		fmt.Fprintf(logFile, "Operation: %s\n", operationType)
		fmt.Fprintf(logFile, "Backup Source: %s\n", mountPoint)
		defer logFile.Close()
	}

	tuiBackupCompleted = false
	tuiBackupError = nil
	tuiBackupCancelling = false

	// Initialize progress tracking like backup operations
	backupStartTime = time.Now()

	// Check if valid backup exists
	backupInfo := filepath.Join(mountPoint, "BACKUP-INFO.txt")
	if _, err := os.Stat(backupInfo); os.IsNotExist(err) {
		tuiBackupCompleted = true
		tuiBackupError = fmt.Errorf("no valid backup found at %s", mountPoint)
		return
	}

	// Detect backup type for source path determination
	backupType, err := detectBackupType(mountPoint)
	if err != nil {
		tuiBackupCompleted = true
		tuiBackupError = fmt.Errorf("cannot determine backup type: %v", err)
		return
	}

	// Determine source path based on operation and backup type
	var sourcePath string

	switch operationType {
	case "system_verify":
		if backupType != "system" {
			tuiBackupCompleted = true
			tuiBackupError = fmt.Errorf("selected system verification but backup is %s type", backupType)
			return
		}
		sourcePath = "/"

	case "home_verify":
		if backupType != "home" {
			tuiBackupCompleted = true
			tuiBackupError = fmt.Errorf("selected home verification but backup is %s type", backupType)
			return
		}
		// Get the actual user's home directory
		username := getCurrentUser()
		sourcePath = "/home/" + username

	case "auto_verify":
		// Auto-detection: Set source path based on detected backup type
		if logFile != nil {
			fmt.Fprintf(logFile, "Auto-detected backup type: %s\n", backupType)
		}

		if backupType == "system" {
			sourcePath = "/"
			if logFile != nil {
				fmt.Fprintf(logFile, "Auto-verify: Verifying complete system backup\n")
			}
		} else if backupType == "home" {
			// Get the actual user's home directory
			username := getCurrentUser()
			sourcePath = "/home/" + username
			if logFile != nil {
				fmt.Fprintf(logFile, "Auto-verify: Verifying home directory backup for user: %s\n", username)
			}
		} else {
			tuiBackupCompleted = true
			tuiBackupError = fmt.Errorf("cannot auto-verify: unknown backup type '%s'", backupType)
			return
		}

	default:
		tuiBackupCompleted = true
		tuiBackupError = fmt.Errorf("unknown verification type: %s", operationType)
		return
	}

	// For home verification, check if this is a selective backup
	var selectiveExclusions []string
	if backupType == "home" {
		// Try to load selective backup folder list
		folderList, err := loadBackupFolderList(mountPoint, logFile)
		if err == nil && len(folderList.ExcludedFolders) > 0 {
			// This is a selective backup - add excluded folders to verification exclusions
			selectiveExclusions = folderList.ExcludedFolders
			if logFile != nil {
				fmt.Fprintf(logFile, "Selective backup detected: excluding %d folders from verification\n", len(selectiveExclusions))
				for _, folder := range selectiveExclusions {
					fmt.Fprintf(logFile, "  Excluding from verification: %s\n", folder)
				}
			}
		}
	}

	// Determine exclusion patterns based on backup type
	var excludePatterns []string
	switch backupType {
	case "system":
		// System verification: Use system exclusions PLUS cache exclusions
		// Cache files change constantly and should never be verified
		excludePatterns = append(ExcludePatterns,
			"/home/*/.cache/*", // User cache directories
			"/root/.cache/*",   // Root cache directory
			"/.cache/*",        // Any other cache directories
			"/var/cache/*",     // System package cache
			"/tmp/*",           // Already in ExcludePatterns but being explicit
		)
	case "home":
		excludePatterns = []string{".cache/*", ".local/share/Trash/*"} // Use home exclusions
		// Add selective backup exclusions if any
		excludePatterns = append(excludePatterns, selectiveExclusions...)
	default:
		excludePatterns = []string{} // No exclusions for unknown types
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Backup type detected: %s\n", backupType)
		fmt.Fprintf(logFile, "Source path: %s\n", sourcePath)
		fmt.Fprintf(logFile, "Backup path: %s\n", mountPoint)
		fmt.Fprintf(logFile, "Exclusion patterns: %v\n", excludePatterns)
		fmt.Fprintf(logFile, "Starting verification...\n")
	}

	// Perform the actual verification
	err = performStandaloneVerification(sourcePath, mountPoint, excludePatterns, logFile)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "VERIFICATION ERROR: %v\n", err)
		}
		tuiBackupCompleted = true
		if shouldCancelBackup() {
			tuiBackupCancelling = false // Cancellation complete
			tuiBackupError = fmt.Errorf("verification canceled by user")
		} else {
			tuiBackupError = fmt.Errorf("verification failed: %v", err)
		}
	} else {
		if logFile != nil {
			fmt.Fprintf(logFile, "VERIFICATION SUCCESS: completed\n")
		}
		tuiBackupCompleted = true
		tuiBackupError = nil
	}
}

// hasSubfolders checks if a given folder path has subfolders in the HomeFolders metadata.
// This is used to distinguish between parent folders and actual subfolders for smart inclusion logic.
func hasSubfolders(folderPath string, homeFolders []HomeFolderInfo) bool {
	for _, folder := range homeFolders {
		if folder.Path == folderPath && folder.HasSubfolders {
			return true
		}
	}
	return false
}
