// internal/models/gemini.go
package models

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type GeminiModel struct {
	BaseModel
	cliPath string
	workDir string

	cmd    *exec.Cmd
	cancel context.CancelFunc
	mu     sync.Mutex
}

func NewGemini(cliPath string) *GeminiModel {
	return &GeminiModel{
		BaseModel: NewBaseModel(ModelInfo{
			ID:      "gemini",
			Name:    "Gemini",
			Color:   "#FF00FF", // Magenta
			CanExec: false,
			CanRead: true,
		}),
		cliPath: cliPath,
	}
}

func (m *GeminiModel) SetWorkDir(dir string) {
	m.workDir = dir
}

func (m *GeminiModel) Send(ctx context.Context, history []Message, prompt string) <-chan Chunk {
	ch := make(chan Chunk, 100)

	go func() {
		defer close(ch)
		m.SetStatus(StatusResponding)
		defer m.SetStatus(StatusIdle)

		// Build context from history
		var contextPrompt strings.Builder
		contextPrompt.WriteString("You are participating in a multi-model debate. Previous messages:\n\n")
		for _, msg := range history {
			contextPrompt.WriteString(fmt.Sprintf("[%s]: %s\n\n", msg.Source, msg.Content))
		}
		contextPrompt.WriteString("Now respond to:\n")
		contextPrompt.WriteString(prompt)

		cmdCtx, cancel := context.WithCancel(ctx)
		m.mu.Lock()
		m.cancel = cancel
		m.mu.Unlock()

		// Gemini CLI with stream-json output
		args := []string{
			"--output-format", "stream-json",
			contextPrompt.String(),
		}

		cmd := exec.CommandContext(cmdCtx, m.cliPath, args...)
		if m.workDir != "" {
			cmd.Dir = m.workDir
		}

		m.mu.Lock()
		m.cmd = cmd
		m.mu.Unlock()

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			ch <- Chunk{Error: fmt.Errorf("stdout pipe: %w", err)}
			return
		}

		if err := cmd.Start(); err != nil {
			ch <- Chunk{Error: fmt.Errorf("start: %w", err)}
			return
		}

		scanner := bufio.NewScanner(stdout)
		buf := make([]byte, 0, 1024*1024)
		scanner.Buffer(buf, 10*1024*1024)

		var fullText strings.Builder

		for scanner.Scan() {
			select {
			case <-cmdCtx.Done():
				ch <- Chunk{Done: true}
				return
			default:
			}

			line := scanner.Text()
			chunk := m.parseLine(line, &fullText)
			if chunk != nil {
				ch <- *chunk
			}
		}

		cmd.Wait()
		ch <- Chunk{Text: fullText.String(), Done: true}
	}()

	return ch
}

func (m *GeminiModel) parseLine(line string, fullText *strings.Builder) *Chunk {
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		// Might be plain text output, treat as content
		if line != "" {
			fullText.WriteString(line)
			return &Chunk{Text: line}
		}
		return nil
	}

	eventType, _ := event["type"].(string)

	switch eventType {
	case "assistant", "message":
		msgData, _ := event["message"].(map[string]any)
		content, _ := msgData["content"].([]any)

		for _, block := range content {
			b, _ := block.(map[string]any)
			if blockType, _ := b["type"].(string); blockType == "text" {
				if text, ok := b["text"].(string); ok {
					fullText.WriteString(text)
					return &Chunk{Text: text}
				}
			}
		}

	case "content_block_delta":
		if delta, ok := event["delta"].(map[string]any); ok {
			if text, ok := delta["text"].(string); ok {
				fullText.WriteString(text)
				return &Chunk{Text: text}
			}
		}

	case "result", "done":
		return &Chunk{Done: true}

	case "error":
		errMsg := "unknown error"
		if msg, ok := event["message"].(string); ok {
			errMsg = msg
		}
		return &Chunk{Error: fmt.Errorf("%s", errMsg)}
	}

	return nil
}

func (m *GeminiModel) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
	}
}
