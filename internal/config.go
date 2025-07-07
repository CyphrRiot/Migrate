// Package internal provides configuration management and persistent storage for user preferences.
//
// This module handles:
//   - Saving and loading hierarchical folder selections for selective home backups
//   - User preference persistence across sessions
//   - Configuration file management with proper error handling
//   - Migration of old config formats to new formats
//   - Default configuration setup for new users
//
// The configuration system ensures users don't have to re-select their folder
// preferences every time they perform a backup, while still allowing them to
// modify selections when needed.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SelectiveBackupConfig represents persistent configuration for hierarchical folder selections.
// This structure is saved to disk to remember user preferences between backup sessions.
type SelectiveBackupConfig struct {
	// Metadata
	Version     string    `json:"version"`              // Config format version for migration
	LastUpdated time.Time `json:"last_updated"`         // When this config was last saved
	HomeDir     string    `json:"home_dir"`             // Home directory path for validation
	
	// Hierarchical Selection Data
	FolderSelections map[string]bool `json:"folder_selections"` // folder_path -> selected state
	SubfolderCache   map[string][]SavedSubfolder `json:"subfolder_cache"`   // parent_path -> discovered subfolders
	
	// UI Navigation State (optional - for restoring drill-down state)
	LastFolderPath   string   `json:"last_folder_path,omitempty"`   // Last drilled-down folder
	LastBreadcrumb   []string `json:"last_breadcrumb,omitempty"`    // Last breadcrumb path
}

// SavedSubfolder represents a cached subfolder for persistent storage.
// This allows us to restore subfolder discoveries without re-scanning on every load.
type SavedSubfolder struct {
	Name          string `json:"name"`            // Folder name (e.g., "Personal")
	Path          string `json:"path"`            // Full path (e.g., "/home/user/Videos/Personal")
	Size          int64  `json:"size"`            // Size in bytes
	IsVisible     bool   `json:"is_visible"`      // Whether folder should be shown in UI
	HasSubfolders bool   `json:"has_subfolders"`  // Whether this folder can be drilled down further
}

// getConfigDir returns the appropriate configuration directory for the current user.
// Uses XDG specification on Linux: ~/.config/migrate/
func getConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %v", err)
	}
	
	configDir := filepath.Join(homeDir, ".config", "migrate")
	
	// Create config directory if it doesn't exist
	err = os.MkdirAll(configDir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create config directory: %v", err)
	}
	
	return configDir, nil
}

// getSelectiveBackupConfigPath returns the full path to the selective backup config file.
func getSelectiveBackupConfigPath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	
	return filepath.Join(configDir, "selective_backup.json"), nil
}

// SaveSelectiveBackupConfig persists the current hierarchical folder selections to disk.
// This allows users to resume their selection preferences in future backup sessions.
func SaveSelectiveBackupConfig(selectedFolders map[string]bool, subfolderCache map[string][]HomeFolderInfo, currentPath string, breadcrumb []string) error {
	configPath, err := getSelectiveBackupConfigPath()
	if err != nil {
		return fmt.Errorf("failed to get config path: %v", err)
	}
	
	// Get current user's home directory for validation
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}
	
	// Convert HomeFolderInfo to SavedSubfolder for JSON serialization
	savedSubfolderCache := make(map[string][]SavedSubfolder)
	for parentPath, subfolders := range subfolderCache {
		savedSubfolders := make([]SavedSubfolder, len(subfolders))
		for i, subfolder := range subfolders {
			savedSubfolders[i] = SavedSubfolder{
				Name:          subfolder.Name,
				Path:          subfolder.Path,
				Size:          subfolder.Size,
				IsVisible:     subfolder.IsVisible,
				HasSubfolders: subfolder.HasSubfolders,
			}
		}
		savedSubfolderCache[parentPath] = savedSubfolders
	}
	
	// Create config structure
	config := SelectiveBackupConfig{
		Version:          "1.0",
		LastUpdated:      time.Now(),
		HomeDir:          homeDir,
		FolderSelections: selectedFolders,
		SubfolderCache:   savedSubfolderCache,
		LastFolderPath:   currentPath,
		LastBreadcrumb:   breadcrumb,
	}
	
	// Marshal to JSON with pretty printing for readability
	jsonData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}
	
	// Write to file atomically (write to temp file, then rename)
	tempPath := configPath + ".tmp"
	err = os.WriteFile(tempPath, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write temp config file: %v", err)
	}
	
	// Atomic rename
	err = os.Rename(tempPath, configPath)
	if err != nil {
		os.Remove(tempPath) // Clean up temp file on failure
		return fmt.Errorf("failed to rename config file: %v", err)
	}
	
	return nil
}

// LoadSelectiveBackupConfig restores previously saved hierarchical folder selections.
// Returns the loaded configuration or creates a new empty config if none exists.
func LoadSelectiveBackupConfig() (*SelectiveBackupConfig, error) {
	configPath, err := getSelectiveBackupConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get config path: %v", err)
	}
	
	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// No saved config - return empty config (first time use)
		return &SelectiveBackupConfig{
			Version:          "1.0",
			LastUpdated:      time.Now(),
			FolderSelections: make(map[string]bool),
			SubfolderCache:   make(map[string][]SavedSubfolder),
		}, nil
	}
	
	// Read existing config file
	jsonData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}
	
	// Parse JSON
	var config SelectiveBackupConfig
	err = json.Unmarshal(jsonData, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %v", err)
	}
	
	// Validate config against current home directory
	currentHomeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get current home directory: %v", err)
	}
	
	// If home directory has changed, invalidate the config (user switched accounts)
	if config.HomeDir != currentHomeDir {
		return &SelectiveBackupConfig{
			Version:          "1.0",
			LastUpdated:      time.Now(),
			HomeDir:          currentHomeDir,
			FolderSelections: make(map[string]bool),
			SubfolderCache:   make(map[string][]SavedSubfolder),
		}, nil
	}
	
	return &config, nil
}

// ConvertSavedSubfoldersToHomeFolders converts saved subfolder data back to HomeFolderInfo structures.
// This is used when loading config to restore the subfolder cache in the UI model.
func ConvertSavedSubfoldersToHomeFolders(savedSubfolders map[string][]SavedSubfolder) map[string][]HomeFolderInfo {
	result := make(map[string][]HomeFolderInfo)
	
	for parentPath, savedList := range savedSubfolders {
		homeFolders := make([]HomeFolderInfo, len(savedList))
		for i, saved := range savedList {
			homeFolders[i] = HomeFolderInfo{
				Name:          saved.Name,
				Path:          saved.Path,
				Size:          saved.Size,
				IsVisible:     saved.IsVisible,
				Selected:      false, // Will be set from FolderSelections
				AlwaysInclude: false,
				HasSubfolders: saved.HasSubfolders,
				Subfolders:    nil, // Only populate on-demand
				ParentPath:    parentPath,
			}
		}
		result[parentPath] = homeFolders
	}
	
	return result
}

// ApplySelectionsToFolders applies saved selections to a list of HomeFolderInfo structures.
// This is used during model initialization to restore user's previous selections.
func ApplySelectionsToFolders(folders []HomeFolderInfo, savedSelections map[string]bool) []HomeFolderInfo {
	for i := range folders {
		if selected, exists := savedSelections[folders[i].Path]; exists {
			folders[i].Selected = selected
		}
		
		// Apply selections to subfolders as well
		if len(folders[i].Subfolders) > 0 {
			folders[i].Subfolders = ApplySelectionsToFolders(folders[i].Subfolders, savedSelections)
		}
	}
	
	return folders
}

// CleanupOldSelections removes selection entries for folders that no longer exist.
// This prevents the config file from growing indefinitely with stale entries.
func CleanupOldSelections(selections map[string]bool) map[string]bool {
	cleaned := make(map[string]bool)
	
	for folderPath, selected := range selections {
		// Check if the folder still exists
		if _, err := os.Stat(folderPath); err == nil {
			cleaned[folderPath] = selected
		}
		// If folder doesn't exist, we simply don't add it to cleaned map (removes it)
	}
	
	return cleaned
}

// GetConfigSummary returns a human-readable summary of the current saved configuration.
// Useful for debugging and user feedback.
func GetConfigSummary() (string, error) {
	config, err := LoadSelectiveBackupConfig()
	if err != nil {
		return "", err
	}
	
	selectedCount := 0
	totalCount := len(config.FolderSelections)
	
	for _, selected := range config.FolderSelections {
		if selected {
			selectedCount++
		}
	}
	
	cacheCount := 0
	for _, subfolders := range config.SubfolderCache {
		cacheCount += len(subfolders)
	}
	
	summary := fmt.Sprintf(`Configuration Summary:
  Version: %s
  Last Updated: %s
  Home Directory: %s
  Total Folders: %d
  Selected Folders: %d
  Cached Subfolders: %d
  Last Navigation: %s`,
		config.Version,
		config.LastUpdated.Format("2006-01-02 15:04:05"),
		config.HomeDir,
		totalCount,
		selectedCount,
		cacheCount,
		config.LastFolderPath)
	
	return summary, nil
}

// ResetSelectiveBackupConfig clears all saved selections and cache.
// Useful for starting fresh or debugging purposes.
func ResetSelectiveBackupConfig() error {
	configPath, err := getSelectiveBackupConfigPath()
	if err != nil {
		return fmt.Errorf("failed to get config path: %v", err)
	}
	
	// Simply remove the config file
	err = os.Remove(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove config file: %v", err)
	}
	
	return nil
}
