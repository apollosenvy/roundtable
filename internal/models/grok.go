// internal/models/grok.go
package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

type GrokModel struct {
	BaseModel
	apiKey    string
	modelName string
	client    *RetryableClient
	cancel    context.CancelFunc
	mu        sync.Mutex
}

func NewGrok(apiKey, modelName string) *GrokModel {
	return &GrokModel{
		BaseModel: NewBaseModel(ModelInfo{
			ID:      "grok",
			Name:    "Grok",
			Color:   "#FFA500", // Orange
			CanExec: false,
			CanRead: true,
		}),
		apiKey:    apiKey,
		modelName: modelName,
		client:    NewRetryableClient(DefaultRetryConfig()),
	}
}

// NewGrokWithRetry creates a Grok model with custom retry settings
func NewGrokWithRetry(apiKey, modelName string, retryConfig RetryConfig) *GrokModel {
	return &GrokModel{
		BaseModel: NewBaseModel(ModelInfo{
			ID:      "grok",
			Name:    "Grok",
			Color:   "#FFA500",
			CanExec: false,
			CanRead: true,
		}),
		apiKey:    apiKey,
		modelName: modelName,
		client:    NewRetryableClient(retryConfig),
	}
}

type grokMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type grokRequest struct {
	Model    string        `json:"model"`
	Messages []grokMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

func (m *GrokModel) Send(ctx context.Context, history []Message, prompt string) <-chan Chunk {
	ch := make(chan Chunk, 100)

	go func() {
		defer close(ch)
		m.SetStatus(StatusResponding)
		defer m.SetStatus(StatusIdle)

		cmdCtx, cancel := context.WithCancel(ctx)
		m.mu.Lock()
		m.cancel = cancel
		m.mu.Unlock()

		// Build messages array
		messages := []grokMessage{
			{
				Role:    "system",
				Content: "You are participating in a multi-model debate with other AI models. Be direct and opinionated. If you agree, say AGREE: [model]. If you disagree, explain why. If you have something to add, say ADD: [point]. Don't be sycophantic.",
			},
		}

		// Add history
		for _, msg := range history {
			role := "assistant"
			if msg.Source == "user" {
				role = "user"
			}
			content := fmt.Sprintf("[%s]: %s", msg.Source, msg.Content)
			messages = append(messages, grokMessage{Role: role, Content: content})
		}

		// Add current prompt
		messages = append(messages, grokMessage{Role: "user", Content: prompt})

		reqBody := grokRequest{
			Model:    m.modelName,
			Messages: messages,
			Stream:   true,
		}

		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			ch <- Chunk{Error: fmt.Errorf("marshal: %w", err)}
			return
		}

		req, err := NewRequestWithBody(cmdCtx, "POST", "https://api.x.ai/v1/chat/completions", bodyBytes)
		if err != nil {
			ch <- Chunk{Error: fmt.Errorf("request: %w", err)}
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+m.apiKey)

		resp, err := m.client.DoWithRetry(cmdCtx, req)
		if err != nil {
			// Check for timeout
			if errors.Is(err, context.DeadlineExceeded) {
				m.SetStatus(StatusTimeout)
				ch <- Chunk{Error: fmt.Errorf("request timed out"), IsTimeout: true}
				return
			}
			// Check for rate limit
			if errors.Is(err, ErrRateLimit) {
				m.SetStatus(StatusError)
				ch <- Chunk{Error: fmt.Errorf("rate limit exceeded - try again later")}
				return
			}
			m.SetStatus(StatusError)
			ch <- Chunk{Error: fmt.Errorf("connection failed: %w", err)}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			m.SetStatus(StatusError)
			ch <- Chunk{Error: fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))}
			return
		}

		// Parse SSE stream (same format as OpenAI)
		var fullText strings.Builder

		for {
			select {
			case <-cmdCtx.Done():
				ch <- Chunk{Done: true}
				return
			default:
			}

			buf := make([]byte, 4096)
			n, err := resp.Body.Read(buf)
			if err == io.EOF {
				break
			}
			if err != nil {
				ch <- Chunk{Error: err}
				return
			}

			lines := strings.Split(string(buf[:n]), "\n")
			for _, line := range lines {
				line = strings.TrimPrefix(line, "data: ")
				line = strings.TrimSpace(line)

				if line == "" || line == "[DONE]" {
					continue
				}

				var sseData struct {
					Choices []struct {
						Delta struct {
							Content string `json:"content"`
						} `json:"delta"`
						FinishReason string `json:"finish_reason"`
					} `json:"choices"`
				}

				if err := json.Unmarshal([]byte(line), &sseData); err != nil {
					continue
				}

				for _, choice := range sseData.Choices {
					if choice.Delta.Content != "" {
						fullText.WriteString(choice.Delta.Content)
						ch <- Chunk{Text: choice.Delta.Content}
					}
					if choice.FinishReason == "stop" {
						ch <- Chunk{Text: fullText.String(), Done: true}
						return
					}
				}
			}
		}

		ch <- Chunk{Text: fullText.String(), Done: true}
	}()

	return ch
}

func (m *GrokModel) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
	}
}
