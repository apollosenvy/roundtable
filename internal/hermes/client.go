// internal/hermes/client.go
// Hermes event integration for session tracking
// Emits fire-and-forget events to the Hermes daemon at localhost:5965
package hermes

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

const (
	// DefaultEndpoint is the Hermes event endpoint
	DefaultEndpoint = "http://localhost:5965/event"

	// Event types
	EventDebateStarted     = "debate_started"
	EventConsensusReached  = "consensus_reached"
	EventExecutionComplete = "execution_complete"
)

// Event represents a Hermes event payload
type Event struct {
	Type      string            `json:"type"`
	Source    string            `json:"source"`
	Timestamp int64             `json:"timestamp"`
	Data      map[string]string `json:"data,omitempty"`
}

// Client handles communication with the Hermes daemon
type Client struct {
	endpoint        string
	httpClient      *http.Client
	enabled         bool
	connErrorLogged bool // Only log connection errors once
}

// NewClient creates a new Hermes client
func NewClient() *Client {
	return &Client{
		endpoint: DefaultEndpoint,
		httpClient: &http.Client{
			Timeout: 2 * time.Second, // Short timeout for fire-and-forget
		},
		enabled: true,
	}
}

// NewClientWithEndpoint creates a client with a custom endpoint
func NewClientWithEndpoint(endpoint string) *Client {
	c := NewClient()
	c.endpoint = endpoint
	return c
}

// SetEnabled enables or disables event emission
func (c *Client) SetEnabled(enabled bool) {
	c.enabled = enabled
}

// Emit sends an event to Hermes asynchronously (fire and forget)
func (c *Client) Emit(eventType string, data map[string]string) {
	if !c.enabled {
		return
	}

	event := Event{
		Type:      eventType,
		Source:    "roundtable",
		Timestamp: time.Now().Unix(),
		Data:      data,
	}

	// Fire and forget - run in goroutine
	go c.send(event)
}

// send performs the actual HTTP POST (runs in goroutine)
func (c *Client) send(event Event) {
	body, err := json.Marshal(event)
	if err != nil {
		log.Printf("[hermes] failed to marshal event: %v", err)
		return
	}

	resp, err := c.httpClient.Post(c.endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		// Connection failures are expected when Hermes isn't running
		// Silently ignore - no need to log
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("[hermes] event rejected with status %d", resp.StatusCode)
	}
}

// DebateStarted emits a debate_started event
func (c *Client) DebateStarted(debateID, debateName string, modelCount int) {
	c.Emit(EventDebateStarted, map[string]string{
		"debate_id":   debateID,
		"debate_name": debateName,
		"models":      formatModelCount(modelCount),
	})
}

// ConsensusReached emits a consensus_reached event
func (c *Client) ConsensusReached(debateID string, consensusText string) {
	c.Emit(EventConsensusReached, map[string]string{
		"debate_id": debateID,
		"consensus": truncate(consensusText, 200),
	})
}

// ExecutionComplete emits an execution_complete event
func (c *Client) ExecutionComplete(debateID string, success bool, result string) {
	status := "success"
	if !success {
		status = "failure"
	}
	c.Emit(EventExecutionComplete, map[string]string{
		"debate_id": debateID,
		"status":    status,
		"result":    truncate(result, 200),
	})
}

// formatModelCount returns a string representation of model count
func formatModelCount(count int) string {
	switch count {
	case 0:
		return "0"
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3"
	case 4:
		return "4"
	default:
		return "4+"
	}
}

// truncate limits a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
