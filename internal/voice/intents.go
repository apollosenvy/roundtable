// internal/voice/intents.go
// Voice control integration for roundtable
// Provides HTTP endpoints for voice-control.py to interact with roundtable
package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"roundtable/internal/consensus"
	"roundtable/internal/models"
	"roundtable/internal/orchestrator"
)

const (
	// DefaultPort for the voice control server
	DefaultPort = 5970

	// VoiceNotifyEndpoint for TTS feedback
	VoiceNotifyEndpoint = "http://localhost:5959"

	// Intent names matching voice-control.py expectations
	IntentStartRoundtable = "START_ROUNDTABLE"
	IntentRoundtableStatus = "ROUNDTABLE_STATUS"
	IntentRoundtableConsensus = "ROUNDTABLE_CONSENSUS"
	IntentCloseRoundtable = "CLOSE_ROUNDTABLE"
)

// DebateState represents the current state of a voice-initiated debate
type DebateState struct {
	ID          string
	Topic       string
	Status      string // "idle", "debating", "consensus", "closed"
	MessageCount int
	ModelCount   int
	Positions   map[string]consensus.Position
	LastUpdate  time.Time
	Messages    []StateMessage
}

// StateMessage represents a simplified message for voice status
type StateMessage struct {
	Source  string
	Summary string // Truncated content
}

// Manager handles voice control state and operations
type Manager struct {
	mu           sync.RWMutex
	currentState *DebateState
	registry     *models.Registry
	orchestrator *orchestrator.Orchestrator
	httpServer   *http.Server
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewManager creates a new voice control manager
func NewManager(registry *models.Registry, orch *orchestrator.Orchestrator) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		registry:     registry,
		orchestrator: orch,
		ctx:          ctx,
		cancel:       cancel,
		currentState: &DebateState{
			Status:    "idle",
			Positions: make(map[string]consensus.Position),
		},
	}
}

// Start starts the voice control HTTP server
func (m *Manager) Start(port int) error {
	mux := http.NewServeMux()

	// GET /voice/status - returns speakable status
	mux.HandleFunc("/voice/status", m.handleStatus)

	// POST /voice/command - accepts voice commands
	mux.HandleFunc("/voice/command", m.handleCommand)

	// GET /voice/health - health check
	mux.HandleFunc("/voice/health", m.handleHealth)

	m.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("[voice] Starting voice control server on port %d", port)

	go func() {
		if err := m.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[voice] Server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the voice control server
func (m *Manager) Stop() error {
	m.cancel()
	if m.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return m.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleHealth returns server health status
func (m *Manager) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"service": "roundtable-voice",
		"timestamp": time.Now().Unix(),
	})
}

// handleStatus returns the current debate status in a speakable format
func (m *Manager) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	m.mu.RLock()
	state := m.currentState
	m.mu.RUnlock()

	var speakableStatus string

	switch state.Status {
	case "idle":
		speakableStatus = "Roundtable is idle. No active debate."

	case "debating":
		elapsed := time.Since(state.LastUpdate)
		if elapsed < 30*time.Second {
			speakableStatus = fmt.Sprintf("Roundtable is active. Topic: %s. %d messages from %d models. Models are still responding.",
				state.Topic, state.MessageCount, state.ModelCount)
		} else {
			speakableStatus = fmt.Sprintf("Roundtable discussing: %s. %d messages exchanged. Last activity %s ago.",
				state.Topic, state.MessageCount, formatDuration(elapsed))
		}

	case "consensus":
		agrees := 0
		objects := 0
		for _, pos := range state.Positions {
			if pos == consensus.PositionAgree {
				agrees++
			} else if pos == consensus.PositionObject {
				objects++
			}
		}
		if objects == 0 && agrees > 0 {
			speakableStatus = fmt.Sprintf("Consensus reached on %s. All %d models agree.", state.Topic, agrees)
		} else {
			speakableStatus = fmt.Sprintf("Consensus check on %s. %d agree, %d object.", state.Topic, agrees, objects)
		}

	case "closed":
		speakableStatus = fmt.Sprintf("Roundtable on %s is closed. %d messages were exchanged.", state.Topic, state.MessageCount)

	default:
		speakableStatus = fmt.Sprintf("Roundtable status: %s", state.Status)
	}

	response := StatusResponse{
		Status:   state.Status,
		Topic:    state.Topic,
		Speakable: speakableStatus,
		MessageCount: state.MessageCount,
		ModelCount:   state.ModelCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCommand processes voice commands
func (m *Manager) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var cmd CommandRequest
	if err := json.Unmarshal(body, &cmd); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	response := m.processCommand(cmd)

	// Emit TTS feedback
	if response.Speakable != "" {
		go speak(response.Speakable, "normal")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// processCommand handles a voice command and returns a response
func (m *Manager) processCommand(cmd CommandRequest) CommandResponse {
	// Normalize the command text
	text := strings.ToLower(strings.TrimSpace(cmd.Text))

	// Pattern matching for voice commands
	startPattern := regexp.MustCompile(`(?:start|begin|open|launch)\s+(?:a\s+)?roundtable\s+(?:about|on|for|discussing)?\s*(.+)`)
	statusPattern := regexp.MustCompile(`(?:roundtable|debate)\s+status`)
	consensusPattern := regexp.MustCompile(`(?:roundtable|debate)\s+consensus|check\s+consensus|force\s+consensus`)
	closePattern := regexp.MustCompile(`(?:close|end|stop|finish)\s+(?:the\s+)?(?:roundtable|debate)`)

	var response CommandResponse
	response.Success = true

	switch {
	case startPattern.MatchString(text):
		matches := startPattern.FindStringSubmatch(text)
		topic := "general discussion"
		if len(matches) > 1 && matches[1] != "" {
			topic = strings.TrimSpace(matches[1])
		}
		response = m.startRoundtable(topic)
		response.Intent = IntentStartRoundtable

	case statusPattern.MatchString(text):
		response = m.getStatus()
		response.Intent = IntentRoundtableStatus

	case consensusPattern.MatchString(text):
		response = m.checkConsensus()
		response.Intent = IntentRoundtableConsensus

	case closePattern.MatchString(text):
		response = m.closeRoundtable()
		response.Intent = IntentCloseRoundtable

	default:
		response.Success = false
		response.Speakable = "I didn't understand that roundtable command."
		response.Error = "unrecognized command"
	}

	return response
}

// startRoundtable initiates a new debate on the given topic
func (m *Manager) startRoundtable(topic string) CommandResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already debating
	if m.currentState.Status == "debating" {
		return CommandResponse{
			Success:   false,
			Speakable: fmt.Sprintf("Already debating %s. Close this debate first.", m.currentState.Topic),
			Error:     "debate in progress",
		}
	}

	// Initialize new debate state
	m.currentState = &DebateState{
		ID:         fmt.Sprintf("voice-%d", time.Now().Unix()),
		Topic:      topic,
		Status:     "debating",
		ModelCount: m.registry.Count(),
		LastUpdate: time.Now(),
		Positions:  make(map[string]consensus.Position),
	}

	modelNames := make([]string, 0, m.registry.Count())
	for _, model := range m.registry.All() {
		modelNames = append(modelNames, model.Info().Name)
	}

	speakable := fmt.Sprintf("Starting roundtable on %s with %d models: %s.",
		topic, m.currentState.ModelCount, formatModelList(modelNames))

	return CommandResponse{
		Success:   true,
		Speakable: speakable,
		Data: map[string]interface{}{
			"debate_id": m.currentState.ID,
			"topic":     topic,
			"models":    modelNames,
		},
	}
}

// getStatus returns the current debate status
func (m *Manager) getStatus() CommandResponse {
	m.mu.RLock()
	state := m.currentState
	m.mu.RUnlock()

	speakable := m.buildSpeakableStatus(state)

	return CommandResponse{
		Success:   true,
		Speakable: speakable,
		Data: map[string]interface{}{
			"status":        state.Status,
			"topic":         state.Topic,
			"message_count": state.MessageCount,
			"model_count":   state.ModelCount,
		},
	}
}

// checkConsensus triggers a consensus check
func (m *Manager) checkConsensus() CommandResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentState.Status != "debating" {
		return CommandResponse{
			Success:   false,
			Speakable: "No active debate to check consensus on.",
			Error:     "no active debate",
		}
	}

	// Update status
	m.currentState.Status = "consensus"

	// Analyze positions from messages if available
	result := consensus.AnalyzeConsensus(m.getParsedPositions())

	var speakable string
	if result.HasConsensus {
		speakable = fmt.Sprintf("Consensus reached. %d models agree.", result.AgreeCount)
		if result.AgreementTarget != "" {
			speakable += fmt.Sprintf(" They agree with %s's approach.", result.AgreementTarget)
		}
	} else {
		speakable = fmt.Sprintf("No consensus yet. %d agree, %d object, %d positions unknown.",
			result.AgreeCount, result.ObjectCount, result.UnknownCount)
		if len(result.Objections) > 0 {
			speakable += fmt.Sprintf(" Main objection: %s", truncate(result.Objections[0], 50))
		}
	}

	return CommandResponse{
		Success:   true,
		Speakable: speakable,
		Data: map[string]interface{}{
			"has_consensus":  result.HasConsensus,
			"agree_count":    result.AgreeCount,
			"object_count":   result.ObjectCount,
			"objections":     result.Objections,
			"additions":      result.Additions,
		},
	}
}

// closeRoundtable closes the current debate
func (m *Manager) closeRoundtable() CommandResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentState.Status == "idle" || m.currentState.Status == "closed" {
		return CommandResponse{
			Success:   false,
			Speakable: "No active roundtable to close.",
			Error:     "no active debate",
		}
	}

	topic := m.currentState.Topic
	messageCount := m.currentState.MessageCount

	m.currentState.Status = "closed"

	speakable := fmt.Sprintf("Roundtable on %s is now closed. %d messages were exchanged.",
		topic, messageCount)

	return CommandResponse{
		Success:   true,
		Speakable: speakable,
		Data: map[string]interface{}{
			"topic":         topic,
			"message_count": messageCount,
		},
	}
}

// UpdateState updates the voice manager's state from the TUI
// This is called by the main app to keep voice state in sync
func (m *Manager) UpdateState(debateID, topic, status string, messageCount int, messages []StateMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentState.ID = debateID
	m.currentState.Topic = topic
	m.currentState.Status = status
	m.currentState.MessageCount = messageCount
	m.currentState.LastUpdate = time.Now()
	m.currentState.Messages = messages

	// Parse positions from recent messages
	for _, msg := range messages {
		if msg.Source != "user" && msg.Source != "system" {
			pos := consensus.DetectPosition(msg.Summary)
			m.currentState.Positions[msg.Source] = pos
		}
	}
}

// getParsedPositions returns parsed positions for consensus analysis
func (m *Manager) getParsedPositions() map[string]consensus.ParsedPosition {
	parsed := make(map[string]consensus.ParsedPosition)
	for _, msg := range m.currentState.Messages {
		if msg.Source != "user" && msg.Source != "system" {
			parsed[msg.Source] = consensus.ParseResponse(msg.Summary)
		}
	}
	return parsed
}

// buildSpeakableStatus creates a natural language status description
func (m *Manager) buildSpeakableStatus(state *DebateState) string {
	switch state.Status {
	case "idle":
		return "Roundtable is idle. Say 'start roundtable about' followed by a topic to begin."

	case "debating":
		elapsed := time.Since(state.LastUpdate)
		if elapsed < 30*time.Second {
			return fmt.Sprintf("Actively debating %s. %d messages from %d models.",
				state.Topic, state.MessageCount, state.ModelCount)
		}
		return fmt.Sprintf("Debating %s. %d messages. Last activity %s ago.",
			state.Topic, state.MessageCount, formatDuration(elapsed))

	case "consensus":
		agrees := 0
		objects := 0
		for _, pos := range state.Positions {
			if pos == consensus.PositionAgree {
				agrees++
			} else if pos == consensus.PositionObject {
				objects++
			}
		}
		return fmt.Sprintf("Checking consensus on %s. %d agree, %d object.",
			state.Topic, agrees, objects)

	case "closed":
		return fmt.Sprintf("Roundtable on %s is closed.", state.Topic)

	default:
		return fmt.Sprintf("Roundtable status: %s", state.Status)
	}
}

// speak sends text to voice-notify for TTS
func speak(text string, priority string) {
	payload := map[string]string{
		"text":     text,
		"priority": priority,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[voice] Failed to marshal TTS payload: %v", err)
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(VoiceNotifyEndpoint+"/speak", "application/json", bytes.NewReader(body))
	if err != nil {
		// Expected when voice-notify isn't running
		log.Printf("[voice] TTS delivery failed (voice-notify may not be running): %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("[voice] TTS rejected with status %d", resp.StatusCode)
	}
}

// formatDuration formats a duration in human-readable form
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	return fmt.Sprintf("%d hours", int(d.Hours()))
}

// formatModelList creates a natural language list of models
func formatModelList(models []string) string {
	switch len(models) {
	case 0:
		return "no models"
	case 1:
		return models[0]
	case 2:
		return models[0] + " and " + models[1]
	default:
		return strings.Join(models[:len(models)-1], ", ") + ", and " + models[len(models)-1]
	}
}

// truncate limits a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// --- Request/Response types ---

// StatusResponse is returned by GET /voice/status
type StatusResponse struct {
	Status       string `json:"status"`
	Topic        string `json:"topic,omitempty"`
	Speakable    string `json:"speakable"`
	MessageCount int    `json:"message_count"`
	ModelCount   int    `json:"model_count"`
}

// CommandRequest is accepted by POST /voice/command
type CommandRequest struct {
	Text   string `json:"text"`   // The voice command text
	Intent string `json:"intent"` // Optional pre-classified intent
}

// CommandResponse is returned by POST /voice/command
type CommandResponse struct {
	Success   bool                   `json:"success"`
	Intent    string                 `json:"intent,omitempty"`
	Speakable string                 `json:"speakable"`
	Error     string                 `json:"error,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}
