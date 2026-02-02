// internal/hermes/client_test.go
package hermes

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestNewClient verifies that NewClient creates a client with default endpoint
func TestNewClient(t *testing.T) {
	c := NewClient()

	if c == nil {
		t.Fatal("NewClient returned nil")
	}

	if c.endpoint != DefaultEndpoint {
		t.Errorf("expected endpoint %q, got %q", DefaultEndpoint, c.endpoint)
	}

	if c.httpClient == nil {
		t.Error("httpClient is nil")
	}

	if !c.enabled {
		t.Error("expected client to be enabled by default")
	}
}

// TestNewClientWithEndpoint verifies custom endpoint is used
func TestNewClientWithEndpoint(t *testing.T) {
	customEndpoint := "http://custom:9999/events"
	c := NewClientWithEndpoint(customEndpoint)

	if c == nil {
		t.Fatal("NewClientWithEndpoint returned nil")
	}

	if c.endpoint != customEndpoint {
		t.Errorf("expected endpoint %q, got %q", customEndpoint, c.endpoint)
	}

	if c.httpClient == nil {
		t.Error("httpClient is nil")
	}

	if !c.enabled {
		t.Error("expected client to be enabled by default")
	}
}

// TestSetEnabled verifies that SetEnabled toggles the enabled state
func TestSetEnabled(t *testing.T) {
	c := NewClient()

	// Start enabled
	if !c.enabled {
		t.Error("expected client to be enabled by default")
	}

	// Disable
	c.SetEnabled(false)
	if c.enabled {
		t.Error("expected client to be disabled after SetEnabled(false)")
	}

	// Re-enable
	c.SetEnabled(true)
	if !c.enabled {
		t.Error("expected client to be enabled after SetEnabled(true)")
	}
}

// TestEmitWhenDisabled verifies that Emit does nothing when disabled
func TestEmitWhenDisabled(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClientWithEndpoint(server.URL)
	c.SetEnabled(false)

	c.Emit("test_event", map[string]string{"key": "value"})

	// Give goroutine time to run if it was going to (it shouldn't)
	time.Sleep(50 * time.Millisecond)

	if called {
		t.Error("Emit should not send events when disabled")
	}
}

// TestEmitSendsCorrectPayload verifies the JSON payload structure
func TestEmitSendsCorrectPayload(t *testing.T) {
	var received Event
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		// Verify method and content type
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
			return
		}

		if err := json.Unmarshal(body, &received); err != nil {
			t.Errorf("failed to unmarshal body: %v", err)
			return
		}

		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer server.Close()

	c := NewClientWithEndpoint(server.URL)

	beforeEmit := time.Now().Unix()
	c.Emit("test_event", map[string]string{"foo": "bar", "baz": "qux"})

	// Wait for the async request
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	mu.Lock()
	defer mu.Unlock()

	if received.Type != "test_event" {
		t.Errorf("expected type %q, got %q", "test_event", received.Type)
	}

	if received.Source != "roundtable" {
		t.Errorf("expected source %q, got %q", "roundtable", received.Source)
	}

	if received.Timestamp < beforeEmit {
		t.Errorf("timestamp %d is before emit time %d", received.Timestamp, beforeEmit)
	}

	if received.Data["foo"] != "bar" {
		t.Errorf("expected data[foo]=%q, got %q", "bar", received.Data["foo"])
	}

	if received.Data["baz"] != "qux" {
		t.Errorf("expected data[baz]=%q, got %q", "qux", received.Data["baz"])
	}
}

// TestDebateStarted verifies the DebateStarted convenience method
func TestDebateStarted(t *testing.T) {
	var received Event
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer server.Close()

	c := NewClientWithEndpoint(server.URL)
	c.DebateStarted("debate-123", "Test Debate", 3)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	mu.Lock()
	defer mu.Unlock()

	if received.Type != EventDebateStarted {
		t.Errorf("expected type %q, got %q", EventDebateStarted, received.Type)
	}

	if received.Data["debate_id"] != "debate-123" {
		t.Errorf("expected debate_id %q, got %q", "debate-123", received.Data["debate_id"])
	}

	if received.Data["debate_name"] != "Test Debate" {
		t.Errorf("expected debate_name %q, got %q", "Test Debate", received.Data["debate_name"])
	}

	if received.Data["models"] != "3" {
		t.Errorf("expected models %q, got %q", "3", received.Data["models"])
	}
}

// TestConsensusReached verifies the ConsensusReached convenience method
func TestConsensusReached(t *testing.T) {
	var received Event
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer server.Close()

	c := NewClientWithEndpoint(server.URL)
	c.ConsensusReached("debate-456", "The models agreed on X")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	mu.Lock()
	defer mu.Unlock()

	if received.Type != EventConsensusReached {
		t.Errorf("expected type %q, got %q", EventConsensusReached, received.Type)
	}

	if received.Data["debate_id"] != "debate-456" {
		t.Errorf("expected debate_id %q, got %q", "debate-456", received.Data["debate_id"])
	}

	if received.Data["consensus"] != "The models agreed on X" {
		t.Errorf("expected consensus %q, got %q", "The models agreed on X", received.Data["consensus"])
	}
}

// TestConsensusReachedTruncatesLongText verifies text truncation
func TestConsensusReachedTruncatesLongText(t *testing.T) {
	var received Event
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer server.Close()

	c := NewClientWithEndpoint(server.URL)

	// Create a string longer than 200 characters
	longText := strings.Repeat("x", 250)
	c.ConsensusReached("debate-789", longText)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	mu.Lock()
	defer mu.Unlock()

	consensus := received.Data["consensus"]
	if len(consensus) != 200 {
		t.Errorf("expected truncated length 200, got %d", len(consensus))
	}

	if !strings.HasSuffix(consensus, "...") {
		t.Error("expected truncated text to end with '...'")
	}
}

// TestExecutionCompleteSuccess verifies ExecutionComplete with success=true
func TestExecutionCompleteSuccess(t *testing.T) {
	var received Event
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer server.Close()

	c := NewClientWithEndpoint(server.URL)
	c.ExecutionComplete("debate-success", true, "Executed successfully")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	mu.Lock()
	defer mu.Unlock()

	if received.Type != EventExecutionComplete {
		t.Errorf("expected type %q, got %q", EventExecutionComplete, received.Type)
	}

	if received.Data["debate_id"] != "debate-success" {
		t.Errorf("expected debate_id %q, got %q", "debate-success", received.Data["debate_id"])
	}

	if received.Data["status"] != "success" {
		t.Errorf("expected status %q, got %q", "success", received.Data["status"])
	}

	if received.Data["result"] != "Executed successfully" {
		t.Errorf("expected result %q, got %q", "Executed successfully", received.Data["result"])
	}
}

// TestExecutionCompleteFailure verifies ExecutionComplete with success=false
func TestExecutionCompleteFailure(t *testing.T) {
	var received Event
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer server.Close()

	c := NewClientWithEndpoint(server.URL)
	c.ExecutionComplete("debate-failure", false, "Something went wrong")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	mu.Lock()
	defer mu.Unlock()

	if received.Data["status"] != "failure" {
		t.Errorf("expected status %q, got %q", "failure", received.Data["status"])
	}
}

// TestFormatModelCount verifies the formatModelCount helper
func TestFormatModelCount(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{2, "2"},
		{3, "3"},
		{4, "4"},
		{5, "4+"},
		{10, "4+"},
		{100, "4+"},
	}

	for _, tt := range tests {
		result := formatModelCount(tt.input)
		if result != tt.expected {
			t.Errorf("formatModelCount(%d) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

// TestTruncate verifies the truncate helper function
func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "shorter than max",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "equal to max",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "longer than max",
			input:    "hello world",
			maxLen:   8,
			expected: "hello...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "exactly 3 chars over max",
			input:    "abcdefghij",
			maxLen:   7,
			expected: "abcd...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncate(%q, %d) = %q, expected %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

// TestEmitWithNilData verifies that Emit handles nil data gracefully
func TestEmitWithNilData(t *testing.T) {
	var received Event
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer server.Close()

	c := NewClientWithEndpoint(server.URL)
	c.Emit("test_nil_data", nil)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	mu.Lock()
	defer mu.Unlock()

	if received.Type != "test_nil_data" {
		t.Errorf("expected type %q, got %q", "test_nil_data", received.Type)
	}

	// Data can be nil or empty map depending on JSON marshaling
	if received.Data != nil && len(received.Data) > 0 {
		t.Errorf("expected nil or empty data, got %v", received.Data)
	}
}

// TestEmitHandlesServerError verifies that server errors don't panic
func TestEmitHandlesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClientWithEndpoint(server.URL)

	// This should not panic
	c.Emit("test_error", map[string]string{"key": "value"})

	// Give the goroutine time to complete
	time.Sleep(50 * time.Millisecond)
}

// TestEmitHandlesConnectionError verifies that connection errors don't panic
func TestEmitHandlesConnectionError(t *testing.T) {
	// Use an invalid endpoint that will fail to connect
	c := NewClientWithEndpoint("http://localhost:1/nonexistent")

	// This should not panic
	c.Emit("test_connection_error", map[string]string{"key": "value"})

	// Give the goroutine time to attempt and fail
	time.Sleep(50 * time.Millisecond)
}

// TestEventConstants verifies the event type constants
func TestEventConstants(t *testing.T) {
	if EventDebateStarted != "debate_started" {
		t.Errorf("EventDebateStarted = %q, expected %q", EventDebateStarted, "debate_started")
	}

	if EventConsensusReached != "consensus_reached" {
		t.Errorf("EventConsensusReached = %q, expected %q", EventConsensusReached, "consensus_reached")
	}

	if EventExecutionComplete != "execution_complete" {
		t.Errorf("EventExecutionComplete = %q, expected %q", EventExecutionComplete, "execution_complete")
	}
}

// TestDefaultEndpointConstant verifies the default endpoint constant
func TestDefaultEndpointConstant(t *testing.T) {
	expected := "http://localhost:5965/event"
	if DefaultEndpoint != expected {
		t.Errorf("DefaultEndpoint = %q, expected %q", DefaultEndpoint, expected)
	}
}
