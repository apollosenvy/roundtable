// internal/guardian/guardian_test.go
package guardian

import (
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	g := New()
	if g == nil {
		t.Fatal("Expected non-nil Guardian")
	}
	if !g.enabled {
		t.Error("Expected Guardian to be enabled by default")
	}
	if g.canaryScriptPath != CheckCanaryScript {
		t.Errorf("Expected canary script path %s, got %s", CheckCanaryScript, g.canaryScriptPath)
	}
}

func TestNewWithPath(t *testing.T) {
	customPath := "/custom/path/check-canary.py"
	g := NewWithPath(customPath)
	if g.canaryScriptPath != customPath {
		t.Errorf("Expected canary script path %s, got %s", customPath, g.canaryScriptPath)
	}
}

func TestSetEnabled(t *testing.T) {
	g := New()
	g.SetEnabled(false)
	if g.enabled {
		t.Error("Expected Guardian to be disabled")
	}
	g.SetEnabled(true)
	if !g.enabled {
		t.Error("Expected Guardian to be enabled")
	}
}

func TestDetectDestructive(t *testing.T) {
	g := New()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "rm -rf",
			content:  "Let me run rm -rf /tmp/test",
			expected: true,
		},
		{
			name:     "git push --force",
			content:  "I'll git push --force to main",
			expected: true,
		},
		{
			name:     "git reset --hard",
			content:  "Running git reset --hard HEAD~5",
			expected: true,
		},
		{
			name:     "DROP TABLE",
			content:  "Execute DROP TABLE users;",
			expected: true,
		},
		{
			name:     "TRUNCATE",
			content:  "TRUNCATE TABLE logs;",
			expected: true,
		},
		{
			name:     "systemctl stop",
			content:  "systemctl stop nginx",
			expected: true,
		},
		{
			name:     "kill -9",
			content:  "kill -9 1234",
			expected: true,
		},
		{
			name:     "safe operation",
			content:  "Let me create a new file and write some code",
			expected: false,
		},
		{
			name:     "git push (without force)",
			content:  "git push origin main",
			expected: false,
		},
		{
			name:     "SELECT query",
			content:  "SELECT * FROM users WHERE id = 1",
			expected: false,
		},
		{
			name:     "case insensitive - DROP table",
			content:  "drop table if exists temp_data;",
			expected: true,
		},
		{
			name:     "git branch -D",
			content:  "git branch -D feature/old-branch",
			expected: true,
		},
		{
			name:     "chmod 777",
			content:  "chmod 777 /var/www",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := g.DetectDestructive(tt.content)
			got := len(matches) > 0
			if got != tt.expected {
				t.Errorf("DetectDestructive(%q) = %v (matches: %v), want %v",
					tt.content, got, matches, tt.expected)
			}
		})
	}
}

func TestRequiresCanary(t *testing.T) {
	g := New()

	// Destructive content requires canary
	if !g.RequiresCanary("rm -rf /tmp") {
		t.Error("Expected destructive content to require canary")
	}

	// Safe content doesn't require canary
	if g.RequiresCanary("create file test.txt") {
		t.Error("Expected safe content to not require canary")
	}

	// Disabled guardian doesn't require canary
	g.SetEnabled(false)
	if g.RequiresCanary("rm -rf /tmp") {
		t.Error("Expected disabled Guardian to not require canary")
	}
}

func TestValidateExecution_SafeContent(t *testing.T) {
	g := New()

	allowed, reason := g.ValidateExecution("create a new function", "")
	if !allowed {
		t.Errorf("Expected safe content to be allowed, got reason: %s", reason)
	}
	if !strings.Contains(reason, "No destructive") {
		t.Errorf("Expected reason to mention no destructive patterns, got: %s", reason)
	}
}

func TestValidateExecution_DestructiveNoCanary(t *testing.T) {
	g := New()

	allowed, reason := g.ValidateExecution("rm -rf /tmp/data", "")
	if allowed {
		t.Error("Expected destructive content without canary to be blocked")
	}
	if !strings.Contains(reason, "Canary required") {
		t.Errorf("Expected reason to require canary, got: %s", reason)
	}
}

func TestValidateExecution_Disabled(t *testing.T) {
	g := New()
	g.SetEnabled(false)

	allowed, reason := g.ValidateExecution("rm -rf /", "")
	if !allowed {
		t.Errorf("Expected disabled Guardian to allow all, got reason: %s", reason)
	}
	if !strings.Contains(reason, "disabled") {
		t.Errorf("Expected reason to mention disabled, got: %s", reason)
	}
}

func TestExtractCanary(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{
			name:     "standard canary",
			text:     "phosphor-7482: yes, delete it",
			expected: "phosphor-7482",
		},
		{
			name:     "canary in sentence",
			text:     "The canary is azure-1234 for this operation",
			expected: "azure-1234",
		},
		{
			name:     "uppercase canary",
			text:     "BEACON-5678: proceed",
			expected: "beacon-5678",
		},
		{
			name:     "no canary",
			text:     "Just delete the files please",
			expected: "",
		},
		{
			name:     "partial match - not enough digits",
			text:     "canary-12 is too short",
			expected: "",
		},
		{
			name:     "multiple words - takes first",
			text:     "alpha-1111 and beta-2222",
			expected: "alpha-1111",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCanary(tt.text)
			if got != tt.expected {
				t.Errorf("ExtractCanary(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}

func TestFormatWarning(t *testing.T) {
	patterns := []string{"rm -rf", "git push --force"}
	warning := FormatWarning(patterns)

	if !strings.Contains(warning, "GUARDIAN ALERT") {
		t.Error("Warning should contain GUARDIAN ALERT")
	}
	if !strings.Contains(warning, "rm -rf") {
		t.Error("Warning should list the detected patterns")
	}
	if !strings.Contains(warning, "git push --force") {
		t.Error("Warning should list all detected patterns")
	}
	if !strings.Contains(warning, "canary") {
		t.Error("Warning should mention canary requirement")
	}
}

func TestDetectDestructive_MultipleMatches(t *testing.T) {
	g := New()

	content := "First rm -rf /tmp, then git push --force, and DROP TABLE users"
	matches := g.DetectDestructive(content)

	if len(matches) < 3 {
		t.Errorf("Expected at least 3 matches, got %d: %v", len(matches), matches)
	}
}
