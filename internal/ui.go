// Package internal provides the user interface rendering and styling system for Migrate.
//
// This package implements the visual layer of the TUI using the Lipgloss library for styling.
// The UI system provides:
//   - Tokyo Night color theme with authentic palette implementation
//   - Gradient color system for progress bars and status indicators  
//   - Responsive rendering for different terminal sizes
//   - Screen-specific render methods for each application state
//   - Progress visualization with animated cylon effects
//   - Consistent styling across all UI components
//
// The styling system is built around the Tokyo Night theme specification,
// providing a cohesive dark theme experience with carefully chosen colors
// for different UI elements and states.
package internal

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// GradientColors defines a color gradient with start and end points for smooth transitions.
// Used throughout the UI to create visually appealing progress bars and status indicators
// that follow the Tokyo Night color scheme.
type GradientColors struct {
	Start lipgloss.Color // Starting color of the gradient
	End   lipgloss.Color // Ending color of the gradient
}

var (
	// 🌃 AUTHENTIC TOKYO NIGHT COLOR PALETTE 🌃
	// Based on the official Tokyo Night theme specification
	
	// Tokyo Night Blue - Primary brand color
	blueGradient = GradientColors{
		Start: lipgloss.Color("#7aa2f7"), // Tokyo Night blue (primary)
		End:   lipgloss.Color("#3d59a1"), // Tokyo Night blue dark
	}
	
	// Tokyo Night Purple - Accent and keywords
	purpleGradient = GradientColors{
		Start: lipgloss.Color("#bb9af7"), // Tokyo Night purple (bright)
		End:   lipgloss.Color("#9d7cd8"), // Tokyo Night purple (medium)
	}
	
	// Tokyo Night Green - Success and strings  
	greenGradient = GradientColors{
		Start: lipgloss.Color("#9ece6a"), // Tokyo Night green (primary)
		End:   lipgloss.Color("#73b957"), // Tokyo Night green dark
	}
	
	// Tokyo Night Orange/Red - Warnings and errors
	orangeGradient = GradientColors{
		Start: lipgloss.Color("#e0af68"), // Tokyo Night orange (warm)
		End:   lipgloss.Color("#f7768e"), // Tokyo Night red/pink
	}
	
	// Tokyo Night Cyan - Special highlights
	tealGradient = GradientColors{
		Start: lipgloss.Color("#73daca"), // Tokyo Night cyan (bright)
		End:   lipgloss.Color("#2ac3de"), // Tokyo Night cyan dark
	}

	// Status-specific gradients using authentic Tokyo Night colors
	successGradient = greenGradient    // Tokyo Night green for success
	warningGradient = orangeGradient   // Tokyo Night orange for warnings  
	errorGradient = GradientColors{
		Start: lipgloss.Color("#f7768e"), // Tokyo Night red/pink
		End:   lipgloss.Color("#ff5555"), // Tokyo Night red intense
	}
	infoGradient = blueGradient        // Tokyo Night blue for info
	progressGradient = purpleGradient  // Tokyo Night purple for progress

	// Base Tokyo Night colors - authentic theme foundation
	primaryColor    = lipgloss.Color("#7aa2f7") // Tokyo Night blue
	secondaryColor  = lipgloss.Color("#9ece6a") // Tokyo Night green  
	accentColor     = lipgloss.Color("#bb9af7") // Tokyo Night purple
	warningColor    = lipgloss.Color("#e0af68") // Tokyo Night orange
	errorColor      = lipgloss.Color("#f7768e") // Tokyo Night red/pink
	successColor    = lipgloss.Color("#9ece6a") // Tokyo Night green
	textColor       = lipgloss.Color("#c0caf5") // Tokyo Night foreground
	dimColor        = lipgloss.Color("#565f89") // Tokyo Night comment/dim
	backgroundColor = lipgloss.Color("#1a1b26") // Tokyo Night background  
	borderColor     = lipgloss.Color("#414868") // Tokyo Night border/line
)

// GetColor returns a color from the gradient based on position (0.0 to 1.0).
// Currently implements a simple 50% threshold transition - could be enhanced
// with proper color interpolation for smoother gradients.
//
// Parameters:
//   position: Float between 0.0 (start color) and 1.0 (end color)
func (g GradientColors) GetColor(position float64) lipgloss.Color {
	// Clamp position between 0 and 1
	if position < 0 {
		position = 0
	}
	if position > 1 {
		position = 1
	}
	
	// Simple implementation: threshold at 50%
	// TODO: Consider implementing proper color interpolation for smoother transitions
	if position < 0.5 {
		return g.Start
	}
	return g.End
}

// GetColorFromPercentage converts a percentage (0-100) to a gradient position and returns the color.
// This is a convenience method for progress bars and percentage-based displays.
func (g GradientColors) GetColorFromPercentage(percentage float64) lipgloss.Color {
	return g.GetColor(percentage / 100.0)
}

// GetStatusGradient returns the appropriate gradient colors for a given status string.
// This provides semantic color coding throughout the UI for consistent status representation.
func GetStatusGradient(status string) GradientColors {
	switch status {
	case "success", "complete", "done":
		return successGradient
	case "warning", "caution":
		return warningGradient
	case "error", "failed", "fail":
		return errorGradient
	case "info", "information":
		return infoGradient
	case "progress", "running", "working":
		return progressGradient
	default:
		return blueGradient
	}
}

// formatBytesWithColor formats byte values with appropriate color coding based on context.
// The style parameter determines the color scheme used (e.g., "speed" for transfer rates, "size" for file sizes).
func formatBytesWithColor(bytes int64, style string) string {
	formatted := FormatBytes(bytes)
	
	var colorStyle lipgloss.Style
	switch style {
	case "speed":
		// Color code based on speed ranges
		if bytes >= 100*1024*1024 { // > 100MB/s
			colorStyle = lipgloss.NewStyle().Foreground(successGradient.End).Bold(true)
		} else if bytes >= 10*1024*1024 { // > 10MB/s
			colorStyle = lipgloss.NewStyle().Foreground(greenGradient.Start)
		} else if bytes >= 1*1024*1024 { // > 1MB/s
			colorStyle = lipgloss.NewStyle().Foreground(blueGradient.End)
		} else {
			colorStyle = lipgloss.NewStyle().Foreground(dimColor)
		}
	case "size":
		// Color code based on file/data sizes
		if bytes >= 10*1024*1024*1024 { // > 10GB
			colorStyle = lipgloss.NewStyle().Foreground(purpleGradient.End).Bold(true)
		} else if bytes >= 1024*1024*1024 { // > 1GB
			colorStyle = lipgloss.NewStyle().Foreground(blueGradient.End)
		} else if bytes >= 100*1024*1024 { // > 100MB
			colorStyle = lipgloss.NewStyle().Foreground(greenGradient.Start)
		} else {
			colorStyle = lipgloss.NewStyle().Foreground(textColor)
		}
	default:
		colorStyle = lipgloss.NewStyle().Foreground(textColor)
	}
	
	return colorStyle.Render(formatted)
}

// formatNumberWithColor formats numeric values with color coding based on significance level.
// The significance parameter determines the color thresholds (e.g., "high" for important metrics, "files" for file counts).
func formatNumberWithColor(n int64, significance string) string {
	formatted := FormatNumber(n)
	
	var colorStyle lipgloss.Style
	switch significance {
	case "high":
		if n >= 100000 {
			colorStyle = lipgloss.NewStyle().Foreground(successGradient.End).Bold(true)
		} else if n >= 10000 {
			colorStyle = lipgloss.NewStyle().Foreground(blueGradient.End).Bold(true)
		} else {
			colorStyle = lipgloss.NewStyle().Foreground(greenGradient.Start)
		}
	case "files":
		if n >= 50000 {
			colorStyle = lipgloss.NewStyle().Foreground(purpleGradient.End).Bold(true)
		} else if n >= 10000 {
			colorStyle = lipgloss.NewStyle().Foreground(blueGradient.End)
		} else if n >= 1000 {
			colorStyle = lipgloss.NewStyle().Foreground(greenGradient.Start)
		} else {
			colorStyle = lipgloss.NewStyle().Foreground(textColor)
		}
	default:
		colorStyle = lipgloss.NewStyle().Foreground(textColor)
	}
	
	return colorStyle.Render(formatted)
}

// renderDataMetric creates a formatted data display with icon, label, and value.
// Used for consistent presentation of metrics throughout the UI.
func renderDataMetric(label, value, icon string) string {
	labelStyle := lipgloss.NewStyle().Foreground(dimColor)
	iconStyle := lipgloss.NewStyle().Foreground(blueGradient.Start)
	
	return fmt.Sprintf("%s %s %s", 
		iconStyle.Render(icon),
		labelStyle.Render(label+":"),
		value)
}

// renderProgressStats creates a formatted display of backup/restore progress statistics.
// Shows counts for files copied, skipped, and total with appropriate color coding.
func renderProgressStats(filesCopied, filesSkipped, totalFiles int64) string {
	var parts []string
	
	if filesCopied > 0 {
		copiedText := formatNumberWithColor(filesCopied, "files")
		parts = append(parts, renderDataMetric("Copied", copiedText, "📄"))
	}
	
	if filesSkipped > 0 {
		skippedText := formatNumberWithColor(filesSkipped, "files") 
		parts = append(parts, renderDataMetric("Skipped", skippedText, "⚡"))
	}
	
	if totalFiles > 0 {
		totalText := formatNumberWithColor(totalFiles, "files")
		parts = append(parts, renderDataMetric("Total", totalText, "📁"))
	}
	
	if len(parts) == 0 {
		return ""
	}
	
	return strings.Join(parts, " • ")
}

var (
	asciiStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true).
			Align(lipgloss.Center).
			MarginBottom(1)

	titleStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true).
			Align(lipgloss.Center).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(textColor).
			Align(lipgloss.Center).
			MarginBottom(1)  // Reduced from 2 to 1

	// Subdued style for version/author info
	versionStyle = lipgloss.NewStyle().
			Foreground(dimColor).  // Much darker and less prominent
			Align(lipgloss.Center).
			MarginBottom(1)

	// Modern Panel System - authentic Tokyo Night styling
	modernPanelStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(borderColor).  // Tokyo Night border
				Background(lipgloss.Color("#24283b")).  // Tokyo Night dark panel bg
				Padding(3, 4).  // More generous padding
				Margin(1, 2)   // Better spacing

	// Enhanced border system - Tokyo Night themed
	borderStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).  // Tokyo Night border color
			Padding(2, 3).
			Margin(1)

	// Clean info panels with Tokyo Night colors
	infoBoxStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#24283b")).   // Tokyo Night panel bg
			Foreground(textColor).
			Padding(0, 1).   // Minimal padding
			Margin(0).       // No margins
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor)  // Tokyo Night border

	// Clean status styles with dark Tokyo Night backgrounds
	warningStyle = lipgloss.NewStyle().
			Foreground(textColor).  // Use normal text color
			Background(lipgloss.Color("#1e2030")).  // Much darker background, closer to main bg
			Bold(true).
			Align(lipgloss.Center).
			Padding(0, 2).   // Back to minimal padding
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#e0af68"))  // Keep border as accent only

	// Info style for neutral confirmations (like unmount)
	infoStyle = lipgloss.NewStyle().
			Foreground(textColor).  // Use normal text color
			Background(lipgloss.Color("#1e2030")).  // Same dark background
			Bold(true).
			Align(lipgloss.Center).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7aa2f7"))  // Tokyo Night blue border

	errorStyle = lipgloss.NewStyle().
			Foreground(textColor).  // Use normal text color
			Background(lipgloss.Color("#1e2030")).  // Same dark background
			Bold(true).
			Align(lipgloss.Center).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#f7768e"))  // Tokyo Night red border

	successStyle = lipgloss.NewStyle().
			Foreground(textColor).  // Use normal text color  
			Background(lipgloss.Color("#1e2030")).  // Same dark background
			Bold(true).
			Align(lipgloss.Center).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#9ece6a"))  // Tokyo Night green border

	// Tokyo Night menu selection styling
	selectedMenuItemStyle = lipgloss.NewStyle().
				PaddingLeft(2).   // Back to normal padding
				PaddingRight(2).
				Background(primaryColor).  // Tokyo Night blue
				Foreground(backgroundColor).  // Tokyo Night dark bg as text
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(blueGradient.End)  // Deeper Tokyo Night blue border

	// Clean menu items
	menuItemStyle = lipgloss.NewStyle().
			PaddingLeft(2).   // Normal padding
			PaddingRight(2).
			Foreground(textColor)

	inactiveMenuItemStyle = menuItemStyle.Copy().
				Foreground(dimColor)

	// Beautiful progress bar
	progressBarStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor).
				Padding(0, 1)

	// Enhanced help style
	helpStyle = lipgloss.NewStyle().
			Foreground(dimColor).
			Align(lipgloss.Center).
			Italic(true).
			MarginTop(2)
)

// MigrateASCII contains the ASCII art logo displayed in headers throughout the application.
// Uses a sleek design that fits the Tokyo Night aesthetic.
const MigrateASCII = `▖  ▖▘      ▗     ▐▘▄▖    ▗      ▜ 
▛▖▞▌▌▛▌▛▘▀▌▜▘█▌  ▐ ▚ ▌▌▛▘▜▘█▌▛▛▌▐ 
▌▝ ▌▌▙▌▌ █▌▐▖▙▖  ▐ ▄▌▙▌▄▌▐▖▙▖▌▌▌▐ 
     ▄▌          ▝▘  ▄▌         ▀ `

// renderMainMenu renders the primary application menu screen.
// Displays the main navigation options with the application header and help text.
func (m Model) renderMainMenu() string {
	var s strings.Builder

	// Header
	header := m.renderHeader()
	s.WriteString(header + "\n\n")

	// Menu options with beautiful styling
	for i, choice := range m.choices {
		if m.cursor == i {
			s.WriteString(selectedMenuItemStyle.Render("❯ "+choice) + "\n")
		} else {
			s.WriteString(menuItemStyle.Render("  "+choice) + "\n")
		}
	}

	// Help text
	help := m.renderHelp()
	s.WriteString("\n" + help)

	// Center the content with beautiful border
	content := borderStyle.Width(m.width - 8).Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// renderBackupMenu renders the backup type selection screen.
// Shows options for complete system backup or home directory backup with explanatory info.
func (m Model) renderBackupMenu() string {
	var s strings.Builder

	// Header with ASCII art
	ascii := asciiStyle.Render(MigrateASCII)
	s.WriteString(ascii + "\n")
	s.WriteString(titleStyle.Render("🚀 Backup Options") + "\n\n")

	// Menu options with beautiful styling
	for i, choice := range m.choices {
		if m.cursor == i {
			s.WriteString(selectedMenuItemStyle.Render("❯ "+choice) + "\n")
		} else {
			s.WriteString(menuItemStyle.Render("  "+choice) + "\n")
		}
	}

	// Enhanced info box
	info := infoBoxStyle.Render(`📁 Complete System: Full 1:1 backup of entire system
🏠 Home Directory: Personal files and settings only`)

	s.WriteString(info)

	// Help text
	help := m.renderHelp()
	s.WriteString("\n" + help)

	// Center the content with beautiful border
	content := borderStyle.Width(m.width - 8).Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// Render restore menu
func (m Model) renderRestoreMenu() string {
	var s strings.Builder

	// Header with ASCII art (same as backup menu)
	ascii := asciiStyle.Render(MigrateASCII)
	s.WriteString(ascii + "\n")
	s.WriteString(titleStyle.Render("🔄 Restore Options") + "\n\n")

	// Menu options with beautiful styling
	for i, choice := range m.choices {
		if m.cursor == i {
			s.WriteString(selectedMenuItemStyle.Render("❯ "+choice) + "\n")
		} else {
			s.WriteString(menuItemStyle.Render("  "+choice) + "\n")
		}
	}

	// Enhanced info box with warning
	warning := warningStyle.Render("⚠️  WARNING: Restore operations will overwrite existing files!")
	s.WriteString("\n" + warning)

	// Help text
	help := m.renderHelp()
	s.WriteString("\n" + help)

	// Center the content with beautiful border
	content := borderStyle.Width(m.width - 8).Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// Render verify menu
func (m Model) renderVerifyMenu() string {
	var s strings.Builder

	// Header with ASCII art (consistent with other screens)
	ascii := asciiStyle.Render(MigrateASCII)
	s.WriteString(ascii + "\n")
	s.WriteString(titleStyle.Render("🔍 Verify Options") + "\n\n")

	// Menu options with beautiful styling
	for i, choice := range m.choices {
		if m.cursor == i {
			s.WriteString(selectedMenuItemStyle.Render("❯ "+choice) + "\n")
		} else {
			s.WriteString(menuItemStyle.Render("  "+choice) + "\n")
		}
	}

	// Enhanced info box
	info := infoBoxStyle.Render(`🔍 Complete System:  Verify entire system backup integrity
🏠 Home Directory:   Verify home directory backup only`)

	s.WriteString(info)

	// Additional verification info
	verifyInfo := infoBoxStyle.Render("🛡️  Verification compares backup files with source using SHA256 checksums")
	s.WriteString("\n" + verifyInfo)

	// Help text
	help := m.renderHelp()
	s.WriteString("\n" + help)

	// Center the content with beautiful border
	content := borderStyle.Width(m.width - 8).Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// Render about screen
func (m Model) renderAbout() string {
	var s strings.Builder

	// Header with ASCII art (consistent with other screens)
	ascii := asciiStyle.Render(MigrateASCII)
	s.WriteString(ascii + "\n")
	s.WriteString(titleStyle.Render("ℹ️ About Migrate") + "\n\n")

	// About content with enhanced data visualization
	features := []string{
		renderDataMetric("Implementation", "Pure Go", "⚡"),
		renderDataMetric("Dependencies", "Zero external", "🛡️"),  
		renderDataMetric("UI Framework", "Bubble Tea + Lipgloss", "🎨"),
		renderDataMetric("Progress Tracking", "Real-time file-based", "📊"),
		renderDataMetric("Deduplication", "SHA256 checksums", "🔍"),
		renderDataMetric("Encryption", "LUKS drive support", "🔐"),
		renderDataMetric("Sync Method", "rsync --delete equivalent", "🔄"),
		renderDataMetric("Portability", "Static binary", "📦"),
	}
	
	aboutContent := GetAboutText() + "\n\n" +
		"Created by " + AppAuthor + "\n\n" +
		"🔗 GitHub: https://github.com/CyphrRiot/Migrate\n" +
		"🐦 X: https://x.com/CyphrRiot\n\n" +
		"✨ Authentic Tokyo Night interface with neon gradient progress bars\n\n" +
		"Key Features:\n" + 
		strings.Join(features, "\n") + "\n\n" +
		"Press any key to return to main menu"

	info := lipgloss.NewStyle().
		Foreground(textColor).
		Margin(0, 2).
		Align(lipgloss.Left).
		Render(aboutContent)

	s.WriteString(info)

	// Center the content with beautiful border
	content := borderStyle.Width(m.width - 8).Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// Render confirmation dialog
func (m Model) renderConfirmation() string {
	var s strings.Builder

	// Header
	s.WriteString(titleStyle.Render("⚠️  Confirmation Required") + "\n\n")

	// Choose appropriate style based on confirmation content
	var confirmStyle lipgloss.Style
	if strings.Contains(m.confirmation, "unmount") || strings.Contains(m.confirmation, "Backup completed successfully") {
		// Use neutral info style for unmount confirmations
		confirmStyle = infoStyle
	} else if strings.Contains(m.confirmation, "OVERWRITE") || strings.Contains(m.confirmation, "overwrite") {
		// Use warning style for destructive operations
		confirmStyle = warningStyle
	} else {
		// Default to info style for other confirmations
		confirmStyle = infoStyle
	}

	// Confirmation message with appropriate styling
	confirmMsg := confirmStyle.Render(m.confirmation)
	s.WriteString(confirmMsg + "\n\n")

	// Yes/No options
	choices := []string{"✅ Yes, Continue", "❌ No, Cancel"}
	for i, choice := range choices {
		cursor := " "
		if m.cursor == i {
			cursor = "❯"
			s.WriteString(fmt.Sprintf("%s %s\n", 
				selectedMenuItemStyle.Render(cursor+" "+choice),
				""))
		} else {
			s.WriteString(fmt.Sprintf("%s %s\n", 
				menuItemStyle.Render(cursor+" "+choice),
				""))
		}
	}

	// Help text
	help := helpStyle.Render("↑/↓: navigate • enter: select • esc: cancel")
	s.WriteString("\n" + help)

	// Center the content
	content := borderStyle.Width(m.width - 4).Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// Render progress screen
func (m Model) renderProgress() string {
	var s strings.Builder

	// App branding header
	ascii := asciiStyle.Render(MigrateASCII)
	s.WriteString(ascii + "\n")
	title := titleStyle.Render(AppDesc)
	s.WriteString(title + "\n")
	version := versionStyle.Render(GetSubtitle())  // Use dimmer versionStyle instead of subtitleStyle
	s.WriteString(version + "\n\n")

	// Operation title
	if m.canceling {
		s.WriteString(titleStyle.Render("🛑 Canceling Operation") + "\n\n")
	} else {
		// Make title specific to the operation type
		var operationTitle string
		if strings.Contains(m.operation, "backup") {
			operationTitle = "💾 Backup in Progress"
		} else if strings.Contains(m.operation, "restore") {
			operationTitle = "🔄 Restore in Progress"
		} else {
			operationTitle = "⚙️ Operation in Progress"
		}
		s.WriteString(titleStyle.Render(operationTitle) + "\n\n")
	}

	// Operation info - show source and destination drives with proper styling
	logPath := getLogFilePath() // Get log path for display
	if m.operation == "system_backup" {
		backupTypeStyle := lipgloss.NewStyle().Foreground(greenGradient.Start).Bold(true) // Slightly highlighted
		logStyle := lipgloss.NewStyle().Foreground(dimColor) // Darker/subdued
		
		s.WriteString(backupTypeStyle.Render("📁 Backup Type:    Complete System") + "\n")
		s.WriteString("📂 Source:         /\n")
		s.WriteString("💾 Destination:    " + m.selectedDrive + "\n")
		s.WriteString(logStyle.Render("📋 Log:            " + logPath) + "\n\n")
	} else if m.operation == "home_backup" {
		backupTypeStyle := lipgloss.NewStyle().Foreground(greenGradient.Start).Bold(true) // Slightly highlighted
		logStyle := lipgloss.NewStyle().Foreground(dimColor) // Darker/subdued
		
		s.WriteString(backupTypeStyle.Render("📁 Backup Type:    Home Directory Only") + "\n")
		s.WriteString("📂 Source:         ~/\n")  
		s.WriteString("💾 Destination:    " + m.selectedDrive + "\n")
		s.WriteString(logStyle.Render("📋 Log:            " + logPath) + "\n\n")
	} else {
		opInfo := fmt.Sprintf("Running: %s", m.operation)
		s.WriteString(subtitleStyle.Render(opInfo) + "\n")
		s.WriteString("📋 Log:            " + logPath + "\n\n")
	}

	// Progress bar (only show if not canceling)
	if !m.canceling {
		progressBar := m.renderProgressBarWithMessage(m.message)
		s.WriteString(progressBar + "\n\n")
	}

	// Status message with Tokyo Night gradient-matching styling
	if m.message != "" {
		var statusStyle lipgloss.Style
		if m.canceling || strings.Contains(m.message, "Cancel") {
			statusStyle = warningStyle
		} else if strings.Contains(m.message, "Deleting") || strings.Contains(m.message, "deletion") {
			// Deletion messages in Tokyo Night red/pink
			statusStyle = lipgloss.NewStyle().
				Foreground(errorColor).  // Tokyo Night red/pink
				Bold(true).
				Align(lipgloss.Center)
		} else {
			// Match the Tokyo Night progress bar colors
			var progressColor lipgloss.Color
			if m.progress < 0.25 {
				progressColor = blueGradient.Start
			} else if m.progress < 0.50 {
				progressColor = purpleGradient.Start
			} else if m.progress < 0.75 {
				progressColor = tealGradient.Start
			} else {
				progressColor = greenGradient.Start
			}
			
			statusStyle = lipgloss.NewStyle().
				Foreground(progressColor)
		}
		statusMsg := statusStyle.Render(m.message)
		s.WriteString(statusMsg + "\n")
	}

	// Help text
	var help string
	if m.canceling {
		help = helpStyle.Render("Please wait for cleanup to complete...")
	} else {
		help = helpStyle.Render("Please wait... • Ctrl+C: cancel")
	}
	s.WriteString("\n" + help)

	// Center the content
	content := borderStyle.Width(m.width - 4).Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// getProgressEmoji returns an appropriate emoji for the current operation phase.
// Analyzes the message content to provide contextual visual indicators.
func getProgressEmoji(message string, progress float64) string {
	// Check message content for specific operations
	if strings.Contains(message, "Scanning") || strings.Contains(message, "Scan") {
		return "🔍" // Magnifying glass for scanning
	}
	if strings.Contains(message, "Deleting") || strings.Contains(message, "deletion") {
		return "🗑️"  // Trash can for deletion
	}
	if strings.Contains(message, "cleaned up") {
		return "🗑️"  // Trash can for cleanup
	}
	if strings.Contains(message, "Preparing") {
		return "⏳" // Hourglass for preparation
	}
	
	// Default to processing/working for active operations
	return "💫" // Dizzy for processing/working
}

// renderProgressBarWithMessage creates the main progress visualization with Tokyo Night styling.
// Supports both determinate progress (0.0-1.0) and indeterminate progress (-1) with animated effects.
// The progress bar features authentic Tokyo Night color gradients and a sweeping cylon animation.
func (m Model) renderProgressBarWithMessage(message string) string {
	width := 60 // Increased width for better visual impact
	
	// Check if this is indeterminate progress (-1)
	if m.progress < 0 {
		// Beautiful Tokyo Night animated indeterminate progress
		now := time.Now().Unix()
		pos := int(now/1) % width // Move every second
		
		var segments []string
		for i := 0; i < width; i++ {
			distance := ((pos - i + width) % width)
			if distance <= 3 { // Create a 4-character moving Tokyo Night highlight
				switch distance {
				case 0:
					segments = append(segments, lipgloss.NewStyle().Foreground(purpleGradient.Start).Render("█"))
				case 1:
					segments = append(segments, lipgloss.NewStyle().Foreground(blueGradient.Start).Render("▓"))
				case 2:
					segments = append(segments, lipgloss.NewStyle().Foreground(tealGradient.Start).Render("▒"))
				case 3:
					segments = append(segments, lipgloss.NewStyle().Foreground(dimColor).Render("░"))
				}
			} else {
				segments = append(segments, lipgloss.NewStyle().Foreground(lipgloss.Color("#24283b")).Render("░"))
			}
		}
		
		bar := strings.Join(segments, "")
		emoji := getProgressEmoji(message, -1)
		progressText := fmt.Sprintf("%s %s Working...", emoji, bar)
		
		return lipgloss.NewStyle().
			Align(lipgloss.Center).
			Render(progressText)
	}
	
	// Beautiful gradient progress bar based on percentage
	percentage := fmt.Sprintf("%.1f%%", m.progress*100)
	filled := int(m.progress * float64(width))
	
	// Calculate cylon position (sweeps back and forth) 
	cylonPos := m.cylonFrame
	if cylonPos >= 10 {
		cylonPos = 20 - cylonPos // Reverse direction for second half
	}
	cylonPos = cylonPos * (width - 1) / 10 // Scale to full progress bar width (0 to width-1)
	
	// Create authentic Tokyo Night gradient segments based on progress
	var segments []string
	for i := 0; i < width; i++ {
		progressPos := float64(i) / float64(width)
		
		if i < filled {
			// Filled portion - Tokyo Night rainbow gradient
			// This creates the signature Tokyo Night color progression
			var segmentColor lipgloss.Color
			
			if progressPos < 0.25 {
				// 0-25% of bar: Tokyo Night Blue → Deep Blue
				ratio := progressPos * 4 // Scale to 0-1 for this segment
				segmentColor = blueGradient.GetColor(ratio)
			} else if progressPos < 0.50 {
				// 25-50% of bar: Tokyo Night Purple spectrum
				ratio := (progressPos - 0.25) * 4
				segmentColor = purpleGradient.GetColor(ratio)
			} else if progressPos < 0.75 {
				// 50-75% of bar: Tokyo Night Cyan spectrum  
				ratio := (progressPos - 0.50) * 4
				segmentColor = tealGradient.GetColor(ratio)
			} else {
				// 75-100% of bar: Tokyo Night Green spectrum (success!)
				ratio := (progressPos - 0.75) * 4
				segmentColor = greenGradient.GetColor(ratio)
			}
			
			// Cylon overlay effect with Tokyo Night accent
			if i == cylonPos || i == cylonPos+1 {
				segments = append(segments, lipgloss.NewStyle().Foreground(lipgloss.Color("#c0caf5")).Render("▓"))
			} else {
				segments = append(segments, lipgloss.NewStyle().Foreground(segmentColor).Render("█"))
			}
		} else {
			// Empty portion with subtle Tokyo Night cylon highlighting
			if i == cylonPos || i == cylonPos+1 {
				segments = append(segments, lipgloss.NewStyle().Foreground(dimColor).Render("▒"))
			} else {
				segments = append(segments, lipgloss.NewStyle().Foreground(lipgloss.Color("#414868")).Render("░"))
			}
		}
	}
	
	bar := strings.Join(segments, "")
	
	// Enhanced progress text with Tokyo Night percentage styling
	var percentageStyle lipgloss.Style
	if m.progress < 0.25 {
		percentageStyle = lipgloss.NewStyle().Foreground(blueGradient.Start).Bold(true)
	} else if m.progress < 0.50 {
		percentageStyle = lipgloss.NewStyle().Foreground(purpleGradient.Start).Bold(true)
	} else if m.progress < 0.75 {
		percentageStyle = lipgloss.NewStyle().Foreground(tealGradient.Start).Bold(true)
	} else {
		percentageStyle = lipgloss.NewStyle().Foreground(greenGradient.Start).Bold(true)
	}
	
	emoji := getProgressEmoji(message, m.progress)
	progressText := fmt.Sprintf("%s %s %s", emoji, bar, percentageStyle.Render(percentage))
	
	return lipgloss.NewStyle().
		Align(lipgloss.Center).
		Render(progressText)
}

// renderHeader creates the standard application header with ASCII art, title, and version.
// Used consistently across most screens for brand recognition and navigation context.
func (m Model) renderHeader() string {
	ascii := asciiStyle.Render(MigrateASCII)
	title := titleStyle.Render(AppDesc)
	version := versionStyle.Render(GetSubtitle())  // Use dimmer versionStyle instead of subtitleStyle
	
	return ascii + "\n" + title + "\n" + version
}

// Render drive selection screen
func (m Model) renderDriveSelect() string {
	var s strings.Builder

	// Header
	s.WriteString(titleStyle.Render("💾 Mount External Drive") + "\n\n")

	if len(m.drives) == 0 {
		// Loading or no drives found
		if len(m.choices) == 0 {
			s.WriteString(infoBoxStyle.Render("🔍 Scanning for external drives...") + "\n")
		} else {
			s.WriteString(warningStyle.Render("⚠️  No external drives found") + "\n")
		}
	} else {
		// Show available drives
		info := infoBoxStyle.Render("Select a drive to mount.")
		s.WriteString(info + "\n\n")
		
		// Add LUKS warning
		luksWarning := warningStyle.Render("⚠️  LUKS encrypted drives must be unlocked manually first")
		s.WriteString(luksWarning + "\n\n")

		for i, choice := range m.choices {
			if m.cursor == i {
				s.WriteString(selectedMenuItemStyle.Render("❯ "+choice) + "\n")
			} else {
				s.WriteString(menuItemStyle.Render("  "+choice) + "\n")
			}
		}
	}

	// Show operation message if any
	if m.message != "" {
		var msgStyle lipgloss.Style
		if strings.Contains(m.message, "Success") {
			msgStyle = successStyle
		} else {
			msgStyle = errorStyle
		}
		s.WriteString("\n" + msgStyle.Render(m.message) + "\n")
	}

	// Help text
	help := m.renderHelp()
	s.WriteString("\n" + help)

	// Center the content with beautiful border
	content := borderStyle.Width(m.width - 8).Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// renderHelp creates standardized help text for keyboard navigation.
// Provides consistent guidance across all interactive screens.
func (m Model) renderHelp() string {
	return helpStyle.Render("↑/↓: navigate • enter: select • q: quit • esc: back")
}

// Render error screen that requires manual dismissal
func (m Model) renderError() string {
	var s strings.Builder

	// Parse error message for better formatting
	errorText := m.message
	
	// Remove "Error: " prefix if present
	if strings.HasPrefix(errorText, "Error: ") {
		errorText = strings.TrimPrefix(errorText, "Error: ")
	}
	
	// Parse the nested error structure
	var displayLines []string
	
	if strings.Contains(errorText, "backup failed: verification phase failed: verification of new files failed:") {
		// Verification error - format nicely
		displayLines = []string{
			"🔍 Verification Error:",
			"Backup completed but file verification failed",
			"",
		}
		
		// Extract the specific error details - find everything after the last ":"
		lastColonIndex := strings.LastIndex(errorText, ":")
		if lastColonIndex != -1 && lastColonIndex < len(errorText)-1 {
			details := strings.TrimSpace(errorText[lastColonIndex+1:])
			if details != "" {
				displayLines = append(displayLines, details)
			}
		}
	} else if strings.Contains(errorText, "verification") {
		// Other verification errors
		displayLines = []string{
			"🔍 Verification Error:",
			"",
			errorText,
		}
	} else if strings.Contains(errorText, ":") {
		// Other structured error - break on colons
		parts := strings.Split(errorText, ":")
		if len(parts) >= 2 {
			displayLines = []string{
				"❌ " + strings.TrimSpace(parts[0]) + ":",
				"",
			}
			for i := 1; i < len(parts); i++ {
				part := strings.TrimSpace(parts[i])
				if part != "" {
					displayLines = append(displayLines, part)
				}
			}
		} else {
			displayLines = []string{
				"❌ Error:",
				"",
				errorText,
			}
		}
	} else {
		// Simple error
		displayLines = []string{
			"❌ Error:",
			"",
			errorText,
		}
	}
	
	// Ensure we have at least some content
	if len(displayLines) == 0 || (len(displayLines) == 1 && displayLines[0] == "") {
		displayLines = []string{
			"❌ Error:",
			"",
			"An unknown error occurred",
			"Original message: " + m.message,
		}
	}
	
	// Header
	s.WriteString(titleStyle.Render("❌ Error") + "\n\n")

	// Format error message with proper line breaks
	for i, line := range displayLines {
		if i == 0 {
			// First line in red/bold
			s.WriteString(lipgloss.NewStyle().Foreground(errorColor).Bold(true).Render(line) + "\n")
		} else if line == "" {
			s.WriteString("\n")
		} else {
			// Other lines in normal text
			s.WriteString(lipgloss.NewStyle().Foreground(textColor).Render(line) + "\n")
		}
	}
	
	s.WriteString("\n")

	// Help text - emphasize manual dismissal
	help := helpStyle.Render("📖 Please read the error details above • Press ESC or any key when ready to continue")
	s.WriteString(help)

	// Center the content with beautiful border
	content := borderStyle.Width(m.width - 8).Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// Render completion screen that requires manual dismissal
func (m Model) renderComplete() string {
	var s strings.Builder

	// Header
	s.WriteString(titleStyle.Render("✅ Operation Complete") + "\n\n")

	// Success message with enhanced styling
	successMsg := successStyle.Render(m.message)
	s.WriteString(successMsg + "\n\n")

	// Help text - emphasize manual dismissal
	help := helpStyle.Render("🎉 Operation completed successfully • Press any key to continue")
	s.WriteString(help)

	// Center the content with beautiful border
	content := borderStyle.Width(m.width - 8).Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// Render home folder selection screen
func (m Model) renderHomeFolderSelect() string {
	var s strings.Builder

	// MUCH SMALLER header to save vertical space
	s.WriteString(titleStyle.Render("📁 Select Backup Folders") + "\n")

	// If still loading
	if len(m.homeFolders) == 0 {
		s.WriteString(infoBoxStyle.Render("🔍 Scanning home directory...") + "\n")
		help := helpStyle.Render("Please wait...")
		s.WriteString("\n" + help)
		content := borderStyle.Width(m.width - 8).Render(s.String())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}

	// Compact instructions
	s.WriteString("Choose folders to backup:\n\n")

	// Get visible folders for display (filter out 0 B folders)
	visibleFolders := make([]HomeFolderInfo, 0)
	for _, folder := range m.homeFolders {
		if folder.IsVisible && folder.Size > 0 { // Only show non-empty visible folders
			visibleFolders = append(visibleFolders, folder)
		}
	}
	
	// Calculate layout with controls at TOP
	numFolders := len(visibleFolders)
	
	// Controls FIRST
	controls := []string{
		"➡️ Continue",
		"⬅️ Back", 
	}
	
	for i, control := range controls {
		if m.cursor == i {
			s.WriteString(selectedMenuItemStyle.Render("❯ "+control) + "\n")
		} else {
			s.WriteString(menuItemStyle.Render("  "+control) + "\n")
		}
	}
	
	s.WriteString("\n") // Line between controls and folders
	
	// Then folders in two columns
	rowCount := (numFolders + 1) / 2 // Number of rows needed

	// TWO-COLUMN LAYOUT - COMPLETELY REWRITTEN to fix ANSI styling issues
	for row := 0; row < rowCount; row++ {
		var leftText, rightText string
		var leftStyled, rightStyled string
		
		// Left column: items 0, 1, 2, 3, 4, 5, ... (first half)
		leftIndex := row
		if leftIndex < numFolders {
			folder := visibleFolders[leftIndex]
			checkbox := "[ ]"
			if m.selectedFolders[folder.Path] {
				checkbox = "[✓]"
			}
			sizeText := FormatBytes(folder.Size)
			leftText = fmt.Sprintf("%s %s (%s)", checkbox, folder.Name, sizeText)
			
			// Adjust cursor for controls offset
			folderCursor := leftIndex + len(controls)
			if m.cursor == folderCursor {
				leftStyled = selectedMenuItemStyle.Render("❯ "+leftText)
			} else {
				leftStyled = menuItemStyle.Render("  "+leftText)
			}
		}
		
		// Right column: items 6, 7, 8, 9, 10, 11 (second half)
		rightIndex := row + rowCount
		if rightIndex < numFolders {
			folder := visibleFolders[rightIndex]
			checkbox := "[ ]"
			if m.selectedFolders[folder.Path] {
				checkbox = "[✓]"
			}
			sizeText := FormatBytes(folder.Size)
			rightText = fmt.Sprintf("%s %s (%s)", checkbox, folder.Name, sizeText)
			
			// Adjust cursor for controls offset
			folderCursor := rightIndex + len(controls)
			if m.cursor == folderCursor {
				rightStyled = selectedMenuItemStyle.Render("❯ "+rightText)
			} else {
				rightStyled = menuItemStyle.Render("  "+rightText)
			}
		}
		
		// Build the final line using lipgloss.JoinHorizontal for proper alignment
		if rightStyled != "" {
			line := lipgloss.JoinHorizontal(lipgloss.Left, 
				lipgloss.NewStyle().Width(50).Render(leftStyled),
				rightStyled,
			)
			s.WriteString(line + "\n")
		} else {
			s.WriteString(leftStyled + "\n")
		}
	}

	s.WriteString("\n") // Line after folders

	// Control options (REMOVED - now at top)

	s.WriteString("\n")

	// Proper total backup size display (restored)
	totalSizeText := fmt.Sprintf("Total Backup Size: %s", FormatBytes(m.totalBackupSize))
	totalSizeStyle := lipgloss.NewStyle().
		Foreground(blueGradient.End).
		Bold(true).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(blueGradient.Start).
		Padding(0, 2).
		Align(lipgloss.Center)
	
	s.WriteString(totalSizeStyle.Render(totalSizeText) + "\n\n")

	// Compact help text
	help := helpStyle.Render("↑/↓: navigate • space: toggle • A: all • X: none")
	s.WriteString(help)

	// Center the content with beautiful border
	content := borderStyle.Width(m.width - 8).Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// progressMsg is a message type for triggering progress updates in the TUI.
type progressMsg struct{}

// tickMsg carries timestamp information for periodic UI updates and animations.
type tickMsg time.Time
