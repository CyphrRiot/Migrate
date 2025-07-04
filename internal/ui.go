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
		Start: lipgloss.Color("#4facfe"), // Light blue
		End:   lipgloss.Color("#00c6ff"), // Bright cyan
	}
	
	purpleGradient = GradientColors{
		Start: lipgloss.Color("#a855f7"), // Purple
		End:   lipgloss.Color("#8b5cf6"), // Violet
	}
	
	greenGradient = GradientColors{
		Start: lipgloss.Color("#10b981"), // Emerald
		End:   lipgloss.Color("#06b6d4"), // Cyan
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
	successGradient = greenGradient
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

// ASCII art for the program name
const asciiArt = `▖  ▖▘      ▗   
▛▖▞▌▌▛▌▛▘▀▌▜▘█▌
▌▝ ▌▌▙▌▌ █▌▐▖▙▖
     ▄▌        `

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
	ascii := asciiStyle.Render(asciiArt)
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
	ascii := asciiStyle.Render(asciiArt)
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
	ascii := asciiStyle.Render(asciiArt)
	s.WriteString(ascii + "\n")
	s.WriteString(titleStyle.Render("ℹ️ About Migrate") + "\n\n")

	// About content
	about := GetAboutText() + `

Created by ` + AppAuthor + `

🔗 GitHub: https://github.com/CyphrRiot/Migrate
🐦 X: https://x.com/CyphrRiot

Powered by Bubble Tea & Lipgloss

Features:
• Pure Go implementation with zero dependencies
• Beautiful TUI with Tokyo Night theming
• Complete system or home directory backup
• Real-time progress with file-based tracking
• Smart sync with SHA256 deduplication
• LUKS encrypted drive support
• rsync --delete equivalent functionality
• Static binary for maximum portability

Press any key to return to main menu`

	info := lipgloss.NewStyle().
		Foreground(textColor).
		Margin(0, 2).
		Align(lipgloss.Left).
		Render(about)

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
	ascii := asciiStyle.Render(asciiArt)
	s.WriteString(ascii + "\n")
	title := titleStyle.Render(AppDesc)
	s.WriteString(title + "\n")
	subtitle := subtitleStyle.Render(GetSubtitle())
	s.WriteString(subtitle + "\n\n")

	// Operation title
	if m.canceling {
		s.WriteString(titleStyle.Render("🛑 Canceling Operation") + "\n\n")
	} else {
		s.WriteString(titleStyle.Render("🔄 Operation in Progress") + "\n\n")
	}

	// Operation info - show source and destination drives  
	logPath := getLogFilePath() // Get log path for display
	if m.operation == "system_backup" {
		s.WriteString("📁 Backup Type: Complete System (1:1)\n")
		s.WriteString("📂 Source: / (Internal Drive)\n")
		s.WriteString("💾 Destination: " + m.selectedDrive + " (External Drive)\n")
		s.WriteString("📋 Log: " + logPath + "\n\n")
	} else if m.operation == "home_backup" {
		s.WriteString("📁 Backup Type: Home Directory Only\n")
		s.WriteString("📂 Source: ~/ (Home Directory)\n")  
		s.WriteString("💾 Destination: " + m.selectedDrive + " (External Drive)\n")
		s.WriteString("📋 Log: " + logPath + "\n\n")
	} else {
		opInfo := fmt.Sprintf("Running: %s", m.operation)
		s.WriteString(subtitleStyle.Render(opInfo) + "\n")
		s.WriteString("📋 Log: " + logPath + "\n\n")
	}

	// Progress bar (only show if not canceling)
	if !m.canceling {
		progressBar := m.renderProgressBar()
		s.WriteString(progressBar + "\n\n")
	}

	// Status message
	if m.message != "" {
		var statusStyle lipgloss.Style
		if m.canceling || strings.Contains(m.message, "Cancel") {
			statusStyle = warningStyle
		} else {
			statusStyle = lipgloss.NewStyle().
				Foreground(secondaryColor).
				Align(lipgloss.Center)
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

// Render gradient progress bar with btop-inspired modern styling
func (m Model) renderProgressBar() string {
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
		progressText := fmt.Sprintf(" %s Working...", bar)
		
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
	cylonPos = cylonPos * width / 10 // Scale to progress bar width
	
	// Create gradient segments based on progress
	var segments []string
	for i := 0; i < width; i++ {
		progressPos := float64(i) / float64(width)
		
		if i < filled {
			// Filled portion - gradient based on progress percentage
			var segmentColor lipgloss.Color
			
			if m.progress < 0.33 {
				// 0-33%: Blue gradient
				segmentColor = blueGradient.GetColor(progressPos * 3) // Scale to use full blue range
			} else if m.progress < 0.66 {
				// 33-66%: Purple gradient  
				segmentColor = purpleGradient.GetColor((progressPos - 0.33) * 3)
			} else {
				// 66-100%: Green gradient
				segmentColor = greenGradient.GetColor((progressPos - 0.66) * 3)
			}
			
			// Cylon overlay effect
			if i == cylonPos || i == cylonPos+1 {
				segments = append(segments, lipgloss.NewStyle().Foreground(tealGradient.End).Render("▓"))
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
	
	progressText := fmt.Sprintf(" %s %s", bar, percentageStyle.Render(percentage))
	
	return lipgloss.NewStyle().
		Align(lipgloss.Center).
		Render(progressText)
}

// Render header with beautiful ASCII art
func (m Model) renderHeader() string {
	ascii := asciiStyle.Render(asciiArt)
	title := titleStyle.Render(AppDesc)
	subtitle := subtitleStyle.Render(GetSubtitle())
	
	return ascii + "\n" + title + "\n" + subtitle
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

	// Header
	s.WriteString(titleStyle.Render("❌ Error") + "\n\n")

	// Error message with enhanced styling
	errorMsg := errorStyle.Render(m.message)
	s.WriteString(errorMsg + "\n\n")

	// Help text - emphasize manual dismissal
	help := helpStyle.Render("📖 Please read the instructions above • Press ESC or any key when ready to continue")
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

// Progress message type
type progressMsg struct{}

// Tick message for progress updates
type tickMsg time.Time
