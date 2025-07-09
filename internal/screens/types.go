package screens

// Screen represents the different screens/views in the application
type Screen int

// Screen constants define all possible screens in the application
const (
	ScreenMain Screen = iota
	ScreenBackup
	ScreenRestore
	ScreenVerify
	ScreenAbout
	ScreenConfirm
	ScreenProgress
	ScreenDriveSelect
	ScreenError
	ScreenComplete
	ScreenRestoreOptions
	ScreenHomeFolderSelect
	ScreenHomeSubfolderSelect
	ScreenVerificationErrors
	ScreenRestoreFolderSelect
)

// String returns the string representation of a screen
func (s Screen) String() string {
	switch s {
	case ScreenMain:
		return "Main Menu"
	case ScreenBackup:
		return "Backup Menu"
	case ScreenRestore:
		return "Restore Menu"
	case ScreenVerify:
		return "Verify Menu"
	case ScreenAbout:
		return "About"
	case ScreenConfirm:
		return "Confirmation"
	case ScreenProgress:
		return "Progress"
	case ScreenDriveSelect:
		return "Drive Selection"
	case ScreenError:
		return "Error"
	case ScreenComplete:
		return "Complete"
	case ScreenRestoreOptions:
		return "Restore Options"
	case ScreenHomeFolderSelect:
		return "Home Folder Selection"
	case ScreenHomeSubfolderSelect:
		return "Home Subfolder Selection"
	case ScreenVerificationErrors:
		return "Verification Errors"
	case ScreenRestoreFolderSelect:
		return "Restore Folder Selection"
	default:
		return "Unknown"
	}
}
