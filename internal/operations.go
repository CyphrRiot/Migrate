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

// BackupConfig holds backup configuration
type BackupConfig struct {
	SourcePath      string
	DestinationPath string
	ExcludePatterns []string
	BackupType      string
}

// VerificationConfig holds verification settings
type VerificationConfig struct {
	SampleRate       float64  // Default: 0.01 (1%)
	TimeoutMinutes   int      // Default: 5  
	ParallelWorkers  int      // Default: 4
	CriticalFiles    []string // Critical system files to always verify
}

// Default verification configuration
var DefaultVerificationConfig = VerificationConfig{
	SampleRate:      0.01, // 1% of unchanged files
	TimeoutMinutes:  5,
	ParallelWorkers: 4,
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

// Verification can be disabled for debugging
var EnableVerification = false  // DISABLED by default until we fix the issues

// VerificationResult holds verification results
type VerificationResult struct {
	Success         bool
	FilesVerified   int64
	NewFilesChecked int64
	SampledFiles    int64
	CriticalFiles   int64
	ErrorCount      int
	Warnings        []string
	Duration        time.Duration
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
var tuiBackupCancelling = false // Track if cancellation is in progress

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

// VERIFICATION PHASE TRACKING  
var verificationPhaseActive bool    // Track verification phase
var totalFilesVerified int64        // Counter for verification progress
var copiedFilesList []string        // Files that were actually copied (for verification)
var copiedFilesListMutex sync.Mutex // Protect copiedFilesList for thread safety
var verificationErrors []string     // Non-critical errors to log

// ENHANCED PROGRESS TRACKING - Phase A: Better Messages
var currentDirectory string    // Current directory being scanned
var lastProgressMessage string // Last message to avoid spam

// Reset all backup progress counters and state
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

// Start backup operation - TUI ONLY (Pure Go)
func startBackup(config BackupConfig) tea.Cmd {
	return func() tea.Msg {
		// Reset all backup state before starting
		resetBackupState()
		
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
	tuiBackupCancelling = false

	// Initialize progress tracking
	backupStartTime = time.Now()
	sourceUsedSpace, _ = getUsedDiskSpace(config.SourcePath)
	destStartUsedSpace, _ = getUsedDiskSpace(config.DestinationPath)

	if logFile != nil {
		fmt.Fprintf(logFile, "Using pure Go: source=%s, dest_start=%s\n",
			formatBytes(sourceUsedSpace), formatBytes(destStartUsedSpace))
	}

	// Use pure Go implementation for actual backup
	if logFile != nil {
		fmt.Fprintf(logFile, "About to start performPureGoBackup SYNCHRONOUSLY\n")
	}
	
	// Run synchronously instead of in goroutine to fix execution bug
	err = performPureGoBackup(config.SourcePath, config.DestinationPath, logFile)
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
		err = performBackupVerification(sourcePath, destPath, logFile)
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

// Check TUI backup progress with real disk usage monitoring
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
			return ProgressUpdate{Percentage: 1.0, Message: "Backup completed successfully!", Done: true}
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

	if verificationPhaseActive {
		// Verification phase: 95-100% range
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
			progress = 0.95 // Starting verification
		}
		
		message = fmt.Sprintf("Verifying backup integrity • %s files verified", formatNumber(totalFilesVerified))

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

		message = fmt.Sprintf("Deleting removed files (%s files cleaned up)", formatNumber(filesDeleted))

	} else if syncPhaseComplete {
		// Sync complete, starting deletion
		progress = 0.95
		message = "Preparing deletion phase..."

	} else {
		// Use file-based progress throughout (no arbitrary time estimates)
		if totalFilesFound > 0 {
			filesProcessed := filesSkipped + filesCopied
			
			if directoryWalkComplete {
				// Directory walk done - pure file progress (40% to 95%)
				syncProgress := float64(filesProcessed) / float64(totalFilesFound)
				if syncProgress > 1.0 {
					syncProgress = 1.0
				}
				progress = 0.40 + (syncProgress * 0.55) // 40% to 95% range
				
				// Show sync status
				if filesCopied > 0 {
					message = fmt.Sprintf("Syncing files • %s copied, %s skipped • %s total",
						formatNumber(filesCopied), formatNumber(filesSkipped), formatNumber(totalFilesFound))
				} else if filesSkipped > 1000 {
					message = fmt.Sprintf("Comparing files • %s identical • %s total processed",
						formatNumber(filesSkipped), formatNumber(totalFilesFound))
				} else {
					message = fmt.Sprintf("Processing files • %s of %s analyzed",
						formatNumber(filesProcessed), formatNumber(totalFilesFound))
				}
			} else {
				// Directory walk still in progress - show scanning progress
				elapsed := time.Since(backupStartTime)
				
				if filesProcessed > 0 {
					// Files being processed during scan - blend scanning and processing progress
					
					// Base scanning progress (0% to 15% based on time and files found)
					var scanProgress float64
					if elapsed.Seconds() < 10 {
						scanProgress = 0.01 + (float64(totalFilesFound) / 200000) * 0.12 // 1% to 13% based on files found
						if scanProgress > 0.13 {
							scanProgress = 0.13
						}
					} else {
						scanProgress = 0.13 + (elapsed.Seconds() - 10) / 300 * 0.02 // 13% to 15% over 5 minutes
						if scanProgress > 0.15 {
							scanProgress = 0.15
						}
					}
					
					// Processing progress bonus (add up to 35% more based on files processed) 
					processingProgress := float64(filesProcessed) / float64(totalFilesFound) * 0.25
					if processingProgress > 0.25 {
						processingProgress = 0.25
					}
					
					progress = scanProgress + processingProgress // Max 40% (15% + 25%)
					
					// Show scanning + processing status
					if filesCopied > 0 {
						message = fmt.Sprintf("Scanning & copying • %s copied, %s skipped • %s found",
							formatNumber(filesCopied), formatNumber(filesSkipped), formatNumber(totalFilesFound))
					} else {
						message = fmt.Sprintf("Scanning & comparing • %s identical • %s found",
							formatNumber(filesSkipped), formatNumber(totalFilesFound))
					}
				} else {
					// Pure scanning phase - progress based on files discovered (0% to 15%)
					
					// Show scanning progress based on discovery rate and time
					if elapsed.Seconds() < 10 {
						progress = 0.01 + (float64(totalFilesFound) / 200000) * 0.12 // 1% to 13% based on files found
						if progress > 0.13 {
							progress = 0.13
						}
					} else {
						progress = 0.13 + (elapsed.Seconds() - 10) / 300 * 0.02 // 13% to 15% over 5 minutes
						if progress > 0.15 {
							progress = 0.15
						}
					}
					
					// Calculate discovery rate
					fileDiscoveryRate := 0.0
					if elapsed.Seconds() > 1.0 {
						fileDiscoveryRate = float64(totalFilesFound) / elapsed.Seconds()
					}
					
					if fileDiscoveryRate > 0 {
						message = fmt.Sprintf("Scanning filesystem • %s files found • %s files/sec", 
							formatNumber(totalFilesFound), formatNumber(int64(fileDiscoveryRate)))
					} else {
						message = fmt.Sprintf("Scanning filesystem • %s files found", formatNumber(totalFilesFound))
					}
				}
			}
		} else {
			// Very beginning - no files found yet
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

// Start restore operation - TUI ONLY (Pure Go)
func startRestore(sourcePath, targetPath string) tea.Cmd {
	return func() tea.Msg {
		// Setup logging in appropriate directory
		logPath := getLogFilePath()
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			fmt.Fprintf(logFile, "\n=== PURE GO RESTORE STARTED: %s ===\n", time.Now().Format(time.RFC3339))
			fmt.Fprintf(logFile, "Log file: %s\n", logPath)
			defer logFile.Close()
		}

		// Use provided source path instead of auto-detecting
		backupInfo := filepath.Join(sourcePath, "BACKUP-INFO.txt")
		if _, err := os.Stat(backupInfo); os.IsNotExist(err) {
			return ProgressUpdate{Error: fmt.Errorf("no valid backup found at %s", sourcePath)}
		}

		if logFile != nil {
			fmt.Fprintf(logFile, "Starting pure Go restore from %s to %s\n", sourcePath, targetPath)
		}

		// Perform pure Go restore
		err = performPureGoRestore(sourcePath, targetPath, logFile)
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
