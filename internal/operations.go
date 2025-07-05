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
	// NEW: Selective backup support
	IsSelectiveBackup bool
	SelectedFolders   map[string]bool  // folder -> selected state
	HomeFolders       []HomeFolderInfo // folder information
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

// Pure Go backup implementation (no external dependencies)
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

	// SELECTIVE BACKUP: Handle folder-specific backup
	if config.IsSelectiveBackup {
		if logFile != nil {
			fmt.Fprintf(logFile, "=== SELECTIVE BACKUP MODE ===\n")
			fmt.Fprintf(logFile, "Selected folders: %+v\n", config.SelectedFolders)
			fmt.Fprintf(logFile, "DEBUG: Folder selection details:\n")
			for folderPath, isSelected := range config.SelectedFolders {
				fmt.Fprintf(logFile, "  Folder: '%s' -> Selected: %t\n", folderPath, isSelected)
			}
		}
		
		// SIMPLE APPROACH: Do full backup but with exclusions for unselected folders
		// Build exclusion patterns for unselected folders
		enhancedExcludes := make([]string, len(config.ExcludePatterns))
		copy(enhancedExcludes, config.ExcludePatterns)
		
		// Add unselected folders to exclusion patterns
		for folderPath, isSelected := range config.SelectedFolders {
			if !isSelected {
				// folderPath is already absolute path like "/home/grendel/Videos" 
				// Don't join with homeDir again - use it directly
				enhancedExcludes = append(enhancedExcludes, folderPath)
				if logFile != nil {
					fmt.Fprintf(logFile, "EXCLUDING unselected folder: %s\n", folderPath)
				}
			} else {
				if logFile != nil {
					fmt.Fprintf(logFile, "INCLUDING selected folder: %s\n", folderPath)
				}
			}
		}
		
		// Update config with enhanced exclusions
		config.ExcludePatterns = enhancedExcludes
		if logFile != nil {
			fmt.Fprintf(logFile, "Enhanced exclusion patterns: %v\n", enhancedExcludes)
		}
		
		// Now run NORMAL backup with enhanced exclusions - SAME AS FULL BACKUP
	}

	// REGULAR BACKUP: Sync entire source directory
	err = syncDirectoriesWithExclusions(config.SourcePath, config.DestinationPath, config.ExcludePatterns, logFile)
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
		err = performBackupVerification(config.SourcePath, config.DestinationPath, logFile)
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
				// Directory walk done - file progress from 1% to 95%
				syncProgress := float64(filesProcessed) / float64(totalFilesFound)
				if syncProgress > 1.0 {
					syncProgress = 1.0
				}
				progress = 0.01 + (syncProgress * 0.94) // 1% to 95% range (94% span)
				
				// Show sync status
				if filesCopied > 0 {
					message = fmt.Sprintf("📁 Syncing files • %s copied, %s skipped • %s total",
						formatNumber(filesCopied), formatNumber(filesSkipped), formatNumber(totalFilesFound))
				} else if filesSkipped > 1000 {
					message = fmt.Sprintf("⚡ Comparing files • %s identical • %s total processed",
						formatNumber(filesSkipped), formatNumber(totalFilesFound))
				} else {
					message = fmt.Sprintf("🔄 Processing files • %s of %s analyzed",
						formatNumber(filesProcessed), formatNumber(totalFilesFound))
				}
				
				// Add current directory info if available - FIXED WIDTH with truncation
				if currentDirectory != "" {
					// Clean up the path - remove /home/grendel prefix and show just the relative folder
					displayPath := currentDirectory
					if strings.HasPrefix(displayPath, "/home/grendel/") {
						displayPath = strings.TrimPrefix(displayPath, "/home/grendel/")
					}
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
					progress = 0.001 + (float64(totalFilesFound) / 500000) * 0.009 // 0.1% to 1% based on files found
					if progress > 0.01 {
						progress = 0.01 // Cap at 1%
					}
				} else {
					// After 10 seconds of scanning, approach 1% gradually
					progress = 0.005 + (elapsed.Seconds() - 10) / 300 * 0.005 // 0.5% to 1% over 5 minutes
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
						formatNumber(totalFilesFound), formatNumber(int64(fileDiscoveryRate)))
				} else {
					message = fmt.Sprintf("🔍 Scanning filesystem • %s files found", formatNumber(totalFilesFound))
				}
				
				// Add current directory info if available - FIXED WIDTH with truncation
				if currentDirectory != "" {
					// Clean up the path - remove /home/grendel prefix and show just the relative folder
					displayPath := currentDirectory
					if strings.HasPrefix(displayPath, "/home/grendel/") {
						displayPath = strings.TrimPrefix(displayPath, "/home/grendel/")
					}
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

// Start smart restore operation - detects backup type and auto-targets appropriately
func startRestore(sourcePath, targetPath string) tea.Cmd {
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

		// Perform the actual restore
		err = performPureGoRestore(sourcePath, actualTargetPath, logFile)
		if err != nil {
			return ProgressUpdate{Error: err, Done: true}
		}

		return ProgressUpdate{Percentage: 1.0, Message: fmt.Sprintf("%s completed successfully!", operationDesc), Done: true}
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

// Detect backup type from BACKUP-INFO.txt file
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

// Get current username (handles sudo properly)
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
				IsSelectiveBackup: false,
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
				SourcePath:      homeDir,
				DestinationPath: mountPoint,
				ExcludePatterns: []string{".cache/*", ".local/share/Trash/*"},
				BackupType:      "Home Directory",
				IsSelectiveBackup: false, // Regular home backup
			}
		default:
			return ProgressUpdate{Error: fmt.Errorf("unknown backup type: %s", operationType)}
		}

		// Start the backup
		cmd := startBackup(config)
		return cmd()
	}
}

// Start selective home backup with user-selected folders
func startSelectiveHomeBackup(mountPoint string, homeFolders []HomeFolderInfo, selectedFolders map[string]bool) tea.Cmd {
	return func() tea.Msg {
		// Handle SUDO_USER properly - get the actual user's home directory
		var homeDir string
		if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
			homeDir = "/home/" + sudoUser
		} else {
			homeDir, _ = os.UserHomeDir()
		}
		
		config := BackupConfig{
			SourcePath:        homeDir,
			DestinationPath:   mountPoint,
			ExcludePatterns:   []string{
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
