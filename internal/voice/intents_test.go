// internal/voice/intents_test.go
package voice

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"roundtable/internal/config"
	"roundtable/internal/consensus"
	"roundtable/internal/models"
)

// createTestRegistry creates a registry with no models enabled for testing
// This creates a valid but empty registry that won't panic
func createTestRegistry() *models.Registry {
	cfg := &config.Config{}
	// All models disabled by default
	return models.NewRegistry(cfg)
}

// TestNewManager tests manager creation
func TestNewManager(t *testing.T) {
	registry := createTestRegistry()
	m := NewManager(registry, nil)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}

	if m.currentState == nil {
		t.Error("currentState should be initialized")
	}

	if m.currentState.Status != "idle" {
		t.Errorf("initial status = %q, want %q", m.currentState.Status, "idle")
	}

	if m.currentState.Positions == nil {
		t.Error("Positions map should be initialized")
	}

	// Cleanup
	m.Stop()
}

// TestHandleHealth tests the health endpoint
func TestHandleHealth(t *testing.T) {
	registry := createTestRegistry()
	m := NewManager(registry, nil)
	defer m.Stop()

	tests := []struct {
		name       string
		method     string
		wantStatus int
	}{
		{
			name:       "GET request succeeds",
			method:     http.MethodGet,
			wantStatus: http.StatusOK,
		},
		{
			name:       "POST request rejected",
			method:     http.MethodPost,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "PUT request rejected",
			method:     http.MethodPut,
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/voice/health", nil)
			w := httptest.NewRecorder()

			m.handleHealth(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var resp map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to parse response: %v", err)
				}
				if resp["status"] != "ok" {
					t.Errorf("response status = %v, want 'ok'", resp["status"])
				}
				if resp["service"] != "roundtable-voice" {
					t.Errorf("response service = %v, want 'roundtable-voice'", resp["service"])
				}
			}
		})
	}
}

// TestHandleStatus tests the status endpoint
func TestHandleStatus(t *testing.T) {
	registry := createTestRegistry()
	m := NewManager(registry, nil)
	defer m.Stop()

	tests := []struct {
		name         string
		method       string
		setupState   func()
		wantStatus   int
		checkResp    func(t *testing.T, resp StatusResponse)
	}{
		{
			name:       "GET request - idle state",
			method:     http.MethodGet,
			wantStatus: http.StatusOK,
			checkResp: func(t *testing.T, resp StatusResponse) {
				if resp.Status != "idle" {
					t.Errorf("status = %q, want %q", resp.Status, "idle")
				}
				if resp.Speakable == "" {
					t.Error("speakable should not be empty")
				}
			},
		},
		{
			name:   "GET request - debating state",
			method: http.MethodGet,
			setupState: func() {
				m.mu.Lock()
				m.currentState.Status = "debating"
				m.currentState.Topic = "test topic"
				m.currentState.MessageCount = 5
				m.currentState.ModelCount = 3
				m.currentState.LastUpdate = time.Now()
				m.mu.Unlock()
			},
			wantStatus: http.StatusOK,
			checkResp: func(t *testing.T, resp StatusResponse) {
				if resp.Status != "debating" {
					t.Errorf("status = %q, want %q", resp.Status, "debating")
				}
				if resp.Topic != "test topic" {
					t.Errorf("topic = %q, want %q", resp.Topic, "test topic")
				}
				if resp.MessageCount != 5 {
					t.Errorf("message_count = %d, want 5", resp.MessageCount)
				}
			},
		},
		{
			name:       "POST request rejected",
			method:     http.MethodPost,
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state before each test
			m.mu.Lock()
			m.currentState = &DebateState{
				Status:    "idle",
				Positions: make(map[string]consensus.Position),
			}
			m.mu.Unlock()

			if tt.setupState != nil {
				tt.setupState()
			}

			req := httptest.NewRequest(tt.method, "/voice/status", nil)
			w := httptest.NewRecorder()

			m.handleStatus(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK && tt.checkResp != nil {
				var resp StatusResponse
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to parse response: %v", err)
				}
				tt.checkResp(t, resp)
			}
		})
	}
}

// TestHandleCommand tests the command endpoint
func TestHandleCommand(t *testing.T) {
	registry := createTestRegistry()
	m := NewManager(registry, nil)
	defer m.Stop()

	tests := []struct {
		name       string
		method     string
		body       string
		wantStatus int
	}{
		{
			name:       "POST with valid JSON",
			method:     http.MethodPost,
			body:       `{"text": "roundtable status"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET request rejected",
			method:     http.MethodGet,
			body:       "",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "POST with invalid JSON",
			method:     http.MethodPost,
			body:       `{invalid json}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "POST with empty body",
			method:     http.MethodPost,
			body:       `{}`,
			wantStatus: http.StatusOK, // Empty command is valid, just unrecognized
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/voice/command", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			m.handleCommand(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

// TestProcessCommand tests command pattern matching
func TestProcessCommand(t *testing.T) {
	// Create a manager with a valid registry
	registry := createTestRegistry()
	m := NewManager(registry, nil)
	defer m.Stop()

	tests := []struct {
		name        string
		commandText string
		wantIntent  string
		wantSuccess bool
		setupState  func()
	}{
		{
			name:        "start roundtable",
			commandText: "start roundtable about AI safety",
			wantIntent:  IntentStartRoundtable,
			wantSuccess: true,
		},
		{
			name:        "begin roundtable",
			commandText: "begin a roundtable discussing quantum computing",
			wantIntent:  IntentStartRoundtable,
			wantSuccess: true,
		},
		{
			name:        "open roundtable",
			commandText: "open roundtable for code review",
			wantIntent:  IntentStartRoundtable,
			wantSuccess: true,
		},
		{
			name:        "launch roundtable",
			commandText: "launch roundtable on machine learning",
			wantIntent:  IntentStartRoundtable,
			wantSuccess: true,
		},
		{
			name:        "roundtable status",
			commandText: "roundtable status",
			wantIntent:  IntentRoundtableStatus,
			wantSuccess: true,
		},
		{
			name:        "debate status",
			commandText: "debate status",
			wantIntent:  IntentRoundtableStatus,
			wantSuccess: true,
		},
		{
			name:        "roundtable consensus - no active debate",
			commandText: "roundtable consensus",
			wantIntent:  IntentRoundtableConsensus,
			wantSuccess: false, // No active debate
		},
		{
			name:        "roundtable consensus - with active debate",
			commandText: "check consensus",
			wantIntent:  IntentRoundtableConsensus,
			wantSuccess: true,
			setupState: func() {
				m.mu.Lock()
				m.currentState.Status = "debating"
				m.currentState.Topic = "test"
				m.mu.Unlock()
			},
		},
		{
			name:        "force consensus",
			commandText: "force consensus",
			wantIntent:  IntentRoundtableConsensus,
			wantSuccess: true,
			setupState: func() {
				m.mu.Lock()
				m.currentState.Status = "debating"
				m.currentState.Topic = "test"
				m.mu.Unlock()
			},
		},
		{
			name:        "close roundtable - no active debate",
			commandText: "close roundtable",
			wantIntent:  IntentCloseRoundtable,
			wantSuccess: false, // No active debate
		},
		{
			name:        "close roundtable - with active debate",
			commandText: "close the roundtable",
			wantIntent:  IntentCloseRoundtable,
			wantSuccess: true,
			setupState: func() {
				m.mu.Lock()
				m.currentState.Status = "debating"
				m.currentState.Topic = "test topic"
				m.mu.Unlock()
			},
		},
		{
			name:        "end debate",
			commandText: "end the debate",
			wantIntent:  IntentCloseRoundtable,
			wantSuccess: true,
			setupState: func() {
				m.mu.Lock()
				m.currentState.Status = "debating"
				m.currentState.Topic = "test topic"
				m.mu.Unlock()
			},
		},
		{
			name:        "stop roundtable",
			commandText: "stop roundtable",
			wantIntent:  IntentCloseRoundtable,
			wantSuccess: true,
			setupState: func() {
				m.mu.Lock()
				m.currentState.Status = "debating"
				m.currentState.Topic = "test topic"
				m.mu.Unlock()
			},
		},
		{
			name:        "finish debate",
			commandText: "finish debate",
			wantIntent:  IntentCloseRoundtable,
			wantSuccess: true,
			setupState: func() {
				m.mu.Lock()
				m.currentState.Status = "debating"
				m.currentState.Topic = "test topic"
				m.mu.Unlock()
			},
		},
		{
			name:        "unrecognized command",
			commandText: "random gibberish",
			wantIntent:  "",
			wantSuccess: false,
		},
		{
			name:        "uppercase command",
			commandText: "START ROUNDTABLE ABOUT TESTING",
			wantIntent:  IntentStartRoundtable,
			wantSuccess: true,
		},
		{
			name:        "mixed case command",
			commandText: "StArT rOuNdTaBlE aBoUt CaSe TeSt",
			wantIntent:  IntentStartRoundtable,
			wantSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state before each test
			m.mu.Lock()
			m.currentState = &DebateState{
				Status:    "idle",
				Positions: make(map[string]consensus.Position),
			}
			m.mu.Unlock()

			if tt.setupState != nil {
				tt.setupState()
			}

			cmd := CommandRequest{Text: tt.commandText}
			resp := m.processCommand(cmd)

			if resp.Intent != tt.wantIntent {
				t.Errorf("intent = %q, want %q", resp.Intent, tt.wantIntent)
			}

			if resp.Success != tt.wantSuccess {
				t.Errorf("success = %v, want %v", resp.Success, tt.wantSuccess)
			}

			// All responses should have speakable text
			if resp.Speakable == "" {
				t.Error("speakable should not be empty")
			}
		})
	}
}

// TestStartRoundtable tests starting a new debate
func TestStartRoundtable(t *testing.T) {
	registry := createTestRegistry()
	m := NewManager(registry, nil)
	defer m.Stop()

	t.Run("start new debate", func(t *testing.T) {
		resp := m.startRoundtable("test topic")

		if !resp.Success {
			t.Error("starting roundtable should succeed")
		}

		m.mu.RLock()
		status := m.currentState.Status
		topic := m.currentState.Topic
		m.mu.RUnlock()

		if status != "debating" {
			t.Errorf("status = %q, want %q", status, "debating")
		}

		if topic != "test topic" {
			t.Errorf("topic = %q, want %q", topic, "test topic")
		}
	})

	t.Run("cannot start while debating", func(t *testing.T) {
		// State is already debating from previous test
		resp := m.startRoundtable("another topic")

		if resp.Success {
			t.Error("starting roundtable while debating should fail")
		}

		if resp.Error != "debate in progress" {
			t.Errorf("error = %q, want %q", resp.Error, "debate in progress")
		}
	})
}

// TestCloseRoundtable tests closing a debate
func TestCloseRoundtable(t *testing.T) {
	registry := createTestRegistry()
	m := NewManager(registry, nil)
	defer m.Stop()

	t.Run("cannot close when idle", func(t *testing.T) {
		resp := m.closeRoundtable()

		if resp.Success {
			t.Error("closing idle roundtable should fail")
		}

		if resp.Error != "no active debate" {
			t.Errorf("error = %q, want %q", resp.Error, "no active debate")
		}
	})

	t.Run("close active debate", func(t *testing.T) {
		m.mu.Lock()
		m.currentState.Status = "debating"
		m.currentState.Topic = "test topic"
		m.currentState.MessageCount = 10
		m.mu.Unlock()

		resp := m.closeRoundtable()

		if !resp.Success {
			t.Error("closing active roundtable should succeed")
		}

		m.mu.RLock()
		status := m.currentState.Status
		m.mu.RUnlock()

		if status != "closed" {
			t.Errorf("status = %q, want %q", status, "closed")
		}
	})

	t.Run("cannot close already closed", func(t *testing.T) {
		resp := m.closeRoundtable()

		if resp.Success {
			t.Error("closing already closed roundtable should fail")
		}
	})
}

// TestCheckConsensus tests consensus checking
func TestCheckConsensus(t *testing.T) {
	registry := createTestRegistry()
	m := NewManager(registry, nil)
	defer m.Stop()

	t.Run("no active debate", func(t *testing.T) {
		resp := m.checkConsensus()

		if resp.Success {
			t.Error("checking consensus without active debate should fail")
		}

		if resp.Error != "no active debate" {
			t.Errorf("error = %q, want %q", resp.Error, "no active debate")
		}
	})

	t.Run("with active debate", func(t *testing.T) {
		m.mu.Lock()
		m.currentState.Status = "debating"
		m.currentState.Topic = "test topic"
		m.currentState.Messages = []StateMessage{
			{Source: "claude", Summary: "AGREE: [gpt] Good approach"},
			{Source: "gpt", Summary: "I think this is correct"},
		}
		m.mu.Unlock()

		resp := m.checkConsensus()

		if !resp.Success {
			t.Error("checking consensus with active debate should succeed")
		}

		m.mu.RLock()
		status := m.currentState.Status
		m.mu.RUnlock()

		if status != "consensus" {
			t.Errorf("status = %q, want %q", status, "consensus")
		}
	})
}

// TestGetStatus tests status retrieval
func TestGetStatus(t *testing.T) {
	registry := createTestRegistry()
	m := NewManager(registry, nil)
	defer m.Stop()

	tests := []struct {
		name       string
		setupState func()
		wantStatus string
	}{
		{
			name:       "idle state",
			wantStatus: "idle",
		},
		{
			name: "debating state",
			setupState: func() {
				m.mu.Lock()
				m.currentState.Status = "debating"
				m.currentState.Topic = "test"
				m.currentState.LastUpdate = time.Now()
				m.mu.Unlock()
			},
			wantStatus: "debating",
		},
		{
			name: "consensus state",
			setupState: func() {
				m.mu.Lock()
				m.currentState.Status = "consensus"
				m.currentState.Topic = "test"
				m.mu.Unlock()
			},
			wantStatus: "consensus",
		},
		{
			name: "closed state",
			setupState: func() {
				m.mu.Lock()
				m.currentState.Status = "closed"
				m.currentState.Topic = "test"
				m.mu.Unlock()
			},
			wantStatus: "closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.mu.Lock()
			m.currentState = &DebateState{
				Status:    "idle",
				Positions: make(map[string]consensus.Position),
			}
			m.mu.Unlock()

			if tt.setupState != nil {
				tt.setupState()
			}

			resp := m.getStatus()

			if !resp.Success {
				t.Error("getStatus should always succeed")
			}

			data, ok := resp.Data["status"].(string)
			if !ok || data != tt.wantStatus {
				t.Errorf("data status = %v, want %q", data, tt.wantStatus)
			}

			if resp.Speakable == "" {
				t.Error("speakable should not be empty")
			}
		})
	}
}

// TestUpdateState tests state updates from TUI
func TestUpdateState(t *testing.T) {
	registry := createTestRegistry()
	m := NewManager(registry, nil)
	defer m.Stop()

	messages := []StateMessage{
		{Source: "claude", Summary: "AGREE: [gpt] Great idea"},
		{Source: "gpt", Summary: "OBJECT: Performance concerns"},
		{Source: "gemini", Summary: "I would add error handling"},
	}

	m.UpdateState("debate-123", "test topic", "debating", 15, messages)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.currentState.ID != "debate-123" {
		t.Errorf("ID = %q, want %q", m.currentState.ID, "debate-123")
	}

	if m.currentState.Topic != "test topic" {
		t.Errorf("Topic = %q, want %q", m.currentState.Topic, "test topic")
	}

	if m.currentState.Status != "debating" {
		t.Errorf("Status = %q, want %q", m.currentState.Status, "debating")
	}

	if m.currentState.MessageCount != 15 {
		t.Errorf("MessageCount = %d, want 15", m.currentState.MessageCount)
	}

	if len(m.currentState.Messages) != 3 {
		t.Errorf("Messages count = %d, want 3", len(m.currentState.Messages))
	}

	// Check positions were parsed
	if pos, ok := m.currentState.Positions["claude"]; !ok || pos != consensus.PositionAgree {
		t.Errorf("claude position = %v, want PositionAgree", pos)
	}

	if pos, ok := m.currentState.Positions["gpt"]; !ok || pos != consensus.PositionObject {
		t.Errorf("gpt position = %v, want PositionObject", pos)
	}

	// "gemini" should be PositionAdd due to "I would add"
	if pos, ok := m.currentState.Positions["gemini"]; !ok || pos != consensus.PositionAdd {
		t.Errorf("gemini position = %v, want PositionAdd", pos)
	}
}

// TestBuildSpeakableStatus tests speakable status generation
func TestBuildSpeakableStatus(t *testing.T) {
	registry := createTestRegistry()
	m := NewManager(registry, nil)
	defer m.Stop()

	tests := []struct {
		name       string
		state      *DebateState
		wantSubstr string
	}{
		{
			name: "idle state",
			state: &DebateState{
				Status: "idle",
			},
			wantSubstr: "idle",
		},
		{
			name: "debating - recent activity",
			state: &DebateState{
				Status:       "debating",
				Topic:        "AI",
				MessageCount: 5,
				ModelCount:   3,
				LastUpdate:   time.Now(),
			},
			wantSubstr: "Actively debating",
		},
		{
			name: "debating - old activity",
			state: &DebateState{
				Status:       "debating",
				Topic:        "AI",
				MessageCount: 5,
				ModelCount:   3,
				LastUpdate:   time.Now().Add(-2 * time.Minute),
			},
			wantSubstr: "ago",
		},
		{
			name: "consensus state",
			state: &DebateState{
				Status: "consensus",
				Topic:  "test",
				Positions: map[string]consensus.Position{
					"claude": consensus.PositionAgree,
					"gpt":    consensus.PositionObject,
				},
			},
			wantSubstr: "consensus",
		},
		{
			name: "closed state",
			state: &DebateState{
				Status: "closed",
				Topic:  "test",
			},
			wantSubstr: "closed",
		},
		{
			name: "unknown state",
			state: &DebateState{
				Status: "unknown_state",
			},
			wantSubstr: "unknown_state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.buildSpeakableStatus(tt.state)

			if result == "" {
				t.Error("speakable status should not be empty")
			}

			if tt.wantSubstr != "" && !containsIgnoreCase(result, tt.wantSubstr) {
				t.Errorf("speakable = %q, should contain %q", result, tt.wantSubstr)
			}
		})
	}
}

// TestFormatDuration tests duration formatting
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{30 * time.Second, "30 seconds"},
		{59 * time.Second, "59 seconds"},
		{1 * time.Minute, "1 minutes"},
		{5 * time.Minute, "5 minutes"},
		{59 * time.Minute, "59 minutes"},
		{1 * time.Hour, "1 hours"},
		{2 * time.Hour, "2 hours"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

// TestFormatModelList tests model list formatting
func TestFormatModelList(t *testing.T) {
	tests := []struct {
		name   string
		models []string
		want   string
	}{
		{
			name:   "empty list",
			models: []string{},
			want:   "no models",
		},
		{
			name:   "single model",
			models: []string{"Claude"},
			want:   "Claude",
		},
		{
			name:   "two models",
			models: []string{"Claude", "GPT"},
			want:   "Claude and GPT",
		},
		{
			name:   "three models",
			models: []string{"Claude", "GPT", "Gemini"},
			want:   "Claude, GPT, and Gemini",
		},
		{
			name:   "four models",
			models: []string{"Claude", "GPT", "Gemini", "Grok"},
			want:   "Claude, GPT, Gemini, and Grok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatModelList(tt.models)
			if got != tt.want {
				t.Errorf("formatModelList(%v) = %q, want %q", tt.models, got, tt.want)
			}
		})
	}
}

// TestTruncate tests string truncation
func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "needs truncation",
			input:  "hello world",
			maxLen: 8,
			want:   "hello...",
		},
		{
			name:   "minimal truncation",
			input:  "abcdefghij",
			maxLen: 6,
			want:   "abc...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// TestServerStartStop tests server lifecycle
func TestServerStartStop(t *testing.T) {
	registry := createTestRegistry()
	m := NewManager(registry, nil)

	// Start on a random port to avoid conflicts
	err := m.Start(0) // Port 0 lets the OS pick an available port
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Stop should work without error
	err = m.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

// TestHTTPIntegration tests the full HTTP flow
func TestHTTPIntegration(t *testing.T) {
	registry := createTestRegistry()
	m := NewManager(registry, nil)
	defer m.Stop()

	// Create a test server with the handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/voice/status", m.handleStatus)
	mux.HandleFunc("/voice/command", m.handleCommand)
	mux.HandleFunc("/voice/health", m.handleHealth)

	server := httptest.NewServer(mux)
	defer server.Close()

	t.Run("health check", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/voice/health")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("status check", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/voice/status")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var statusResp StatusResponse
		if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if statusResp.Status != "idle" {
			t.Errorf("status = %q, want %q", statusResp.Status, "idle")
		}
	})

	t.Run("send command", func(t *testing.T) {
		cmdBody := `{"text": "roundtable status"}`
		resp, err := http.Post(
			server.URL+"/voice/command",
			"application/json",
			bytes.NewBufferString(cmdBody),
		)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var cmdResp CommandResponse
		if err := json.NewDecoder(resp.Body).Decode(&cmdResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !cmdResp.Success {
			t.Error("command should succeed")
		}

		if cmdResp.Intent != IntentRoundtableStatus {
			t.Errorf("intent = %q, want %q", cmdResp.Intent, IntentRoundtableStatus)
		}
	})
}

// TestConcurrentAccess tests thread safety
func TestConcurrentAccess(t *testing.T) {
	registry := createTestRegistry()
	m := NewManager(registry, nil)
	defer m.Stop()

	done := make(chan bool)

	// Concurrent readers
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				m.getStatus()
			}
			done <- true
		}()
	}

	// Concurrent writers
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				m.UpdateState("test", "topic", "debating", j, nil)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}
}

// Helper function
func containsIgnoreCase(s, substr string) bool {
	return bytes.Contains(
		bytes.ToLower([]byte(s)),
		bytes.ToLower([]byte(substr)),
	)
}
