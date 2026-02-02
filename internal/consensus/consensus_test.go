// internal/consensus/consensus_test.go
package consensus

import (
	"testing"
)

func TestDetectPosition(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected Position
	}{
		{
			name:     "explicit agree with model",
			content:  "AGREE: [Claude] - I think this approach is solid",
			expected: PositionAgree,
		},
		{
			name:     "explicit agree without brackets",
			content:  "AGREE: GPT - The analysis is correct",
			expected: PositionAgree,
		},
		{
			name:     "explicit object",
			content:  "OBJECT: This approach has a fundamental flaw",
			expected: PositionObject,
		},
		{
			name:     "explicit add",
			content:  "ADD: We should also consider caching",
			expected: PositionAdd,
		},
		{
			name:     "implicit agree",
			content:  "I agree with the proposed solution",
			expected: PositionAgree,
		},
		{
			name:     "implicit disagree",
			content:  "I disagree with this approach",
			expected: PositionObject,
		},
		{
			name:     "implicit add",
			content:  "I would add that we need tests",
			expected: PositionAdd,
		},
		{
			name:     "unknown position",
			content:  "Let me explain the architecture",
			expected: PositionUnknown,
		},
		{
			name:     "case insensitive agree",
			content:  "agree: [gemini] sounds good",
			expected: PositionAgree,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectPosition(tt.content)
			if got != tt.expected {
				t.Errorf("DetectPosition() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseAgree(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		expectedModel string
		expectedFound bool
	}{
		{
			name:          "with brackets",
			content:       "AGREE: [Claude] - Great analysis",
			expectedModel: "Claude",
			expectedFound: true,
		},
		{
			name:          "without brackets",
			content:       "AGREE: GPT makes a good point",
			expectedModel: "GPT makes a good point",
			expectedFound: true,
		},
		{
			name:          "no agree statement",
			content:       "I think this is correct",
			expectedModel: "",
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, found := ParseAgree(tt.content)
			if found != tt.expectedFound {
				t.Errorf("ParseAgree() found = %v, want %v", found, tt.expectedFound)
			}
			if model != tt.expectedModel {
				t.Errorf("ParseAgree() model = %v, want %v", model, tt.expectedModel)
			}
		})
	}
}

func TestParseObject(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		expectedReason string
		expectedFound  bool
	}{
		{
			name:           "with reason",
			content:        "OBJECT: This breaks backwards compatibility",
			expectedReason: "This breaks backwards compatibility",
			expectedFound:  true,
		},
		{
			name:           "no object statement",
			content:        "I think there might be an issue",
			expectedReason: "",
			expectedFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, found := ParseObject(tt.content)
			if found != tt.expectedFound {
				t.Errorf("ParseObject() found = %v, want %v", found, tt.expectedFound)
			}
			if reason != tt.expectedReason {
				t.Errorf("ParseObject() reason = %v, want %v", reason, tt.expectedReason)
			}
		})
	}
}

func TestParseAdd(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		expectedPoint string
		expectedFound bool
	}{
		{
			name:          "with point",
			content:       "ADD: We need error handling",
			expectedPoint: "We need error handling",
			expectedFound: true,
		},
		{
			name:          "no add statement",
			content:       "Additionally, consider caching",
			expectedPoint: "",
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			point, found := ParseAdd(tt.content)
			if found != tt.expectedFound {
				t.Errorf("ParseAdd() found = %v, want %v", found, tt.expectedFound)
			}
			if point != tt.expectedPoint {
				t.Errorf("ParseAdd() point = %v, want %v", point, tt.expectedPoint)
			}
		})
	}
}

func TestCheckConsensus(t *testing.T) {
	tests := []struct {
		name      string
		positions map[string]Position
		expected  bool
	}{
		{
			name: "all agree",
			positions: map[string]Position{
				"claude": PositionAgree,
				"gpt":    PositionAgree,
				"gemini": PositionAgree,
			},
			expected: true,
		},
		{
			name: "majority agree no objections",
			positions: map[string]Position{
				"claude": PositionAgree,
				"gpt":    PositionAgree,
				"gemini": PositionAdd,
			},
			expected: true,
		},
		{
			name: "one objection blocks",
			positions: map[string]Position{
				"claude": PositionAgree,
				"gpt":    PositionAgree,
				"gemini": PositionObject,
			},
			expected: false,
		},
		{
			name: "no majority",
			positions: map[string]Position{
				"claude": PositionAgree,
				"gpt":    PositionAdd,
				"gemini": PositionAdd,
				"grok":   PositionAdd,
			},
			expected: false,
		},
		{
			name:      "empty positions",
			positions: map[string]Position{},
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckConsensus(tt.positions)
			if got != tt.expected {
				t.Errorf("CheckConsensus() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCheckStrictConsensus(t *testing.T) {
	tests := []struct {
		name      string
		positions map[string]Position
		expected  bool
	}{
		{
			name: "all agree",
			positions: map[string]Position{
				"claude": PositionAgree,
				"gpt":    PositionAgree,
			},
			expected: true,
		},
		{
			name: "agree and add",
			positions: map[string]Position{
				"claude": PositionAgree,
				"gpt":    PositionAdd,
			},
			expected: true,
		},
		{
			name: "all add no agree",
			positions: map[string]Position{
				"claude": PositionAdd,
				"gpt":    PositionAdd,
			},
			expected: false,
		},
		{
			name: "one unknown",
			positions: map[string]Position{
				"claude": PositionAgree,
				"gpt":    PositionUnknown,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckStrictConsensus(tt.positions)
			if got != tt.expected {
				t.Errorf("CheckStrictConsensus() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAnalyzeConsensus(t *testing.T) {
	positions := map[string]ParsedPosition{
		"claude": {Position: PositionAgree, Target: "gpt"},
		"gpt":    {Position: PositionAgree, Target: "claude"},
		"gemini": {Position: PositionAdd, Point: "Consider caching"},
		"grok":   {Position: PositionObject, Reason: "Performance concerns"},
	}

	result := AnalyzeConsensus(positions)

	if result.TotalCount != 4 {
		t.Errorf("TotalCount = %d, want 4", result.TotalCount)
	}
	if result.AgreeCount != 2 {
		t.Errorf("AgreeCount = %d, want 2", result.AgreeCount)
	}
	if result.ObjectCount != 1 {
		t.Errorf("ObjectCount = %d, want 1", result.ObjectCount)
	}
	if result.AddCount != 1 {
		t.Errorf("AddCount = %d, want 1", result.AddCount)
	}
	if result.HasConsensus {
		t.Error("HasConsensus should be false with objections")
	}
	if len(result.Objections) != 1 {
		t.Errorf("Objections count = %d, want 1", len(result.Objections))
	}
	if len(result.Additions) != 1 {
		t.Errorf("Additions count = %d, want 1", len(result.Additions))
	}
}
