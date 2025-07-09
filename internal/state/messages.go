package state

import "time"

// CylonAnimateMsg is sent to trigger cylon animation updates during progress display.
// This message is sent periodically during progress display to create the sweeping animation.
type CylonAnimateMsg struct{}

// ProgressMsg represents a progress update message
type ProgressMsg struct {
	Percentage float64
	Message    string
	Done       bool
}

// ErrorMsg represents an error message that may require dismissal
type ErrorMsg struct {
	Message               string
	RequiresManualDismiss bool
}

// CompletionMsg represents a successful operation completion
type CompletionMsg struct {
	Message string
}

// CancelMsg represents a cancellation request
type CancelMsg struct{}

// TimeoutMsg represents a timeout event
type TimeoutMsg struct {
	Time time.Time
}
