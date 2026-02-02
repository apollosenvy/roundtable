// internal/export/markdown_test.go
package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExportDebate(t *testing.T) {
	debate := &DebateExport{
		ID:          "abc123",
		Name:        "Test Debate",
		ProjectPath: "/home/test/project",
		CreatedAt:   time.Date(2026, 2, 1, 14, 30, 0, 0, time.UTC),
		Messages: []DebateMessage{
			{
				Source:    "user",
				Content:   "What's the best approach for implementing a cache?",
				Timestamp: time.Date(2026, 2, 1, 14, 30, 0, 0, time.UTC),
			},
			{
				Source:    "claude",
				Content:   "I recommend using an LRU cache with the following considerations:\n\n1. Memory efficiency\n2. O(1) lookups\n3. Automatic eviction",
				Timestamp: time.Date(2026, 2, 1, 14, 30, 15, 0, time.UTC),
			},
			{
				Source:    "gpt",
				Content:   "AGREE: Claude. LRU is a good choice. ADD: Consider using sync.Map for concurrent access.",
				Timestamp: time.Date(2026, 2, 1, 14, 30, 30, 0, time.UTC),
			},
		},
		ContextFiles: []string{"/home/test/project/cache.go"},
		Participants: []string{"claude", "gpt"},
	}

	result := ExportDebate(debate)

	// Check title
	if !strings.Contains(result, "# Test Debate") {
		t.Error("Expected title '# Test Debate' in output")
	}

	// Check metadata
	if !strings.Contains(result, "**Debate ID:** `abc123`") {
		t.Error("Expected debate ID in output")
	}

	// Check participants
	if !strings.Contains(result, "Claude, GPT") {
		t.Error("Expected participants in output")
	}

	// Check context files
	if !strings.Contains(result, "`/home/test/project/cache.go`") {
		t.Error("Expected context file in output")
	}

	// Check messages
	if !strings.Contains(result, "### [14:30:00] User") {
		t.Error("Expected user message header in output")
	}
	if !strings.Contains(result, "### [14:30:15] Claude") {
		t.Error("Expected Claude message header in output")
	}

	// Check content preservation
	if !strings.Contains(result, "LRU cache") {
		t.Error("Expected message content in output")
	}
}

func TestExportDebateWithCodeBlocks(t *testing.T) {
	debate := &DebateExport{
		ID:        "code123",
		Name:      "Code Discussion",
		CreatedAt: time.Now(),
		Messages: []DebateMessage{
			{
				Source: "claude",
				Content: "Here's the implementation:\n\n```go\ntype Cache struct {\n    data map[string]any\n}\n```",
				Timestamp: time.Now(),
			},
		},
	}

	result := ExportDebate(debate)

	// Content with code blocks should not be wrapped in blockquotes
	if strings.Contains(result, "> ```go") {
		t.Error("Code blocks should not be wrapped in blockquotes")
	}

	// Code block should be preserved
	if !strings.Contains(result, "```go") {
		t.Error("Expected code block to be preserved")
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Simple Name", "simple-name"},
		{"Test/Debate", "testdebate"},
		{"Debate #1!", "debate-1"},
		{"   spaces   ", "spaces"},
		{"Multiple---Hyphens", "multiple-hyphens"},
		{"", "debate"},
		{"This is a very long name that should be truncated to fifty characters maximum", "this-is-a-very-long-name-that-should-be-truncated-"},
	}

	for _, test := range tests {
		result := sanitizeFilename(test.input)
		if result != test.expected {
			t.Errorf("sanitizeFilename(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestWriteDebate(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	debate := &DebateExport{
		ID:        "write123",
		Name:      "Write Test",
		CreatedAt: time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC),
		Messages: []DebateMessage{
			{
				Source:    "user",
				Content:   "Test message",
				Timestamp: time.Now(),
			},
		},
	}

	path, err := WriteDebate(debate, tmpDir)
	if err != nil {
		t.Fatalf("WriteDebate() failed: %v", err)
	}

	// Check file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("Expected file to exist at %s", path)
	}

	// Check filename format
	expectedFilename := "2026-02-01-write-test.md"
	if filepath.Base(path) != expectedFilename {
		t.Errorf("Expected filename %q, got %q", expectedFilename, filepath.Base(path))
	}

	// Check file content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if !strings.Contains(string(content), "# Write Test") {
		t.Error("Expected title in file content")
	}
}

func TestFormatParticipants(t *testing.T) {
	participants := []string{"claude", "gpt", "gemini", "grok"}
	result := formatParticipants(participants)

	expected := []string{"Claude", "GPT", "Gemini", "Grok"}
	for i, p := range result {
		if p != expected[i] {
			t.Errorf("formatParticipants[%d] = %q, expected %q", i, p, expected[i])
		}
	}
}
