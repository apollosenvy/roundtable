// internal/orchestrator/orchestrator.go
package orchestrator

import (
	"context"
	"sync"
	"time"

	"roundtable/internal/models"
)

// Response represents a model's response
type Response struct {
	ModelID string
	Content string
	Error   error
	Done    bool
}

// Orchestrator manages multi-model debate
type Orchestrator struct {
	registry *models.Registry
	timeout  time.Duration
}

func New(registry *models.Registry, timeout time.Duration) *Orchestrator {
	return &Orchestrator{
		registry: registry,
		timeout:  timeout,
	}
}

// ParallelSeed sends the initial prompt to all models in parallel
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

			ctx, cancel := context.WithTimeout(ctx, o.timeout)
			defer cancel()

			chunks := m.Send(ctx, history, prompt)

			for chunk := range chunks {
				if chunk.Error != nil {
					responses <- Response{
						ModelID: id,
						Error:   chunk.Error,
					}
					return
				}
				if chunk.Text != "" {
					responses <- Response{
						ModelID: id,
						Content: chunk.Text,
					}
				}
				if chunk.Done {
					responses <- Response{
						ModelID: id,
						Done:    true,
					}
					return
				}
			}
		}(model, modelID)
	}

	// Close responses channel when all models done
	go func() {
		wg.Wait()
		close(responses)
	}()

	return responses
}

// SendToModel sends a prompt to a specific model
func (o *Orchestrator) SendToModel(ctx context.Context, modelID string, history []models.Message, prompt string) <-chan Response {
	responses := make(chan Response, 10)

	model := o.registry.Get(modelID)
	if model == nil {
		close(responses)
		return responses
	}

	go func() {
		defer close(responses)

		ctx, cancel := context.WithTimeout(ctx, o.timeout)
		defer cancel()

		chunks := model.Send(ctx, history, prompt)

		for chunk := range chunks {
			if chunk.Error != nil {
				responses <- Response{
					ModelID: modelID,
					Error:   chunk.Error,
				}
				return
			}
			if chunk.Text != "" {
				responses <- Response{
					ModelID: modelID,
					Content: chunk.Text,
				}
			}
			if chunk.Done {
				responses <- Response{
					ModelID: modelID,
					Done:    true,
				}
				return
			}
		}
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
