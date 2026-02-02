// internal/guardian/guardian.go
// Aegis Guardian integration for roundtable
// Implements canary verification for destructive operations
package guardian

import (
	"os/exec"
	"regexp"
	"strings"
)

const (
	// CheckCanaryScript is the path to the canary verification script
	CheckCanaryScript = "/home/aegis/Projects/scripts/aegis-guardian/check-canary.py"
)

// DestructivePatterns are patterns that indicate potentially destructive operations
var DestructivePatterns = []string{
	// File operations
	`rm\s+-rf`,
	`rm\s+.*-r`,
	`unlink`,
	`delete.*file`,
	`remove.*directory`,

	// Git operations
	`git\s+push\s+--force`,
	`git\s+push\s+-f`,
	`git\s+reset\s+--hard`,
	`git\s+clean`,
	`git\s+branch\s+-D`,

	// Database operations
	`DROP\s+TABLE`,
	`DROP\s+DATABASE`,
	`TRUNCATE`,
	`DELETE\s+FROM.*WHERE\s+1\s*=\s*1`,
	`DELETE\s+FROM\s+\w+\s*;`, // DELETE without WHERE

	// Service operations
	`systemctl\s+stop`,
	`systemctl\s+disable`,
	`service.*stop`,
	`kill\s+-9`,
	`pkill`,

	// Credential/config operations
	`chmod\s+777`,
	`chown.*root`,
	`modify.*credentials`,
	`update.*password`,
	`change.*secret`,
}

var destructiveRegexes []*regexp.Regexp

func init() {
	destructiveRegexes = make([]*regexp.Regexp, len(DestructivePatterns))
	for i, pattern := range DestructivePatterns {
		destructiveRegexes[i] = regexp.MustCompile("(?i)" + pattern)
	}
}

// Guardian provides protection against destructive operations
type Guardian struct {
	canaryScriptPath string
	enabled          bool
}

// New creates a new Guardian instance
func New() *Guardian {
	return &Guardian{
		canaryScriptPath: CheckCanaryScript,
		enabled:          true,
	}
}

// NewWithPath creates a Guardian with a custom canary script path
func NewWithPath(canaryScriptPath string) *Guardian {
	return &Guardian{
		canaryScriptPath: canaryScriptPath,
		enabled:          true,
	}
}

// SetEnabled enables or disables Guardian protection
func (g *Guardian) SetEnabled(enabled bool) {
	g.enabled = enabled
}

// IsEnabled returns whether Guardian protection is active
func (g *Guardian) IsEnabled() bool {
	return g.enabled
}

// VerifyCanary checks if the provided canary value is valid
func (g *Guardian) VerifyCanary(canary string) bool {
	if !g.enabled {
		return true // Guardian disabled, allow all
	}

	canary = strings.TrimSpace(canary)
	if canary == "" {
		return false
	}

	cmd := exec.Command("python3", g.canaryScriptPath, canary)
	err := cmd.Run()
	return err == nil // Exit code 0 means valid
}

// DetectDestructive checks if the content describes potentially destructive operations
func (g *Guardian) DetectDestructive(content string) []string {
	var matches []string
	seen := make(map[string]bool)

	for i, re := range destructiveRegexes {
		if re.MatchString(content) {
			pattern := DestructivePatterns[i]
			if !seen[pattern] {
				matches = append(matches, pattern)
				seen[pattern] = true
			}
		}
	}

	return matches
}

// RequiresCanary returns true if the content contains destructive patterns
func (g *Guardian) RequiresCanary(content string) bool {
	if !g.enabled {
		return false
	}
	return len(g.DetectDestructive(content)) > 0
}

// ValidateExecution checks if execution should proceed
// Returns (allowed, reason string)
// If destructive patterns detected and no valid canary provided, returns false
func (g *Guardian) ValidateExecution(content, canary string) (bool, string) {
	if !g.enabled {
		return true, "Guardian disabled"
	}

	matches := g.DetectDestructive(content)
	if len(matches) == 0 {
		return true, "No destructive patterns detected"
	}

	// Destructive patterns found - require canary
	if canary == "" {
		return false, "Destructive operation detected (" + strings.Join(matches, ", ") + "). Canary required."
	}

	if !g.VerifyCanary(canary) {
		return false, "Invalid canary. Cannot proceed with destructive operation."
	}

	return true, "Canary verified, proceeding with caution"
}

// ExtractCanary attempts to extract a canary from user input
// Canaries follow the pattern: word-digits (e.g., "phosphor-7482")
var canaryPattern = regexp.MustCompile(`\b([a-z]+-\d{4})\b`)

// ExtractCanary extracts a canary pattern from text
func ExtractCanary(text string) string {
	matches := canaryPattern.FindStringSubmatch(strings.ToLower(text))
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// FormatWarning creates a warning message for destructive operations
func FormatWarning(patterns []string) string {
	var sb strings.Builder
	sb.WriteString("⚠️ GUARDIAN ALERT: Destructive operation detected\n\n")
	sb.WriteString("Detected patterns:\n")
	for _, p := range patterns {
		sb.WriteString("  • ")
		sb.WriteString(p)
		sb.WriteString("\n")
	}
	sb.WriteString("\nTo proceed, provide a valid canary in your next message.\n")
	sb.WriteString("Example: \"phosphor-1234: yes, proceed with deletion\"\n")
	sb.WriteString("\nAlternatively, reformulate the request to avoid destructive operations.")
	return sb.String()
}
