// internal/pensive/bridge_test.go
package pensive

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"roundtable/internal/db"
)

func TestNewBridge(t *testing.T) {
	b := NewBridge()

	if b.pensiveURL != DefaultPensiveURL {
		t.Errorf("expected pensiveURL %s, got %s", DefaultPensiveURL, b.pensiveURL)
	}

	if !b.enabled {
		t.Error("expected bridge to be enabled by default")
	}
}

func TestBridgeWithEnvConfig(t *testing.T) {
	os.Setenv("PENSIVE_URL", "http://custom:9999")
	defer os.Unsetenv("PENSIVE_URL")

	b := NewBridge()

	if b.pensiveURL != "http://custom:9999" {
		t.Errorf("expected custom URL, got %s", b.pensiveURL)
	}
}

func TestSetEnabled(t *testing.T) {
	b := NewBridge()
	b.SetEnabled(false)

	if b.enabled {
		t.Error("expected bridge to be disabled")
	}

	// Should not try to connect when disabled
	ctx := context.Background()
	if b.IsAvailable(ctx) {
		t.Error("disabled bridge should not report available")
	}
}

func TestBuildDebateRecord(t *testing.T) {
	b := NewBridge()

	debate := &db.Debate{
		ID:          "test-123",
		Name:        "Test Debate",
		ProjectPath: "/home/user/project",
		Status:      "resolved",
		Consensus:   "We agreed on approach X",
		CreatedAt:   time.Now().Add(-1 * time.Hour),
		UpdatedAt:   time.Now(),
	}

	messages := []db.Message{
		{
			Source:    "claude",
			Content:   "I think we should use approach X",
			MsgType:   "model",
			CreatedAt: time.Now().Add(-30 * time.Minute),
		},
		{
			Source:    "gpt",
			Content:   "AGREE: [claude] That approach makes sense",
			MsgType:   "model",
			CreatedAt: time.Now().Add(-20 * time.Minute),
		},
		{
			Source:    "gemini",
			Content:   "I concur with the proposed solution",
			MsgType:   "model",
			CreatedAt: time.Now().Add(-10 * time.Minute),
		},
	}

	record := b.buildDebateRecord(debate, messages)

	if record.DebateID != "test-123" {
		t.Errorf("expected debate ID test-123, got %s", record.DebateID)
	}

	if record.DebateName != "Test Debate" {
		t.Errorf("expected debate name, got %s", record.DebateName)
	}

	if record.Outcome != "resolved" {
		t.Errorf("expected outcome resolved, got %s", record.Outcome)
	}

	if len(record.Participants) != 3 {
		t.Errorf("expected 3 participants, got %d", len(record.Participants))
	}

	// Should have key decisions from AGREE statements
	if len(record.KeyDecisions) == 0 {
		t.Error("expected key decisions to be extracted")
	}

	// Should have hash
	if record.Hash == "" {
		t.Error("expected hash to be generated")
	}

	// Should have tags
	if len(record.Tags) == 0 {
		t.Error("expected tags to be generated")
	}

	// Check for required tags
	hasOutcomeTag := false
	for _, tag := range record.Tags {
		if tag == "outcome:resolved" {
			hasOutcomeTag = true
		}
	}
	if !hasOutcomeTag {
		t.Error("expected outcome tag")
	}
}

func TestStoreToFile(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	b := NewBridge()
	b.fallbackDir = tmpDir

	record := &DebateRecord{
		Hash:       "abc123",
		DebateID:   "test-debate",
		DebateName: "Test",
		Summary:    "A test debate",
		UpdatedAt:  time.Now(),
	}

	err := b.storeToFile(record)
	if err != nil {
		t.Fatalf("storeToFile failed: %v", err)
	}

	// Check file was created
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	// Verify file contents
	content, err := os.ReadFile(filepath.Join(tmpDir, files[0].Name()))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var loaded DebateRecord
	if err := json.Unmarshal(content, &loaded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if loaded.DebateID != "test-debate" {
		t.Errorf("expected debate ID, got %s", loaded.DebateID)
	}
}

func TestIsAvailableWithMockServer(t *testing.T) {
	// Create mock Pensive server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stats" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"vectors_in_index": 100,
				"db_rows":          100,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	b := NewBridgeWithConfig(server.URL, "")
	ctx := context.Background()

	if !b.IsAvailable(ctx) {
		t.Error("expected bridge to be available")
	}
}

func TestQueryRelevantDebatesWithMockServer(t *testing.T) {
	// Create mock embedding server
	embedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a dummy embedding
		embedding := make([]float64, 384)
		for i := range embedding {
			embedding[i] = 0.1
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"embedding": embedding,
		})
	}))
	defer embedServer.Close()

	// Create mock Pensive server
	pensiveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			results := []PensiveSearchResult{
				{
					Hash:    "prev-debate-1",
					Summary: "Previous debate about architecture",
					Score:   0.95,
				},
				{
					Hash:    "prev-debate-2",
					Summary: "Earlier discussion on testing",
					Score:   0.87,
				},
			}
			json.NewEncoder(w).Encode(results)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer pensiveServer.Close()

	b := NewBridgeWithConfig(pensiveServer.URL, embedServer.URL+"/embed")
	ctx := context.Background()

	debates, err := b.QueryRelevantDebates(ctx, "architecture decisions", 5)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(debates) != 2 {
		t.Errorf("expected 2 debates, got %d", len(debates))
	}
}

func TestFormatContextForDebate(t *testing.T) {
	b := NewBridge()

	debates := []RetrievedDebate{
		{
			DebateID:     "debate-1",
			DebateName:   "Architecture Discussion",
			Summary:      "We discussed system architecture",
			Consensus:    "Use microservices",
			KeyDecisions: []string{"Use Go for backend", "Use React for frontend"},
			WasSuccessful: true,
		},
		{
			DebateID:   "debate-2",
			DebateName: "Testing Strategy",
			Summary:    "Discussion about testing approach",
		},
	}

	context := b.FormatContextForDebate(debates)

	if context == "" {
		t.Error("expected non-empty context")
	}

	// Check for key elements
	if !contains(context, "Architecture Discussion") {
		t.Error("expected debate name in context")
	}

	if !contains(context, "Use microservices") {
		t.Error("expected consensus in context")
	}

	if !contains(context, "implemented successfully") {
		t.Error("expected success indication in context")
	}
}

func TestUpdateExecutionOutcome(t *testing.T) {
	tmpDir := t.TempDir()

	b := NewBridge()
	b.fallbackDir = tmpDir

	ctx := context.Background()
	err := b.UpdateExecutionOutcome(ctx, "test-debate", true, "Code deployed successfully")
	if err != nil {
		t.Fatalf("UpdateExecutionOutcome failed: %v", err)
	}

	// Check file was created
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}

	found := false
	for _, f := range files {
		if contains(f.Name(), "execution_test-debate") {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected execution outcome file to be created")
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
		{"", 10, ""},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestGenerateHash(t *testing.T) {
	hash1 := generateHash("test content")
	hash2 := generateHash("test content")
	hash3 := generateHash("different content")

	if hash1 != hash2 {
		t.Error("same content should produce same hash")
	}

	if hash1 == hash3 {
		t.Error("different content should produce different hash")
	}

	if len(hash1) != 16 {
		t.Errorf("expected hash length 16, got %d", len(hash1))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
