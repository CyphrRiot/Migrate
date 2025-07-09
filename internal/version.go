// Package internal provides version information and build metadata for the Migrate application.
//
// This module centralizes all version-related constants and provides formatted strings
// for consistent display across the application. To update the version, simply change
// the AppVersion constant - all other version strings will be automatically updated.
//
// The version system supports:
//   - Semantic versioning (major.minor.patch format)
//   - Automatic version string generation for UI components
//   - Consistent branding and author attribution
//   - Backup metadata generation with version tracking
package internal

// Application metadata constants.
// These constants define the core identity and versioning information for Migrate.
//
// TO UPDATE THE VERSION: Change only AppVersion below - all other version-related
// strings throughout the application will be automatically updated.
const (
	// AppName is the official name of the application
	AppName = "Migrate"

	// AppVersion follows semantic versioning (major.minor.patch)
	// ⬅️ CHANGE VERSION HERE ONLY - this updates everywhere automatically
	AppVersion = "1.0.53"

	// AppAuthor contains author information with social media reference
	AppAuthor = "Cypher Riot - https://x.com/CyphrRiot"

	// AppDesc is the tagline/description used in UI and documentation
	AppDesc = "The Beautiful Live System Backup & Restore"
)

// GetVersionString returns just the version number for programmatic use.
// Example: "1.0.22"
func GetVersionString() string {
	return AppVersion
}

// GetFullVersionString returns the application name with version for display.
// Example: "Migrate v1.0.22"
func GetFullVersionString() string {
	return AppName + " v" + AppVersion
}

// GetAppTitle returns the complete application title including description.
// Used for window titles and main application headers.
// Example: "Migrate v1.0.22 - A Beautiful Live System Backup & Restore"
func GetAppTitle() string {
	return AppName + " v" + AppVersion + " - " + AppDesc
}

// GetSubtitle returns a compact version and author string for UI footers.
// Used in application subtitles and about dialogs.
// Example: "v1.0.22 by Cypher Riot (x.com/CyphrRiot)"
func GetSubtitle() string {
	return "v" + AppVersion + " by " + AppAuthor
}

// GetAboutText returns the standard about text for help screens.
// Provides a concise application identity string.
// Example: "Migrate v1.0.22 - A Beautiful Live System Backup & Restore"
func GetAboutText() string {
	return AppName + " v" + AppVersion + " - " + AppDesc
}

// GetBackupInfoHeader generates version information for backup metadata files.
// The backupType parameter is included for context (e.g., "Complete System", "Home Directory").
// This text is embedded in BACKUP-INFO.txt files to track which version created the backup.
//
// Example output:
//
//	"This backup was created using Migrate v1.0.22\n
//	 A Beautiful Live System Backup & Restore by Cypher Riot (x.com/CyphrRiot)"
func GetBackupInfoHeader(backupType string) string {
	return "This backup was created using " + AppName + " v" + AppVersion + "\n" +
		AppDesc + " by " + AppAuthor
}
