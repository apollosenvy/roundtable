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

// UpdateDebateName updates the name of a debate
func (s *Store) UpdateDebateName(id, name string) error {
	_, err := s.db.Exec(
		`UPDATE debates SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		name, id,
	)
	return err
}

// RemoveContextFile removes a context file from a debate
func (s *Store) RemoveContextFile(debateID, path string) error {
	_, err := s.db.Exec(
		`DELETE FROM context_files WHERE debate_id = ? AND path = ?`,
		debateID, path,
	)
	return err
}
