// internal/ui/help.go
package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Help overlay content and rendering

var (
	// Help section title style
	helpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Cyan).
			MarginBottom(1)

	// Help section header style
	helpSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(Yellow).
				MarginTop(1)

	// Help key style (for keybindings)
	helpKeyStyle = lipgloss.NewStyle().
			Foreground(Green).
			Bold(true)

	// Help command style (for slash commands)
	helpCmdStyle = lipgloss.NewStyle().
			Foreground(Magenta)

	// Help description style
	helpDescStyle = lipgloss.NewStyle().
			Foreground(White)

	// Help dim style (for secondary info)
	helpDimStyle = lipgloss.NewStyle().
			Foreground(Dim)

	// Status indicator styles for help
	helpStatusOK   = lipgloss.NewStyle().Foreground(Green).Bold(true)
	helpStatusWarn = lipgloss.NewStyle().Foreground(Orange).Bold(true)
	helpStatusDim  = lipgloss.NewStyle().Foreground(Dim)
	helpStatusErr  = lipgloss.NewStyle().Foreground(Red).Bold(true)
)

// HelpContent returns the formatted help overlay content
func HelpContent(width, height int) string {
	var content strings.Builder

	// Title
	title := helpTitleStyle.Render("ROUNDTABLE HELP")
	content.WriteString(title)
	content.WriteString("\n\n")

	// Keybindings section
	content.WriteString(helpSectionStyle.Render("KEYBINDINGS"))
	content.WriteString("\n\n")

	keybindings := []struct {
		key  string
		desc string
	}{
		{"Alt+1-9", "Switch to debate tab 1-9"},
		{"Alt+[", "Previous tab"},
		{"Alt+]", "Next tab"},
		{"Alt+N", "Create new debate tab"},
		{"Alt+W", "Close current tab"},
		{"Alt+H", "Browse past debates (history)"},
		{"Ctrl+Enter", "Send message to all models"},
		{"F1 / ?", "Toggle this help overlay"},
		{"Tab", "Cycle focus forward (Input -> Chat -> Context -> Models)"},
		{"Shift+Tab", "Cycle focus backward"},
		{"Esc", "Close help / Return to input"},
		{"Ctrl+C / Ctrl+Q", "Quit Roundtable"},
	}

	for _, kb := range keybindings {
		key := helpKeyStyle.Width(14).Render(kb.key)
		desc := helpDescStyle.Render(kb.desc)
		content.WriteString("  " + key + "  " + desc + "\n")
	}

	// Slash commands section
	content.WriteString("\n")
	content.WriteString(helpSectionStyle.Render("SLASH COMMANDS"))
	content.WriteString("\n\n")

	commands := []struct {
		cmd  string
		desc string
	}{
		{"/help", "Show this help overlay"},
		{"/new [name]", "Create a new debate (optional name)"},
		{"/close", "Close the current debate tab"},
		{"/context add <path>", "Load a file into debate context"},
		{"/context list", "List all loaded context files"},
		{"/context remove <path>", "Remove a file from context"},
		{"/models", "Open model picker/configuration"},
		{"/consensus", "Force a consensus check among models"},
		{"/execute", "Execute the agreed-upon approach"},
		{"/pause", "Pause automatic debate progression"},
		{"/resume", "Resume automatic debate progression"},
		{"/history", "Browse past debate sessions"},
		{"/export [format]", "Export debate to markdown or JSON"},
	}

	for _, cmd := range commands {
		cmdStr := helpCmdStyle.Width(22).Render(cmd.cmd)
		desc := helpDescStyle.Render(cmd.desc)
		content.WriteString("  " + cmdStr + "  " + desc + "\n")
	}

	// Model status indicators section
	content.WriteString("\n")
	content.WriteString(helpSectionStyle.Render("MODEL STATUS INDICATORS"))
	content.WriteString("\n\n")

	indicators := []struct {
		symbol string
		style  lipgloss.Style
		desc   string
	}{
		{"●", helpStatusOK, "Ready/Idle - Model is available and waiting"},
		{"●", helpStatusWarn, "Responding - Model is currently generating a response"},
		{"○", helpStatusDim, "Waiting - Model is queued, waiting for its turn"},
		{"◌", helpStatusDim, "Timeout - Model response timed out"},
		{"✗", helpStatusErr, "Error - Model encountered an error"},
	}

	for _, ind := range indicators {
		symbol := ind.style.Width(3).Render(ind.symbol)
		desc := helpDescStyle.Render(ind.desc)
		content.WriteString("  " + symbol + "  " + desc + "\n")
	}

	// Debate protocol section
	content.WriteString("\n")
	content.WriteString(helpSectionStyle.Render("DEBATE PROTOCOL"))
	content.WriteString("\n\n")

	protocol := []string{
		"Roundtable orchestrates multi-model debates for code decisions.",
		"",
		"1. User poses a question or problem to the roundtable",
		"2. All active models respond with their perspective",
		"3. Models review and critique each other's responses",
		"4. Discussion continues until consensus emerges",
		"5. User can /execute the agreed approach or guide the debate",
		"",
		"Use /pause to stop auto-progression and manually control flow.",
		"Use /consensus to force a consensus check at any point.",
	}

	for _, line := range protocol {
		if line == "" {
			content.WriteString("\n")
		} else {
			content.WriteString("  " + helpDimStyle.Render(line) + "\n")
		}
	}

	// Footer
	content.WriteString("\n")
	footer := helpDimStyle.Render("Press F1 or Esc to close this help")
	content.WriteString(lipgloss.PlaceHorizontal(width-8, lipgloss.Center, footer))

	// Build the overlay box
	overlayStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Cyan).
		Padding(1, 3).
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

// renderHelp renders the help overlay (called from app.go)
func (m Model) renderHelp() string {
	return HelpContent(m.width, m.height)
}
