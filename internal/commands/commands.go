// Package commands handles slash command parsing for the roundtable TUI.
package commands

import (
	"strings"
)

// Command interface for all command types
type Command interface {
	Type() string
}

// Help returns help text
type Help struct{}

func (Help) Type() string { return "help" }

// NewDebate creates a new debate
type NewDebate struct {
	Name string
}

func (NewDebate) Type() string { return "new" }

// CloseDebate closes the current debate
type CloseDebate struct{}

func (CloseDebate) Type() string { return "close" }

// RenameDebate renames the current debate
type RenameDebate struct {
	Name string
}

func (RenameDebate) Type() string { return "rename" }

// AddContext adds a context file/path
type AddContext struct {
	Path string
}

func (AddContext) Type() string { return "context_add" }

// RemoveContext removes a context file/path
type RemoveContext struct {
	Path string
}

func (RemoveContext) Type() string { return "context_remove" }

// ListContext lists all context files
type ListContext struct{}

func (ListContext) Type() string { return "context_list" }

// ToggleModels toggles model selection panel
type ToggleModels struct{}

func (ToggleModels) Type() string { return "models" }

// ForceConsensus forces a consensus check
type ForceConsensus struct{}

func (ForceConsensus) Type() string { return "consensus" }

// Execute executes a tool or action
type Execute struct{}

func (Execute) Type() string { return "execute" }

// Pause pauses the current debate
type Pause struct{}

func (Pause) Type() string { return "pause" }

// Resume resumes a paused debate
type Resume struct{}

func (Resume) Type() string { return "resume" }

// ShowHistory shows debate history
type ShowHistory struct{}

func (ShowHistory) Type() string { return "history" }

// Export exports the current debate
type Export struct{}

func (Export) Type() string { return "export" }

// ParseError represents a command parsing error
type ParseError struct {
	Message string
}

func (ParseError) Type() string { return "error" }

// Parse parses user input and returns the appropriate Command.
// Returns nil if the input is not a slash command.
func Parse(input string) Command {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return nil
	}

	// Split into command and arguments
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "/help":
		return Help{}

	case "/new":
		name := strings.Join(args, " ")
		return NewDebate{Name: name}

	case "/close":
		return CloseDebate{}

	case "/rename":
		name := strings.Join(args, " ")
		if name == "" {
			return ParseError{Message: "/rename requires a name"}
		}
		return RenameDebate{Name: name}

	case "/context":
		if len(args) == 0 {
			return ParseError{Message: "/context requires a subcommand: add, remove, or list"}
		}
		subCmd := strings.ToLower(args[0])
		subArgs := args[1:]

		switch subCmd {
		case "add":
			if len(subArgs) == 0 {
				return ParseError{Message: "/context add requires a path"}
			}
			path := strings.Join(subArgs, " ")
			return AddContext{Path: path}
		case "remove":
			if len(subArgs) == 0 {
				return ParseError{Message: "/context remove requires a path"}
			}
			path := strings.Join(subArgs, " ")
			return RemoveContext{Path: path}
		case "list":
			return ListContext{}
		default:
			return ParseError{Message: "unknown context subcommand: " + subCmd}
		}

	case "/models":
		return ToggleModels{}

	case "/consensus":
		return ForceConsensus{}

	case "/execute":
		return Execute{}

	case "/pause":
		return Pause{}

	case "/resume":
		return Resume{}

	case "/history":
		return ShowHistory{}

	case "/export":
		return Export{}

	default:
		return ParseError{Message: "unknown command: " + cmd}
	}
}

// HelpText returns the help text for all available commands.
func HelpText() string {
	return `Available commands:
  /help                  - Show this help
  /new [name]            - Start a new debate
  /close                 - Close the current debate
  /rename <name>         - Rename the current debate
  /context add <path>    - Add a file/directory as context
  /context remove <path> - Remove a context file/directory
  /context list          - List all context files
  /models                - Toggle model selection panel
  /consensus             - Force a consensus check
  /execute               - Execute the agreed-upon action
  /pause                 - Pause the current debate
  /resume                - Resume a paused debate
  /history               - Show debate history
  /export                - Export the current debate`
}
