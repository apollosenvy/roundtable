// internal/models/gpt.go
package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

type GPTModel struct {
	BaseModel
	apiKey      string
	modelName   string
	client      *http.Client
	cancel      context.CancelFunc
	mu          sync.Mutex
}

func NewGPT(apiKey, modelName string) *GPTModel {
	return &GPTModel{
		BaseModel: NewBaseModel(ModelInfo{
			ID:      "gpt",
			Name:    "GPT",
			Color:   "#00FF00", // Green
			CanExec: false,
			CanRead: true,
		}),
		apiKey:    apiKey,
		modelName: modelName,
		client:    &http.Client{},
	}
}

type gptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type gptRequest struct {
	Model    string       `json:"model"`
	Messages []gptMessage `json:"messages"`
	Stream   bool         `json:"stream"`
}

func (m *GPTModel) Send(ctx context.Context, history []Message, prompt string) <-chan Chunk {
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
		messages := []gptMessage{
			{
				Role:    "system",
				Content: "You are participating in a multi-model debate. Other AI models may respond before or after you. Be direct and substantive. If you agree, say AGREE: [model]. If you disagree, explain why. If you have something to add, say ADD: [point].",
			},
		}

		// Add history
		for _, msg := range history {
			role := "assistant"
			if msg.Source == "user" {
				role = "user"
			}
			content := fmt.Sprintf("[%s]: %s", msg.Source, msg.Content)
			messages = append(messages, gptMessage{Role: role, Content: content})
		}

		// Add current prompt
		messages = append(messages, gptMessage{Role: "user", Content: prompt})

		reqBody := gptRequest{
			Model:    m.modelName,
			Messages: messages,
			Stream:   true,
		}

		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			ch <- Chunk{Error: fmt.Errorf("marshal: %w", err)}
			return
		}

		req, err := http.NewRequestWithContext(cmdCtx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(bodyBytes))
		if err != nil {
			ch <- Chunk{Error: fmt.Errorf("request: %w", err)}
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+m.apiKey)

		resp, err := m.client.Do(req)
		if err != nil {
			ch <- Chunk{Error: fmt.Errorf("do: %w", err)}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			ch <- Chunk{Error: fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))}
			return
		}

		// Parse SSE stream
		var fullText strings.Builder
		decoder := json.NewDecoder(resp.Body)

		for {
			select {
			case <-cmdCtx.Done():
				ch <- Chunk{Done: true}
				return
			default:
			}

			// Read line-by-line for SSE
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

		_ = decoder // silence unused warning
		ch <- Chunk{Text: fullText.String(), Done: true}
	}()

	return ch
}

func (m *GPTModel) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
	}
}
