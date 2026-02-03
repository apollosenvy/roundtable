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

		// Build the full prompt with debate context
		var fullPrompt strings.Builder

		// Add system context explaining the debate format
		fullPrompt.WriteString("You are participating in a multi-model debate called Roundtable. ")
		fullPrompt.WriteString("Other AI models (GPT, Gemini, Grok) respond alongside you. ")
		fullPrompt.WriteString("Be direct and substantive. ")
		fullPrompt.WriteString("If you agree with another model, say AGREE: [reason]. ")
		fullPrompt.WriteString("If you disagree, say OBJECT: [reason]. ")
		fullPrompt.WriteString("If you have something to add, say ADD: [point].\n\n")

		// Add conversation history if present
		if len(history) > 0 {
			fullPrompt.WriteString("=== CONVERSATION SO FAR ===\n")
			for _, msg := range history {
				fullPrompt.WriteString(fmt.Sprintf("[%s]: %s\n\n", msg.Source, msg.Content))
			}
			fullPrompt.WriteString("=== END CONVERSATION ===\n\n")
		}

		// Add the current prompt
		fullPrompt.WriteString("Current prompt:\n")
		fullPrompt.WriteString(prompt)

		// Use --print for non-interactive mode with JSON output
		// This gives a clean single JSON object with the result
		// NOTE: We do NOT use --continue because we build our own conversation
		// history from all models. Using --continue would cause Claude to ignore
		// our injected context and only see its own session history.
		args := []string{
			"--print",
			"--output-format", "json",
			"-p", fullPrompt.String(),
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

		stderr, err := cmd.StderrPipe()
		if err != nil {
			ch <- Chunk{Error: fmt.Errorf("stderr pipe: %w", err)}
			return
		}

		if err := cmd.Start(); err != nil {
			ch <- Chunk{Error: fmt.Errorf("start: %w", err)}
			return
		}

		// Capture stderr in background
		var stderrBuf strings.Builder
		go func() {
			stderrScanner := bufio.NewScanner(stderr)
			for stderrScanner.Scan() {
				stderrBuf.WriteString(stderrScanner.Text())
				stderrBuf.WriteString("\n")
			}
		}()

		scanner := bufio.NewScanner(stdout)
		buf := make([]byte, 0, 1024*1024)
		scanner.Buffer(buf, 10*1024*1024)

		var fullText strings.Builder
		var gotResponse bool

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
				if chunk.Text != "" {
					gotResponse = true
				}
				ch <- *chunk
			}
		}

		cmd.Wait()

		// If we got no response and there's stderr output, report it as error
		if !gotResponse && stderrBuf.Len() > 0 {
			ch <- Chunk{Error: fmt.Errorf("claude stderr: %s", stderrBuf.String())}
			return
		}

		ch <- Chunk{Done: true}
	}()

	return ch
}

func (m *ClaudeModel) parseLine(line string, fullText *strings.Builder) *Chunk {
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		// Not valid JSON - might be plain text output, ignore
		return nil
	}

	eventType, _ := event["type"].(string)

	switch eventType {
	case "system":
		// Extract session_id if present
		if sid, ok := event["session_id"].(string); ok {
			m.sessionID = sid
		}
		return nil

	case "result":
		// This is the main response format from --output-format json
		// The actual text is in the "result" field
		if sid, ok := event["session_id"].(string); ok {
			m.sessionID = sid
		}

		if result, ok := event["result"].(string); ok && result != "" {
			fullText.WriteString(result)
			return &Chunk{Text: result, Done: true}
		}

		// Check for errors in result
		if isError, ok := event["is_error"].(bool); ok && isError {
			errMsg := "Claude returned an error"
			if result, ok := event["result"].(string); ok {
				errMsg = result
			}
			return &Chunk{Error: fmt.Errorf("%s", errMsg)}
		}

		return &Chunk{Done: true}

	case "assistant":
		// Handle stream-json format (for verbose mode)
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
		// Handle streaming chunks in verbose mode
		if delta, ok := event["delta"].(map[string]any); ok {
			if text, ok := delta["text"].(string); ok {
				fullText.WriteString(text)
				return &Chunk{Text: text}
			}
		}

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
