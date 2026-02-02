// internal/pensive/bridge.go
// Pensive memory bridge - integrates roundtable debates with AEGIS Pensive memory system.
// On debate completion, stores the debate transcript with semantic embeddings in Pensive L2.
// On new debate start, queries Pensive for relevant past debates to provide context.
package pensive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"roundtable/internal/consensus"
	"roundtable/internal/db"
)

const (
	// DefaultPensiveURL is the default Pensive vector service endpoint
	DefaultPensiveURL = "http://127.0.0.1:8009"

	// DefaultEmbedderURL is the default local embedding service endpoint
	// If not available, we'll store debates in a file for later indexing
	DefaultEmbedderURL = "http://127.0.0.1:8010/embed"

	// FallbackLogDir is where debates are logged when Pensive is unavailable
	FallbackLogDir = ".local/share/roundtable/pensive-pending"
)

// DebateRecord represents a debate stored in Pensive
type DebateRecord struct {
	Hash          string            `json:"hash"`
	DebateID      string            `json:"debate_id"`
	DebateName    string            `json:"debate_name"`
	ProjectPath   string            `json:"project_path,omitempty"`
	Summary       string            `json:"summary"`
	Transcript    string            `json:"transcript"`
	Consensus     string            `json:"consensus,omitempty"`
	Participants  []string          `json:"participants"`
	Outcome       string            `json:"outcome"` // resolved, abandoned, active
	KeyDecisions  []string          `json:"key_decisions,omitempty"`
	Entities      []string          `json:"entities,omitempty"`
	Dates         []string          `json:"dates,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	ExecutionMeta *ExecutionMeta    `json:"execution_meta,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// ExecutionMeta tracks execution outcomes for a debate
type ExecutionMeta struct {
	Executed     bool      `json:"executed"`
	Success      bool      `json:"success"`
	ExecutedAt   time.Time `json:"executed_at,omitempty"`
	ResultSummary string   `json:"result_summary,omitempty"`
}

// RetrievedDebate represents a past debate returned from Pensive
type RetrievedDebate struct {
	DebateID     string   `json:"debate_id"`
	DebateName   string   `json:"debate_name"`
	Summary      string   `json:"summary"`
	Consensus    string   `json:"consensus"`
	KeyDecisions []string `json:"key_decisions"`
	Outcome      string   `json:"outcome"`
	Score        float64  `json:"score"`
	WasSuccessful bool    `json:"was_successful"`
}

// PensiveSearchResult represents a result from Pensive vector search
type PensiveSearchResult struct {
	Hash     string   `json:"hash"`
	Summary  string   `json:"summary"`
	Score    float64  `json:"score"`
	Dates    []string `json:"dates"`
	Entities []string `json:"entities"`
	FaissID  int      `json:"faiss_id"`
}

// Bridge handles communication with the AEGIS Pensive memory system
type Bridge struct {
	pensiveURL   string
	embedderURL  string
	httpClient   *http.Client
	enabled      bool
	fallbackDir  string
	debugLog     bool
}

// NewBridge creates a new Pensive bridge with default settings
func NewBridge() *Bridge {
	homeDir, _ := os.UserHomeDir()
	fallbackDir := filepath.Join(homeDir, FallbackLogDir)

	return &Bridge{
		pensiveURL:  getEnvOr("PENSIVE_URL", DefaultPensiveURL),
		embedderURL: getEnvOr("EMBEDDER_URL", DefaultEmbedderURL),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		enabled:     true,
		fallbackDir: fallbackDir,
		debugLog:    os.Getenv("ROUNDTABLE_DEBUG") != "",
	}
}

// NewBridgeWithConfig creates a bridge with custom configuration
func NewBridgeWithConfig(pensiveURL, embedderURL string) *Bridge {
	b := NewBridge()
	if pensiveURL != "" {
		b.pensiveURL = pensiveURL
	}
	if embedderURL != "" {
		b.embedderURL = embedderURL
	}
	return b
}

// SetEnabled enables or disables the bridge
func (b *Bridge) SetEnabled(enabled bool) {
	b.enabled = enabled
}

// IsAvailable checks if the Pensive service is reachable
func (b *Bridge) IsAvailable(ctx context.Context) bool {
	if !b.enabled {
		return false
	}

	req, err := http.NewRequestWithContext(ctx, "GET", b.pensiveURL+"/stats", nil)
	if err != nil {
		return false
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// StoreDebate stores a completed debate in Pensive L2 memory
func (b *Bridge) StoreDebate(ctx context.Context, debate *db.Debate, messages []db.Message) error {
	if !b.enabled {
		return nil
	}

	record := b.buildDebateRecord(debate, messages)

	// Try to store in Pensive
	if err := b.storeToPensive(ctx, record); err != nil {
		b.log("Pensive storage failed, falling back to file: %v", err)
		// Fall back to file storage for later indexing
		return b.storeToFile(record)
	}

	return nil
}

// buildDebateRecord creates a DebateRecord from debate data
func (b *Bridge) buildDebateRecord(debate *db.Debate, messages []db.Message) *DebateRecord {
	// Build transcript
	var transcript strings.Builder
	participants := make(map[string]bool)
	var keyDecisions []string

	for _, msg := range messages {
		// Track participants
		if msg.Source != "system" && msg.Source != "user" {
			participants[msg.Source] = true
		}

		// Add to transcript
		transcript.WriteString(fmt.Sprintf("[%s] %s:\n%s\n\n",
			msg.CreatedAt.Format("15:04:05"),
			msg.Source,
			msg.Content))

		// Extract key decisions from consensus-style messages
		parsed := consensus.ParseResponse(msg.Content)
		if parsed.Position == consensus.PositionAgree && parsed.Target != "" {
			keyDecisions = append(keyDecisions, fmt.Sprintf("Agreement with %s: %s", parsed.Target, truncate(msg.Content, 200)))
		}
	}

	// Build participant list
	var participantList []string
	for p := range participants {
		participantList = append(participantList, p)
	}

	// Generate summary from consensus or last few messages
	summary := b.generateSummary(debate, messages)

	// Extract entities (model names, file paths, tech terms)
	entities := b.extractEntities(messages)

	// Extract date references
	dates := b.extractDates(messages)

	// Generate hash
	transcriptStr := transcript.String()
	hash := generateHash(debate.ID + transcriptStr)

	return &DebateRecord{
		Hash:         hash,
		DebateID:     debate.ID,
		DebateName:   debate.Name,
		ProjectPath:  debate.ProjectPath,
		Summary:      summary,
		Transcript:   transcriptStr,
		Consensus:    debate.Consensus,
		Participants: participantList,
		Outcome:      debate.Status,
		KeyDecisions: keyDecisions,
		Entities:     entities,
		Dates:        dates,
		Tags:         b.generateTags(debate, messages),
		CreatedAt:    debate.CreatedAt,
		UpdatedAt:    time.Now(),
		Metadata:     make(map[string]string),
	}
}

// generateSummary creates a summary of the debate
func (b *Bridge) generateSummary(debate *db.Debate, messages []db.Message) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Debate: %s\n", debate.Name))

	if debate.Consensus != "" {
		sb.WriteString(fmt.Sprintf("Consensus: %s\n", truncate(debate.Consensus, 500)))
	}

	// Add last substantive message as context
	for i := len(messages) - 1; i >= 0 && i > len(messages)-3; i-- {
		msg := messages[i]
		if msg.Source != "system" && len(msg.Content) > 50 {
			sb.WriteString(fmt.Sprintf("Final discussion: %s\n", truncate(msg.Content, 300)))
			break
		}
	}

	return sb.String()
}

// extractEntities extracts meaningful entities from messages
func (b *Bridge) extractEntities(messages []db.Message) []string {
	entities := make(map[string]bool)

	for _, msg := range messages {
		// Extract model names
		for _, model := range []string{"claude", "gpt", "gemini", "grok"} {
			if strings.Contains(strings.ToLower(msg.Content), model) {
				entities[model] = true
			}
		}

		// Extract file paths (simple heuristic)
		words := strings.Fields(msg.Content)
		for _, word := range words {
			if strings.Contains(word, "/") && strings.Contains(word, ".") {
				// Likely a file path
				clean := strings.Trim(word, "\"'`(),;:")
				if len(clean) > 5 && len(clean) < 200 {
					entities[clean] = true
				}
			}
		}
	}

	var result []string
	for e := range entities {
		result = append(result, e)
	}
	return result
}

// extractDates extracts date references from messages
func (b *Bridge) extractDates(messages []db.Message) []string {
	dates := make(map[string]bool)

	// Add debate dates
	for _, msg := range messages {
		dateStr := msg.CreatedAt.Format("2006-01-02")
		dates[dateStr] = true
	}

	var result []string
	for d := range dates {
		result = append(result, d)
	}
	return result
}

// generateTags creates tags for the debate
func (b *Bridge) generateTags(debate *db.Debate, messages []db.Message) []string {
	tags := []string{"roundtable-debate"}

	// Add outcome tag
	tags = append(tags, "outcome:"+debate.Status)

	// Add project tag if set
	if debate.ProjectPath != "" {
		tags = append(tags, "project:"+filepath.Base(debate.ProjectPath))
	}

	// Add participant tags
	participants := make(map[string]bool)
	for _, msg := range messages {
		if msg.Source != "system" && msg.Source != "user" {
			participants[msg.Source] = true
		}
	}
	for p := range participants {
		tags = append(tags, "participant:"+p)
	}

	return tags
}

// storeToPensive stores the record via Pensive HTTP API
func (b *Bridge) storeToPensive(ctx context.Context, record *DebateRecord) error {
	// First, try to get embedding for the summary
	embedding, err := b.getEmbedding(ctx, record.Summary)
	if err != nil {
		b.log("Embedding service unavailable: %v", err)
		// Store without embedding - Pensive can index later
		return b.storeMetadataOnly(ctx, record)
	}

	// Prepare insert payload
	vectors := [][]float64{embedding}
	metas := []map[string]interface{}{
		{
			"hash":         record.Hash,
			"debate_id":    record.DebateID,
			"debate_name":  record.DebateName,
			"summary":      record.Summary,
			"consensus":    record.Consensus,
			"dates":        record.Dates,
			"entities":     record.Entities,
			"outcome":      record.Outcome,
			"participants": strings.Join(record.Participants, ","),
		},
	}

	payload := map[string]interface{}{
		"vectors": vectors,
		"metas":   metas,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", b.pensiveURL+"/insert", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pensive returned %d: %s", resp.StatusCode, string(respBody))
	}

	b.log("Stored debate %s in Pensive", record.DebateID)
	return nil
}

// storeMetadataOnly stores the debate record without embedding
// This creates a file that can be batch-indexed later
func (b *Bridge) storeMetadataOnly(ctx context.Context, record *DebateRecord) error {
	return b.storeToFile(record)
}

// storeToFile stores the debate record to a JSON file for later indexing
func (b *Bridge) storeToFile(record *DebateRecord) error {
	if err := os.MkdirAll(b.fallbackDir, 0755); err != nil {
		return fmt.Errorf("create fallback dir: %w", err)
	}

	filename := fmt.Sprintf("debate_%s_%s.json",
		record.DebateID,
		record.UpdatedAt.Format("20060102_150405"))
	path := filepath.Join(b.fallbackDir, filename)

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	b.log("Stored debate to fallback file: %s", path)
	return nil
}

// getEmbedding gets an embedding vector from the local embedding service
func (b *Bridge) getEmbedding(ctx context.Context, text string) ([]float64, error) {
	payload := map[string]string{"text": text}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", b.embedderURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("embedder returned %d", resp.StatusCode)
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Embedding, nil
}

// QueryRelevantDebates queries Pensive for debates relevant to the given topic
func (b *Bridge) QueryRelevantDebates(ctx context.Context, topic string, topK int) ([]RetrievedDebate, error) {
	if !b.enabled {
		return nil, nil
	}

	// Get embedding for the topic
	embedding, err := b.getEmbedding(ctx, topic)
	if err != nil {
		b.log("Cannot query Pensive - embedding service unavailable: %v", err)
		return nil, nil // Graceful degradation
	}

	// Query Pensive
	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return nil, err
	}

	queryURL := fmt.Sprintf("%s/search?q=%s&k=%d",
		b.pensiveURL,
		url.QueryEscape(string(embeddingJSON)),
		topK)

	req, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.log("Pensive query failed: %v", err)
		return nil, nil // Graceful degradation
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, nil // Graceful degradation
	}

	var results []PensiveSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	// Convert to RetrievedDebate format
	var debates []RetrievedDebate
	for _, r := range results {
		// Only include roundtable debates (filter by hash prefix or metadata)
		// In a full implementation, we'd store and retrieve more metadata
		debates = append(debates, RetrievedDebate{
			DebateID:   r.Hash, // Would be actual debate_id from metadata
			Summary:    r.Summary,
			Score:      r.Score,
		})
	}

	return debates, nil
}

// FormatContextForDebate formats retrieved debates as context for a new debate
func (b *Bridge) FormatContextForDebate(debates []RetrievedDebate) string {
	if len(debates) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Relevant Past Debates\n\n")

	for i, d := range debates {
		if i >= 3 { // Limit to top 3
			break
		}

		sb.WriteString(fmt.Sprintf("### Previous Discussion: %s\n", d.DebateName))

		if d.Summary != "" {
			sb.WriteString(fmt.Sprintf("Summary: %s\n", d.Summary))
		}

		if d.Consensus != "" {
			sb.WriteString(fmt.Sprintf("Consensus reached: %s\n", d.Consensus))
		}

		if d.WasSuccessful {
			sb.WriteString("Outcome: This approach was implemented successfully.\n")
		}

		if len(d.KeyDecisions) > 0 {
			sb.WriteString("Key decisions:\n")
			for _, decision := range d.KeyDecisions {
				sb.WriteString(fmt.Sprintf("- %s\n", decision))
			}
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// UpdateExecutionOutcome updates the execution metadata for a stored debate
func (b *Bridge) UpdateExecutionOutcome(ctx context.Context, debateID string, success bool, resultSummary string) error {
	if !b.enabled {
		return nil
	}

	// For now, store execution outcomes in a separate tracking file
	// A full implementation would update the Pensive record's metadata
	record := ExecutionMeta{
		Executed:      true,
		Success:       success,
		ExecutedAt:    time.Now(),
		ResultSummary: resultSummary,
	}

	// Store execution outcome for later analysis
	if err := os.MkdirAll(b.fallbackDir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("execution_%s_%s.json",
		debateID,
		time.Now().Format("20060102_150405"))
	path := filepath.Join(b.fallbackDir, filename)

	data, err := json.MarshalIndent(map[string]interface{}{
		"debate_id": debateID,
		"execution": record,
	}, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	b.log("Stored execution outcome for debate %s: success=%v", debateID, success)
	return nil
}

// GetSuccessfulPatterns retrieves debates that led to successful outcomes
// This can be used to build a knowledge base of "what works"
func (b *Bridge) GetSuccessfulPatterns(ctx context.Context, topic string, limit int) ([]RetrievedDebate, error) {
	// Query all relevant debates
	debates, err := b.QueryRelevantDebates(ctx, topic, limit*2)
	if err != nil {
		return nil, err
	}

	// Filter to only successful ones
	// In a full implementation, this would be done via Pensive metadata filtering
	var successful []RetrievedDebate
	for _, d := range debates {
		if d.WasSuccessful {
			successful = append(successful, d)
			if len(successful) >= limit {
				break
			}
		}
	}

	return successful, nil
}

// log outputs debug information when debug mode is enabled
func (b *Bridge) log(format string, args ...interface{}) {
	if b.debugLog {
		log.Printf("[pensive-bridge] "+format, args...)
	}
}

// Helper functions

func getEnvOr(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func generateHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])[:16] // Short hash
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
