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
	Source    string    // claude, gpt, gemini, grok, user, system
	Content   string
	Timestamp time.Time
}

// Debate represents a single debate session
type Debate struct {
	ID           string
	Name         string
	ProjectPath  string
	Messages     []DebateMessage
	ContextFiles map[string]string // path -> content
	Paused       bool

	// Model states
	ModelStatus map[string]models.ModelStatus
}

func NewDebate(id, name string) *Debate {
	return &Debate{
		ID:           id,
		Name:         name,
		Messages:     []DebateMessage{},
		ContextFiles: make(map[string]string),
		ModelStatus:  make(map[string]models.ModelStatus),
	}
}

func (d *Debate) AddMessage(source, content string) {
	d.Messages = append(d.Messages, DebateMessage{
		Source:    source,
		Content:   content,
		Timestamp: time.Now(),
	})
}

func (d *Debate) RenderMessages(width int) string {
	var sb strings.Builder

	for _, msg := range d.Messages {
		ts := msg.Timestamp.Format("15:04")
		style := ModelStyle(msg.Source)

		// Model name header
		header := style.Render(fmt.Sprintf("[%s] %s:", ts, formatSource(msg.Source)))
		sb.WriteString(header)
		sb.WriteString("\n")

		// Message content with indent
		lines := strings.Split(msg.Content, "\n")
		for _, line := range lines {
			sb.WriteString("  ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
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
		if status == models.StatusResponding {
			name += "..."
		}

		sb.WriteString(fmt.Sprintf("%s %s\n", indicator, style.Render(name)))
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
