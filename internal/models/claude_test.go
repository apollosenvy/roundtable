// internal/models/claude_test.go
package models

import (
	"os/exec"
	"testing"
)

func TestClaudeInfo(t *testing.T) {
	claude := NewClaude("claude", "opus")
	info := claude.Info()

	if info.ID != "claude" {
		t.Errorf("Expected ID 'claude', got %s", info.ID)
	}
	if !info.CanExec {
		t.Error("Claude should be able to execute")
	}
	if !info.CanRead {
		t.Error("Claude should be able to read files")
	}
}

func TestClaudeCliExists(t *testing.T) {
	_, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("Claude CLI not installed, skipping integration test")
	}
	// CLI exists, test passes
}
