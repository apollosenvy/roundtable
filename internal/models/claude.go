// internal/models/claude.go
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

type ClaudeModel struct {
	BaseModel
	cliPath   string
	modelName string
	sessionID string
	workDir   string

	cmd    *exec.Cmd
	cancel context.CancelFunc
	mu     sync.Mutex
}

func NewClaude(cliPath, modelName string) *ClaudeModel {
	return &ClaudeModel{
		BaseModel: NewBaseModel(ModelInfo{
			ID:      "claude",
			Name:    "Claude",
			Color:   "#00FFFF", // Cyan
			CanExec: true,
			CanRead: true,
		}),
		cliPath:   cliPath,
		modelName: modelName,
	}
}

func (m *ClaudeModel) SetWorkDir(dir string) {
	m.workDir = dir
}

func (m *ClaudeModel) SetSessionID(id string) {
	m.sessionID = id
}

func (m *ClaudeModel) GetSessionID() string {
	return m.sessionID
}

func (m *ClaudeModel) Send(ctx context.Context, history []Message, prompt string) <-chan Chunk {
	ch := make(chan Chunk, 100)

	go func() {
		defer close(ch)
		m.SetStatus(StatusResponding)
		defer m.SetStatus(StatusIdle)

		// Build command
		cmdCtx, cancel := context.WithCancel(ctx)
		m.mu.Lock()
		m.cancel = cancel
		m.mu.Unlock()

		args := []string{
			"--output-format", "stream-json",
			"--verbose",
		}

		if m.sessionID != "" {
			args = append(args, "--continue", m.sessionID)
		}

		args = append(args, "-p", prompt)

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

func (m *ClaudeModel) parseLine(line string, fullText *strings.Builder) *Chunk {
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return nil
	}

	eventType, _ := event["type"].(string)

	switch eventType {
	case "system":
		if sid, ok := event["session_id"].(string); ok {
			m.sessionID = sid
		}
		return nil

	case "assistant":
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

	case "result":
		if sid, ok := event["session_id"].(string); ok {
			m.sessionID = sid
		}
		return &Chunk{Done: true}

	case "error":
		errMsg := "unknown error"
		if errData, ok := event["error"].(map[string]any); ok {
			if msg, ok := errData["message"].(string); ok {
				errMsg = msg
			}
		} else if msg, ok := event["message"].(string); ok {
			errMsg = msg
		}
		return &Chunk{Error: fmt.Errorf("%s", errMsg)}
	}

	return nil
}

func (m *ClaudeModel) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
	}
}
