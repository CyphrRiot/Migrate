package internal

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	// Enhanced color palette - Tokyo Night inspired
	primaryColor    = lipgloss.Color("#7aa2f7") // Tokyo Night blue
	secondaryColor  = lipgloss.Color("#9ece6a") // Tokyo Night green
	accentColor     = lipgloss.Color("#f7768e") // Tokyo Night red/pink
	warningColor    = lipgloss.Color("#e0af68") // Tokyo Night yellow
	errorColor      = lipgloss.Color("#f7768e") // Tokyo Night red
	successColor    = lipgloss.Color("#9ece6a") // Tokyo Night green
	textColor       = lipgloss.Color("#c0caf5") // Tokyo Night foreground
	dimColor        = lipgloss.Color("#565f89") // Tokyo Night comment
	backgroundColor = lipgloss.Color("#1a1b26") // Tokyo Night background
	borderColor     = lipgloss.Color("#414868") // Tokyo Night border

	// Enhanced base styles
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
			MarginBottom(2)

	menuItemStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			PaddingRight(2).
			Foreground(textColor)

	// Menu selection styles - beautiful borders WITHOUT any margins/shadows!
	selectedMenuItemStyle = lipgloss.NewStyle().
				PaddingLeft(2).
				PaddingRight(2).
				Background(primaryColor).
				Foreground(backgroundColor).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor)

	inactiveMenuItemStyle = menuItemStyle.Copy().
				Foreground(dimColor)

	// Enhanced border WITHOUT background
	borderStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(2, 3).
			Margin(1)

	// Enhanced warning with background
	warningStyle = lipgloss.NewStyle().
			Foreground(backgroundColor).
			Background(warningColor).
			Bold(true).
			Align(lipgloss.Center).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(warningColor)

	errorStyle = lipgloss.NewStyle().
			Foreground(backgroundColor).
			Background(errorColor).
			Bold(true).
			Align(lipgloss.Center).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(errorColor)

	successStyle = lipgloss.NewStyle().
			Foreground(backgroundColor).
			Background(successColor).
			Bold(true).
			Align(lipgloss.Center).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(successColor)

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

	// Info box styles
	infoBoxStyle = lipgloss.NewStyle().
			Background(borderColor).
			Foreground(textColor).
			Padding(0, 1).
			Margin(0).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(dimColor)
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

	// Header
	s.WriteString(titleStyle.Render("🔄 Restore Options") + "\n\n")

	// Menu options
	for i, choice := range m.choices {
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

	// Warning text
	warning := warningStyle.Render("⚠️  WARNING: Restore operations will overwrite existing files!")
	s.WriteString("\n" + warning)

	// Help text
	help := m.renderHelp()
	s.WriteString("\n" + help)

	// Center the content
	content := borderStyle.Width(m.width - 4).Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// Render about screen
func (m Model) renderAbout() string {
	var s strings.Builder

	// Header
	s.WriteString(titleStyle.Render("ℹ️ About Migrate") + "\n\n")

	// About content
	about := GetAboutText() + `

Created by: ` + AppAuthor + `

🔗 Links:
   GitHub: https://github.com/CyphrRiot/Migrate
   X:      https://x.com/CyphrRiot

Powered by: Bubble Tea & Lipgloss

Features:
• Complete 1:1 system backups
• Interactive TUI interface
• Real-time progress tracking
• Safe backup verification
• Cross-platform compatibility

Original bash version: ~/.local/bin/migrate
New Go version with beautiful TUI interface

Press any key to return to main menu`

	info := lipgloss.NewStyle().
		Foreground(textColor).
		Margin(0, 2).
		Align(lipgloss.Left).
		Render(about)

	s.WriteString(info)

	// Center the content
	content := borderStyle.Width(m.width - 4).Render(s.String())
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

// Render progress bar with beautiful styling and cylon animation
func (m Model) renderProgressBar() string {
	width := 50
	
	// Check if this is indeterminate progress (-1)
	if m.progress < 0 {
		// Create animated spinner-like progress bar
		// Use time to create a moving animation
		now := time.Now().Unix()
		pos := int(now/1) % width // Move every second
		
		var bar strings.Builder
		for i := 0; i < width; i++ {
			if i == pos || i == (pos+1)%width || i == (pos+2)%width {
				bar.WriteString("█")
			} else {
				bar.WriteString("░")
			}
		}
		
		// Indeterminate progress shows no percentage
		progressText := fmt.Sprintf("Progress: [%s] Working...", bar.String())
		
		return lipgloss.NewStyle().
			Foreground(primaryColor).
			Align(lipgloss.Center).
			Render(progressText)
	}
	
	// Normal percentage-based progress WITH cylon animation overlay
	percentage := fmt.Sprintf("%.2f%%", m.progress*100)
	filled := int(m.progress * float64(width))
	
	// Calculate cylon position (sweeps back and forth)
	cylonPos := m.cylonFrame
	if cylonPos >= 10 {
		cylonPos = 20 - cylonPos // Reverse direction for second half
	}
	cylonPos = cylonPos * width / 10 // Scale to progress bar width
	
	var bar strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			// Filled portion
			if i == cylonPos || i == cylonPos+1 {
				bar.WriteString("▓") // Cylon highlight on filled area
			} else {
				bar.WriteString("█") // Normal filled
			}
		} else {
			// Empty portion  
			if i == cylonPos || i == cylonPos+1 {
				bar.WriteString("▒") // Cylon highlight on empty area
			} else {
				bar.WriteString("░") // Normal empty
			}
		}
	}
	
	progressText := fmt.Sprintf("Progress: [%s] %s", bar.String(), percentage)
	
	return lipgloss.NewStyle().
		Foreground(primaryColor).
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
		info := infoBoxStyle.Render("Select a drive to mount. LUKS encrypted drives will prompt for password.")
		s.WriteString(info + "\n\n")

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

// Progress message type
type progressMsg struct{}

// Tick message for progress updates
type tickMsg time.Time
