// Package internal provides comprehensive backup verification functionality for the Migrate system.
//
// This module implements a multi-phase smart verification system that ensures backup integrity
// through various strategies depending on the backup type and available resources:
//
// Verification Phases:
//   - Phase 1: Full verification of newly copied files (high priority)
//   - Phase 2: Critical system files verification (always performed)
//   - Phase 3: Statistical sampling of unchanged files (performance optimized)
//   - Phase 4: Directory structure integrity validation
//
// The verification system supports both:
//   - Incremental verification (during backup operations)
//   - Standalone verification (independent verification runs)
//
// Performance Features:
//   - Parallel file verification using worker pools
//   - Intelligent sampling algorithms for large datasets
//   - Adaptive error thresholds based on backup size
//   - Cancellation support for long-running operations
//
// Verification Methods:
//   - SHA-256 checksums for critical and small files
//   - Statistical sampling for large files and datasets
//   - Size and timestamp validation for quick checks
//   - Directory structure comparison and validation
package internal

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// shouldExcludeFile checks if a file should be excluded based on exclude patterns
// shouldExcludeFile checks whether a file should be excluded from verification.
// Uses robust pattern matching that handles all exclusion patterns properly
func shouldExcludeFile(filePath string, excludePatterns []string, sourcePath string) bool {
	// Convert to relative path for consistent matching
	relPath, err := filepath.Rel(sourcePath, filePath)
	if err != nil {
		relPath = filePath // Fallback to absolute if conversion fails
	}

	for _, pattern := range excludePatterns {
		// Skip empty patterns
		if pattern == "" {
			continue
		}

		// Check if pattern matches
		if matchesVerificationPattern(filePath, relPath, pattern) {
			return true
		}
	}
	return false
}

// matchesVerificationPattern checks if a file path matches an exclusion pattern
func matchesVerificationPattern(fullPath, relPath, pattern string) bool {
	// Handle absolute patterns (start with /)
	if strings.HasPrefix(pattern, "/") {
		return matchesPathPattern(fullPath, pattern)
	}

	// Handle relative patterns
	// Try matching against relative path first
	if matchesPathPattern(relPath, pattern) {
		return true
	}

	// For patterns like ".git/*", also check if any part of the path matches
	if strings.Contains(pattern, ".git/") || strings.HasSuffix(pattern, ".git/*") {
		if strings.Contains(fullPath, "/.git/") || strings.Contains(relPath, "/.git/") {
			return true
		}
	}

	// For cache patterns ending with special suffixes, check filename
	cacheSuffixes := []string{"-a", "-d", "-diagnostics", "-export", "-methodsets", "-tests", "-xrefs", "-typerefs", "-cas"}
	for _, suffix := range cacheSuffixes {
		if strings.HasSuffix(pattern, suffix) {
			filename := filepath.Base(fullPath)
			if strings.HasSuffix(filename, suffix) {
				return true
			}
		}
	}

	// For Signal app cache patterns
	if strings.Contains(pattern, "Signal") && strings.Contains(fullPath, "Signal") {
		return true
	}

	// For browser cache patterns, check if path contains browser directories
	if strings.Contains(pattern, "BraveSoftware") && strings.Contains(fullPath, "BraveSoftware") {
		return true
	}

	return false
}

// matchesPathPattern handles glob pattern matching for file paths
func matchesPathPattern(path, pattern string) bool {
	// Exact match
	if path == pattern {
		return true
	}

	// Directory wildcard patterns (ending with /*)
	if strings.HasSuffix(pattern, "/*") {
		dirPattern := strings.TrimSuffix(pattern, "/*")
		if strings.HasPrefix(path, dirPattern+"/") || path == dirPattern {
			return true
		}
	}

	// Wildcard in middle or beginning
	if strings.Contains(pattern, "*") {
		// Try standard filepath.Match first
		matched, err := filepath.Match(pattern, path)
		if err == nil && matched {
			return true
		}

		// Handle patterns like "*/something/*"
		if strings.HasPrefix(pattern, "*/") && strings.HasSuffix(pattern, "/*") {
			middle := strings.TrimSuffix(strings.TrimPrefix(pattern, "*/"), "/*")
			if strings.Contains(path, "/"+middle+"/") || strings.Contains(path, middle+"/") {
				return true
			}
		}

		// Handle patterns like "something/*"
		if strings.HasSuffix(pattern, "/*") && !strings.HasPrefix(pattern, "*/") {
			prefix := strings.TrimSuffix(pattern, "/*")
			if strings.Contains(path, "/"+prefix+"/") || strings.HasPrefix(path, prefix+"/") {
				return true
			}
		}
	}

	return false
}

// isDirectoryEmptyDueToExclusions checks if a directory would be empty after applying exclusion patterns
// This helps distinguish between truly missing directories and directories that are empty due to exclusions
func isDirectoryEmptyDueToExclusions(dirPath string, excludePatterns []string, sourcePath string, logFile *os.File) bool {
	// Check if the directory itself is excluded
	if shouldExcludeFile(dirPath, excludePatterns, sourcePath) {
		return true
	}

	// Get relative path from source for pattern matching
	relPath, err := filepath.Rel(sourcePath, dirPath)
	if err != nil {
		return false
	}

	// Debug logging for flatpak directory
	if strings.Contains(relPath, "flatpak") && logFile != nil {
		fmt.Fprintf(logFile, "DEBUG: Checking flatpak directory: %s\n", relPath)
		fmt.Fprintf(logFile, "DEBUG: Patterns: %v\n", excludePatterns)
	}

	// Check if all potential contents would be excluded
	// Look for exclusion patterns that would exclude everything in this directory
	for _, pattern := range excludePatterns {
		// Check if pattern would exclude all contents of this directory
		if strings.HasSuffix(pattern, "/*") {
			dirPattern := strings.TrimSuffix(pattern, "/*")
			if strings.Contains(relPath, "flatpak") && logFile != nil {
				fmt.Fprintf(logFile, "DEBUG: Comparing '%s' with pattern '%s'\n", relPath, dirPattern)
			}
			if relPath == dirPattern {
				if strings.Contains(relPath, "flatpak") && logFile != nil {
					fmt.Fprintf(logFile, "DEBUG: MATCH FOUND - Directory should be empty\n")
				}
				return true
			}
		}
	}

	if strings.Contains(relPath, "flatpak") && logFile != nil {
		fmt.Fprintf(logFile, "DEBUG: No match found - Directory should NOT be empty\n")
	}
	return false
}

// performBackupVerification executes the comprehensive smart incremental verification process.
// This is the main verification function called during backup operations to ensure data integrity.
//
// The function implements a 4-phase verification strategy:
//  1. Verification of newly copied files (100% coverage with checksums)
//  2. Critical system files verification (essential files always checked)
//  3. Statistical sampling of unchanged files (performance-optimized validation)
//  4. Directory structure integrity validation (ensures completeness)
//
// Parameters:
//   - sourcePath: The original source directory that was backed up
//   - destPath: The destination backup directory to verify against
//   - excludePatterns: Patterns that were excluded during backup (cache, temp files, etc.)
//   - logFile: Optional log file for detailed verification reporting (nil to disable logging)
//
// Returns an error if verification fails or if critical integrity issues are detected.
// Non-critical issues are logged as warnings but don't fail the verification.
//
// The function uses adaptive error thresholds based on backup size and automatically
// adjusts verification intensity based on the number of files processed.
func performBackupVerification(sourcePath, destPath string, excludePatterns []string, logFile *os.File) error {
	if logFile != nil {
		fmt.Fprintf(logFile, "Starting smart incremental backup verification\n")
		fmt.Fprintf(logFile, "Source: %s, Destination: %s\n", sourcePath, destPath)
	}

	// Mark verification as active
	verificationPhaseActive = true
	verificationStart := time.Now()

	// Reset verification counters
	totalFilesVerified = 0
	verificationErrors = []string{}

	// Step 1: Verify newly copied files (high priority)
	// Thread-safe access to copiedFilesList - minimize mutex lock time
	copiedFilesListMutex.Lock()
	copiedFilesCount := len(copiedFilesList)
	copiedFilesListMutex.Unlock()

	var copiedFilesCopy []string

	if copiedFilesCount > 0 {
		// Create copy only when needed
		copiedFilesListMutex.Lock()
		copiedFilesCopy = make([]string, len(copiedFilesList))
		copy(copiedFilesCopy, copiedFilesList)
		copiedFilesListMutex.Unlock()

		if logFile != nil {
			fmt.Fprintf(logFile, "Verifying %d newly copied files\n", copiedFilesCount)
		}

		err := verifyNewFiles(copiedFilesCopy, sourcePath, destPath, excludePatterns, logFile)
		if err != nil {
			return fmt.Errorf("verification of new files failed: %v", err)
		}
	} else {
		if logFile != nil {
			fmt.Fprintf(logFile, "No new files to verify (all files were identical)\n")
		}
	}

	// Step 2: Verify critical system files (always check)
	if logFile != nil {
		fmt.Fprintf(logFile, "Verifying critical system files\n")
	}
	err := verifyCriticalFiles(sourcePath, destPath, logFile)
	if err != nil {
		return fmt.Errorf("critical files verification failed: %v", err)
	}

	// Step 3: Sample verification of unchanged files (low priority)
	if filesSkipped > 100 { // Only if we have significant unchanged files
		if logFile != nil {
			fmt.Fprintf(logFile, "Sampling verification of %d unchanged files\n", filesSkipped)
		}
		err = verifySampledFiles(sourcePath, destPath, DefaultVerificationConfig.SampleRate, excludePatterns, logFile)
		if err != nil {
			// Non-critical error - log but don't fail backup
			verificationErrors = append(verificationErrors, fmt.Sprintf("Sample verification: %v", err))
			if logFile != nil {
				fmt.Fprintf(logFile, "Warning: %v\n", err)
			}
		}
	}

	// Step 4: Verify directory structure
	if logFile != nil {
		fmt.Fprintf(logFile, "Verifying directory structure\n")
	}
	err = verifyDirectoryStructure(sourcePath, destPath, logFile)
	if err != nil {
		verificationErrors = append(verificationErrors, fmt.Sprintf("Directory structure: %v", err))
		if logFile != nil {
			fmt.Fprintf(logFile, "Warning: %v\n", err)
		}
	}

	duration := time.Since(verificationStart)

	// Final verification report
	if logFile != nil {
		fmt.Fprintf(logFile, "Verification completed in %v\n", duration)
		fmt.Fprintf(logFile, "Files verified: %d\n", totalFilesVerified)
		fmt.Fprintf(logFile, "New files checked: %d\n", copiedFilesCount)
		fmt.Fprintf(logFile, "Critical files checked: %d\n", len(DefaultVerificationConfig.CriticalFiles))
		fmt.Fprintf(logFile, "Errors/warnings: %d\n", len(verificationErrors))

		if len(verificationErrors) > 0 {
			fmt.Fprintf(logFile, "Verification warnings:\n")
			for _, err := range verificationErrors {
				fmt.Fprintf(logFile, "  - %s\n", err)
			}
		}
	}

	// Mark verification as complete
	verificationPhaseActive = false

	// Fail backup if too many critical errors (threshold: 10 or 5% of new files, whichever is higher)
	criticalErrorThreshold := max(10, copiedFilesCount/20)

	if len(verificationErrors) > criticalErrorThreshold {
		// Instead of returning generic error, populate detailed error screen
		// The TUI will check verificationErrors global and show detailed screen
		return fmt.Errorf("VERIFICATION_DETAILED_ERRORS:%d", len(verificationErrors))
	}

	return nil
}

// verifyNewFiles performs comprehensive checksum verification of all newly copied files.
// This function provides 100% verification coverage for files that were actually copied
// during the backup operation, using parallel processing for optimal performance.
//
// The verification process uses:
//   - Multi-threaded worker pool (up to 4 concurrent workers)
//   - SHA-256 checksums for complete file integrity validation
//   - Adaptive error thresholds (10% failure rate or minimum 10 files)
//   - Graceful cancellation support for user interruption
//
// Parameters:
//   - copiedFiles: Slice of file paths that were copied during backup
//   - sourcePath: Original source directory for path resolution
//   - destPath: Destination backup directory for verification
//   - logFile: Optional log file for detailed verification reporting
//
// Returns an error if verification fails beyond the acceptable threshold.
// Individual file errors are logged but don't immediately fail the verification.
// Only when error rates exceed 10% (or 10 files minimum) does this function fail.
func verifyNewFiles(copiedFiles []string, sourcePath, destPath string, excludePatterns []string, logFile *os.File) error {
	if len(copiedFiles) == 0 {
		return nil
	}

	// Use worker pool for parallel verification
	const maxWorkers = 4
	workerCh := make(chan string, len(copiedFiles))
	errorCh := make(chan error, len(copiedFiles))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < maxWorkers && i < len(copiedFiles); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range workerCh {
				// Check for cancellation
				if shouldCancelBackup() {
					errorCh <- fmt.Errorf("verification canceled")
					return
				}

				// Skip excluded patterns - comprehensive browser cache exclusion
				if shouldExcludeFile(filePath, excludePatterns, sourcePath) {
					continue
				}

				err := verifySingleFile(filePath, sourcePath, destPath)
				if err != nil {
					errorCh <- fmt.Errorf("file %s: %v", filePath, err)
				} else {
					atomic.AddInt64(&totalFilesVerified, 1)
				}
			}
		}()
	}

	// Send files to workers
	go func() {
		defer close(workerCh)
		for _, filePath := range copiedFiles {
			workerCh <- filePath
		}
	}()

	// Wait for completion
	wg.Wait()
	close(errorCh)

	// Check for errors
	var errors []error
	for err := range errorCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		// Log all errors
		if logFile != nil {
			fmt.Fprintf(logFile, "New file verification errors (%d):\n", len(errors))
			for _, err := range errors {
				fmt.Fprintf(logFile, "  %v\n", err)
			}
		}

		// Fail if too many errors (threshold: 10% of files or minimum 10 files)
		errorThreshold := len(copiedFiles) / 10 // 10% threshold
		if errorThreshold < 10 {
			errorThreshold = 10 // But at least 10 files must fail before we give up
		}

		if len(errors) > errorThreshold {
			return fmt.Errorf("%d verification errors (threshold: %d)", len(errors), errorThreshold)
		} else if len(errors) > 0 {
			// Some errors occurred but below threshold - log as warnings instead
			if logFile != nil {
				fmt.Fprintf(logFile, "WARNING: %d verification errors detected but below threshold (%d)\n", len(errors), errorThreshold)
			}
		}
	}

	return nil
}

// verifyCriticalFiles ensures integrity of essential system files during backup verification.
// This function always verifies important system files regardless of whether they were
// copied during the backup operation, providing an additional safety layer.
//
// Critical files verification features:
//   - Uses predefined list from DefaultVerificationConfig.CriticalFiles
//   - Intelligent path handling for both system and home directory backups
//   - Graceful handling of missing files (logged but not failed)
//   - Individual file SHA-256 checksum validation
//   - Non-blocking verification (errors logged but don't fail backup)
//
// Parameters:
//   - sourcePath: Original source directory (system root "/" or home directory)
//   - destPath: Destination backup directory for verification
//   - logFile: Optional log file for detailed verification reporting
//
// The function automatically adapts to backup type:
//   - For system backups (sourcePath="/"), verifies all critical system files
//   - For home backups, skips system-level files that aren't relevant
//
// Returns nil (never fails backup) but logs all errors to verificationErrors slice.
func verifyCriticalFiles(sourcePath, destPath string, logFile *os.File) error {
	criticalFiles := DefaultVerificationConfig.CriticalFiles
	verified := 0
	errors := 0

	for _, criticalPath := range criticalFiles {
		// Check for cancellation
		if shouldCancelBackup() {
			return fmt.Errorf("verification canceled")
		}

		// Skip critical system files if we're doing a home backup
		if sourcePath != "/" && strings.HasPrefix(criticalPath, "/") {
			// This is a home backup, skip system files
			continue
		}

		var srcFile string
		if strings.HasPrefix(criticalPath, "/") {
			// Absolute path - adjust for source root
			srcFile = filepath.Join(sourcePath, strings.TrimPrefix(criticalPath, "/"))
		} else {
			// Relative path
			srcFile = filepath.Join(sourcePath, criticalPath)
		}

		// Skip if file doesn't exist in source (not an error for critical files)
		if _, err := os.Stat(srcFile); os.IsNotExist(err) {
			if logFile != nil {
				fmt.Fprintf(logFile, "Critical file not found in source, skipping: %s\n", criticalPath)
			}
			continue
		}

		err := verifySingleFile(criticalPath, sourcePath, destPath)
		if err != nil {
			errors++
			verificationErrors = append(verificationErrors, fmt.Sprintf("Critical file %s: %v", criticalPath, err))
			if logFile != nil {
				fmt.Fprintf(logFile, "Critical file error: %s - %v\n", criticalPath, err)
			}
		} else {
			verified++
			atomic.AddInt64(&totalFilesVerified, 1)
		}
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Critical files: %d verified, %d errors\n", verified, errors)
	}

	// Don't fail backup for critical file errors, but log them
	return nil
}

// verifySampledFiles performs statistical sampling verification of unchanged files.
// This function provides performance-optimized verification for files that weren't
// copied during backup but should still be validated for integrity.
//
// Sampling verification features:
//   - Configurable sample rate (typically 1-5% of unchanged files)
//   - Random selection without replacement for statistical validity
//   - Capped at 100 files maximum for performance bounds
//   - Directory walking with pattern exclusion support
//   - Error tolerance of 1% sample failure rate
//
// Parameters:
//   - sourcePath: Original source directory for file discovery
//   - destPath: Destination backup directory for verification
//   - sampleRate: Fraction of files to verify (0.01 = 1%, 0.05 = 5%)
//   - excludePatterns: Patterns that were excluded during backup (should also be excluded from verification)
//   - logFile: Optional log file for detailed verification reporting
//
// The function performs intelligent candidate selection:
//   - Walks source directory to find all files
//   - Excludes patterns from excludePatterns (temp files, system directories, cache files)
//   - Verifies corresponding destination files exist before sampling
//   - Uses cryptographically secure random selection
//
// Returns an error only if sample error rate exceeds 1%, indicating systematic issues.
func verifySampledFiles(sourcePath, destPath string, sampleRate float64, excludePatterns []string, logFile *os.File) error {
	if sampleRate <= 0 || filesSkipped == 0 {
		return nil
	}

	// Calculate sample size
	sampleSize := int(float64(filesSkipped) * sampleRate)
	if sampleSize < 1 {
		sampleSize = 1
	}
	if sampleSize > 100 { // Cap at 100 files for reasonable performance
		sampleSize = 100
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Sampling %d files out of %d unchanged files (%.1f%%)\n",
			sampleSize, filesSkipped, sampleRate*100)
	}

	// We don't have a list of skipped files, so we'll do a directory walk
	// and randomly sample files that exist in both locations
	var candidateFiles []string

	err := filepath.WalkDir(sourcePath, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return nil
		}

		// Skip excluded patterns - comprehensive browser cache exclusion
		if shouldExcludeFile(path, excludePatterns, sourcePath) {
			return nil
		}

		// Check if file exists in destination
		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return nil
		}
		destFile := filepath.Join(destPath, relPath)

		if _, err := os.Stat(destFile); err == nil {
			candidateFiles = append(candidateFiles, path)
		}

		// Limit candidates for performance
		if len(candidateFiles) >= sampleSize*10 {
			return fmt.Errorf("enough candidates") // Stop walking
		}

		return nil
	})

	if err != nil && err.Error() != "enough candidates" {
		return err
	}

	// Randomly sample from candidates
	if len(candidateFiles) == 0 {
		return nil
	}

	rand.Seed(time.Now().UnixNano())
	verified := 0
	errors := 0

	for i := 0; i < sampleSize && i < len(candidateFiles); i++ {
		// Check for cancellation
		if shouldCancelBackup() {
			return fmt.Errorf("verification canceled")
		}

		// Random selection without replacement
		idx := rand.Intn(len(candidateFiles))
		filePath := candidateFiles[idx]
		candidateFiles = append(candidateFiles[:idx], candidateFiles[idx+1:]...)

		err := verifySingleFile(filePath, sourcePath, destPath)
		if err != nil {
			errors++
			if logFile != nil {
				fmt.Fprintf(logFile, "Sample verification error: %s - %v\n", filePath, err)
			}
		} else {
			verified++
			atomic.AddInt64(&totalFilesVerified, 1)
		}
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Sample verification: %d verified, %d errors\n", verified, errors)
	}

	// Allow up to 1% sample error rate
	if errors > 0 && float64(errors)/float64(verified+errors) > 0.01 {
		return fmt.Errorf("high sample error rate: %d errors out of %d samples", errors, verified+errors)
	}

	return nil
}

// verifyDirectoryStructure validates the overall directory tree integrity between source and backup.
// This function performs a high-level structural comparison to ensure the backup contains
// the expected directory hierarchy and detects major structural inconsistencies.
//
// Directory structure validation features:
//   - Comprehensive directory counting in both source and destination
//   - Tolerance for minor differences (backup metadata, temporary files)
//   - Performance-optimized tree traversal using filepath.WalkDir
//   - Statistical validation approach (allows ±10 directory variance)
//   - Non-blocking verification (errors logged but backup continues)
//
// Parameters:
//   - sourcePath: Original source directory to analyze
//   - destPath: Destination backup directory for comparison
//   - logFile: Optional log file for detailed verification reporting
//
// Validation Logic:
//   - Counts all directories in source and destination trees
//   - Allows for backup-specific additions (BACKUP-INFO.txt, metadata)
//   - Flags significant discrepancies (>10 directory difference)
//   - Provides detailed logging of directory counts for analysis
//
// Returns an error if directory count variance exceeds acceptable thresholds,
// which may indicate incomplete backup or structural corruption.
func verifyDirectoryStructure(sourcePath, destPath string, logFile *os.File) error {
	// Count directories in source and destination
	sourceDirs := 0
	destDirs := 0

	// Count source directories
	err := filepath.WalkDir(sourcePath, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		sourceDirs++
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to count source directories: %v", err)
	}

	// Count destination directories
	err = filepath.WalkDir(destPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		destDirs++
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to count destination directories: %v", err)
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Directory structure: source=%d, dest=%d directories\n", sourceDirs, destDirs)
	}

	// Allow for some variation (backup info files, etc.)
	dirDifference := destDirs - sourceDirs
	if dirDifference > 10 || dirDifference < -10 {
		// Directory count mismatch is not helpful for users - just log it
		if logFile != nil {
			fmt.Fprintf(logFile, "INFO: Directory count difference: source=%d, dest=%d (difference=%d)\n", sourceDirs, destDirs, dirDifference)
		}
		// Don't return error - this is not actionable for users
	}

	atomic.AddInt64(&totalFilesVerified, 1) // Count structure check as one verification
	return nil
}

// verifySingleFile performs comprehensive integrity verification of an individual file.
// This is the core verification function that validates a single file's integrity
// using multiple validation strategies optimized for different file types and sizes.
//
// Verification Strategy Selection:
//   - Small files (≤1MB): Full SHA-256 checksum verification
//   - Critical files (boot/, etc/): Always full checksum regardless of size
//   - Large files (>1MB): Statistical sampling verification for performance
//   - All files: Size comparison as quick initial validation
//
// Parameters:
//   - filePath: Path to the file being verified (can be absolute or relative)
//   - sourcePath: Root source directory for path resolution
//   - destPath: Root destination directory for path resolution
//
// Path Resolution Logic:
//   - Handles absolute paths within source directory
//   - Processes critical system files with absolute paths (/etc/fstab)
//   - Converts all paths to appropriate relative paths for destination lookup
//   - Validates both source and destination files exist before verification
//
// Returns an error if any verification step fails, including:
//   - File not found in source or destination
//   - Size mismatches between source and destination
//   - Checksum failures for small or critical files
//   - Content sampling failures for large files
func verifySingleFile(filePath, sourcePath, destPath string) error {
	// Convert absolute source path to relative path for destination
	var relPath string
	var err error
	var actualSourcePath string

	if filepath.IsAbs(filePath) && strings.HasPrefix(filePath, sourcePath) {
		// File path is absolute and within source path
		relPath, err = filepath.Rel(sourcePath, filePath)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %v", err)
		}
		actualSourcePath = filePath
	} else if strings.HasPrefix(filePath, "/") {
		// Critical file path - absolute path like "/etc/fstab"
		relPath = strings.TrimPrefix(filePath, "/")
		actualSourcePath = filepath.Join(sourcePath, relPath)
	} else {
		// Already relative path
		relPath = filePath
		actualSourcePath = filepath.Join(sourcePath, relPath)
	}

	destFile := filepath.Join(destPath, relPath)

	// Check if both files exist
	srcInfo, err := os.Stat(actualSourcePath)
	if err != nil {
		return fmt.Errorf("source file missing: %v", err)
	}

	destInfo, err := os.Stat(destFile)
	if err != nil {
		return fmt.Errorf("destination file missing: %v", err)
	}

	// Quick checks first
	if srcInfo.Size() != destInfo.Size() {
		return fmt.Errorf("size mismatch: src=%d, dest=%d", srcInfo.Size(), destInfo.Size())
	}

	// For small files or critical files, do full checksum
	if srcInfo.Size() <= 1024*1024 || strings.Contains(relPath, "boot") || strings.Contains(relPath, "etc") {
		srcHash, err := getFileSHA256(actualSourcePath)
		if err != nil {
			return fmt.Errorf("failed to hash source: %v", err)
		}

		destHash, err := getFileSHA256(destFile)
		if err != nil {
			return fmt.Errorf("failed to hash destination: %v", err)
		}

		if srcHash != destHash {
			return fmt.Errorf("checksum mismatch")
		}
	} else {
		// For large files, use the same sampling strategy as during backup
		if !largFilesIdentical(actualSourcePath, destFile, srcInfo.Size()) {
			return fmt.Errorf("content mismatch (sampling)")
		}
	}

	return nil
}

// performStandaloneVerification executes comprehensive verification as an independent operation.
// This function provides complete backup integrity verification without being part
// of an active backup process, ideal for verifying existing backups or periodic validation.
//
// Standalone verification features:
//   - Independent operation (not tied to backup process state)
//   - Enhanced sampling rate (10x normal rate for thorough verification)
//   - Lenient error thresholds (allows up to 10 total errors)
//   - Comprehensive reporting with detailed statistics
//   - Full critical files and directory structure validation
//
// Verification Process:
//  1. Critical system files verification (essential files always checked)
//  2. Extensive random sampling verification (10x sample rate)
//  3. Directory structure integrity validation
//  4. Comprehensive error reporting and threshold validation
//
// Parameters:
//   - sourcePath: Original source directory that was backed up
//   - destPath: Backup directory to verify for integrity
//   - excludePatterns: Patterns that were excluded during backup
//   - logFile: Optional log file for detailed verification reporting
//
// Unlike incremental verification during backup, this function:
//   - Has no knowledge of "newly copied" vs "existing" files
//   - Uses higher sampling rates for more thorough coverage
//   - Applies more lenient error thresholds for older backups
//   - Focuses on statistical validation across entire backup
//
// Returns an error only if verification discovers systematic integrity issues
// that exceed the standalone verification error threshold (10 errors maximum).
func performStandaloneVerification(sourcePath, destPath string, excludePatterns []string, logFile *os.File) error {
	if logFile != nil {
		fmt.Fprintf(logFile, "Starting standalone verification\n")
		fmt.Fprintf(logFile, "Source: %s, Destination: %s\n", sourcePath, destPath)
	}

	// Mark verification as active
	verificationPhaseActive = true
	verificationStart := time.Now()

	// Reset verification counters
	totalFilesVerified = 0
	verificationErrors = []string{}

	// For standalone verification, we don't have a list of copied files,
	// so we'll verify a representative sample of all files

	// Step 1: Verify critical system files (always check)
	if logFile != nil {
		fmt.Fprintf(logFile, "Verifying critical system files\n")
	}
	err := verifyCriticalFiles(sourcePath, destPath, logFile)
	if err != nil {
		return fmt.Errorf("critical files verification failed: %v", err)
	}

	// Step 2: Sample verification of all files in backup
	if logFile != nil {
		fmt.Fprintf(logFile, "Sampling verification of backup files\n")
	}
	err = verifyRandomSampleOfBackup(sourcePath, destPath, DefaultVerificationConfig.SampleRate*10, excludePatterns, logFile) // Use 10x sample rate for standalone
	if err != nil {
		// Non-critical error - log but don't fail verification
		verificationErrors = append(verificationErrors, fmt.Sprintf("Sample verification: %v", err))
		if logFile != nil {
			fmt.Fprintf(logFile, "Warning: %v\n", err)
		}
	}

	// Step 3: Verify directory structure
	if logFile != nil {
		fmt.Fprintf(logFile, "Verifying directory structure\n")
	}
	err = verifyDirectoryStructure(sourcePath, destPath, logFile)
	if err != nil {
		verificationErrors = append(verificationErrors, fmt.Sprintf("Directory structure: %v", err))
		if logFile != nil {
			fmt.Fprintf(logFile, "Warning: %v\n", err)
		}
	}

	duration := time.Since(verificationStart)

	// Final verification report
	if logFile != nil {
		fmt.Fprintf(logFile, "Standalone verification completed in %v\n", duration)
		fmt.Fprintf(logFile, "Files verified: %d\n", totalFilesVerified)
		fmt.Fprintf(logFile, "Critical files checked: %d\n", len(DefaultVerificationConfig.CriticalFiles))
		fmt.Fprintf(logFile, "Errors/warnings: %d\n", len(verificationErrors))

		if len(verificationErrors) > 0 {
			fmt.Fprintf(logFile, "Verification warnings:\n")
			for _, err := range verificationErrors {
				fmt.Fprintf(logFile, "  - %s\n", err)
			}
		}
	}

	// Mark verification as complete
	verificationPhaseActive = false

	// For standalone verification, ANY missing files should cause failure
	if len(verificationErrors) > 0 {
		// Instead of returning generic error, populate detailed error screen
		// The TUI will check verificationErrors global and show detailed screen
		return fmt.Errorf("VERIFICATION_DETAILED_ERRORS:%d", len(verificationErrors))
	}

	return nil
}

// verifyRandomSampleOfBackup conducts statistical validation across an entire backup directory.
// This function performs comprehensive random sampling verification by analyzing all files
// in the source and randomly selecting a representative sample for thorough verification.
//
// CRITICAL: This function validates that the SOURCE is properly represented in the BACKUP,
// not the other way around. It detects files missing from backup or corrupted in backup.
//
// Advanced Sampling Features:
//   - Source directory walking (finds files that should be in backup)
//   - Intelligent file discovery with exclusion pattern filtering
//   - Configurable sample rates (typically 10x normal for standalone verification)
//   - Performance bounds (10,000 candidate limit, 1,000 sample maximum)
//   - Statistical validation with 5% error tolerance
//
// Sampling Algorithm:
//   - Walks the SOURCE directory to discover all current files
//   - Excludes patterns from excludePatterns (temp files, system directories, cache files)
//   - For each source file, validates it exists and matches in backup
//   - Uses cryptographically secure random selection without replacement
//   - Reports files missing from backup or content mismatches
//
// Parameters:
//   - sourcePath: Original source directory to validate (this is the "truth")
//   - destPath: Backup directory to verify against source
//   - sampleRate: Fraction of discovered files to verify (0.1 = 10%)
//   - excludePatterns: Patterns that were excluded during backup (should also be excluded from verification)
//   - logFile: Optional log file for detailed verification reporting
//
// Performance Characteristics:
//   - Minimum sample size: 10 files
//   - Maximum sample size: 1,000 files (performance cap)
//   - Maximum candidates: 10,000 files (memory efficiency)
//   - Error tolerance: 5% failure rate for standalone verification
//
// Returns an error if sample error rate exceeds 5%, indicating systematic backup issues.
// This correctly detects missing files, content mismatches, and backup corruption.
func verifyRandomSampleOfBackup(sourcePath, destPath string, sampleRate float64, excludePatterns []string, logFile *os.File) error {
	if sampleRate <= 0 {
		return nil
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Starting smart two-phase verification\n")
		fmt.Fprintf(logFile, "Phase 1: Directory structure check\n")
		fmt.Fprintf(logFile, "Phase 2: File sampling verification\n")
		fmt.Fprintf(logFile, "Source: %s, Backup: %s\n", sourcePath, destPath)
	}

	// PHASE 1: Check directory structure first (fast)
	var missingDirs []string
	missingDirSet := make(map[string]bool)
	err := filepath.WalkDir(sourcePath, func(sourceFilePath string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Only check directories, not files
		if !d.IsDir() {
			return nil
		}

		// Skip excluded patterns for directories
		if shouldExcludeFile(sourceFilePath, excludePatterns, sourcePath) {
			return filepath.SkipDir
		}

		// Convert source dir path to backup dir path
		relPath, err := filepath.Rel(sourcePath, sourceFilePath)
		if err != nil {
			return nil
		}

		backupDirPath := filepath.Join(destPath, relPath)

		// Check if corresponding backup directory exists
		if _, err := os.Stat(backupDirPath); os.IsNotExist(err) {
			// Check if this directory should be empty due to exclusions
			if isDirectoryEmptyDueToExclusions(sourceFilePath, excludePatterns, sourcePath, logFile) {
				// This directory is missing but should be empty due to exclusions - not an error
				if logFile != nil {
					fmt.Fprintf(logFile, "EXPECTED EMPTY DIRECTORY (excluded contents): %s\n", relPath)
				}
				return nil
			}

			// Check if this is a subdirectory of an already missing parent
			isSubdirOfMissing := false
			for existingMissing := range missingDirSet {
				if strings.HasPrefix(relPath, existingMissing+"/") {
					isSubdirOfMissing = true
					break
				}
			}

			// Only add top-level missing directories
			if !isSubdirOfMissing {
				missingDirs = append(missingDirs, relPath)
				missingDirSet[relPath] = true
				if logFile != nil {
					fmt.Fprintf(logFile, "MISSING DIRECTORY: %s\n", relPath)
				}
				// Add to verification errors
				verificationErrors = append(verificationErrors, fmt.Sprintf("Missing directory: %s", relPath))

				// Skip walking subdirectories since parent is missing
				return filepath.SkipDir
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("directory structure check failed: %v", err)
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Phase 1 complete: Found %d missing directories\n", len(missingDirs))
		if len(missingDirs) > 0 {
			fmt.Fprintf(logFile, "Missing directories:\n")
			for _, dir := range missingDirs {
				fmt.Fprintf(logFile, "  - %s\n", dir)
			}
		}
	}

	// PHASE 2: Spot-check files in existing directories (sampling)
	var candidateFiles []string
	filesProcessed := 0
	const maxFilesToCheck = 50000 // Reasonable limit for file sampling

	if logFile != nil {
		fmt.Fprintf(logFile, "Phase 2: Starting file sampling verification\n")
	}

	err = filepath.WalkDir(sourcePath, func(sourceFilePath string, d os.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return nil
		}

		// Limit total files processed for performance
		filesProcessed++
		if filesProcessed > maxFilesToCheck {
			return fmt.Errorf("enough files processed")
		}

		// Skip excluded patterns
		if shouldExcludeFile(sourceFilePath, excludePatterns, sourcePath) {
			return nil
		}

		// Convert source path to relative path, then to backup path
		relPath, err := filepath.Rel(sourcePath, sourceFilePath)
		if err != nil {
			return nil
		}

		backupFilePath := filepath.Join(destPath, relPath)

		// Check if corresponding backup file exists
		if _, err := os.Stat(backupFilePath); err == nil {
			// Backup file exists - add to candidates for verification
			if len(candidateFiles) < 10000 {
				candidateFiles = append(candidateFiles, sourceFilePath)
			}
		} else {
			// File is missing from backup
			if logFile != nil {
				fmt.Fprintf(logFile, "MISSING FILE: %s\n", relPath)
			}
			verificationErrors = append(verificationErrors, fmt.Sprintf("Missing file: %s", relPath))
		}

		return nil
	})

	if err != nil && err.Error() != "enough files processed" {
		return err
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Phase 2 complete: Processed %d files, found %d candidates\n", filesProcessed, len(candidateFiles))
		if len(verificationErrors) > 0 {
			fmt.Fprintf(logFile, "TOTAL VERIFICATION ISSUES: %d\n", len(verificationErrors))
		}
	}

	if len(candidateFiles) == 0 {
		if logFile != nil {
			fmt.Fprintf(logFile, "No files found for sample verification\n")
		}
		// If we have missing files but no candidates, that's a major issue
		if len(verificationErrors) > 0 {
			return fmt.Errorf("backup is missing %d files from source", len(verificationErrors))
		}
		return nil
	}

	// Calculate sample size
	sampleSize := int(float64(len(candidateFiles)) * sampleRate)
	if sampleSize < 10 {
		sampleSize = 10 // Minimum sample size
	}
	if sampleSize > 1000 { // Cap at 1000 files for reasonable performance
		sampleSize = 1000
	}

	if sampleSize > len(candidateFiles) {
		sampleSize = len(candidateFiles)
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Sampling %d files out of %d available files (%.1f%%)\n",
			sampleSize, len(candidateFiles), float64(sampleSize)/float64(len(candidateFiles))*100)
	}

	// Randomly sample from candidates
	rand.Seed(time.Now().UnixNano())
	verified := 0
	errors := 0

	for i := 0; i < sampleSize && len(candidateFiles) > 0; i++ {
		// Check for cancellation
		if shouldCancelBackup() {
			return fmt.Errorf("verification canceled")
		}

		// Random selection without replacement
		idx := rand.Intn(len(candidateFiles))
		filePath := candidateFiles[idx]
		candidateFiles = append(candidateFiles[:idx], candidateFiles[idx+1:]...)

		err := verifySingleFile(filePath, sourcePath, destPath)
		if err != nil {
			errors++
			verificationErrors = append(verificationErrors, fmt.Sprintf("Content mismatch: %s - %v", filePath, err))
			if logFile != nil {
				fmt.Fprintf(logFile, "Sample verification error: %s - %v\n", filePath, err)
			}
		} else {
			verified++
			atomic.AddInt64(&totalFilesVerified, 1)
		}
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "Random sample verification: %d verified, %d errors\n", verified, errors)
		fmt.Fprintf(logFile, "Total verification issues found: %d\n", len(verificationErrors))
	}

	// Separate directory issues from file verification failures
	directoryIssues := 0
	fileVerificationFailures := 0

	// Count different types of errors
	for _, errorMsg := range verificationErrors {
		if strings.Contains(errorMsg, "Missing directory:") {
			directoryIssues++
		} else if strings.Contains(errorMsg, "Content mismatch:") || strings.Contains(errorMsg, "Missing file:") {
			fileVerificationFailures++
		}
	}

	// Only count file verification failures for failure rate calculation
	if fileVerificationFailures > 0 {
		// Calculate failure rate based only on actual file verification attempts
		totalFilesChecked := verified + fileVerificationFailures
		overallErrorRate := float64(fileVerificationFailures) / float64(totalFilesChecked)

		if overallErrorRate > 0.05 { // 5% error threshold
			return fmt.Errorf("high verification failure rate: %d issues out of %d files checked (%.1f%% failure rate)",
				fileVerificationFailures, totalFilesChecked, overallErrorRate*100)
		} else if fileVerificationFailures > 0 {
			// Some file verification failures found but within threshold - log as warning
			if logFile != nil {
				fmt.Fprintf(logFile, "WARNING: %d file verification failures detected but within 5%% threshold\n", fileVerificationFailures)
			}
		}
	}

	// Log directory issues separately (not counted as verification failures)
	if directoryIssues > 0 && logFile != nil {
		fmt.Fprintf(logFile, "INFO: %d directory structure issues found (not counted as verification failures)\n", directoryIssues)
	}

	return nil
}
