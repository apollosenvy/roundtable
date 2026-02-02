# Roundtable TUI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a multi-model debate TUI where Claude, Gemini, GPT, and Grok discuss problems in parallel before Claude executes the agreed solution.

**Architecture:** Go/bubbletea TUI with unified Model interface. CLI subprocesses for Claude/Gemini (--output-format stream-json), direct HTTP for GPT/Grok. SQLite for persistence. Debate orchestration with parallel seeding and explicit consensus.

**Tech Stack:** Go 1.25, bubbletea, bubbles, lipgloss, glamour, go-sqlite3

---

## Phase 1: Core Scaffolding

### Task 1: Initialize Go Module

**Files:**
- Create: `go.mod`
- Create: `go.sum`

**Step 1: Initialize module**

```bash
cd ~/Projects/roundtable && go mod init roundtable
```

**Step 2: Add dependencies**

```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/glamour@latest
go get github.com/mattn/go-sqlite3@latest
go get gopkg.in/yaml.v3@latest
```

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "Initialize Go module with dependencies"
```

---

### Task 2: Create Project Structure

**Files:**
- Create: `cmd/roundtable/main.go`
- Create: `internal/models/model.go`
- Create: `internal/ui/app.go`
- Create: `internal/db/store.go`
- Create: `internal/config/config.go`

**Step 1: Create directories**

```bash
mkdir -p cmd/roundtable internal/models internal/ui internal/db internal/config
```

**Step 2: Create minimal main.go**

```go
// cmd/roundtable/main.go
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"roundtable/internal/ui"
)

func main() {
	p := tea.NewProgram(ui.New(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 3: Create minimal app.go**

```go
// internal/ui/app.go
package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

type Model struct {
	width, height int
	ready         bool
}

func New() Model {
	return Model{}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
	}
	return m, nil
}

func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}
	return "Roundtable TUI - Press 'q' to quit"
}
```

**Step 4: Verify it builds**

```bash
go build -o roundtable ./cmd/roundtable
./roundtable
```

Expected: TUI launches, shows message, 'q' quits.

**Step 5: Commit**

```bash
git add cmd/ internal/
git commit -m "Add project structure with minimal TUI skeleton"
```

---

### Task 3: Add Configuration Loading

**Files:**
- Create: `internal/config/config.go`
- Create: `~/.config/roundtable/config.yaml` (example)

**Step 1: Write config.go**

```go
// internal/config/config.go
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ModelConfig struct {
	Enabled      bool   `yaml:"enabled"`
	CLIPath      string `yaml:"cli_path,omitempty"`
	APIKey       string `yaml:"api_key,omitempty"`
	DefaultModel string `yaml:"default_model,omitempty"`
}

type Config struct {
	Models struct {
		Claude ModelConfig `yaml:"claude"`
		Gemini ModelConfig `yaml:"gemini"`
		GPT    ModelConfig `yaml:"gpt"`
		Grok   ModelConfig `yaml:"grok"`
	} `yaml:"models"`
	Defaults struct {
		AutoDebate       bool `yaml:"auto_debate"`
		ConsensusTimeout int  `yaml:"consensus_timeout"`
		ModelTimeout     int  `yaml:"model_timeout"`
	} `yaml:"defaults"`
}

func Load() (*Config, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = os.ExpandEnv("$HOME/.config")
	}

	path := filepath.Join(configDir, "roundtable", "config.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		// Return defaults if no config file
		return defaultConfig(), nil
	}

	// Expand environment variables in config
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, err
	}

	// Apply defaults for unset values
	applyDefaults(&cfg)

	return &cfg, nil
}

func defaultConfig() *Config {
	cfg := &Config{}
	cfg.Models.Claude.Enabled = true
	cfg.Models.Claude.CLIPath = "claude"
	cfg.Models.Claude.DefaultModel = "opus"
	cfg.Models.Gemini.Enabled = true
	cfg.Models.Gemini.CLIPath = "gemini"
	cfg.Models.GPT.Enabled = false
	cfg.Models.Grok.Enabled = false
	cfg.Defaults.AutoDebate = true
	cfg.Defaults.ConsensusTimeout = 30
	cfg.Defaults.ModelTimeout = 60
	return cfg
}

func applyDefaults(cfg *Config) {
	if cfg.Models.Claude.CLIPath == "" {
		cfg.Models.Claude.CLIPath = "claude"
	}
	if cfg.Models.Gemini.CLIPath == "" {
		cfg.Models.Gemini.CLIPath = "gemini"
	}
	if cfg.Defaults.ConsensusTimeout == 0 {
		cfg.Defaults.ConsensusTimeout = 30
	}
	if cfg.Defaults.ModelTimeout == 0 {
		cfg.Defaults.ModelTimeout = 60
	}
}

func ConfigPath() string {
	configDir, _ := os.UserConfigDir()
	if configDir == "" {
		configDir = os.ExpandEnv("$HOME/.config")
	}
	return filepath.Join(configDir, "roundtable", "config.yaml")
}
```

**Step 2: Create example config**

```bash
mkdir -p ~/.config/roundtable
cat > ~/.config/roundtable/config.yaml << 'EOF'
models:
  claude:
    enabled: true
    cli_path: claude
    default_model: opus
  gemini:
    enabled: true
    cli_path: gemini
  gpt:
    enabled: true
    api_key: ${OPENAI_API_KEY}
    default_model: gpt-4o
  grok:
    enabled: false
    api_key: ${GROK_API_KEY}
    default_model: grok-2

defaults:
  auto_debate: true
  consensus_timeout: 30
  model_timeout: 60
EOF
```

**Step 3: Write test for config loading**

```go
// internal/config/config_test.go
package config

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if !cfg.Models.Claude.Enabled {
		t.Error("Claude should be enabled by default")
	}
	if cfg.Models.Claude.CLIPath != "claude" {
		t.Errorf("Claude CLI path should be 'claude', got %s", cfg.Models.Claude.CLIPath)
	}
	if cfg.Defaults.ConsensusTimeout != 30 {
		t.Errorf("ConsensusTimeout should be 30, got %d", cfg.Defaults.ConsensusTimeout)
	}
}

func TestLoad(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}
}
```

**Step 4: Run test**

```bash
go test ./internal/config/...
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "Add configuration loading with YAML support"
```

---

### Task 4: Add SQLite Database Schema

**Files:**
- Create: `internal/db/store.go`
- Create: `internal/db/store_test.go`

**Step 1: Write store.go**

```go
// internal/db/store.go
package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

type Debate struct {
	ID          string
	Name        string
	ProjectPath string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Status      string // active, resolved, abandoned
	Consensus   string
}

type Message struct {
	ID        int64
	DebateID  string
	Source    string // claude, gpt, gemini, grok, user, system
	Content   string
	MsgType   string // model, user, system, tool, meta
	CreatedAt time.Time
}

type ContextFile struct {
	ID       int64
	DebateID string
	Path     string
	Content  string
	AddedAt  time.Time
}

func Open() (*Store, error) {
	dataDir, err := dataDir()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dataDir, "debates.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func dataDir() (string, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "roundtable"), nil
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS debates (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		project_path TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		status TEXT DEFAULT 'active',
		consensus TEXT
	);

	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		debate_id TEXT NOT NULL REFERENCES debates(id),
		source TEXT NOT NULL,
		content TEXT NOT NULL,
		msg_type TEXT DEFAULT 'model',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_messages_debate ON messages(debate_id);

	CREATE TABLE IF NOT EXISTS context_files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		debate_id TEXT NOT NULL REFERENCES debates(id),
		path TEXT NOT NULL,
		content TEXT NOT NULL,
		added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_context_debate ON context_files(debate_id);

	CREATE TABLE IF NOT EXISTS model_state (
		debate_id TEXT NOT NULL REFERENCES debates(id),
		model_id TEXT NOT NULL,
		last_seen_msg INTEGER REFERENCES messages(id),
		status TEXT DEFAULT 'idle',
		PRIMARY KEY (debate_id, model_id)
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}

// CreateDebate creates a new debate and returns its ID
func (s *Store) CreateDebate(id, name, projectPath string) error {
	_, err := s.db.Exec(
		`INSERT INTO debates (id, name, project_path) VALUES (?, ?, ?)`,
		id, name, projectPath,
	)
	return err
}

// GetDebate retrieves a debate by ID
func (s *Store) GetDebate(id string) (*Debate, error) {
	row := s.db.QueryRow(
		`SELECT id, name, project_path, created_at, updated_at, status, consensus
		 FROM debates WHERE id = ?`, id,
	)

	var d Debate
	var projectPath, consensus sql.NullString
	err := row.Scan(&d.ID, &d.Name, &projectPath, &d.CreatedAt, &d.UpdatedAt, &d.Status, &consensus)
	if err != nil {
		return nil, err
	}
	d.ProjectPath = projectPath.String
	d.Consensus = consensus.String
	return &d, nil
}

// ListDebates returns all debates ordered by update time
func (s *Store) ListDebates() ([]Debate, error) {
	rows, err := s.db.Query(
		`SELECT id, name, project_path, created_at, updated_at, status, consensus
		 FROM debates ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var debates []Debate
	for rows.Next() {
		var d Debate
		var projectPath, consensus sql.NullString
		if err := rows.Scan(&d.ID, &d.Name, &projectPath, &d.CreatedAt, &d.UpdatedAt, &d.Status, &consensus); err != nil {
			return nil, err
		}
		d.ProjectPath = projectPath.String
		d.Consensus = consensus.String
		debates = append(debates, d)
	}
	return debates, rows.Err()
}

// AddMessage adds a message to a debate
func (s *Store) AddMessage(debateID, source, content, msgType string) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO messages (debate_id, source, content, msg_type) VALUES (?, ?, ?, ?)`,
		debateID, source, content, msgType,
	)
	if err != nil {
		return 0, err
	}

	// Update debate's updated_at
	s.db.Exec(`UPDATE debates SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, debateID)

	return result.LastInsertId()
}

// GetMessages retrieves all messages for a debate
func (s *Store) GetMessages(debateID string) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT id, debate_id, source, content, msg_type, created_at
		 FROM messages WHERE debate_id = ? ORDER BY id`,
		debateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.DebateID, &m.Source, &m.Content, &m.MsgType, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// AddContextFile adds a file to the debate's shared context
func (s *Store) AddContextFile(debateID, path, content string) error {
	_, err := s.db.Exec(
		`INSERT INTO context_files (debate_id, path, content) VALUES (?, ?, ?)`,
		debateID, path, content,
	)
	return err
}

// GetContextFiles retrieves all context files for a debate
func (s *Store) GetContextFiles(debateID string) ([]ContextFile, error) {
	rows, err := s.db.Query(
		`SELECT id, debate_id, path, content, added_at
		 FROM context_files WHERE debate_id = ? ORDER BY added_at`,
		debateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []ContextFile
	for rows.Next() {
		var f ContextFile
		if err := rows.Scan(&f.ID, &f.DebateID, &f.Path, &f.Content, &f.AddedAt); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// UpdateDebateStatus updates the status of a debate
func (s *Store) UpdateDebateStatus(id, status, consensus string) error {
	_, err := s.db.Exec(
		`UPDATE debates SET status = ?, consensus = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, consensus, id,
	)
	return err
}
```

**Step 2: Write store_test.go**

```go
// internal/db/store_test.go
package db

import (
	"os"
	"testing"
)

func TestStore(t *testing.T) {
	// Use temp dir for test
	os.Setenv("XDG_DATA_HOME", t.TempDir())

	store, err := Open()
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer store.Close()

	// Test create debate
	err = store.CreateDebate("test-1", "Test Debate", "/home/test/project")
	if err != nil {
		t.Fatalf("CreateDebate() failed: %v", err)
	}

	// Test get debate
	debate, err := store.GetDebate("test-1")
	if err != nil {
		t.Fatalf("GetDebate() failed: %v", err)
	}
	if debate.Name != "Test Debate" {
		t.Errorf("Expected name 'Test Debate', got %s", debate.Name)
	}

	// Test add message
	msgID, err := store.AddMessage("test-1", "claude", "Hello from Claude", "model")
	if err != nil {
		t.Fatalf("AddMessage() failed: %v", err)
	}
	if msgID == 0 {
		t.Error("Expected non-zero message ID")
	}

	// Test get messages
	messages, err := store.GetMessages("test-1")
	if err != nil {
		t.Fatalf("GetMessages() failed: %v", err)
	}
	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}

	// Test list debates
	debates, err := store.ListDebates()
	if err != nil {
		t.Fatalf("ListDebates() failed: %v", err)
	}
	if len(debates) != 1 {
		t.Errorf("Expected 1 debate, got %d", len(debates))
	}
}
```

**Step 3: Run tests**

```bash
go test ./internal/db/...
```

Expected: PASS

**Step 4: Commit**

```bash
git add internal/db/
git commit -m "Add SQLite persistence layer for debates"
```

---

## Phase 2: Model Backends

### Task 5: Define Model Interface

**Files:**
- Create: `internal/models/model.go`
- Create: `internal/models/types.go`

**Step 1: Write types.go**

```go
// internal/models/types.go
package models

import "time"

// Chunk represents a piece of streaming response
type Chunk struct {
	Text  string
	Done  bool
	Error error
}

// Message represents a message in the debate
type Message struct {
	Source    string    // claude, gpt, gemini, grok, user, system
	Content   string
	Type      string    // model, user, system, tool, meta
	Timestamp time.Time
	ToolName  string    // for tool messages
}

// ModelStatus represents the current state of a model
type ModelStatus int

const (
	StatusIdle ModelStatus = iota
	StatusResponding
	StatusWaiting
	StatusError
	StatusTimeout
)

func (s ModelStatus) String() string {
	switch s {
	case StatusIdle:
		return "idle"
	case StatusResponding:
		return "responding"
	case StatusWaiting:
		return "waiting"
	case StatusError:
		return "error"
	case StatusTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// ModelInfo contains display information for a model
type ModelInfo struct {
	ID       string // claude, gpt, gemini, grok
	Name     string // Display name
	Color    string // Hex color for UI
	CanExec  bool   // Can execute tools
	CanRead  bool   // Can read files
}
```

**Step 2: Write model.go**

```go
// internal/models/model.go
package models

import (
	"context"
)

// Model is the interface all model backends must implement
type Model interface {
	// Info returns display information about the model
	Info() ModelInfo

	// Send sends a prompt with conversation history and returns a channel of chunks
	Send(ctx context.Context, history []Message, prompt string) <-chan Chunk

	// Stop interrupts any in-progress generation
	Stop()

	// Status returns the current status of the model
	Status() ModelStatus

	// SetStatus updates the model status
	SetStatus(status ModelStatus)
}

// BaseModel provides common functionality for all models
type BaseModel struct {
	info   ModelInfo
	status ModelStatus
}

func NewBaseModel(info ModelInfo) BaseModel {
	return BaseModel{
		info:   info,
		status: StatusIdle,
	}
}

func (m *BaseModel) Info() ModelInfo {
	return m.info
}

func (m *BaseModel) Status() ModelStatus {
	return m.status
}

func (m *BaseModel) SetStatus(status ModelStatus) {
	m.status = status
}
```

**Step 3: Commit**

```bash
git add internal/models/
git commit -m "Define Model interface and types"
```

---

### Task 6: Implement Claude CLI Backend

**Files:**
- Create: `internal/models/claude.go`
- Create: `internal/models/claude_test.go`

**Step 1: Write claude.go**

```go
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

	cmd       *exec.Cmd
	cancel    context.CancelFunc
	mu        sync.Mutex
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
		return &Chunk{Error: fmt.Errorf(errMsg)}
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
```

**Step 2: Write claude_test.go**

```go
// internal/models/claude_test.go
package models

import (
	"os/exec"
	"testing"
)

func TestClaudeInfo(t *testing.T) {
	claude := NewClaude("claude", "opus")
	info := claude.Info()

	if info.ID != "claude" {
		t.Errorf("Expected ID 'claude', got %s", info.ID)
	}
	if !info.CanExec {
		t.Error("Claude should be able to execute")
	}
	if !info.CanRead {
		t.Error("Claude should be able to read files")
	}
}

func TestClaudeCliExists(t *testing.T) {
	_, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("Claude CLI not installed, skipping integration test")
	}
	// CLI exists, test passes
}
```

**Step 3: Run tests**

```bash
go test ./internal/models/... -v
```

**Step 4: Commit**

```bash
git add internal/models/claude*.go
git commit -m "Implement Claude CLI backend"
```

---

### Task 7: Implement Gemini CLI Backend

**Files:**
- Create: `internal/models/gemini.go`

**Step 1: Write gemini.go**

```go
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
		return &Chunk{Error: fmt.Errorf(errMsg)}
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
```

**Step 2: Commit**

```bash
git add internal/models/gemini.go
git commit -m "Implement Gemini CLI backend"
```

---

### Task 8: Implement GPT API Backend

**Files:**
- Create: `internal/models/gpt.go`

**Step 1: Write gpt.go**

```go
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
```

**Step 2: Commit**

```bash
git add internal/models/gpt.go
git commit -m "Implement GPT API backend"
```

---

### Task 9: Implement Grok API Backend

**Files:**
- Create: `internal/models/grok.go`

**Step 1: Write grok.go**

```go
// internal/models/grok.go
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

type GrokModel struct {
	BaseModel
	apiKey    string
	modelName string
	client    *http.Client
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
		client:    &http.Client{},
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

		req, err := http.NewRequestWithContext(cmdCtx, "POST", "https://api.x.ai/v1/chat/completions", bytes.NewReader(bodyBytes))
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
```

**Step 2: Commit**

```bash
git add internal/models/grok.go
git commit -m "Implement Grok API backend"
```

---

### Task 10: Create Model Registry

**Files:**
- Create: `internal/models/registry.go`

**Step 1: Write registry.go**

```go
// internal/models/registry.go
package models

import (
	"roundtable/internal/config"
)

// Registry holds all available models
type Registry struct {
	models map[string]Model
	order  []string // Preserve order for consistent display
}

// NewRegistry creates a registry from config
func NewRegistry(cfg *config.Config) *Registry {
	r := &Registry{
		models: make(map[string]Model),
		order:  []string{},
	}

	// Add Claude if enabled
	if cfg.Models.Claude.Enabled {
		claude := NewClaude(cfg.Models.Claude.CLIPath, cfg.Models.Claude.DefaultModel)
		r.models["claude"] = claude
		r.order = append(r.order, "claude")
	}

	// Add Gemini if enabled
	if cfg.Models.Gemini.Enabled {
		gemini := NewGemini(cfg.Models.Gemini.CLIPath)
		r.models["gemini"] = gemini
		r.order = append(r.order, "gemini")
	}

	// Add GPT if enabled and has API key
	if cfg.Models.GPT.Enabled && cfg.Models.GPT.APIKey != "" {
		gpt := NewGPT(cfg.Models.GPT.APIKey, cfg.Models.GPT.DefaultModel)
		r.models["gpt"] = gpt
		r.order = append(r.order, "gpt")
	}

	// Add Grok if enabled and has API key
	if cfg.Models.Grok.Enabled && cfg.Models.Grok.APIKey != "" {
		grok := NewGrok(cfg.Models.Grok.APIKey, cfg.Models.Grok.DefaultModel)
		r.models["grok"] = grok
		r.order = append(r.order, "grok")
	}

	return r
}

// Get returns a model by ID
func (r *Registry) Get(id string) Model {
	return r.models[id]
}

// All returns all models in order
func (r *Registry) All() []Model {
	result := make([]Model, 0, len(r.order))
	for _, id := range r.order {
		if m, ok := r.models[id]; ok {
			result = append(result, m)
		}
	}
	return result
}

// Enabled returns IDs of all enabled models
func (r *Registry) Enabled() []string {
	return r.order
}

// Count returns number of enabled models
func (r *Registry) Count() int {
	return len(r.order)
}
```

**Step 2: Commit**

```bash
git add internal/models/registry.go
git commit -m "Add model registry for managing backends"
```

---

## Phase 3: UI Components

### Task 11: Create Styles Package

**Files:**
- Create: `internal/ui/styles.go`

**Step 1: Write styles.go**

```go
// internal/ui/styles.go
package ui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	Cyan     = lipgloss.Color("#00FFFF")
	Green    = lipgloss.Color("#00FF00")
	Yellow   = lipgloss.Color("#FFD700")
	Orange   = lipgloss.Color("#FFA500")
	Red      = lipgloss.Color("#FF6B6B")
	Magenta  = lipgloss.Color("#FF00FF")
	SkyBlue  = lipgloss.Color("#87CEEB")
	Dim      = lipgloss.Color("#555555")
	White    = lipgloss.Color("#FFFFFF")
	DarkGray = lipgloss.Color("#333333")

	// Model colors
	ClaudeColor = Cyan
	GPTColor    = Green
	GeminiColor = Magenta
	GrokColor   = Orange
	UserColor   = SkyBlue
	SystemColor = Yellow

	// Box styles
	ActiveBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Cyan)

	InactiveBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Dim)

	// Text styles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Cyan)

	UserStyle = lipgloss.NewStyle().
			Foreground(SkyBlue).
			Bold(true)

	SystemStyle = lipgloss.NewStyle().
			Foreground(Yellow)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Red).
			Bold(true)

	DimStyle = lipgloss.NewStyle().
			Foreground(Dim)

	// Status indicators
	StatusOK   = lipgloss.NewStyle().Foreground(Green).Bold(true)
	StatusWarn = lipgloss.NewStyle().Foreground(Orange).Bold(true)
	StatusCrit = lipgloss.NewStyle().Foreground(Red).Bold(true)

	// Tab styles
	ActiveTabStyle = lipgloss.NewStyle().
			Foreground(Cyan).
			Bold(true)

	InactiveTabStyle = lipgloss.NewStyle().
				Foreground(Dim)
)

// ModelStyle returns the style for a given model ID
func ModelStyle(modelID string) lipgloss.Style {
	switch modelID {
	case "claude":
		return lipgloss.NewStyle().Foreground(ClaudeColor).Bold(true)
	case "gpt":
		return lipgloss.NewStyle().Foreground(GPTColor).Bold(true)
	case "gemini":
		return lipgloss.NewStyle().Foreground(GeminiColor).Bold(true)
	case "grok":
		return lipgloss.NewStyle().Foreground(GrokColor).Bold(true)
	case "user":
		return UserStyle
	case "system":
		return SystemStyle
	default:
		return lipgloss.NewStyle().Foreground(White)
	}
}

// ModelColor returns the color for a given model ID
func ModelColor(modelID string) lipgloss.Color {
	switch modelID {
	case "claude":
		return ClaudeColor
	case "gpt":
		return GPTColor
	case "gemini":
		return GeminiColor
	case "grok":
		return GrokColor
	case "user":
		return SkyBlue
	case "system":
		return Yellow
	default:
		return White
	}
}
```

**Step 2: Commit**

```bash
git add internal/ui/styles.go
git commit -m "Add UI styles with model color coding"
```

---

### Task 12: Create Debate Component

**Files:**
- Create: `internal/ui/debate.go`

**Step 1: Write debate.go**

```go
// internal/ui/debate.go
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"roundtable/internal/models"
)

// DebateMessage represents a message in the debate
type DebateMessage struct {
	Source    string    // claude, gpt, gemini, grok, user, system
	Content   string
	Timestamp time.Time
}

// Debate represents a single debate session
type Debate struct {
	ID           string
	Name         string
	ProjectPath  string
	Messages     []DebateMessage
	ContextFiles map[string]string // path -> content
	Paused       bool

	// Model states
	ModelStatus map[string]models.ModelStatus
}

func NewDebate(id, name string) *Debate {
	return &Debate{
		ID:           id,
		Name:         name,
		Messages:     []DebateMessage{},
		ContextFiles: make(map[string]string),
		ModelStatus:  make(map[string]models.ModelStatus),
	}
}

func (d *Debate) AddMessage(source, content string) {
	d.Messages = append(d.Messages, DebateMessage{
		Source:    source,
		Content:   content,
		Timestamp: time.Now(),
	})
}

func (d *Debate) RenderMessages(width int) string {
	var sb strings.Builder

	for _, msg := range d.Messages {
		ts := msg.Timestamp.Format("15:04")
		style := ModelStyle(msg.Source)

		// Model name header
		header := style.Render(fmt.Sprintf("[%s] %s:", ts, formatSource(msg.Source)))
		sb.WriteString(header)
		sb.WriteString("\n")

		// Message content with indent
		lines := strings.Split(msg.Content, "\n")
		for _, line := range lines {
			sb.WriteString("  ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatSource(source string) string {
	switch source {
	case "claude":
		return "Claude"
	case "gpt":
		return "GPT"
	case "gemini":
		return "Gemini"
	case "grok":
		return "Grok"
	case "user":
		return "You"
	case "system":
		return "System"
	default:
		return source
	}
}

// RenderModelStatus renders the model status sidebar
func (d *Debate) RenderModelStatus(modelIDs []string, height int) string {
	var sb strings.Builder

	sb.WriteString(TitleStyle.Render("MODELS"))
	sb.WriteString("\n\n")

	for _, id := range modelIDs {
		status := d.ModelStatus[id]
		indicator := statusIndicator(status)
		style := ModelStyle(id)

		name := formatSource(id)
		if status == models.StatusResponding {
			name += "..."
		}

		sb.WriteString(fmt.Sprintf("%s %s\n", indicator, style.Render(name)))
	}

	return sb.String()
}

func statusIndicator(status models.ModelStatus) string {
	switch status {
	case models.StatusResponding:
		return StatusWarn.Render("●")
	case models.StatusWaiting:
		return DimStyle.Render("○")
	case models.StatusError:
		return StatusCrit.Render("✗")
	case models.StatusTimeout:
		return DimStyle.Render("◌")
	default: // Idle
		return StatusOK.Render("●")
	}
}

// DebateView wraps a debate with viewport for scrolling
type DebateView struct {
	Debate   *Debate
	Viewport viewport.Model
}

func NewDebateView(debate *Debate, width, height int) *DebateView {
	vp := viewport.New(width, height)
	vp.Style = lipgloss.NewStyle()
	vp.MouseWheelEnabled = true

	return &DebateView{
		Debate:   debate,
		Viewport: vp,
	}
}

func (v *DebateView) Update() {
	content := v.Debate.RenderMessages(v.Viewport.Width)
	v.Viewport.SetContent(content)
	v.Viewport.GotoBottom()
}
```

**Step 2: Commit**

```bash
git add internal/ui/debate.go
git commit -m "Add debate component with message rendering"
```

---

### Task 13: Create Full UI Model

**Files:**
- Modify: `internal/ui/app.go`

**Step 1: Rewrite app.go with full UI**

```go
// internal/ui/app.go
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"

	"roundtable/internal/config"
	"roundtable/internal/db"
	"roundtable/internal/models"
)

// Focus states
type FocusPane int

const (
	FocusInput FocusPane = iota
	FocusChat
	FocusContext
	FocusModels
)

// Model is the main application model
type Model struct {
	// Dimensions
	width, height int
	ready         bool

	// Config and dependencies
	config   *config.Config
	store    *db.Store
	registry *models.Registry

	// UI Components
	focus       FocusPane
	input       textarea.Model
	chatView    viewport.Model
	contextView viewport.Model

	// Debate state
	debates   []*Debate
	activeTab int

	// Command state
	showHelp bool
}

func New() Model {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	// Open database
	store, _ := db.Open()

	// Create model registry
	registry := models.NewRegistry(cfg)

	// Text input
	ta := textarea.New()
	ta.Placeholder = "Enter your prompt... (Ctrl+Enter to send)"
	ta.Focus()
	ta.CharLimit = 8192
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	// Create initial debate
	debateID := uuid.New().String()[:8]
	firstDebate := NewDebate(debateID, "New Debate")

	return Model{
		config:    cfg,
		store:     store,
		registry:  registry,
		input:     ta,
		debates:   []*Debate{firstDebate},
		activeTab: 0,
		focus:     FocusInput,
	}
}

func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

func (m *Model) activeDebate() *Debate {
	if m.activeTab >= 0 && m.activeTab < len(m.debates) {
		return m.debates[m.activeTab]
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			return m, tea.Quit

		case "ctrl+enter":
			// Send message
			prompt := strings.TrimSpace(m.input.Value())
			if prompt != "" && m.activeDebate() != nil {
				m.activeDebate().AddMessage("user", prompt)
				m.input.Reset()
				m.updateChatView()
				// TODO: Dispatch to models
			}
			return m, nil

		case "f1", "?":
			m.showHelp = !m.showHelp
			return m, nil

		case "esc":
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
			m.focus = FocusInput
			m.input.Focus()
			return m, nil

		case "tab":
			m.cycleFocus(1)
			return m, nil

		case "shift+tab":
			m.cycleFocus(-1)
			return m, nil

		// Tab switching
		case "alt+1":
			m.switchTab(0)
		case "alt+2":
			m.switchTab(1)
		case "alt+3":
			m.switchTab(2)
		case "alt+]":
			if len(m.debates) > 1 {
				m.switchTab((m.activeTab + 1) % len(m.debates))
			}
		case "alt+[":
			if len(m.debates) > 1 {
				m.switchTab((m.activeTab - 1 + len(m.debates)) % len(m.debates))
			}

		case "alt+n":
			m.createTab()
			return m, nil

		case "alt+w":
			m.closeTab(m.activeTab)
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		m.ready = true
	}

	// Update focused component
	if m.focus == FocusInput {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.focus == FocusChat {
		var cmd tea.Cmd
		m.chatView, cmd = m.chatView.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) cycleFocus(dir int) {
	panes := []FocusPane{FocusInput, FocusChat, FocusContext, FocusModels}
	current := 0
	for i, p := range panes {
		if p == m.focus {
			current = i
			break
		}
	}
	next := (current + dir + len(panes)) % len(panes)
	m.focus = panes[next]

	m.input.Blur()
	if m.focus == FocusInput {
		m.input.Focus()
	}
}

func (m *Model) createTab() {
	debateID := uuid.New().String()[:8]
	debate := NewDebate(debateID, fmt.Sprintf("Debate %d", len(m.debates)+1))
	m.debates = append(m.debates, debate)
	m.activeTab = len(m.debates) - 1
	m.updateChatView()
}

func (m *Model) closeTab(idx int) {
	if idx < 0 || idx >= len(m.debates) || len(m.debates) <= 1 {
		return
	}

	m.debates = append(m.debates[:idx], m.debates[idx+1:]...)

	if m.activeTab >= len(m.debates) {
		m.activeTab = len(m.debates) - 1
	}
	m.updateChatView()
}

func (m *Model) switchTab(idx int) {
	if idx >= 0 && idx < len(m.debates) {
		m.activeTab = idx
		m.updateChatView()
	}
}

func (m *Model) updateLayout() {
	contextWidth := 25
	modelsWidth := 15
	chatWidth := m.width - contextWidth - modelsWidth - 6
	contentHeight := m.height - 10

	m.chatView = viewport.New(chatWidth, contentHeight)
	m.chatView.Style = lipgloss.NewStyle()
	m.chatView.MouseWheelEnabled = true

	m.contextView = viewport.New(contextWidth-2, contentHeight)
	m.contextView.Style = lipgloss.NewStyle()

	m.input.SetWidth(m.width - 4)

	m.updateChatView()
}

func (m *Model) updateChatView() {
	debate := m.activeDebate()
	if debate == nil {
		return
	}

	content := debate.RenderMessages(m.chatView.Width)
	m.chatView.SetContent(content)
	m.chatView.GotoBottom()
}

func (m Model) View() string {
	if !m.ready {
		return "Loading Roundtable..."
	}

	if m.showHelp {
		return m.renderHelp()
	}

	// Title bar
	title := m.renderTitle()

	// Tab bar
	tabBar := m.renderTabBar()

	// Main content (3 panes)
	contextPane := m.renderContextPane()
	chatPane := m.renderChatPane()
	modelsPane := m.renderModelsPane()

	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, contextPane, chatPane, modelsPane)

	// Status bar
	statusBar := m.renderStatusBar()

	// Input
	inputPane := m.renderInputPane()

	return lipgloss.JoinVertical(lipgloss.Left, title, tabBar, mainContent, statusBar, inputPane)
}

func (m Model) renderTitle() string {
	debate := m.activeDebate()
	left := TitleStyle.Render("╭─ ROUNDTABLE ─╮")

	name := "New Debate"
	if debate != nil {
		name = debate.Name
	}
	middle := DimStyle.Render(fmt.Sprintf(" %s ", name))

	modelCount := fmt.Sprintf("%d models", m.registry.Count())
	right := DimStyle.Render(modelCount)

	padding := m.width - lipgloss.Width(left) - lipgloss.Width(middle) - lipgloss.Width(right) - 2
	if padding < 0 {
		padding = 0
	}

	return left + middle + strings.Repeat(" ", padding) + right
}

func (m Model) renderTabBar() string {
	var tabs []string

	for i, d := range m.debates {
		label := d.Name
		if len(label) > 15 {
			label = label[:15] + ".."
		}

		var tabText string
		if i == m.activeTab {
			tabText = ActiveTabStyle.Render(fmt.Sprintf(" %d:%s ", i+1, label))
		} else {
			tabText = InactiveTabStyle.Render(fmt.Sprintf(" %d:%s ", i+1, label))
		}
		tabs = append(tabs, tabText)
	}

	bar := strings.Join(tabs, DimStyle.Render("│"))
	newTab := DimStyle.Render("  [Alt+N: new]")

	return " " + bar + newTab
}

func (m Model) renderContextPane() string {
	style := InactiveBox
	if m.focus == FocusContext {
		style = ActiveBox
	}

	debate := m.activeDebate()
	var content strings.Builder

	content.WriteString(TitleStyle.Render("CONTEXT"))
	content.WriteString("\n\n")

	if debate != nil && len(debate.ContextFiles) > 0 {
		for path := range debate.ContextFiles {
			content.WriteString(DimStyle.Render("• " + path))
			content.WriteString("\n")
		}
	} else {
		content.WriteString(DimStyle.Render("No files loaded"))
		content.WriteString("\n")
		content.WriteString(DimStyle.Render("/context add <path>"))
	}

	return style.Width(25).Height(m.height - 10).Render(content.String())
}

func (m Model) renderChatPane() string {
	style := InactiveBox
	if m.focus == FocusChat {
		style = ActiveBox
	}

	debate := m.activeDebate()
	title := TitleStyle.Render("DEBATE")
	if m.focus == FocusChat {
		title += " <"
	}

	msgCount := 0
	if debate != nil {
		msgCount = len(debate.Messages)
	}
	title += DimStyle.Render(fmt.Sprintf(" (%d msgs)", msgCount))

	chatWidth := m.width - 25 - 15 - 6

	return style.Width(chatWidth).Height(m.height - 10).Render(
		lipgloss.JoinVertical(lipgloss.Left, title, m.chatView.View()),
	)
}

func (m Model) renderModelsPane() string {
	style := InactiveBox
	if m.focus == FocusModels {
		style = ActiveBox
	}

	var content strings.Builder
	content.WriteString(TitleStyle.Render("MODELS"))
	content.WriteString("\n\n")

	for _, model := range m.registry.All() {
		info := model.Info()
		status := model.Status()
		indicator := statusIndicator(status)
		style := ModelStyle(info.ID)

		name := info.Name
		if status == models.StatusResponding {
			name += "..."
		}

		content.WriteString(fmt.Sprintf("%s %s\n", indicator, style.Render(name)))
	}

	return style.Width(15).Height(m.height - 10).Render(content.String())
}

func (m Model) renderStatusBar() string {
	debate := m.activeDebate()

	// Debate status
	status := StatusOK.Render("● READY")
	if debate != nil && debate.Paused {
		status = StatusWarn.Render("● PAUSED")
	}

	// Tab info
	tabInfo := DimStyle.Render(fmt.Sprintf("[%d/%d]", m.activeTab+1, len(m.debates)))

	// Keybinds
	keys := DimStyle.Render("Ctrl+Enter:send | Alt+N:new | F1:help")

	left := lipgloss.JoinHorizontal(lipgloss.Left, " ", status, "  ", tabInfo)
	right := keys + " "

	padding := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 0 {
		padding = 0
	}

	separator := DimStyle.Render(strings.Repeat("─", m.width))
	return separator + "\n" + left + strings.Repeat(" ", padding) + right
}

func (m Model) renderInputPane() string {
	style := InactiveBox
	if m.focus == FocusInput {
		style = ActiveBox
	}

	label := "Message"
	return style.Width(m.width - 2).Render(
		DimStyle.Render(label) + "\n" + m.input.View(),
	)
}

func (m Model) renderHelp() string {
	help := `
╭─────────────────── ROUNDTABLE HELP ───────────────────╮
│                                                        │
│  NAVIGATION                                            │
│    Tab / Shift+Tab    Cycle focus between panes       │
│    Alt+1-9            Switch to tab N                  │
│    Alt+[ / Alt+]      Previous / Next tab             │
│    Esc                Return to input                  │
│                                                        │
│  ACTIONS                                               │
│    Ctrl+Enter         Send message to all models       │
│    Ctrl+Space         Pause / Resume auto-debate      │
│    Ctrl+E             Execute (after consensus)        │
│                                                        │
│  TABS                                                  │
│    Alt+N              New debate                       │
│    Alt+W              Close current tab                │
│                                                        │
│  COMMANDS                                              │
│    /help              Show this help                   │
│    /new [name]        New debate                       │
│    /close             Close debate                     │
│    /context add PATH  Load file into context          │
│    /context list      List context files              │
│    /models            Toggle model picker              │
│    /consensus         Force consensus check           │
│    /execute           Execute agreed approach          │
│    /pause             Pause auto-debate                │
│    /resume            Resume auto-debate               │
│    /history           Browse past debates              │
│    /export            Export to markdown               │
│                                                        │
│  Press F1 or ? to toggle this help                    │
╰────────────────────────────────────────────────────────╯
`
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Cyan).
			Padding(1, 2).
			Render(help))
}
```

**Step 2: Add uuid dependency**

```bash
go get github.com/google/uuid
```

**Step 3: Build and test**

```bash
go build -o roundtable ./cmd/roundtable
./roundtable
```

Expected: Full TUI with 3 panes, tab bar, help overlay (F1)

**Step 4: Commit**

```bash
git add internal/ui/app.go go.mod go.sum
git commit -m "Add full UI with panes, tabs, and help overlay"
```

---

## Phase 4: Debate Orchestration

### Task 14: Add Message Dispatching

**Files:**
- Create: `internal/orchestrator/orchestrator.go`

**Step 1: Write orchestrator.go**

```go
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
```

**Step 2: Commit**

```bash
mkdir -p internal/orchestrator
git add internal/orchestrator/
git commit -m "Add orchestrator for multi-model dispatch"
```

---

### Task 15: Integrate Orchestrator with UI

**Files:**
- Modify: `internal/ui/app.go`

**Step 1: Add message types for model responses**

Add to app.go after imports:

```go
// Message types for async model responses
type modelResponseMsg struct {
	modelID string
	content string
	done    bool
	err     error
}

type allModelsDoneMsg struct{}
```

**Step 2: Add orchestrator field and dispatch logic**

Update the Model struct:

```go
type Model struct {
	// ... existing fields ...

	// Orchestrator
	orchestrator *orchestrator.Orchestrator
	cancelDebate context.CancelFunc
}
```

**Step 3: Update New() to create orchestrator**

```go
func New() Model {
	// ... existing code ...

	// Create orchestrator
	orch := orchestrator.New(registry, time.Duration(cfg.Defaults.ModelTimeout)*time.Second)

	return Model{
		// ... existing fields ...
		orchestrator: orch,
	}
}
```

**Step 4: Add dispatch command**

```go
func (m *Model) dispatchToModels(prompt string) tea.Cmd {
	return func() tea.Msg {
		debate := m.activeDebate()
		if debate == nil {
			return nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		m.cancelDebate = cancel

		// Convert debate messages to model messages
		var history []models.Message
		for _, msg := range debate.Messages {
			history = append(history, models.Message{
				Source:  msg.Source,
				Content: msg.Content,
			})
		}

		responses := m.orchestrator.ParallelSeed(ctx, history, prompt)

		// Forward responses as tea messages
		go func() {
			for resp := range responses {
				program.Send(modelResponseMsg{
					modelID: resp.ModelID,
					content: resp.Content,
					done:    resp.Done,
					err:     resp.Error,
				})
			}
			program.Send(allModelsDoneMsg{})
		}()

		return nil
	}
}
```

**Step 5: Handle model responses in Update**

Add cases to Update():

```go
case modelResponseMsg:
	debate := m.activeDebate()
	if debate == nil {
		return m, nil
	}

	if msg.err != nil {
		debate.AddMessage("system", fmt.Sprintf("[%s error: %v]", msg.modelID, msg.err))
	} else if msg.content != "" {
		// Append to existing message or create new one
		// For streaming, we accumulate content
		debate.AddMessage(msg.modelID, msg.content)
	}

	if msg.done {
		debate.ModelStatus[msg.modelID] = models.StatusIdle
	} else {
		debate.ModelStatus[msg.modelID] = models.StatusResponding
	}

	m.updateChatView()
	return m, nil

case allModelsDoneMsg:
	// All models finished, could trigger consensus check
	debate := m.activeDebate()
	if debate != nil {
		debate.AddMessage("system", "All models have responded. Any objections or additions?")
		m.updateChatView()
	}
	return m, nil
```

**Step 6: Wire up send button**

Update the ctrl+enter handler:

```go
case "ctrl+enter":
	prompt := strings.TrimSpace(m.input.Value())
	if prompt != "" && m.activeDebate() != nil {
		m.activeDebate().AddMessage("user", prompt)
		m.input.Reset()
		m.updateChatView()
		return m, m.dispatchToModels(prompt)
	}
	return m, nil
```

**Step 7: Add program variable**

At package level:

```go
var program *tea.Program
```

**Step 8: Update main.go**

```go
func main() {
	m := ui.New()
	ui.SetProgram(tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()))

	if _, err := ui.Program().Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 9: Add accessor functions**

```go
func SetProgram(p *tea.Program) {
	program = p
}

func Program() *tea.Program {
	return program
}
```

**Step 10: Commit**

```bash
git add internal/ui/app.go cmd/roundtable/main.go
git commit -m "Integrate orchestrator with UI for parallel model dispatch"
```

---

## Phase 5: Persistence Integration

### Task 16: Wire Up Database Persistence

Add to app.go - save messages to database:

```go
func (m *Model) saveMessage(debateID, source, content, msgType string) {
	if m.store != nil {
		m.store.AddMessage(debateID, source, content, msgType)
	}
}
```

Call saveMessage when adding messages to debates.

### Task 17: Add Debate Resume

Add `/history` command to list and resume past debates from database.

---

## Phase 6: Polish

### Task 18: Add Hermes Integration

Emit events to Hermes for session tracking:
- `debate_started`
- `consensus_reached`
- `execution_complete`

### Task 19: Add Export Command

Implement `/export` to save debate transcript as markdown.

### Task 20: Error Handling and Timeouts

Add proper error handling for:
- Model connection failures
- API rate limits
- Timeouts

---

## Verification Checklist

After implementation, verify:

- [ ] TUI launches and displays correctly
- [ ] Tab switching works (Alt+1-9, Alt+[/])
- [ ] New/close tabs work (Alt+N, Alt+W)
- [ ] Help overlay shows (F1)
- [ ] Config loads from ~/.config/roundtable/config.yaml
- [ ] Claude CLI backend streams responses
- [ ] Gemini CLI backend streams responses
- [ ] GPT API backend streams responses (if key configured)
- [ ] Parallel seeding sends to all models
- [ ] Messages display with correct colors
- [ ] Model status indicators update
- [ ] Debates persist to SQLite
- [ ] Past debates can be resumed
