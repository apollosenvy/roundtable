// internal/memory/session.go
// Session memory integration for learning from debates
// Logs outcomes, failures, and insights to AEGIS session memory system
// so future Claude sessions can learn from roundtable debates.
package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	// HermesEndpoint is the Hermes event endpoint for fire-and-forget events
	HermesEndpoint = "http://localhost:5965/event"

	// ProjectName identifies roundtable in session memory
	ProjectName = "roundtable"
)

// Client handles session memory operations
type Client struct {
	httpClient *http.Client
	enabled    bool

	// Paths to helper scripts
	hermesEmitPath       string
	sessionMemoryPath    string
	sessionReplayPath    string
}

// NewClient creates a new session memory client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
		enabled:           true,
		hermesEmitPath:    "/home/aegis/bin/hermes-emit",
		sessionMemoryPath: "/home/aegis/Projects/scripts/claude-session-memory.py",
		sessionReplayPath: "/home/aegis/Projects/scripts/session-replay.py",
	}
}

// SetEnabled enables or disables session memory logging
func (c *Client) SetEnabled(enabled bool) {
	c.enabled = enabled
}

// IsEnabled returns whether session memory is enabled
func (c *Client) IsEnabled() bool {
	return c.enabled
}

// DebateOutcome represents the result of a debate
type DebateOutcome struct {
	DebateID    string
	DebateName  string
	Consensus   string   // The consensus text if reached
	Summary     string   // Brief summary of what was decided
	ModelVotes  map[string]string // Model ID -> their position (AGREE/OBJECT/ADD)
	Additions   []string // Additional points raised during debate
}

// FailedApproach represents an approach that was rejected during debate
type FailedApproach struct {
	DebateID   string
	Approach   string // What was proposed
	Reason     string // Why it was rejected
	RejectedBy []string // Which models objected
}

// Insight represents a cross-project insight from a debate
type Insight struct {
	DebateID string
	Insight  string
	Projects []string // Related projects mentioned
}

// LogConsensus logs a successful debate outcome to session memory
func (c *Client) LogConsensus(outcome DebateOutcome) {
	if !c.enabled {
		return
	}

	// Build the discovery message
	summary := outcome.Summary
	if summary == "" {
		summary = outcome.Consensus
	}

	// Truncate for readability
	if len(summary) > 300 {
		summary = summary[:297] + "..."
	}

	discovery := fmt.Sprintf("Consensus: %s", summary)

	// Fire and forget via hermes-emit
	go c.emitDiscovery(discovery)

	// Also log to session memory directly for persistence
	go c.logToSessionMemory("log", discovery)
}

// LogFailure logs a rejected approach to session memory
func (c *Client) LogFailure(failure FailedApproach) {
	if !c.enabled {
		return
	}

	// Build the failure message
	rejectors := strings.Join(failure.RejectedBy, ", ")
	message := fmt.Sprintf("Approach '%s' rejected by %s: %s",
		truncate(failure.Approach, 100),
		rejectors,
		truncate(failure.Reason, 150))

	// Fire and forget via hermes-emit
	go c.emitFailure(message)

	// Also log to session memory directly
	go c.logToSessionMemory("fail", message)
}

// LogInsight logs a cross-project insight discovered during debate
func (c *Client) LogInsight(insight Insight) {
	if !c.enabled {
		return
	}

	// Fire and forget via hermes-emit
	go c.emitInsight(insight.Insight)

	// Also log to session memory with project connections
	go c.logInsightToSessionMemory(insight)
}

// QueryRelevantLearnings retrieves past learnings relevant to a debate topic
// Returns formatted context that can be injected into debate prompts
func (c *Client) QueryRelevantLearnings(topic string) (string, error) {
	if !c.enabled {
		return "", nil
	}

	// Query session-replay for context
	cmd := exec.Command("python3", c.sessionReplayPath, "context", topic, "--limit", "5")
	output, err := cmd.Output()
	if err != nil {
		// Session replay might not have relevant data, that's ok
		log.Printf("[memory] session-replay query returned no results: %v", err)
		return "", nil
	}

	result := string(output)
	if strings.Contains(result, "No historical context found") {
		return "", nil
	}

	// Also get roundtable-specific failures to avoid
	failures := c.getRelevantFailures(topic)

	if result == "" && failures == "" {
		return "", nil
	}

	var context strings.Builder
	context.WriteString("=== Past Session Context ===\n")

	if result != "" {
		context.WriteString("\nRelevant History:\n")
		context.WriteString(result)
	}

	if failures != "" {
		context.WriteString("\nApproaches to Avoid (previously rejected):\n")
		context.WriteString(failures)
	}

	context.WriteString("\n=== End Past Context ===\n")

	return context.String(), nil
}

// getRelevantFailures retrieves failed approaches from roundtable debates
func (c *Client) getRelevantFailures(topic string) string {
	cmd := exec.Command("python3", c.sessionMemoryPath, "failures", "--project", ProjectName)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	result := string(output)
	if strings.Contains(result, "No failed approaches") {
		return ""
	}

	// Filter to relevant failures based on topic keywords
	topicWords := extractKeywords(topic)
	var relevant []string

	lines := strings.Split(result, "\n")
	for _, line := range lines {
		lineLower := strings.ToLower(line)
		for _, word := range topicWords {
			if strings.Contains(lineLower, strings.ToLower(word)) {
				relevant = append(relevant, strings.TrimSpace(line))
				break
			}
		}
	}

	if len(relevant) == 0 {
		return ""
	}

	return strings.Join(relevant, "\n")
}

// DetectProjectMentions scans debate content for mentions of known projects
// Used to identify cross-project insights
func (c *Client) DetectProjectMentions(content string) []string {
	// List of known AEGIS projects to detect
	knownProjects := []string{
		"AEGIS", "Pensive", "Mud-Puppy", "Kesagake", "OSINT",
		"Lenora", "Panoptes", "Hermes", "Kairos",
		"l1_cache", "l2_handler", "inference", "triton",
		"voice-control", "nerve-center",
	}

	contentLower := strings.ToLower(content)
	var found []string
	seen := make(map[string]bool)

	for _, project := range knownProjects {
		projectLower := strings.ToLower(project)
		if strings.Contains(contentLower, projectLower) && !seen[project] {
			found = append(found, project)
			seen[project] = true
		}
	}

	return found
}

// emitDiscovery sends a discovery event via hermes-emit
func (c *Client) emitDiscovery(discovery string) {
	// Try hermes-emit first (if available)
	cmd := exec.Command(c.hermesEmitPath, "discovery", ProjectName, discovery)
	if err := cmd.Run(); err != nil {
		// Fall back to HTTP if hermes-emit not available
		c.sendHermesEvent("discovery", map[string]string{
			"project": ProjectName,
			"content": discovery,
		})
	}
}

// emitFailure sends a failure event via hermes-emit
func (c *Client) emitFailure(failure string) {
	cmd := exec.Command(c.hermesEmitPath, "failure", ProjectName, failure)
	if err := cmd.Run(); err != nil {
		c.sendHermesEvent("failure", map[string]string{
			"project": ProjectName,
			"content": failure,
		})
	}
}

// emitInsight sends an insight event via hermes-emit
func (c *Client) emitInsight(insight string) {
	cmd := exec.Command(c.hermesEmitPath, "insight", insight)
	if err := cmd.Run(); err != nil {
		c.sendHermesEvent("insight", map[string]string{
			"content": insight,
		})
	}
}

// sendHermesEvent sends an event directly to the Hermes daemon
func (c *Client) sendHermesEvent(eventType string, data map[string]string) {
	event := map[string]interface{}{
		"type":      eventType,
		"source":    "roundtable",
		"timestamp": time.Now().Unix(),
		"data":      data,
	}

	body, err := json.Marshal(event)
	if err != nil {
		log.Printf("[memory] failed to marshal event: %v", err)
		return
	}

	resp, err := c.httpClient.Post(HermesEndpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		// Hermes might not be running, that's ok
		log.Printf("[memory] hermes event delivery failed (daemon may not be running): %v", err)
		return
	}
	defer resp.Body.Close()
}

// logToSessionMemory calls the session memory script directly
func (c *Client) logToSessionMemory(cmd string, message string) {
	args := []string{c.sessionMemoryPath, cmd, ProjectName, message}
	if err := exec.Command("python3", args...).Run(); err != nil {
		log.Printf("[memory] session memory %s failed: %v", cmd, err)
	}
}

// logInsightToSessionMemory logs a cross-project insight with related projects
func (c *Client) logInsightToSessionMemory(insight Insight) {
	// Build the command with all related projects
	args := []string{c.sessionMemoryPath, "insight", "--text", insight.Insight}
	args = append(args, "--projects")

	// Always include roundtable
	projects := append([]string{ProjectName}, insight.Projects...)
	args = append(args, projects...)

	if err := exec.Command("python3", args...).Run(); err != nil {
		log.Printf("[memory] insight logging failed: %v", err)
	}
}

// extractKeywords extracts significant keywords from a topic string
func extractKeywords(topic string) []string {
	// Remove common words
	stopwords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "can": true,
		"to": true, "of": true, "in": true, "for": true, "on": true,
		"with": true, "at": true, "by": true, "from": true, "about": true,
		"into": true, "through": true, "during": true, "before": true, "after": true,
		"and": true, "or": true, "but": true, "if": true, "then": true,
		"this": true, "that": true, "these": true, "those": true,
		"what": true, "which": true, "who": true, "how": true, "why": true, "when": true,
		"it": true, "its": true,
	}

	// Split and filter
	words := regexp.MustCompile(`\w+`).FindAllString(strings.ToLower(topic), -1)
	var keywords []string
	seen := make(map[string]bool)

	for _, word := range words {
		if len(word) > 2 && !stopwords[word] && !seen[word] {
			keywords = append(keywords, word)
			seen[word] = true
		}
	}

	return keywords
}

// truncate limits a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// AnalyzeDebateForLearnings examines a completed debate and extracts learnings
// This should be called when consensus is reached or debate concludes
type DebateLearning struct {
	Consensus     *DebateOutcome
	Failures      []FailedApproach
	Insights      []Insight
}

// ExtractLearnings analyzes debate messages to find learnings
func (c *Client) ExtractLearnings(debateID, debateName string, messages []DebateMessage) DebateLearning {
	learning := DebateLearning{}

	var objections []FailedApproach
	var modelPositions = make(map[string]string)
	var additions []string
	var consensusText string
	var mentionedProjects = make(map[string]bool)

	// Analyze each message
	for _, msg := range messages {
		content := msg.Content
		source := msg.Source

		// Skip user and system messages for position analysis
		if source == "user" || source == "system" {
			continue
		}

		// Detect positions
		contentUpper := strings.ToUpper(content)

		if strings.Contains(contentUpper, "OBJECT:") || strings.Contains(contentUpper, "I OBJECT") ||
			strings.Contains(contentUpper, "I DISAGREE") {
			// Extract objection reason
			reason := extractAfter(content, "OBJECT:")
			if reason == "" {
				reason = extractAfter(content, "disagree")
			}

			objections = append(objections, FailedApproach{
				DebateID:   debateID,
				Approach:   extractProposalContext(messages, msg),
				Reason:     truncate(reason, 200),
				RejectedBy: []string{source},
			})
			modelPositions[source] = "OBJECT"

		} else if strings.Contains(contentUpper, "AGREE:") || strings.Contains(contentUpper, "I AGREE") ||
			strings.Contains(contentUpper, "CONCUR") {
			modelPositions[source] = "AGREE"

			// If many models agree, this might be consensus
			if strings.Contains(content, "consensus") || strings.Contains(content, "agreed upon") {
				consensusText = extractAfter(content, "AGREE:")
			}

		} else if strings.Contains(contentUpper, "ADD:") || strings.Contains(contentUpper, "I WOULD ADD") ||
			strings.Contains(contentUpper, "ADDITIONALLY") {
			addition := extractAfter(content, "ADD:")
			if addition == "" {
				addition = extractAfter(content, "add")
			}
			additions = append(additions, truncate(addition, 200))
			modelPositions[source] = "ADD"
		}

		// Detect project mentions for cross-project insights
		projects := c.DetectProjectMentions(content)
		for _, p := range projects {
			mentionedProjects[p] = true
		}
	}

	// Check if consensus was reached (majority agree, no objections)
	agreeCount := 0
	objectCount := 0
	for _, pos := range modelPositions {
		if pos == "AGREE" {
			agreeCount++
		} else if pos == "OBJECT" {
			objectCount++
		}
	}

	hasConsensus := agreeCount > 0 && objectCount == 0 && agreeCount >= len(modelPositions)/2

	// Build consensus outcome if reached
	if hasConsensus && consensusText != "" {
		learning.Consensus = &DebateOutcome{
			DebateID:   debateID,
			DebateName: debateName,
			Consensus:  consensusText,
			Summary:    truncate(consensusText, 200),
			ModelVotes: modelPositions,
			Additions:  additions,
		}
	}

	// Collect unique failures
	learning.Failures = deduplicateFailures(objections)

	// Build insights if multiple projects mentioned
	projectList := make([]string, 0, len(mentionedProjects))
	for p := range mentionedProjects {
		projectList = append(projectList, p)
	}

	if len(projectList) > 1 {
		// Cross-project discussion detected
		learning.Insights = append(learning.Insights, Insight{
			DebateID: debateID,
			Insight:  fmt.Sprintf("Debate '%s' connected %s", debateName, strings.Join(projectList, ", ")),
			Projects: projectList,
		})
	}

	return learning
}

// LogLearnings commits all extracted learnings to session memory
func (c *Client) LogLearnings(learning DebateLearning) {
	if !c.enabled {
		return
	}

	// Log consensus if reached
	if learning.Consensus != nil {
		c.LogConsensus(*learning.Consensus)
	}

	// Log failures
	for _, failure := range learning.Failures {
		c.LogFailure(failure)
	}

	// Log insights
	for _, insight := range learning.Insights {
		c.LogInsight(insight)
	}
}

// DebateMessage is imported from the debate package but defined here to avoid circular imports
type DebateMessage struct {
	Source    string
	Content   string
	Timestamp time.Time
}

// extractAfter extracts text after a marker, cleaning it up
func extractAfter(text, marker string) string {
	markerLower := strings.ToLower(marker)
	textLower := strings.ToLower(text)

	idx := strings.Index(textLower, markerLower)
	if idx == -1 {
		return ""
	}

	after := text[idx+len(marker):]
	after = strings.TrimSpace(after)

	// Take first sentence or line
	if endIdx := strings.IndexAny(after, ".\n"); endIdx != -1 {
		after = after[:endIdx]
	}

	return strings.TrimSpace(after)
}

// extractProposalContext tries to find what proposal was being objected to
func extractProposalContext(messages []DebateMessage, objection DebateMessage) string {
	// Look at previous messages for context
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Timestamp.Before(objection.Timestamp) && msg.Source != "system" {
			// Return the first 100 chars as context
			return truncate(msg.Content, 100)
		}
	}
	return "unknown proposal"
}

// deduplicateFailures merges failures with same approach/reason
func deduplicateFailures(failures []FailedApproach) []FailedApproach {
	if len(failures) == 0 {
		return failures
	}

	merged := make(map[string]*FailedApproach)

	for _, f := range failures {
		key := f.Approach + "|" + f.Reason
		if existing, ok := merged[key]; ok {
			// Merge rejectors
			existing.RejectedBy = append(existing.RejectedBy, f.RejectedBy...)
		} else {
			copy := f
			merged[key] = &copy
		}
	}

	result := make([]FailedApproach, 0, len(merged))
	for _, f := range merged {
		result = append(result, *f)
	}

	return result
}
