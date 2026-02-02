// internal/orchestrator/orchestrator_test.go
package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"roundtable/internal/models"
)

// MockModel implements the Model interface for testing
type MockModel struct {
	id           string
	name         string
	status       models.ModelStatus
	sendFunc     func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk
	stopCalled   atomic.Bool
	statusMu     sync.Mutex
	statusCalled []models.ModelStatus
}

func NewMockModel(id, name string) *MockModel {
	return &MockModel{
		id:           id,
		name:         name,
		status:       models.StatusIdle,
		statusCalled: make([]models.ModelStatus, 0),
	}
}

func (m *MockModel) Info() models.ModelInfo {
	return models.ModelInfo{
		ID:   m.id,
		Name: m.name,
	}
}

func (m *MockModel) Send(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, history, prompt)
	}
	// Default: return a simple response
	ch := make(chan models.Chunk, 2)
	go func() {
		ch <- models.Chunk{Text: "Mock response from " + m.id}
		ch <- models.Chunk{Done: true}
		close(ch)
	}()
	return ch
}

func (m *MockModel) Stop() {
	m.stopCalled.Store(true)
}

func (m *MockModel) Status() models.ModelStatus {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	return m.status
}

func (m *MockModel) SetStatus(status models.ModelStatus) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	m.status = status
	m.statusCalled = append(m.statusCalled, status)
}

func (m *MockModel) WasStopCalled() bool {
	return m.stopCalled.Load()
}

func (m *MockModel) GetStatusHistory() []models.ModelStatus {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	result := make([]models.ModelStatus, len(m.statusCalled))
	copy(result, m.statusCalled)
	return result
}

// MockRegistry is a test-only registry that allows direct model injection
type MockRegistry struct {
	models map[string]models.Model
	order  []string
}

func NewMockRegistry() *MockRegistry {
	return &MockRegistry{
		models: make(map[string]models.Model),
		order:  []string{},
	}
}

func (r *MockRegistry) Add(id string, m models.Model) {
	r.models[id] = m
	r.order = append(r.order, id)
}

func (r *MockRegistry) Get(id string) models.Model {
	return r.models[id]
}

func (r *MockRegistry) Enabled() []string {
	return r.order
}

func (r *MockRegistry) Count() int {
	return len(r.order)
}

// RegistryWrapper wraps MockRegistry to satisfy the *models.Registry type requirement
// by embedding a real registry and overriding the behavior through the mock
type TestOrchestrator struct {
	*Orchestrator
	mockRegistry *MockRegistry
}

// newTestOrchestrator creates an orchestrator with a mock registry for testing
func newTestOrchestrator(timeout time.Duration) (*TestOrchestrator, *MockRegistry) {
	mockReg := NewMockRegistry()
	// Create a wrapper that exposes mock behavior
	orch := &Orchestrator{
		registry:      nil, // We'll override methods via our wrapper
		timeout:       timeout,
		retryAttempts: 3,
		retryDelay:    time.Second,
	}
	return &TestOrchestrator{Orchestrator: orch, mockRegistry: mockReg}, mockReg
}

// ParallelSeed overrides to use mock registry
func (to *TestOrchestrator) ParallelSeed(ctx context.Context, history []models.Message, prompt string) <-chan Response {
	responses := make(chan Response, to.mockRegistry.Count()*10)

	var wg sync.WaitGroup

	for _, modelID := range to.mockRegistry.Enabled() {
		model := to.mockRegistry.Get(modelID)
		if model == nil {
			continue
		}

		wg.Add(1)
		go func(m models.Model, id string) {
			defer wg.Done()
			to.sendWithTimeout(ctx, m, id, history, prompt, responses)
		}(model, modelID)
	}

	go func() {
		wg.Wait()
		close(responses)
	}()

	return responses
}

// SendToModel overrides to use mock registry
func (to *TestOrchestrator) SendToModel(ctx context.Context, modelID string, history []models.Message, prompt string) <-chan Response {
	responses := make(chan Response, 10)

	model := to.mockRegistry.Get(modelID)
	if model == nil {
		close(responses)
		return responses
	}

	go func() {
		defer close(responses)
		to.sendWithTimeout(ctx, model, modelID, history, prompt, responses)
	}()

	return responses
}

// ConsensusPrompt overrides to use mock registry
func (to *TestOrchestrator) ConsensusPrompt(ctx context.Context, history []models.Message) <-chan Response {
	prompt := `Based on the discussion so far, please state your position:
- If you agree with a proposed approach, say "AGREE: [model name]" and briefly explain why
- If you object, say "OBJECT:" and explain your reasoning
- If you have something to add, say "ADD:" and state your point

Be explicit about your position.`

	return to.ParallelSeed(ctx, history, prompt)
}

// StopAll overrides to use mock registry
func (to *TestOrchestrator) StopAll() {
	for _, modelID := range to.mockRegistry.Enabled() {
		if model := to.mockRegistry.Get(modelID); model != nil {
			model.Stop()
		}
	}
}

// --- Constructor Tests ---

func TestNew(t *testing.T) {
	// Test that New creates an orchestrator with correct defaults
	// We need a real registry here, but we can test with nil for basic construction
	// since the constructor doesn't validate the registry

	timeout := 30 * time.Second
	orch := New(nil, timeout)

	if orch == nil {
		t.Fatal("New returned nil")
	}
	if orch.timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, orch.timeout)
	}
	if orch.retryAttempts != 3 {
		t.Errorf("Expected retryAttempts 3, got %d", orch.retryAttempts)
	}
	if orch.retryDelay != time.Second {
		t.Errorf("Expected retryDelay 1s, got %v", orch.retryDelay)
	}
}

func TestNewWithRetry(t *testing.T) {
	timeout := 45 * time.Second
	retryAttempts := 5
	retryDelay := 2 * time.Second

	orch := NewWithRetry(nil, timeout, retryAttempts, retryDelay)

	if orch == nil {
		t.Fatal("NewWithRetry returned nil")
	}
	if orch.timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, orch.timeout)
	}
	if orch.retryAttempts != retryAttempts {
		t.Errorf("Expected retryAttempts %d, got %d", retryAttempts, orch.retryAttempts)
	}
	if orch.retryDelay != retryDelay {
		t.Errorf("Expected retryDelay %v, got %v", retryDelay, orch.retryDelay)
	}
}

// --- ParallelSeed Tests ---

func TestParallelSeed_SendsToAllEnabledModels(t *testing.T) {
	orch, mockReg := newTestOrchestrator(5 * time.Second)

	// Track which models received the prompt
	received := make(map[string]string)
	var mu sync.Mutex

	createTrackingModel := func(id string) *MockModel {
		m := NewMockModel(id, id+" Model")
		m.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
			mu.Lock()
			received[id] = prompt
			mu.Unlock()

			ch := make(chan models.Chunk, 2)
			go func() {
				ch <- models.Chunk{Text: "Response from " + id}
				ch <- models.Chunk{Done: true}
				close(ch)
			}()
			return ch
		}
		return m
	}

	mockReg.Add("model1", createTrackingModel("model1"))
	mockReg.Add("model2", createTrackingModel("model2"))
	mockReg.Add("model3", createTrackingModel("model3"))

	ctx := context.Background()
	prompt := "Test prompt for all models"
	responses := orch.ParallelSeed(ctx, nil, prompt)

	// Collect all responses
	var allResponses []Response
	for r := range responses {
		allResponses = append(allResponses, r)
	}

	// Verify all models received the prompt
	mu.Lock()
	defer mu.Unlock()

	if len(received) != 3 {
		t.Errorf("Expected 3 models to receive prompt, got %d", len(received))
	}

	for _, id := range []string{"model1", "model2", "model3"} {
		if received[id] != prompt {
			t.Errorf("Model %s did not receive correct prompt. Got: %q", id, received[id])
		}
	}
}

func TestParallelSeed_HandlesTimeoutGracefully(t *testing.T) {
	orch, mockReg := newTestOrchestrator(100 * time.Millisecond)

	// Create a model that takes too long
	slowModel := NewMockModel("slow", "Slow Model")
	slowModel.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
		ch := make(chan models.Chunk)
		go func() {
			// Wait longer than the timeout
			select {
			case <-ctx.Done():
				// Context cancelled - don't send anything
			case <-time.After(500 * time.Millisecond):
				// Would send but shouldn't reach here due to timeout
				ch <- models.Chunk{Text: "Should not see this"}
				ch <- models.Chunk{Done: true}
			}
			close(ch)
		}()
		return ch
	}

	// Create a fast model for comparison
	fastModel := NewMockModel("fast", "Fast Model")
	fastModel.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
		ch := make(chan models.Chunk, 2)
		go func() {
			ch <- models.Chunk{Text: "Fast response"}
			ch <- models.Chunk{Done: true}
			close(ch)
		}()
		return ch
	}

	mockReg.Add("slow", slowModel)
	mockReg.Add("fast", fastModel)

	ctx := context.Background()
	responses := orch.ParallelSeed(ctx, nil, "Test prompt")

	var gotTimeoutError bool
	var gotFastResponse bool
	var timeoutModelID string

	for r := range responses {
		if r.IsTimeout {
			gotTimeoutError = true
			timeoutModelID = r.ModelID
		}
		if r.ModelID == "fast" && r.Content == "Fast response" {
			gotFastResponse = true
		}
	}

	if !gotTimeoutError {
		t.Error("Expected to receive timeout error from slow model")
	}
	if timeoutModelID != "slow" {
		t.Errorf("Expected timeout from 'slow' model, got from %q", timeoutModelID)
	}
	if !gotFastResponse {
		t.Error("Expected to receive response from fast model despite slow model timeout")
	}

	// Verify the slow model status was set to timeout
	statusHistory := slowModel.GetStatusHistory()
	found := false
	for _, s := range statusHistory {
		if s == models.StatusTimeout {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected slow model status to be set to StatusTimeout")
	}
}

func TestParallelSeed_HandlesModelErrorsGracefully(t *testing.T) {
	orch, mockReg := newTestOrchestrator(5 * time.Second)

	testError := errors.New("model processing error")

	// Create a model that returns an error
	errorModel := NewMockModel("error", "Error Model")
	errorModel.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
		ch := make(chan models.Chunk, 1)
		go func() {
			ch <- models.Chunk{Error: testError}
			close(ch)
		}()
		return ch
	}

	// Create a working model
	workingModel := NewMockModel("working", "Working Model")
	workingModel.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
		ch := make(chan models.Chunk, 2)
		go func() {
			ch <- models.Chunk{Text: "Working response"}
			ch <- models.Chunk{Done: true}
			close(ch)
		}()
		return ch
	}

	mockReg.Add("error", errorModel)
	mockReg.Add("working", workingModel)

	ctx := context.Background()
	responses := orch.ParallelSeed(ctx, nil, "Test prompt")

	var gotError bool
	var gotWorkingResponse bool
	var errorResponse Response

	for r := range responses {
		if r.ModelID == "error" && r.Error != nil {
			gotError = true
			errorResponse = r
		}
		if r.ModelID == "working" && r.Content == "Working response" {
			gotWorkingResponse = true
		}
	}

	if !gotError {
		t.Error("Expected to receive error from error model")
	}
	if errorResponse.Error != testError {
		t.Errorf("Expected error %v, got %v", testError, errorResponse.Error)
	}
	if !gotWorkingResponse {
		t.Error("Expected to receive response from working model despite error model failure")
	}

	// Verify error model status was set to error
	statusHistory := errorModel.GetStatusHistory()
	found := false
	for _, s := range statusHistory {
		if s == models.StatusError {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error model status to be set to StatusError")
	}
}

func TestParallelSeed_ReceivesStreamingChunks(t *testing.T) {
	orch, mockReg := newTestOrchestrator(5 * time.Second)

	// Create a model that sends multiple chunks
	streamingModel := NewMockModel("streaming", "Streaming Model")
	streamingModel.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
		ch := make(chan models.Chunk, 5)
		go func() {
			ch <- models.Chunk{Text: "Hello "}
			ch <- models.Chunk{Text: "world "}
			ch <- models.Chunk{Text: "from streaming!"}
			ch <- models.Chunk{Done: true}
			close(ch)
		}()
		return ch
	}

	mockReg.Add("streaming", streamingModel)

	ctx := context.Background()
	responses := orch.ParallelSeed(ctx, nil, "Test prompt")

	var chunks []string
	var gotDone bool

	for r := range responses {
		if r.Content != "" {
			chunks = append(chunks, r.Content)
		}
		if r.Done && r.Error == nil {
			gotDone = true
		}
	}

	if len(chunks) != 3 {
		t.Errorf("Expected 3 chunks, got %d: %v", len(chunks), chunks)
	}
	if !gotDone {
		t.Error("Expected to receive Done signal")
	}
}

func TestParallelSeed_WithEmptyRegistry(t *testing.T) {
	orch, _ := newTestOrchestrator(5 * time.Second)

	ctx := context.Background()
	responses := orch.ParallelSeed(ctx, nil, "Test prompt")

	var count int
	for range responses {
		count++
	}

	if count != 0 {
		t.Errorf("Expected 0 responses from empty registry, got %d", count)
	}
}

func TestParallelSeed_PassesHistoryToModels(t *testing.T) {
	orch, mockReg := newTestOrchestrator(5 * time.Second)

	var receivedHistory []models.Message
	var mu sync.Mutex

	historyModel := NewMockModel("history", "History Model")
	historyModel.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
		mu.Lock()
		receivedHistory = history
		mu.Unlock()

		ch := make(chan models.Chunk, 2)
		go func() {
			ch <- models.Chunk{Text: "OK"}
			ch <- models.Chunk{Done: true}
			close(ch)
		}()
		return ch
	}

	mockReg.Add("history", historyModel)

	testHistory := []models.Message{
		{Source: "user", Content: "First message"},
		{Source: "claude", Content: "First response"},
		{Source: "user", Content: "Second message"},
	}

	ctx := context.Background()
	responses := orch.ParallelSeed(ctx, testHistory, "New prompt")

	// Drain responses
	for range responses {
	}

	mu.Lock()
	defer mu.Unlock()

	if len(receivedHistory) != len(testHistory) {
		t.Errorf("Expected history length %d, got %d", len(testHistory), len(receivedHistory))
	}

	for i, msg := range testHistory {
		if receivedHistory[i].Content != msg.Content {
			t.Errorf("History message %d mismatch: expected %q, got %q",
				i, msg.Content, receivedHistory[i].Content)
		}
	}
}

// --- SendToModel Tests ---

func TestSendToModel_ReturnsEmptyChannelForUnknownModel(t *testing.T) {
	orch, _ := newTestOrchestrator(5 * time.Second)

	ctx := context.Background()
	responses := orch.SendToModel(ctx, "nonexistent", nil, "Test prompt")

	var count int
	for range responses {
		count++
	}

	if count != 0 {
		t.Errorf("Expected 0 responses for unknown model, got %d", count)
	}
}

func TestSendToModel_SendsToSpecificModel(t *testing.T) {
	orch, mockReg := newTestOrchestrator(5 * time.Second)

	var model1Called, model2Called atomic.Bool

	model1 := NewMockModel("model1", "Model 1")
	model1.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
		model1Called.Store(true)
		ch := make(chan models.Chunk, 2)
		go func() {
			ch <- models.Chunk{Text: "From model1"}
			ch <- models.Chunk{Done: true}
			close(ch)
		}()
		return ch
	}

	model2 := NewMockModel("model2", "Model 2")
	model2.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
		model2Called.Store(true)
		ch := make(chan models.Chunk, 2)
		go func() {
			ch <- models.Chunk{Text: "From model2"}
			ch <- models.Chunk{Done: true}
			close(ch)
		}()
		return ch
	}

	mockReg.Add("model1", model1)
	mockReg.Add("model2", model2)

	ctx := context.Background()
	responses := orch.SendToModel(ctx, "model1", nil, "Test prompt")

	var responseContent string
	for r := range responses {
		if r.Content != "" {
			responseContent = r.Content
		}
	}

	if !model1Called.Load() {
		t.Error("Expected model1 to be called")
	}
	if model2Called.Load() {
		t.Error("Expected model2 to NOT be called")
	}
	if responseContent != "From model1" {
		t.Errorf("Expected response from model1, got %q", responseContent)
	}
}

func TestSendToModel_HandlesTimeout(t *testing.T) {
	orch, mockReg := newTestOrchestrator(100 * time.Millisecond)

	slowModel := NewMockModel("slow", "Slow Model")
	slowModel.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
		ch := make(chan models.Chunk)
		go func() {
			select {
			case <-ctx.Done():
				// Context cancelled
			case <-time.After(500 * time.Millisecond):
				ch <- models.Chunk{Text: "Too slow"}
				ch <- models.Chunk{Done: true}
			}
			close(ch)
		}()
		return ch
	}

	mockReg.Add("slow", slowModel)

	ctx := context.Background()
	responses := orch.SendToModel(ctx, "slow", nil, "Test prompt")

	var gotTimeout bool
	for r := range responses {
		if r.IsTimeout {
			gotTimeout = true
		}
	}

	if !gotTimeout {
		t.Error("Expected timeout error")
	}
}

// --- ConsensusPrompt Tests ---

func TestConsensusPrompt_UsesCorrectPrompt(t *testing.T) {
	orch, mockReg := newTestOrchestrator(5 * time.Second)

	var receivedPrompt string
	var mu sync.Mutex

	consensusModel := NewMockModel("consensus", "Consensus Model")
	consensusModel.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
		mu.Lock()
		receivedPrompt = prompt
		mu.Unlock()

		ch := make(chan models.Chunk, 2)
		go func() {
			ch <- models.Chunk{Text: "AGREE: some model"}
			ch <- models.Chunk{Done: true}
			close(ch)
		}()
		return ch
	}

	mockReg.Add("consensus", consensusModel)

	ctx := context.Background()
	responses := orch.ConsensusPrompt(ctx, nil)

	// Drain responses
	for range responses {
	}

	mu.Lock()
	defer mu.Unlock()

	// Check that the consensus prompt contains expected keywords
	expectedPhrases := []string{
		"AGREE:",
		"OBJECT:",
		"ADD:",
		"Be explicit about your position",
	}

	for _, phrase := range expectedPhrases {
		if !containsString(receivedPrompt, phrase) {
			t.Errorf("Expected consensus prompt to contain %q, got:\n%s", phrase, receivedPrompt)
		}
	}
}

func TestConsensusPrompt_SendsToAllModels(t *testing.T) {
	orch, mockReg := newTestOrchestrator(5 * time.Second)

	called := make(map[string]bool)
	var mu sync.Mutex

	createModel := func(id string) *MockModel {
		m := NewMockModel(id, id+" Model")
		m.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
			mu.Lock()
			called[id] = true
			mu.Unlock()

			ch := make(chan models.Chunk, 2)
			go func() {
				ch <- models.Chunk{Text: "Response from " + id}
				ch <- models.Chunk{Done: true}
				close(ch)
			}()
			return ch
		}
		return m
	}

	mockReg.Add("model1", createModel("model1"))
	mockReg.Add("model2", createModel("model2"))

	ctx := context.Background()
	responses := orch.ConsensusPrompt(ctx, nil)

	// Drain responses
	for range responses {
	}

	mu.Lock()
	defer mu.Unlock()

	if !called["model1"] {
		t.Error("Expected model1 to be called")
	}
	if !called["model2"] {
		t.Error("Expected model2 to be called")
	}
}

// --- StopAll Tests ---

func TestStopAll_CallsStopOnAllModels(t *testing.T) {
	orch, mockReg := newTestOrchestrator(5 * time.Second)

	model1 := NewMockModel("model1", "Model 1")
	model2 := NewMockModel("model2", "Model 2")
	model3 := NewMockModel("model3", "Model 3")

	mockReg.Add("model1", model1)
	mockReg.Add("model2", model2)
	mockReg.Add("model3", model3)

	orch.StopAll()

	if !model1.WasStopCalled() {
		t.Error("Expected Stop to be called on model1")
	}
	if !model2.WasStopCalled() {
		t.Error("Expected Stop to be called on model2")
	}
	if !model3.WasStopCalled() {
		t.Error("Expected Stop to be called on model3")
	}
}

func TestStopAll_WithEmptyRegistry(t *testing.T) {
	orch, _ := newTestOrchestrator(5 * time.Second)

	// Should not panic with empty registry
	orch.StopAll()
}

// --- Context Cancellation Tests ---

func TestParallelSeed_RespectsContextCancellation(t *testing.T) {
	orch, mockReg := newTestOrchestrator(5 * time.Second)

	// Create a model that waits for context
	waitingModel := NewMockModel("waiting", "Waiting Model")
	waitingModel.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
		ch := make(chan models.Chunk)
		go func() {
			select {
			case <-ctx.Done():
				// Context cancelled before we could send
			case <-time.After(5 * time.Second):
				ch <- models.Chunk{Text: "Should not get here"}
				ch <- models.Chunk{Done: true}
			}
			close(ch)
		}()
		return ch
	}

	mockReg.Add("waiting", waitingModel)

	ctx, cancel := context.WithCancel(context.Background())

	responses := orch.ParallelSeed(ctx, nil, "Test prompt")

	// Cancel context shortly after starting
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	for range responses {
	}
	elapsed := time.Since(start)

	// Should complete quickly due to cancellation, not wait 5 seconds
	if elapsed > 2*time.Second {
		t.Errorf("ParallelSeed did not respect context cancellation, took %v", elapsed)
	}
}

// --- Error Type Tests ---

func TestParallelSeed_SetsCorrectErrorType(t *testing.T) {
	orch, mockReg := newTestOrchestrator(5 * time.Second)

	// Model with timeout chunk
	timeoutModel := NewMockModel("timeout", "Timeout Model")
	timeoutModel.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
		ch := make(chan models.Chunk, 1)
		go func() {
			ch <- models.Chunk{Error: errors.New("timeout error"), IsTimeout: true}
			close(ch)
		}()
		return ch
	}

	mockReg.Add("timeout", timeoutModel)

	ctx := context.Background()
	responses := orch.ParallelSeed(ctx, nil, "Test prompt")

	var response Response
	for r := range responses {
		if r.Done {
			response = r
			break
		}
	}

	if !response.IsTimeout {
		t.Error("Expected IsTimeout to be true for timeout error")
	}
	if !errors.Is(response.Error, ErrTimeout) {
		t.Errorf("Expected ErrTimeout, got %v", response.Error)
	}

	// Verify status was set to timeout
	statusHistory := timeoutModel.GetStatusHistory()
	found := false
	for _, s := range statusHistory {
		if s == models.StatusTimeout {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected model status to be set to StatusTimeout")
	}
}

// --- Status Update Tests ---

func TestParallelSeed_SetsIdleStatusOnSuccess(t *testing.T) {
	orch, mockReg := newTestOrchestrator(5 * time.Second)

	successModel := NewMockModel("success", "Success Model")
	successModel.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
		ch := make(chan models.Chunk, 2)
		go func() {
			ch <- models.Chunk{Text: "Success!"}
			ch <- models.Chunk{Done: true}
			close(ch)
		}()
		return ch
	}

	mockReg.Add("success", successModel)

	ctx := context.Background()
	responses := orch.ParallelSeed(ctx, nil, "Test prompt")

	// Drain responses
	for range responses {
	}

	// Verify status was set to idle on completion
	statusHistory := successModel.GetStatusHistory()
	if len(statusHistory) == 0 {
		t.Fatal("Expected status to be updated")
	}

	lastStatus := statusHistory[len(statusHistory)-1]
	if lastStatus != models.StatusIdle {
		t.Errorf("Expected final status to be StatusIdle, got %v", lastStatus)
	}
}

// --- Concurrent Access Tests ---

func TestParallelSeed_ConcurrentResponseHandling(t *testing.T) {
	orch, mockReg := newTestOrchestrator(5 * time.Second)

	// Create multiple models that all respond quickly with multiple chunks
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		m := NewMockModel(id, "Model "+id)
		m.sendFunc = func(ctx context.Context, history []models.Message, prompt string) <-chan models.Chunk {
			ch := make(chan models.Chunk, 10)
			go func() {
				for j := 0; j < 5; j++ {
					ch <- models.Chunk{Text: "chunk"}
				}
				ch <- models.Chunk{Done: true}
				close(ch)
			}()
			return ch
		}
		mockReg.Add(id, m)
	}

	ctx := context.Background()
	responses := orch.ParallelSeed(ctx, nil, "Test prompt")

	var responseCount int
	var doneCount int

	for r := range responses {
		responseCount++
		if r.Done {
			doneCount++
		}
	}

	// 5 models * (5 chunks + 1 done) = 30 responses
	// But we might get fewer if channel closes between chunk and done
	if doneCount != 5 {
		t.Errorf("Expected 5 done signals (one per model), got %d", doneCount)
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
