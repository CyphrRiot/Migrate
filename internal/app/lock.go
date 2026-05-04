// Package app provides application-level bootstrap logic for Migrate.
//
// This package handles singleton locking, terminal sizing, error rendering,
// signal handling, and TUI initialization — keeping main.go as a thin shim.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// lockFilePath is the location of the singleton instance lock file.
// It prevents multiple migrate processes from running concurrently.
var lockFilePath = filepath.Join(os.TempDir(), "migrate.lock")

// CheckSingleInstance verifies that no other migrate process is currently running.
// Stale lock files are automatically cleaned up if the process no longer exists.
func CheckSingleInstance() error {
	if _, err := os.Stat(lockFilePath); err == nil {
		lockContent, readErr := os.ReadFile(lockFilePath)
		if readErr == nil {
			pid := strings.TrimSpace(string(lockContent))
			if pid != "" {
				if pidInt, err := strconv.Atoi(pid); err == nil {
					if process, err := os.FindProcess(pidInt); err == nil {
						if err := process.Signal(syscall.Signal(0)); err == nil {
							return fmt.Errorf("another migrate process is already running (PID: %s)", pid)
						}
					}
				}
			}
		}
		os.Remove(lockFilePath)
	}
	return nil
}

// CreateInstanceLock creates a lock file containing the current process ID.
func CreateInstanceLock() error {
	pid := fmt.Sprintf("%d", os.Getpid())
	return os.WriteFile(lockFilePath, []byte(pid), 0644)
}

// RemoveInstanceLock deletes the singleton lock file.
func RemoveInstanceLock() {
	os.Remove(lockFilePath)
}
