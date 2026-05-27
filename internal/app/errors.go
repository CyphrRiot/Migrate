// Package app provides application-level bootstrap logic for Migrate.
package app

import (
	"strings"
)

// CreateResponsiveErrorBox creates a terminal-responsive error box.
func CreateResponsiveErrorBox(title, message string, details []string) string {
	termWidth, _ := GetTerminalSize()

	boxWidth := termWidth - 4
	if boxWidth < 40 {
		boxWidth = 40
	}
	if boxWidth > 80 {
		boxWidth = 80
	}

	var lines []string
	contentWidth := boxWidth - 6

	titlePadding := (boxWidth - len(title) - 2) / 2
	if titlePadding < 1 {
		titlePadding = 1
	}
	titleLine := "│" + strings.Repeat(" ", titlePadding) + title + strings.Repeat(" ", boxWidth-len(title)-titlePadding-2) + "│"

	lines = append(lines, "┌"+strings.Repeat("─", boxWidth-2)+"┐")
	lines = append(lines, titleLine)
	lines = append(lines, "├"+strings.Repeat("─", boxWidth-2)+"┤")
	lines = append(lines, "│"+strings.Repeat(" ", boxWidth-2)+"│")

	if message != "" {
		wrappedMessage := WrapText(message, contentWidth)
		for _, line := range wrappedMessage {
			padding := contentWidth - len(line)
			lines = append(lines, "│  "+line+strings.Repeat(" ", padding)+"  │")
		}
		lines = append(lines, "│"+strings.Repeat(" ", boxWidth-2)+"│")
	}

	for _, detail := range details {
		wrappedDetail := WrapText(detail, contentWidth)
		for _, line := range wrappedDetail {
			padding := contentWidth - len(line)
			lines = append(lines, "│  "+line+strings.Repeat(" ", padding)+"  │")
		}
	}

	lines = append(lines, "│"+strings.Repeat(" ", boxWidth-2)+"│")
	lines = append(lines, "└"+strings.Repeat("─", boxWidth-2)+"┘")

	return strings.Join(lines, "\n")
}

// WrapText wraps text to fit within the specified width.
func WrapText(text string, width int) []string {
	if width < 10 {
		width = 10
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}

	var lines []string
	var currentLine string

	for _, word := range words {
		if len(currentLine) == 0 {
			currentLine = word
		} else if len(currentLine)+1+len(word) <= width {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}

	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}

	return lines
}
