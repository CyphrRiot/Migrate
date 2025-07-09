// Package internal provides Unicode symbol definitions with fallback support for cross-platform compatibility.
//
// This module ensures consistent visual representation across different terminals and systems
// by providing ASCII fallbacks for Unicode symbols that may not render properly on all platforms.
package internal

import (
	"os"
	"strings"
)

// SymbolSet defines a collection of symbols used throughout the UI
type SymbolSet struct {
	// Status indicators
	Success string
	Error   string
	Warning string
	Info    string
	Search  string

	// File and folder icons
	Folder string
	File   string
	Drive  string

	// Progress indicators
	Progress []string // Animation frames
	Bullet   string
	Arrow    string
	Check    string
	Cross    string

	// UI elements
	BoxTopLeft     string
	BoxTopRight    string
	BoxBottomLeft  string
	BoxBottomRight string
	BoxHorizontal  string
	BoxVertical    string

	// Misc
	Sparkle string
	Party   string
	Home    string
	Chart   string
}

// UnicodeSymbols provides rich Unicode symbols for modern terminals
var UnicodeSymbols = SymbolSet{
	// Status indicators
	Success: "✓",
	Error:   "✗",
	Warning: "⚠️",
	Info:    "🔍",
	Search:  "🔍",

	// File and folder icons
	Folder: "📁",
	File:   "📄",
	Drive:  "💾",

	// Progress indicators
	Progress: []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"},
	Bullet:   "•",
	Arrow:    "➜",
	Check:    "✓",
	Cross:    "❌",

	// UI elements (using rounded corners)
	BoxTopLeft:     "╭",
	BoxTopRight:    "╮",
	BoxBottomLeft:  "╰",
	BoxBottomRight: "╯",
	BoxHorizontal:  "─",
	BoxVertical:    "│",

	// Misc
	Sparkle: "✨",
	Party:   "🎉",
	Home:    "🏠",
	Chart:   "📊",
}

// ASCIISymbols provides ASCII-only fallbacks for compatibility
var ASCIISymbols = SymbolSet{
	// Status indicators
	Success: "[OK]",
	Error:   "[X]",
	Warning: "[!]",
	Info:    "[i]",
	Search:  "[?]",

	// File and folder icons
	Folder: "[D]",
	File:   "[F]",
	Drive:  "[HD]",

	// Progress indicators
	Progress: []string{"|", "/", "-", "\\"},
	Bullet:   "*",
	Arrow:    "->",
	Check:    "[v]",
	Cross:    "[X]",

	// UI elements (using ASCII box drawing)
	BoxTopLeft:     "+",
	BoxTopRight:    "+",
	BoxBottomLeft:  "+",
	BoxBottomRight: "+",
	BoxHorizontal:  "-",
	BoxVertical:    "|",

	// Misc
	Sparkle: "*",
	Party:   "*!*",
	Home:    "[H]",
	Chart:   "[%]",
}

// CurrentSymbols holds the active symbol set based on terminal capabilities
var CurrentSymbols SymbolSet

// init determines which symbol set to use based on environment
func init() {
	CurrentSymbols = detectSymbolSet()
}

// detectSymbolSet determines the appropriate symbol set based on terminal capabilities
func detectSymbolSet() SymbolSet {
	// Check for explicit ASCII mode via environment variable
	if os.Getenv("MIGRATE_ASCII") == "1" || os.Getenv("MIGRATE_ASCII") == "true" {
		return ASCIISymbols
	}

	// Check TERM environment variable for known problematic terminals
	term := strings.ToLower(os.Getenv("TERM"))
	if term == "dumb" || term == "vt100" || strings.HasPrefix(term, "xterm-mono") {
		return ASCIISymbols
	}

	// Check for Windows Console (cmd.exe) which has limited Unicode support
	if os.Getenv("COMSPEC") != "" && !isWindowsTerminal() {
		return ASCIISymbols
	}

	// Check for SSH connections which might have encoding issues
	if os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != "" {
		// Only use ASCII for SSH if locale doesn't support UTF-8
		locale := strings.ToLower(os.Getenv("LANG"))
		if !strings.Contains(locale, "utf-8") && !strings.Contains(locale, "utf8") {
			return ASCIISymbols
		}
	}

	// Default to Unicode for modern terminals
	return UnicodeSymbols
}

// isWindowsTerminal detects if running in Windows Terminal (which supports Unicode well)
func isWindowsTerminal() bool {
	// Windows Terminal sets this environment variable
	return os.Getenv("WT_SESSION") != ""
}

// ForceASCII switches to ASCII symbols regardless of terminal detection
func ForceASCII() {
	CurrentSymbols = ASCIISymbols
}

// ForceUnicode switches to Unicode symbols regardless of terminal detection
func ForceUnicode() {
	CurrentSymbols = UnicodeSymbols
}

// GetProgressFrame returns the current progress animation frame
func GetProgressFrame(tick int) string {
	frames := CurrentSymbols.Progress
	if len(frames) == 0 {
		return ""
	}
	return frames[tick%len(frames)]
}

// FormatSuccess formats a success message with the appropriate symbol
func FormatSuccess(message string) string {
	return CurrentSymbols.Success + " " + message
}

// FormatError formats an error message with the appropriate symbol
func FormatError(message string) string {
	return CurrentSymbols.Error + " " + message
}

// FormatWarning formats a warning message with the appropriate symbol
func FormatWarning(message string) string {
	return CurrentSymbols.Warning + " " + message
}

// FormatInfo formats an info message with the appropriate symbol
func FormatInfo(message string) string {
	return CurrentSymbols.Info + " " + message
}

// FormatFolder formats a folder name with the appropriate symbol
func FormatFolder(name string) string {
	return CurrentSymbols.Folder + " " + name
}

// FormatDrive formats a drive identifier with the appropriate symbol
func FormatDrive(identifier string) string {
	return CurrentSymbols.Drive + " " + identifier
}
