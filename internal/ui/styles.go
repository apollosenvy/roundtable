// internal/ui/styles.go
package ui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	Cyan     = lipgloss.Color("#00FFFF")
	Green    = lipgloss.Color("#00FF00")
	Yellow   = lipgloss.Color("#FFD700")
	Orange   = lipgloss.Color("#FFA500")
	Red      = lipgloss.Color("#FF6B6B")
	Magenta  = lipgloss.Color("#FF00FF")
	SkyBlue  = lipgloss.Color("#87CEEB")
	Dim      = lipgloss.Color("#555555")
	White    = lipgloss.Color("#FFFFFF")
	DarkGray = lipgloss.Color("#333333")

	// Model colors
	ClaudeColor = Cyan
	GPTColor    = Green
	GeminiColor = Magenta
	GrokColor   = Orange
	UserColor   = SkyBlue
	SystemColor = Yellow

	// Box styles
	ActiveBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Cyan)

	InactiveBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Dim)

	// Text styles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Cyan)

	UserStyle = lipgloss.NewStyle().
			Foreground(SkyBlue).
			Bold(true)

	SystemStyle = lipgloss.NewStyle().
			Foreground(Yellow)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Red).
			Bold(true)

	DimStyle = lipgloss.NewStyle().
			Foreground(Dim)

	// Status indicators
	StatusOK   = lipgloss.NewStyle().Foreground(Green).Bold(true)
	StatusWarn = lipgloss.NewStyle().Foreground(Orange).Bold(true)
	StatusCrit = lipgloss.NewStyle().Foreground(Red).Bold(true)

	// Tab styles
	ActiveTabStyle = lipgloss.NewStyle().
			Foreground(Cyan).
			Bold(true)

	InactiveTabStyle = lipgloss.NewStyle().
				Foreground(Dim)
)

// ModelStyle returns the style for a given model ID
func ModelStyle(modelID string) lipgloss.Style {
	switch modelID {
	case "claude":
		return lipgloss.NewStyle().Foreground(ClaudeColor).Bold(true)
	case "gpt":
		return lipgloss.NewStyle().Foreground(GPTColor).Bold(true)
	case "gemini":
		return lipgloss.NewStyle().Foreground(GeminiColor).Bold(true)
	case "grok":
		return lipgloss.NewStyle().Foreground(GrokColor).Bold(true)
	case "user":
		return UserStyle
	case "system":
		return SystemStyle
	default:
		return lipgloss.NewStyle().Foreground(White)
	}
}

// ModelColor returns the color for a given model ID
func ModelColor(modelID string) lipgloss.Color {
	switch modelID {
	case "claude":
		return ClaudeColor
	case "gpt":
		return GPTColor
	case "gemini":
		return GeminiColor
	case "grok":
		return GrokColor
	case "user":
		return SkyBlue
	case "system":
		return Yellow
	default:
		return White
	}
}
