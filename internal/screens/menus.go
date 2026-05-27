package screens

// Menu choice constants for different screens
var (
	// MainMenuChoices defines the main menu options in the correct order
	MainMenuChoices = []string{
		"🚀 Backup System",
		"🔍 Verify Backup",
		"🔄 Restore System",
		"⚙️ Restore Settings",
		"ℹ️ About",
		"❌ Exit",
	}

	// MainMenuChoicesNonRoot defines the main menu options when running as non-root user
	MainMenuChoicesNonRoot = []string{
		"🚀 Backup System",
		"⚙️ Backup System Settings",
		"🔍 Verify Backup",
		"🔄 Restore System",
		"ℹ️ About",
		"❌ Exit",
	}

	// BackupMenuChoices defines the backup menu options
	BackupMenuChoices = []string{
		"🏠 Home Directory Only",
		"📁 Complete System Backup",
		"⬅️ Back",
	}

	// BackupMenuChoicesNonRoot defines the backup menu options when running as non-root user
	BackupMenuChoicesNonRoot = []string{
		"🏠 Home Directory Only",
		"⚙️ Backup System Settings (/etc)",
		"⬅️ Back",
	}

	// RestoreMenuChoices defines the restore menu options
	RestoreMenuChoices = []string{
		"🔄 Restore to Current System",
		"📂 Restore to Custom Path",
		"⬅️ Back",
	}

	// VerifyMenuChoices defines the verify menu options
	VerifyMenuChoices = []string{
		"🔍 Auto-Detect & Verify Backup",
		"⬅️ Back",
	}

	// RestoreOptionsChoices defines the restore options menu
	RestoreOptionsChoices = []string{
		"☑️ Restore Configuration (~/.config)",
		"☑️ Restore Window Managers (Hyprland, GNOME, etc.)",
		"✅ Continue",
		"⬅️ Back",
	}

	// RestoreSettingsMenuChoices defines the restore settings menu options
	RestoreSettingsMenuChoices = []string{
		"⚙️ Restore Configuration Files (~/.config)",
		"📁 Restore Local Data (~/.local)",
		"⬅️ Back",
	}

	// ConfirmationChoices defines standard yes/no choices
	ConfirmationChoices = []string{
		"✅ Yes",
		"❌ No",
	}

	// HomeFolderControlChoices defines the control options for folder selection
	HomeFolderControlChoices = []string{
		"✅ Continue with selection",
		"⬅️ Back",
	}
)

// GetMenuChoices returns the appropriate menu choices for a given screen
func GetMenuChoices(screen Screen) []string {
	switch screen {
	case ScreenMain:
		return MainMenuChoices
	case ScreenBackup:
		return BackupMenuChoices
	case ScreenRestore:
		return RestoreMenuChoices
	case ScreenVerify:
		return VerifyMenuChoices
	case ScreenRestoreOptions:
		return RestoreOptionsChoices
	case ScreenRestoreSettings:
		return RestoreSettingsMenuChoices
	case ScreenConfirm:
		return ConfirmationChoices
	default:
		return []string{}
	}
}

// MenuAction represents the result of a menu selection
type MenuAction struct {
	Screen    Screen
	Operation string
	Index     int
}

// GetMainMenuAction returns the action for a main menu selection
func GetMainMenuAction(index int) MenuAction {
	switch index {
	case 0: // Backup
		return MenuAction{Screen: ScreenBackup}
	case 1: // Verify
		return MenuAction{Screen: ScreenVerify}
	case 2: // Restore
		return MenuAction{Screen: ScreenRestore}
	case 3: // Restore Settings
		return MenuAction{Screen: ScreenRestoreSettings}
	case 4: // About
		return MenuAction{Screen: ScreenAbout}
	case 5: // Exit
		return MenuAction{} // Special case, handled separately
	default:
		return MenuAction{}
	}
}

// GetMainMenuActionNonRoot returns the action for a non-root main menu selection
func GetMainMenuActionNonRoot(index int) MenuAction {
	switch index {
	case 0: // Backup
		return MenuAction{Screen: ScreenBackup}
	case 1: // Backup System Settings
		return MenuAction{Screen: ScreenDriveSelect, Operation: "settings_backup"}
	case 2: // Verify
		return MenuAction{Screen: ScreenVerify}
	case 3: // Restore
		return MenuAction{Screen: ScreenRestore}
	case 4: // About
		return MenuAction{Screen: ScreenAbout}
	case 5: // Exit
		return MenuAction{} // Special case, handled separately
	default:
		return MenuAction{}
	}
}

// GetBackupMenuAction returns the action for a backup menu selection
func GetBackupMenuAction(index int) MenuAction {
	switch index {
	case 0: // Home Directory Only
		return MenuAction{
			Screen:    ScreenHomeFolderSelect,
			Operation: "home_backup",
		}
	case 1: // Complete System Backup
		return MenuAction{
			Screen:    ScreenDriveSelect,
			Operation: "system_backup",
		}
	case 2: // Back
		return MenuAction{Screen: ScreenMain}
	default:
		return MenuAction{}
	}
}

// GetBackupMenuActionNonRoot returns the action for a non-root backup menu selection
func GetBackupMenuActionNonRoot(index int) MenuAction {
	switch index {
	case 0: // Home Directory Only
		return MenuAction{
			Screen:    ScreenHomeFolderSelect,
			Operation: "home_backup",
		}
	case 1: // Backup System Settings
		return MenuAction{
			Screen:    ScreenDriveSelect,
			Operation: "settings_backup",
		}
	case 2: // Back
		return MenuAction{Screen: ScreenMain}
	default:
		return MenuAction{}
	}
}

// GetRestoreMenuAction returns the action for a restore menu selection
func GetRestoreMenuAction(index int) MenuAction {
	switch index {
	case 0: // Restore to Current System
		return MenuAction{
			Screen:    ScreenRestoreOptions,
			Operation: "system_restore",
		}
	case 1: // Restore to Custom Path
		return MenuAction{
			Screen:    ScreenRestoreOptions,
			Operation: "custom_restore",
		}
	case 2: // Back
		return MenuAction{Screen: ScreenMain}
	default:
		return MenuAction{}
	}
}

// GetVerifyMenuAction returns the action for a verify menu selection
func GetVerifyMenuAction(index int) MenuAction {
	switch index {
	case 0: // Auto-detect backup type and verify
		return MenuAction{
			Screen:    ScreenDriveSelect,
			Operation: "auto_verify",
		}
	case 1: // Back
		return MenuAction{Screen: ScreenMain}
	default:
		return MenuAction{}
	}
}
