// internal/orchestrator/orchestrator.go
package orchestrator

import (
	"context"
	"errors"
	"sync"
	"time"

	"roundtable/internal/models"
)

// Common error types
var (
	ErrTimeout     = errors.New("model response timed out")
	ErrRateLimit   = errors.New("rate limit exceeded")
	ErrConnection  = errors.New("connection failed")
)

// Response represents a model's response
type Response struct {
	ModelID   string
	Content   string
	Error     error
	Done      bool
	IsTimeout bool // True if the error was due to timeout
}

// Orchestrator manages multi-model debate
type Orchestrator struct {
	registry      *models.Registry
	timeout       time.Duration
	retryAttempts int
	retryDelay    time.Duration
}

func New(registry *models.Registry, timeout time.Duration) *Orchestrator {
	return &Orchestrator{
		registry:      registry,
		timeout:       timeout,
		retryAttempts: 3,
		retryDelay:    time.Second,
	}
}

// NewWithRetry creates an orchestrator with custom retry settings
func NewWithRetry(registry *models.Registry, timeout time.Duration, retryAttempts int, retryDelay time.Duration) *Orchestrator {
	return &Orchestrator{
		registry:      registry,
		timeout:       timeout,
		retryAttempts: retryAttempts,
		retryDelay:    retryDelay,
	}
}

// ParallelSeed sends the initial prompt to all models in parallel
// Graceful degradation: continues with remaining models if one fails
func (o *Orchestrator) ParallelSeed(ctx context.Context, history []models.Message, prompt string) <-chan Response {
	responses := make(chan Response, o.registry.Count()*10)

	var wg sync.WaitGroup

	for _, modelID := range o.registry.Enabled() {
		model := o.registry.Get(modelID)
		if model == nil {
			continue
		}

		wg.Add(1)
		go func(m models.Model, id string) {
			defer wg.Done()
			o.sendWithTimeout(ctx, m, id, history, prompt, responses)
		}(model, modelID)
	}

	// Close responses channel when all models done
	go func() {
		wg.Wait()
		close(responses)
	}()

	return responses
}

// sendWithTimeout sends a prompt to a model with timeout handling
func (o *Orchestrator) sendWithTimeout(ctx context.Context, m models.Model, id string, history []models.Message, prompt string, responses chan<- Response) {
	timeoutCtx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	chunks := m.Send(timeoutCtx, history, prompt)

	// Channel to detect if we got any response
	gotResponse := false

	for {
		select {
		case <-timeoutCtx.Done():
			// Timeout occurred
			m.SetStatus(models.StatusTimeout)
			responses <- Response{
				ModelID:   id,
				Error:     ErrTimeout,
				IsTimeout: true,
				Done:      true,
			}
			return

		case chunk, ok := <-chunks:
			if !ok {
				// Channel closed without Done - treat as complete if we got content
				if gotResponse {
					responses <- Response{
						ModelID: id,
						Done:    true,
					}
				}
				return
			}

			if chunk.Error != nil {
				// Check if it's a timeout from the model itself
				if chunk.IsTimeout || errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
					m.SetStatus(models.StatusTimeout)
					responses <- Response{
						ModelID:   id,
						Error:     ErrTimeout,
						IsTimeout: true,
						Done:      true,
					}
				} else {
					m.SetStatus(models.StatusError)
					responses <- Response{
						ModelID: id,
						Error:   chunk.Error,
						Done:    true,
					}
				}
				return
			}

			if chunk.Text != "" {
				gotResponse = true
				responses <- Response{
					ModelID: id,
					Content: chunk.Text,
				}
			}

			if chunk.Done {
				m.SetStatus(models.StatusIdle)
				responses <- Response{
					ModelID: id,
					Done:    true,
				}
				return
			}
		}
	}
}

// SendToModel sends a prompt to a specific model with timeout handling
func (o *Orchestrator) SendToModel(ctx context.Context, modelID string, history []models.Message, prompt string) <-chan Response {
	responses := make(chan Response, 10)

	model := o.registry.Get(modelID)
	if model == nil {
		close(responses)
		return responses
	}

	go func() {
		defer close(responses)
		o.sendWithTimeout(ctx, model, modelID, history, prompt, responses)
	}()

	return responses
}

// ConsensusPrompt sends the consensus check prompt to all models
func (o *Orchestrator) ConsensusPrompt(ctx context.Context, history []models.Message) <-chan Response {
	prompt := `Based on the discussion so far, please state your position:
- If you agree with a proposed approach, say "AGREE: [model name]" and briefly explain why
- If you object, say "OBJECT:" and explain your reasoning
- If you have something to add, say "ADD:" and state your point

Be explicit about your position.`

	return o.ParallelSeed(ctx, history, prompt)
}

// StopAll stops all models
func (o *Orchestrator) StopAll() {
	for _, modelID := range o.registry.Enabled() {
		if model := o.registry.Get(modelID); model != nil {
			model.Stop()
		}
	}
}
