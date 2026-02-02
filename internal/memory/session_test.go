// internal/memory/session_test.go
package memory

import (
	"strings"
	"testing"
	"time"
)

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			// "how" is filtered out (len <= 2 after stopword check passes, actually stopword)
			input:    "How should we implement the cache layer?",
			expected: []string{"implement", "cache", "layer"},
		},
		{
			input:    "The best approach is using Redis",
			expected: []string{"best", "approach", "using", "redis"},
		},
		{
			input:    "ROCm kernel optimization for inference",
			expected: []string{"rocm", "kernel", "optimization", "inference"},
		},
	}

	for _, tt := range tests {
		result := extractKeywords(tt.input)
		for _, exp := range tt.expected {
			found := false
			for _, got := range result {
				if got == exp {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("extractKeywords(%q) missing expected keyword %q, got %v", tt.input, exp, result)
			}
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"this is a long string", 10, "this is..."},
		{"exactly10!", 10, "exactly10!"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestDetectProjectMentions(t *testing.T) {
	c := NewClient()

	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    "We should integrate this with AEGIS Pensive for memory",
			expected: []string{"AEGIS", "Pensive"},
		},
		{
			input:    "The Mud-Puppy fine-tuning could use this",
			expected: []string{"Mud-Puppy"},
		},
		{
			input:    "This relates to the l1_cache and l2_handler",
			expected: []string{"l1_cache", "l2_handler"},
		},
		{
			input:    "No project mentions here",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		result := c.DetectProjectMentions(tt.input)

		if len(tt.expected) == 0 && len(result) != 0 {
			t.Errorf("DetectProjectMentions(%q) = %v, want empty", tt.input, result)
			continue
		}

		for _, exp := range tt.expected {
			found := false
			for _, got := range result {
				if strings.EqualFold(got, exp) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("DetectProjectMentions(%q) missing %q, got %v", tt.input, exp, result)
			}
		}
	}
}

func TestExtractAfter(t *testing.T) {
	tests := []struct {
		text     string
		marker   string
		expected string
	}{
		{
			text:     "AGREE: Claude's approach is correct",
			marker:   "AGREE:",
			expected: "Claude's approach is correct",
		},
		{
			text:     "I OBJECT: this won't work because of performance",
			marker:   "OBJECT:",
			expected: "this won't work because of performance",
		},
		{
			text:     "No marker here",
			marker:   "AGREE:",
			expected: "",
		},
		{
			text:     "ADD: we should also consider caching. This would help.",
			marker:   "ADD:",
			expected: "we should also consider caching",
		},
	}

	for _, tt := range tests {
		result := extractAfter(tt.text, tt.marker)
		if result != tt.expected {
			t.Errorf("extractAfter(%q, %q) = %q, want %q", tt.text, tt.marker, result, tt.expected)
		}
	}
}

func TestExtractLearnings(t *testing.T) {
	c := NewClient()
	c.SetEnabled(false) // Don't actually call external services in tests

	debateID := "test-123"
	debateName := "Test Debate"

	messages := []DebateMessage{
		{Source: "user", Content: "How should we implement caching?", Timestamp: time.Now()},
		{Source: "claude", Content: "I suggest using Redis for its speed.", Timestamp: time.Now().Add(time.Second)},
		{Source: "gpt", Content: "AGREE: Claude's Redis suggestion is good. This is the consensus approach.", Timestamp: time.Now().Add(2 * time.Second)},
		{Source: "gemini", Content: "I AGREE with the Redis approach", Timestamp: time.Now().Add(3 * time.Second)},
	}

	learning := c.ExtractLearnings(debateID, debateName, messages)

	// Should have consensus (2 agrees, no objections, and majority agreement)
	// Note: claude doesn't explicitly say AGREE, so only gpt and gemini count
	// 2 agrees out of 3 models = majority, no objections = consensus
	if learning.Consensus == nil {
		// Check what positions were detected
		t.Logf("Detected no consensus - checking model positions...")
		t.Error("Expected consensus to be detected (2/3 models agreed)")
	}

	// Should have no failures (no objections)
	if len(learning.Failures) != 0 {
		t.Errorf("Expected 0 failures, got %d", len(learning.Failures))
	}
}

func TestExtractLearningsWithObjection(t *testing.T) {
	c := NewClient()
	c.SetEnabled(false)

	messages := []DebateMessage{
		{Source: "user", Content: "Should we use global variables?", Timestamp: time.Now()},
		{Source: "claude", Content: "Yes, for simplicity", Timestamp: time.Now().Add(time.Second)},
		{Source: "gpt", Content: "OBJECT: global variables cause race conditions", Timestamp: time.Now().Add(2 * time.Second)},
		{Source: "gemini", Content: "I agree with GPT, this is dangerous", Timestamp: time.Now().Add(3 * time.Second)},
	}

	learning := c.ExtractLearnings("test", "Test", messages)

	// Should not have consensus (objection present)
	if learning.Consensus != nil {
		t.Error("Expected no consensus due to objection")
	}

	// Should have 1 failure
	if len(learning.Failures) != 1 {
		t.Errorf("Expected 1 failure, got %d", len(learning.Failures))
	} else {
		if !strings.Contains(learning.Failures[0].Reason, "race conditions") {
			t.Errorf("Expected failure reason to mention 'race conditions', got %q", learning.Failures[0].Reason)
		}
	}
}

func TestDeduplicateFailures(t *testing.T) {
	failures := []FailedApproach{
		{DebateID: "1", Approach: "use globals", Reason: "bad practice", RejectedBy: []string{"claude"}},
		{DebateID: "1", Approach: "use globals", Reason: "bad practice", RejectedBy: []string{"gpt"}},
		{DebateID: "1", Approach: "skip tests", Reason: "risky", RejectedBy: []string{"claude"}},
	}

	result := deduplicateFailures(failures)

	if len(result) != 2 {
		t.Errorf("Expected 2 unique failures, got %d", len(result))
	}

	// Check that rejectors were merged for the duplicate
	for _, f := range result {
		if f.Approach == "use globals" {
			if len(f.RejectedBy) != 2 {
				t.Errorf("Expected merged rejectors to have 2 models, got %d", len(f.RejectedBy))
			}
		}
	}
}

func TestClientEnableDisable(t *testing.T) {
	c := NewClient()

	if !c.IsEnabled() {
		t.Error("Client should be enabled by default")
	}

	c.SetEnabled(false)
	if c.IsEnabled() {
		t.Error("Client should be disabled after SetEnabled(false)")
	}

	c.SetEnabled(true)
	if !c.IsEnabled() {
		t.Error("Client should be enabled after SetEnabled(true)")
	}
}
