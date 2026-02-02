// internal/export/markdown.go
package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DebateMessage represents a message to export
type DebateMessage struct {
	Source    string
	Content   string
	Timestamp time.Time
}

// DebateExport contains the data needed to export a debate
type DebateExport struct {
	ID           string
	Name         string
	ProjectPath  string
	CreatedAt    time.Time
	Messages     []DebateMessage
	ContextFiles []string // file paths
	Participants []string // model IDs that participated
}

// ExportDebate generates a formatted markdown string from a debate
func ExportDebate(debate *DebateExport) string {
	var sb strings.Builder

	// Title header
	sb.WriteString("# ")
	sb.WriteString(debate.Name)
	sb.WriteString("\n\n")

	// Metadata section
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("**Debate ID:** `%s`\n\n", debate.ID))
	sb.WriteString(fmt.Sprintf("**Created:** %s\n\n", debate.CreatedAt.Format("2006-01-02 15:04:05")))
	if debate.ProjectPath != "" {
		sb.WriteString(fmt.Sprintf("**Project:** `%s`\n\n", debate.ProjectPath))
	}

	// Participants
	if len(debate.Participants) > 0 {
		sb.WriteString("**Participants:** ")
		sb.WriteString(strings.Join(formatParticipants(debate.Participants), ", "))
		sb.WriteString("\n\n")
	}

	sb.WriteString("---\n\n")

	// Context files section (if any)
	if len(debate.ContextFiles) > 0 {
		sb.WriteString("## Context Files\n\n")
		for _, path := range debate.ContextFiles {
			sb.WriteString(fmt.Sprintf("- `%s`\n", path))
		}
		sb.WriteString("\n---\n\n")
	}

	// Messages section
	sb.WriteString("## Transcript\n\n")

	for i, msg := range debate.Messages {
		// Timestamp and source header
		ts := msg.Timestamp.Format("15:04:05")
		sourceName := formatSource(msg.Source)
		sb.WriteString(fmt.Sprintf("### [%s] %s\n\n", ts, sourceName))

		// Message content
		content := strings.TrimSpace(msg.Content)
		if containsCodeBlock(content) {
			// Content already has code blocks, render as-is
			sb.WriteString(content)
		} else {
			// Wrap in blockquote for visual distinction
			lines := strings.Split(content, "\n")
			for _, line := range lines {
				sb.WriteString("> ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")

		// Add horizontal rule between messages (except after last)
		if i < len(debate.Messages)-1 {
			sb.WriteString("---\n\n")
		}
	}

	// Footer
	sb.WriteString("\n---\n\n")
	sb.WriteString(fmt.Sprintf("*Exported from Roundtable on %s*\n", time.Now().Format("2006-01-02 15:04:05")))

	return sb.String()
}

// WriteDebate exports a debate to a markdown file in the debates directory
func WriteDebate(debate *DebateExport, baseDir string) (string, error) {
	// Generate filename: YYYY-MM-DD-name.md
	datePart := debate.CreatedAt.Format("2006-01-02")
	namePart := sanitizeFilename(debate.Name)
	filename := fmt.Sprintf("%s-%s.md", datePart, namePart)

	// Ensure debates directory exists
	debatesDir := filepath.Join(baseDir, "debates")
	if err := os.MkdirAll(debatesDir, 0755); err != nil {
		return "", fmt.Errorf("create debates directory: %w", err)
	}

	// Full path
	path := filepath.Join(debatesDir, filename)

	// Generate markdown content
	content := ExportDebate(debate)

	// Write file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return path, nil
}

// formatSource returns a display name for a message source
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
		return "User"
	case "system":
		return "System"
	default:
		return source
	}
}

// formatParticipants converts model IDs to display names
func formatParticipants(participants []string) []string {
	result := make([]string, len(participants))
	for i, p := range participants {
		result[i] = formatSource(p)
	}
	return result
}

// sanitizeFilename removes/replaces characters unsuitable for filenames
func sanitizeFilename(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")

	// Remove or replace problematic characters
	var sb strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			sb.WriteRune(r)
		case r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '-' || r == '_':
			sb.WriteRune(r)
		default:
			// Skip other characters
		}
	}

	result := sb.String()

	// Collapse multiple hyphens
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}

	// Trim leading/trailing hyphens
	result = strings.Trim(result, "-")

	// Ensure non-empty
	if result == "" {
		result = "debate"
	}

	// Limit length
	if len(result) > 50 {
		result = result[:50]
	}

	return result
}

// containsCodeBlock checks if content already has markdown code blocks
func containsCodeBlock(content string) bool {
	return strings.Contains(content, "```")
}
