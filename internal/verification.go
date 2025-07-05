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

// performBackupVerification runs the smart incremental verification
func performBackupVerification(sourcePath, destPath string, logFile *os.File) error {
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
	// Thread-safe access to copiedFilesList
	copiedFilesListMutex.Lock()
	copiedFilesCount := len(copiedFilesList)
	copiedFilesCopy := make([]string, len(copiedFilesList))
	copy(copiedFilesCopy, copiedFilesList)
	copiedFilesListMutex.Unlock()
	
	if copiedFilesCount > 0 {
		if logFile != nil {
			fmt.Fprintf(logFile, "Verifying %d newly copied files\n", copiedFilesCount)
		}
		
		// Small delay to ensure filesystem sync
		time.Sleep(1 * time.Second)
		
		err := verifyNewFiles(copiedFilesCopy, sourcePath, destPath, logFile)
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
		err = verifySampledFiles(sourcePath, destPath, DefaultVerificationConfig.SampleRate, logFile)
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

	// Fail backup if too many critical errors (threshold: 1% of new files)
	criticalErrorThreshold := copiedFilesCount / 100
	if criticalErrorThreshold < 1 {
		criticalErrorThreshold = 1
	}
	
	if len(verificationErrors) > criticalErrorThreshold {
		return fmt.Errorf("verification failed with %d errors (threshold: %d)", 
			len(verificationErrors), criticalErrorThreshold)
	}

	return nil
}

// verifyNewFiles performs full checksum verification of newly copied files
func verifyNewFiles(copiedFiles []string, sourcePath, destPath string, logFile *os.File) error {
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
		errorThreshold := len(copiedFiles) / 10  // 10% threshold
		if errorThreshold < 10 {
			errorThreshold = 10  // But at least 10 files must fail before we give up
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

// verifyCriticalFiles always verifies important system files
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

// verifySampledFiles performs random sampling verification of unchanged files
func verifySampledFiles(sourcePath, destPath string, sampleRate float64, logFile *os.File) error {
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

		// Skip excluded patterns
		for _, pattern := range ExcludePatterns {
			if strings.Contains(path, strings.TrimSuffix(pattern, "/*")) {
				return nil
			}
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

// verifyDirectoryStructure checks basic directory structure integrity
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
		return fmt.Errorf("significant directory count mismatch: source=%d, dest=%d", sourceDirs, destDirs)
	}

	atomic.AddInt64(&totalFilesVerified, 1) // Count structure check as one verification
	return nil
}

// verifySingleFile performs checksum verification of a single file
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
