// internal/models/types.go
package models

import "time"

// Chunk represents a piece of streaming response
type Chunk struct {
	Text      string
	Done      bool
	Error     error
	IsTimeout bool // Distinguishes timeout from other errors
}

// Message represents a message in the debate
type Message struct {
	Source    string    // claude, gpt, gemini, grok, user, system
	Content   string
	Type      string    // model, user, system, tool, meta
	Timestamp time.Time
	ToolName  string    // for tool messages
}

// ModelStatus represents the current state of a model
type ModelStatus int

const (
	StatusIdle ModelStatus = iota
	StatusResponding
	StatusWaiting
	StatusError
	StatusTimeout
)

func (s ModelStatus) String() string {
	switch s {
	case StatusIdle:
		return "idle"
	case StatusResponding:
		return "responding"
	case StatusWaiting:
		return "waiting"
	case StatusError:
		return "error"
	case StatusTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// ModelInfo contains display information for a model
type ModelInfo struct {
	ID       string // claude, gpt, gemini, grok
	Name     string // Display name
	Color    string // Hex color for UI
	CanExec  bool   // Can execute tools
	CanRead  bool   // Can read files
}
