// internal/ui/debate.go
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"roundtable/internal/models"
)

// DebateMessage represents a message in the debate
type DebateMessage struct {
	Source    string    // claude, gpt, gemini, grok, user, system, error
	Content   string
	Timestamp time.Time
	IsError   bool      // If true, render in error style
	IsTimeout bool      // If true, this is specifically a timeout error
}

// Debate represents a single debate session
type Debate struct {
	ID           string
	Name         string
	ProjectPath  string
	CreatedAt    time.Time
	Messages     []DebateMessage
	ContextFiles map[string]string // path -> content
	Paused       bool

	// Model states
	ModelStatus    map[string]models.ModelStatus
	ModelStartTime map[string]time.Time // When each model started responding
	AnimationFrame int                   // For streaming indicator animation
}

func NewDebate(id, name string) *Debate {
	return &Debate{
		ID:             id,
		Name:           name,
		CreatedAt:      time.Now(),
		Messages:       []DebateMessage{},
		ContextFiles:   make(map[string]string),
		ModelStatus:    make(map[string]models.ModelStatus),
		ModelStartTime: make(map[string]time.Time),
		AnimationFrame: 0,
	}
}

// UpdateModelStatus updates a model's status and tracks timing
func (d *Debate) UpdateModelStatus(modelID string, status models.ModelStatus) {
	oldStatus := d.ModelStatus[modelID]
	d.ModelStatus[modelID] = status

	// Track when model starts responding
	if status == models.StatusResponding && oldStatus != models.StatusResponding {
		d.ModelStartTime[modelID] = time.Now()
	}

	// Clear start time when model stops responding
	if status != models.StatusResponding && oldStatus == models.StatusResponding {
		delete(d.ModelStartTime, modelID)
	}
}

// TickAnimation advances the streaming indicator animation
func (d *Debate) TickAnimation() {
	d.AnimationFrame = (d.AnimationFrame + 1) % 4
}

// streamingIndicator returns the current animation frame for streaming
func (d *Debate) streamingIndicator() string {
	frames := []string{"", ".", "..", "..."}
	return frames[d.AnimationFrame]
}

// formatElapsedTime formats duration in a human-readable way
func formatElapsedTime(elapsed time.Duration) string {
	if elapsed < time.Second {
		return "<1s"
	}
	if elapsed < time.Minute {
		return fmt.Sprintf("%ds", int(elapsed.Seconds()))
	}
	mins := int(elapsed.Minutes())
	secs := int(elapsed.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", mins, secs)
}

func (d *Debate) AddMessage(source, content string) {
	d.Messages = append(d.Messages, DebateMessage{
		Source:    source,
		Content:   content,
		Timestamp: time.Now(),
	})
}

// AddErrorMessage adds an error message that will be rendered in red
func (d *Debate) AddErrorMessage(source, content string, isTimeout bool) {
	d.Messages = append(d.Messages, DebateMessage{
		Source:    source,
		Content:   content,
		Timestamp: time.Now(),
		IsError:   true,
		IsTimeout: isTimeout,
	})
}

func (d *Debate) RenderMessages(width int) string {
	var sb strings.Builder

	// Account for indent (2 spaces) and some padding
	contentWidth := width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}

	for _, msg := range d.Messages {
		ts := msg.Timestamp.Format("15:04")

		// Use error style for error messages, otherwise model style
		var style lipgloss.Style
		var header string

		if msg.IsError {
			style = ErrorStyle
			errorType := "Error"
			if msg.IsTimeout {
				errorType = "Timeout"
			}
			header = style.Render(fmt.Sprintf("[%s] %s %s:", ts, formatSource(msg.Source), errorType))
		} else {
			style = ModelStyle(msg.Source)
			header = style.Render(fmt.Sprintf("[%s] %s:", ts, formatSource(msg.Source)))
		}

		sb.WriteString(header)
		sb.WriteString("\n")

		// Message content with indent and word wrapping
		lines := strings.Split(msg.Content, "\n")
		for _, line := range lines {
			// Word wrap each line
			wrapped := wordWrap(line, contentWidth)
			for _, wline := range wrapped {
				sb.WriteString("  ")
				if msg.IsError {
					sb.WriteString(ErrorStyle.Render(wline))
				} else {
					sb.WriteString(wline)
				}
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// wordWrap wraps text to fit within the specified width
func wordWrap(text string, width int) []string {
	if width <= 0 || len(text) <= width {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	currentLine := words[0]
	for _, word := range words[1:] {
		if len(currentLine)+1+len(word) <= width {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	lines = append(lines, currentLine)

	return lines
}

func formatSource(source string) string {
	switch source {
	case "claude":
		return "Claude"
	case "gpt":
		return "GPT"
	case "gemini":
		return "Gemini"
	case "grok":
		return "Grok"
	case "user":
		return "You"
	case "system":
		return "System"
	default:
		return source
	}
}

// RenderModelStatus renders the model status sidebar
func (d *Debate) RenderModelStatus(modelIDs []string, height int) string {
	var sb strings.Builder

	sb.WriteString(TitleStyle.Render("MODELS"))
	sb.WriteString("\n\n")

	for _, id := range modelIDs {
		status := d.ModelStatus[id]
		indicator := statusIndicator(status)
		style := ModelStyle(id)

		name := formatSource(id)

		// Build status line with optional elapsed time
		var statusLine string
		if status == models.StatusResponding {
			// Add animated streaming indicator
			name += d.streamingIndicator()

			// Add elapsed time if available
			if startTime, ok := d.ModelStartTime[id]; ok {
				elapsed := time.Since(startTime)
				elapsedStr := formatElapsedTime(elapsed)
				statusLine = fmt.Sprintf("%s %s %s",
					indicator,
					style.Render(name),
					DimStyle.Render(fmt.Sprintf("(%s)", elapsedStr)))
			} else {
				statusLine = fmt.Sprintf("%s %s", indicator, style.Render(name))
			}
		} else {
			statusLine = fmt.Sprintf("%s %s", indicator, style.Render(name))
		}

		sb.WriteString(statusLine)
		sb.WriteString("\n")
	}

	return sb.String()
}

func statusIndicator(status models.ModelStatus) string {
	switch status {
	case models.StatusResponding:
		return StatusWarn.Render("●")
	case models.StatusWaiting:
		return DimStyle.Render("○")
	case models.StatusError:
		return StatusCrit.Render("✗")
	case models.StatusTimeout:
		return DimStyle.Render("◌")
	default: // Idle
		return StatusOK.Render("●")
	}
}

// DebateView wraps a debate with viewport for scrolling
type DebateView struct {
	Debate   *Debate
	Viewport viewport.Model
}

func NewDebateView(debate *Debate, width, height int) *DebateView {
	vp := viewport.New(width, height)
	vp.Style = lipgloss.NewStyle()
	vp.MouseWheelEnabled = true

	return &DebateView{
		Debate:   debate,
		Viewport: vp,
	}
}

func (v *DebateView) Update() {
	content := v.Debate.RenderMessages(v.Viewport.Width)
	v.Viewport.SetContent(content)
	v.Viewport.GotoBottom()
}
