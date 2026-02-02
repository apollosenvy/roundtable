# Roundtable TUI - Design Document

Multi-model debate interface for complex operations. An "Agent Discord" where AI models discuss, debate, and reach consensus before execution.

## Core Concept

- **Parallel seeding**: Prompt goes to all models simultaneously, bypassing the "agree with first response" problem
- **Structured debate**: Models explicitly agree, object, or add points
- **Explicit consensus**: No silent agreement - every model states position
- **Single executor**: Only Claude can execute code; others advise with read-only context
- **Persistent sessions**: Resume debates across sessions via tabs

## Tech Stack

- **Language**: Go 1.25+
- **TUI Framework**: Bubbletea (Charmbracelet)
- **Database**: SQLite for persistence
- **Styling**: Lipgloss, Glamour for markdown

## Model Backends

| Model | Method | Auth | Streaming |
|-------|--------|------|-----------|
| Claude | CLI subprocess (`claude --output-format stream-json`) | OAuth (pre-authed) | Yes, JSON lines |
| Gemini | CLI subprocess (`gemini`) | OAuth (pre-authed) | Yes |
| GPT | Direct API (`api.openai.com`) | API key | Yes, SSE |
| Grok | Direct API (`api.x.ai`) | API key | Yes, SSE |

### Unified Interface

```go
type Model interface {
    ID() string                          // "claude-opus", "gpt-4o", etc.
    Name() string                        // Display name for UI
    Color() lipgloss.Color               // Message color

    Send(ctx context.Context, history []Message, prompt string) <-chan Chunk
    CanExecute() bool                    // Only Claude returns true
    ReadFile(path string) (string, error) // Read-only context loading
}

type Chunk struct {
    Text  string
    Done  bool
    Error error
}
```

## Debate Protocol

```
┌─────────────────────────────────────────────────────────────┐
│  1. PARALLEL SEED                                           │
│     User prompt → fan out to all models simultaneously      │
│     Responses stream in as they complete (no blocking)      │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  2. DEBATE ROUNDS                                           │
│     System: "Any objections or additions?"                  │
│     Models respond with:                                    │
│       - AGREE: [model] - explicit endorsement               │
│       - OBJECT: [reason] - disagreement with reasoning      │
│       - ADD: [point] - new consideration                    │
│     User can interject at any time (pauses auto-debate)     │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  3. CONSENSUS CHECK                                         │
│     All models explicitly state agreement or dissent        │
│     Deadlock → user breaks tie                              │
│     Consensus → proceed to execution                        │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  4. EXECUTION (Claude only)                                 │
│     Claude drafts implementation plan                       │
│     User confirms → Claude executes with tools              │
│     Results visible to all models for next iteration        │
└─────────────────────────────────────────────────────────────┘
```

## UI Layout

```
┌──────────────────────────────────────────────────────────────────────────┐
│ Roundtable: API Design Debate                        ○ GPT  ○ Grok  ● All│
├────────────────────────────────────────────────────────────┬─────────────┤
│                                                            │ CONTEXT     │
│  ┌─ Claude (opus) ─────────────────────────────────────┐   │             │
│  │ I'd recommend a REST API with versioned endpoints.  │   │ src/api/    │
│  │ GraphQL adds complexity we don't need yet.          │   │ ├─ routes/  │
│  └─────────────────────────────────────────────────────┘   │ ├─ models/  │
│                                                            │ └─ utils/   │
│  ┌─ GPT-4o ────────────────────────────────────────────┐   │             │
│  │ Disagree. GraphQL gives clients flexibility to      │   │ Loaded:     │
│  │ request exactly what they need. REST over-fetches.  │   │ • schema.ts │
│  └─────────────────────────────────────────────────────┘   │ • types.ts  │
│                                                            │             │
│  ┌─ Gemini ────────────────────────────────────────────┐   ├─────────────┤
│  │ AGREE: Claude. Start simple. We can add GraphQL     │   │ MODELS      │
│  │ later if query patterns demand it.                  │   │             │
│  └─────────────────────────────────────────────────────┘   │ ● Claude    │
│                                                            │ ● GPT-4o    │
│  ┌─ Grok ──────────────────────────────────────────────┐   │ ○ Gemini... │
│  │ ADD: Consider tRPC if this is TypeScript full-stack │   │ ○ Grok...   │
│  │ Gives you type safety without GraphQL overhead.     │   │             │
│  └─────────────────────────────────────────────────────┘   │             │
│                                                            │             │
│  ┌─ SYSTEM ────────────────────────────────────────────┐   │             │
│  │ Consensus check: 2 REST, 1 GraphQL, 1 tRPC          │   │             │
│  │ Any final objections?                               │   │             │
│  └─────────────────────────────────────────────────────┘   │             │
│                                                            │             │
├────────────────────────────────────────────────────────────┴─────────────┤
│ > Let's go with Claude's REST approach, but Grok's tRPC point is good _  │
├──────────────────────────────────────────────────────────────────────────┤
│ Tab 1: API Design │ Tab 2: Auth Flow │ + New                    Ctrl+? │
└──────────────────────────────────────────────────────────────────────────┘
```

### Color Coding

- **Cyan**: Claude messages
- **Green**: GPT messages
- **Magenta**: Gemini messages
- **Orange**: Grok messages
- **Yellow**: System/consensus messages
- **Sky blue**: User messages
- **Dim**: Inactive/waiting models

### Model Status Indicators

- `●` responding
- `○` waiting/idle
- `◌` timed out
- `✗` error

## Database Schema

Location: `~/.local/share/roundtable/debates.db`

```sql
-- Debates (tabs)
CREATE TABLE debates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project_path TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status TEXT DEFAULT 'active',  -- active, resolved, abandoned
    consensus TEXT                  -- final decision if reached
);

-- Messages (the unified stream)
CREATE TABLE messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    debate_id TEXT NOT NULL REFERENCES debates(id),
    source TEXT NOT NULL,           -- claude, gpt, gemini, grok, user, system
    content TEXT NOT NULL,
    msg_type TEXT DEFAULT 'model',  -- model, user, system, tool, meta
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Context files (read-only shared context)
CREATE TABLE context_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    debate_id TEXT NOT NULL REFERENCES debates(id),
    path TEXT NOT NULL,
    content TEXT NOT NULL,
    added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Model state (for resuming mid-debate)
CREATE TABLE model_state (
    debate_id TEXT NOT NULL REFERENCES debates(id),
    model_id TEXT NOT NULL,
    last_seen_msg INTEGER REFERENCES messages(id),
    status TEXT DEFAULT 'idle',     -- idle, responding, waiting, error
    PRIMARY KEY (debate_id, model_id)
);
```

## Configuration

Location: `~/.config/roundtable/config.yaml`

```yaml
models:
  claude:
    enabled: true
    cli_path: claude  # or full path if needed
    default_model: opus
  gemini:
    enabled: true
    cli_path: gemini
  gpt:
    enabled: true
    api_key: ${OPENAI_API_KEY}  # or literal key
    default_model: gpt-4o
  grok:
    enabled: true
    api_key: ${GROK_API_KEY}
    default_model: grok-2

defaults:
  auto_debate: true
  consensus_timeout: 30  # seconds to wait for responses
  model_timeout: 60      # per-model response timeout
```

## Commands

| Command | Description |
|---------|-------------|
| `/help` | Show all commands and keybindings |
| `/new [name]` | Create new debate tab |
| `/close` | Close current debate |
| `/rename [name]` | Rename current debate |
| `/context add <path>` | Load file into shared context |
| `/context remove <path>` | Remove file from context |
| `/context list` | Show loaded files |
| `/models` | Toggle model picker (enable/disable models for this debate) |
| `/consensus` | Force consensus check now |
| `/execute` | Tell Claude to implement agreed approach |
| `/pause` | Pause auto-debate |
| `/resume` | Resume auto-debate |
| `/history` | Show past debates (picker to resume) |
| `/export` | Export debate transcript to markdown |

## Keybindings

| Key | Action |
|-----|--------|
| `Alt+1-9` | Switch tabs |
| `Alt+[/]` | Previous/next tab |
| `Alt+n` | New debate |
| `Alt+w` | Close tab |
| `Ctrl+Enter` | Send message (interject) |
| `Ctrl+Space` | Pause/resume auto-debate |
| `Ctrl+e` | Execute (after consensus) |
| `Tab` | Cycle focus (input → chat → context → models) |
| `Ctrl+/` | Search in debate |
| `Ctrl+k` | Command palette |
| `F1` or `?` | Show help overlay |

## Implementation Phases

### Phase 1: Core Scaffolding
- Go module setup, bubbletea skeleton
- SQLite database init
- Config file loading (API keys, CLI paths)

### Phase 2: Model Backends
- Claude CLI subprocess (adapt from claude-tui)
- Gemini CLI subprocess
- GPT API client
- Grok API client
- Unified Model interface

### Phase 3: UI
- Chat pane with color-coded messages
- Context sidebar
- Model status indicators
- Input box with multiline support
- Tab bar

### Phase 4: Debate Orchestration
- Parallel seed dispatch
- Auto-debate loop with consensus prompts
- User interjection handling
- Consensus detection

### Phase 5: Persistence
- SQLite storage for debates/messages
- Resume from history
- Hermes integration

### Phase 6: Polish
- Help overlay
- Command palette
- Export to markdown
- Error handling and timeouts

## Integration Points

### Hermes Events
- `debate_started` - new debate created
- `consensus_reached` - models agreed on approach
- `execution_complete` - Claude finished implementing

### Session Memory
- Log debate outcomes for cross-session learning
- Record failed approaches to avoid repeating

## Security Notes

- Guardian principles apply: Claude won't execute destructive ops just because other models agree
- API keys stored in config file with 600 permissions
- No credentials in debate transcripts

## Reference

- **claude-tui**: Base TUI architecture, bubbletea patterns
- **llm-mafia-game**: Multi-model orchestration patterns, consensus mechanics
