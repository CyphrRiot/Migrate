package internal

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Gradient Color System - btop-inspired modern palette
type GradientColors struct {
	Start lipgloss.Color
	End   lipgloss.Color
}

var (
	// Modern btop-inspired gradient color palette
	blueGradient = GradientColors{
		Start: lipgloss.Color("#60a5fa"), // Light blue
		End:   lipgloss.Color("#3b82f6"), // True blue - no cyan!
	}
	
	purpleGradient = GradientColors{
		Start: lipgloss.Color("#8b5cf6"), // Lighter violet - better readability
		End:   lipgloss.Color("#7c3aed"), // Medium purple
	}
	
	greenGradient = GradientColors{
		Start: lipgloss.Color("#10b981"), // Muted emerald green
		End:   lipgloss.Color("#059669"), // Darker muted green
	}
	
	orangeGradient = GradientColors{
		Start: lipgloss.Color("#f59e0b"), // Amber
		End:   lipgloss.Color("#ef4444"), // Red
	}
	
	tealGradient = GradientColors{
		Start: lipgloss.Color("#06b6d4"), // Cyan
		End:   lipgloss.Color("#10b981"), // Emerald
	}

	// Status-specific gradients
	successGradient = greenGradient  // Use the muted green for success too
	warningGradient = orangeGradient
	errorGradient = GradientColors{
		Start: lipgloss.Color("#ef4444"), // Red
		End:   lipgloss.Color("#dc2626"), // Dark red
	}
	infoGradient = blueGradient
	progressGradient = purpleGradient

	// Base colors (using gradient starts for compatibility)
	primaryColor    = blueGradient.Start
	secondaryColor  = greenGradient.Start
	accentColor     = purpleGradient.Start
	warningColor    = orangeGradient.Start
	errorColor      = errorGradient.Start
	successColor    = successGradient.Start
	textColor       = lipgloss.Color("#f8fafc") // Modern light text
	dimColor        = lipgloss.Color("#64748b") // Modern dim text
	backgroundColor = lipgloss.Color("#0f172a") // Deep dark background
	borderColor     = lipgloss.Color("#334155") // Modern border
)

// Gradient helper functions for smooth color transitions
func (g GradientColors) GetColor(position float64) lipgloss.Color {
	// Clamp position between 0 and 1
	if position < 0 {
		position = 0
	}
	if position > 1 {
		position = 1
	}
	
	// For now, return the start color for 0-0.5 and end color for 0.5-1
	// This is a simple implementation - could be enhanced with proper color interpolation
	if position < 0.5 {
		return g.Start
	}
	return g.End
}

// Get gradient color based on percentage (0-100)
func (g GradientColors) GetColorFromPercentage(percentage float64) lipgloss.Color {
	return g.GetColor(percentage / 100.0)
}

// Get status-appropriate gradient color
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

// Enhanced Data Visualization Functions - btop-inspired
func formatBytesWithColor(bytes int64, style string) string {
	formatted := formatBytes(bytes)
	
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

func formatNumberWithColor(n int64, significance string) string {
	formatted := formatNumber(n)
	
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

func renderDataMetric(label, value, icon string) string {
	labelStyle := lipgloss.NewStyle().Foreground(dimColor)
	iconStyle := lipgloss.NewStyle().Foreground(blueGradient.Start)
	
	return fmt.Sprintf("%s %s %s", 
		iconStyle.Render(icon),
		labelStyle.Render(label+":"),
		value)
}

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

	// Modern Panel System - btop-inspired with visual depth
	modernPanelStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(blueGradient.Start).
				Background(lipgloss.Color("#1e293b")).  // Subtle dark background
				Padding(3, 4).  // More generous padding
				Margin(1, 2)   // Better spacing

	// Enhanced border system - clean and minimal
	borderStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(blueGradient.Start).
			Padding(2, 3).
			Margin(1)

	// Clean info panels 
	infoBoxStyle = lipgloss.NewStyle().
			Background(borderColor).   // Subtle background, not intrusive
			Foreground(textColor).
			Padding(0, 1).   // Minimal padding
			Margin(0).       // No margins
			Border(lipgloss.RoundedBorder()).
			BorderForeground(blueGradient.Start)

	// Clean status styles without excessive padding
	warningStyle = lipgloss.NewStyle().
			Foreground(backgroundColor).
			Background(warningGradient.Start).
			Bold(true).
			Align(lipgloss.Center).
			Padding(0, 2).   // Back to minimal padding
			Border(lipgloss.RoundedBorder()).
			BorderForeground(warningGradient.End)

	errorStyle = lipgloss.NewStyle().
			Foreground(backgroundColor).
			Background(errorGradient.Start).
			Bold(true).
			Align(lipgloss.Center).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(errorGradient.End)

	successStyle = lipgloss.NewStyle().
			Foreground(backgroundColor).
			Background(successGradient.Start).
			Bold(true).
			Align(lipgloss.Center).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(successGradient.End)

	// Clean menu selection - no excessive padding
	selectedMenuItemStyle = lipgloss.NewStyle().
				PaddingLeft(2).   // Back to normal padding
				PaddingRight(2).
				Background(blueGradient.Start).
				Foreground(backgroundColor).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(blueGradient.End)

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

// ASCII art for the program name with author - sleek design
const MigrateASCII = `▖  ▖▘      ▗     ▐▘▄▖    ▗      ▜ 
▛▖▞▌▌▛▌▛▘▀▌▜▘█▌  ▐ ▚ ▌▌▛▘▜▘█▌▛▛▌▐ 
▌▝ ▌▌▙▌▌ █▌▐▖▙▖  ▐ ▄▌▙▌▄▌▐▖▙▖▌▌▌▐ 
     ▄▌          ▝▘  ▄▌         ▀ `

// Render the main menu
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

// Render backup menu
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
		"✨ Modern btop-inspired interface with gradient progress bars\n\n" +
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

	// Confirmation message
	confirmMsg := warningStyle.Render(m.confirmation)
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

	// Status message with gradient-matching styling
	if m.message != "" {
		var statusStyle lipgloss.Style
		if m.canceling || strings.Contains(m.message, "Cancel") {
			statusStyle = warningStyle
		} else if strings.Contains(m.message, "Deleting") || strings.Contains(m.message, "deletion") {
			// Deletion messages in deep purple/red
			statusStyle = lipgloss.NewStyle().
				Foreground(errorGradient.End).  // Deep red
				Bold(true).
				Align(lipgloss.Center)
		} else {
			// Match the progress bar gradient colors
			var progressColor lipgloss.Color
			if m.progress < 0.33 {
				progressColor = blueGradient.End
			} else if m.progress < 0.66 {
				progressColor = purpleGradient.End
			} else {
				progressColor = greenGradient.End
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

// Get appropriate progress emoji based on operation phase
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

// Render gradient progress bar with btop-inspired modern styling
func (m Model) renderProgressBarWithMessage(message string) string {
	width := 60 // Increased width for better visual impact
	
	// Check if this is indeterminate progress (-1)
	if m.progress < 0 {
		// Beautiful animated indeterminate progress with gradient
		now := time.Now().Unix()
		pos := int(now/1) % width // Move every second
		
		var segments []string
		for i := 0; i < width; i++ {
			distance := ((pos - i + width) % width)
			if distance <= 3 { // Create a 4-character moving highlight
				switch distance {
				case 0:
					segments = append(segments, lipgloss.NewStyle().Foreground(progressGradient.End).Render("█"))
				case 1:
					segments = append(segments, lipgloss.NewStyle().Foreground(progressGradient.Start).Render("▓"))
				case 2, 3:
					segments = append(segments, lipgloss.NewStyle().Foreground(blueGradient.Start).Render("▒"))
				}
			} else {
				segments = append(segments, lipgloss.NewStyle().Foreground(dimColor).Render("░"))
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
	
	// Create gradient segments based on progress
	var segments []string
	for i := 0; i < width; i++ {
		progressPos := float64(i) / float64(width)
		
		if i < filled {
			// Filled portion - rainbow gradient based on BAR POSITION (not overall progress)
			// This ensures smooth color transitions regardless of progress jumps
			var segmentColor lipgloss.Color
			
			if progressPos < 0.33 {
				// 0-33% of bar: Blue gradient
				segmentColor = blueGradient.GetColor(progressPos * 3) // Scale to use full blue range
			} else if progressPos < 0.66 {
				// 33-66% of bar: Purple gradient  
				segmentColor = purpleGradient.GetColor((progressPos - 0.33) * 3)
			} else {
				// 66-100% of bar: Green gradient
				segmentColor = greenGradient.GetColor((progressPos - 0.66) * 3)
			}
			
			// Cylon overlay effect with neutral highlight
			if i == cylonPos || i == cylonPos+1 {
				segments = append(segments, lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Render("▓"))
			} else {
				segments = append(segments, lipgloss.NewStyle().Foreground(segmentColor).Render("█"))
			}
		} else {
			// Empty portion with subtle cylon highlighting
			if i == cylonPos || i == cylonPos+1 {
				segments = append(segments, lipgloss.NewStyle().Foreground(blueGradient.Start).Render("▒"))
			} else {
				segments = append(segments, lipgloss.NewStyle().Foreground(dimColor).Render("░"))
			}
		}
	}
	
	bar := strings.Join(segments, "")
	
	// Enhanced progress text with floating percentage
	var percentageStyle lipgloss.Style
	if m.progress < 0.33 {
		percentageStyle = lipgloss.NewStyle().Foreground(blueGradient.End).Bold(true)
	} else if m.progress < 0.66 {
		percentageStyle = lipgloss.NewStyle().Foreground(purpleGradient.End).Bold(true)
	} else {
		percentageStyle = lipgloss.NewStyle().Foreground(greenGradient.End).Bold(true)
	}
	
	emoji := getProgressEmoji(message, m.progress)
	progressText := fmt.Sprintf("%s %s %s", emoji, bar, percentageStyle.Render(percentage))
	
	return lipgloss.NewStyle().
		Align(lipgloss.Center).
		Render(progressText)
}

// Render header with beautiful ASCII art
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

// Render help text
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

// Progress message type
type progressMsg struct{}

// Tick message for progress updates
type tickMsg time.Time
