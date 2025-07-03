package internal

// Version information for Migrate
// ================================
// TO UPDATE THE VERSION: Change AppVersion below and it will update everywhere automatically.
// All other version-related strings will be generated from these constants.

const (
	AppName    = "Migrate"
	AppVersion = "1.0.3"  // ⬅️ CHANGE VERSION HERE ONLY
	AppAuthor  = "Cypher Riot"
	AppDesc    = "Live Backup & Restore"
)

// GetVersionString returns the formatted version string for display
func GetVersionString() string {
	return AppVersion
}

// GetFullVersionString returns the full app name and version
func GetFullVersionString() string {
	return AppName + " v" + AppVersion
}

// GetAppTitle returns the formatted app title
func GetAppTitle() string {
	return AppName + " v" + AppVersion + " - " + AppDesc
}

// GetSubtitle returns the formatted subtitle with version and author
func GetSubtitle() string {
	return "v" + AppVersion + " • Created by " + AppAuthor
}

// GetAboutText returns the formatted about text
func GetAboutText() string {
	return AppName + " v" + AppVersion + " - " + AppDesc
}

// GetBackupInfoHeader returns the backup info header text
func GetBackupInfoHeader(backupType string) string {
	return "This backup was created using " + AppName + " v" + AppVersion + "\n" +
		AppDesc + " by " + AppAuthor
}
