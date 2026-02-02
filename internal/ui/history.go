// internal/ui/history.go
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"roundtable/internal/db"
)

// ViewMode represents the current view state
type ViewMode int

const (
	ViewNormal ViewMode = iota
	ViewHistory
)

// HistoryState holds the state for the history browser
type HistoryState struct {
	debates   []db.Debate
	cursor    int
	scrollTop int
	maxHeight int
}

// NewHistoryState creates a new history state
func NewHistoryState() *HistoryState {
	return &HistoryState{
		debates:   nil,
		cursor:    0,
		scrollTop: 0,
		maxHeight: 20, // default, will be updated based on terminal size
	}
}

// Up moves the cursor up
func (h *HistoryState) Up() {
	if h.cursor > 0 {
		h.cursor--
		// Adjust scroll if cursor goes above visible area
		if h.cursor < h.scrollTop {
			h.scrollTop = h.cursor
		}
	}
}

// Down moves the cursor down
func (h *HistoryState) Down() {
	if h.cursor < len(h.debates)-1 {
		h.cursor++
		// Adjust scroll if cursor goes below visible area
		if h.cursor >= h.scrollTop+h.maxHeight {
			h.scrollTop = h.cursor - h.maxHeight + 1
		}
	}
}

// Selected returns the currently selected debate, or nil if none
func (h *HistoryState) Selected() *db.Debate {
	if h.cursor >= 0 && h.cursor < len(h.debates) {
		return &h.debates[h.cursor]
	}
	return nil
}

// LoadDebates loads debates from the database
func (h *HistoryState) LoadDebates(store *db.Store) error {
	if store == nil {
		return fmt.Errorf("database not available")
	}
	debates, err := store.ListDebates()
	if err != nil {
		return err
	}
	h.debates = debates
	h.cursor = 0
	h.scrollTop = 0
	return nil
}

// SetMaxHeight updates the max visible height
func (h *HistoryState) SetMaxHeight(height int) {
	h.maxHeight = height - 10 // Leave room for header/footer
	if h.maxHeight < 5 {
		h.maxHeight = 5
	}
}

// Render renders the history browser overlay
func (h *HistoryState) Render(width, height int) string {
	var content strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(Cyan).
		Render("DEBATE HISTORY")
	content.WriteString(title)
	content.WriteString("\n")
	content.WriteString(DimStyle.Render("Select a past debate to resume"))
	content.WriteString("\n\n")

	if len(h.debates) == 0 {
		content.WriteString(DimStyle.Render("No past debates found."))
		content.WriteString("\n\n")
		content.WriteString(DimStyle.Render("Start a new debate and it will appear here."))
	} else {
		// Calculate visible range
		visibleEnd := h.scrollTop + h.maxHeight
		if visibleEnd > len(h.debates) {
			visibleEnd = len(h.debates)
		}

		// Header row
		header := fmt.Sprintf("  %-8s  %-20s  %-10s  %-19s  %s",
			"ID", "Name", "Status", "Updated", "Messages")
		content.WriteString(DimStyle.Render(header))
		content.WriteString("\n")
		content.WriteString(DimStyle.Render(strings.Repeat("-", 75)))
		content.WriteString("\n")

		// Debate rows
		for i := h.scrollTop; i < visibleEnd; i++ {
			d := h.debates[i]

			// Truncate name if too long
			name := d.Name
			if len(name) > 18 {
				name = name[:18] + ".."
			}

			// Format time
			timeStr := d.UpdatedAt.Format("2006-01-02 15:04")
			if time.Since(d.UpdatedAt) < 24*time.Hour {
				timeStr = d.UpdatedAt.Format("Today 15:04")
			}

			// Status with color
			var statusStyle lipgloss.Style
			switch d.Status {
			case "active":
				statusStyle = StatusOK
			case "resolved":
				statusStyle = lipgloss.NewStyle().Foreground(Green)
			case "abandoned":
				statusStyle = DimStyle
			default:
				statusStyle = DimStyle
			}

			// Build the line
			cursor := "  "
			lineStyle := DimStyle
			if i == h.cursor {
				cursor = "> "
				lineStyle = lipgloss.NewStyle().Foreground(Cyan)
			}

			statusStr := statusStyle.Width(10).Render(d.Status)
			line := fmt.Sprintf("%-8s  %-20s  %s  %-19s",
				d.ID[:8], name, statusStr, timeStr)

			content.WriteString(cursor)
			content.WriteString(lineStyle.Render(line))
			content.WriteString("\n")
		}

		// Scroll indicator
		if len(h.debates) > h.maxHeight {
			scrollInfo := fmt.Sprintf("Showing %d-%d of %d",
				h.scrollTop+1, visibleEnd, len(h.debates))
			content.WriteString("\n")
			content.WriteString(DimStyle.Render(scrollInfo))
		}
	}

	// Footer with keybindings
	content.WriteString("\n\n")
	footer := DimStyle.Render("Up/Down: Navigate | Enter: Resume | Esc: Cancel")
	content.WriteString(footer)

	// Build the overlay box
	overlayStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Cyan).
		Padding(1, 2).
		MaxWidth(width - 10).
		MaxHeight(height - 4)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		overlayStyle.Render(content.String()),
	)
}

// ResumeDebate loads a debate from the database into a Debate struct
func ResumeDebate(store *db.Store, debateID string) (*Debate, error) {
	if store == nil {
		return nil, fmt.Errorf("database not available")
	}

	// Get the debate metadata
	dbDebate, err := store.GetDebate(debateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get debate: %w", err)
	}

	// Create the UI Debate struct
	debate := NewDebate(dbDebate.ID, dbDebate.Name)
	debate.ProjectPath = dbDebate.ProjectPath
	debate.Paused = dbDebate.Status != "active"

	// Load messages from database
	messages, err := store.GetMessages(debateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	// Populate debate.Messages from stored messages
	for _, msg := range messages {
		debate.Messages = append(debate.Messages, DebateMessage{
			Source:    msg.Source,
			Content:   msg.Content,
			Timestamp: msg.CreatedAt,
		})
	}

	// Load context files
	contextFiles, err := store.GetContextFiles(debateID)
	if err != nil {
		// Non-fatal, just log and continue
		return debate, nil
	}

	for _, cf := range contextFiles {
		debate.ContextFiles[cf.Path] = cf.Content
	}

	return debate, nil
}
