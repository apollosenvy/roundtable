# Roundtable

Multi-model debate TUI for complex decisions. An "Agent Discord" where AI models discuss, debate, and reach consensus before anything executes.

## What Is It

Roundtable lets you have structured debates with multiple AI models simultaneously. Instead of asking one model and accepting its answer, you:

1. **Parallel seed** a prompt to all models at once
2. **Watch them debate** - explicitly agree, object, or add points
3. **Reach consensus** or have the human break ties
4. **Execute** (Claude only) with read-only context from all other models

No sycophancy. No "they all agree with the first model." No silent consensus. Every model states its position explicitly.

## Why It Exists

Single-model reasoning has a problem: the first response anchors everything else. You ask Claude a question, you get an answer. You ask it again differently, you probably get the same answer. It's optimized to be helpful on the first try, which means it won't contradict itself.

Roundtable breaks that. By seeding multiple models simultaneously with the same prompt, you force:

- **Genuine disagreement**: GPT doesn't know what Claude said yet
- **Explicit reasoning**: Models must justify their positions
- **Diverse perspectives**: Each model brings different biases and strengths
- **Clear consensus**: You see where models actually agree vs. where they diverge

This is especially valuable for:
- Architecture decisions (REST vs GraphQL vs tRPC)
- Security reviews (models cross-check each other)
- Complex problem decomposition
- Catching blind spots in reasoning

## Installation

### From Release

```bash
wget https://github.com/example/roundtable/releases/download/v0.1.0/roundtable-linux-amd64
chmod +x roundtable-linux-amd64
sudo mv roundtable-linux-amd64 /usr/local/bin/roundtable
```

### From Source

```bash
git clone https://github.com/example/roundtable.git
cd roundtable
go build -o roundtable ./cmd/roundtable
sudo mv roundtable /usr/local/bin/
```

### Requirements

- Go 1.25+ (if building from source)
- Claude CLI: `command -v claude >/dev/null 2>&1` (with OAuth already configured)
- Gemini CLI: `command -v gemini >/dev/null 2>&1` (optional, for Gemini support)
- OpenAI API key (optional, for GPT-4o)
- Grok API key (optional, for Grok-2)

## Configuration

Create `~/.config/roundtable/config.yaml`:

```yaml
models:
  claude:
    enabled: true
    cli_path: claude        # Full path if not in PATH
    default_model: opus

  gemini:
    enabled: true
    cli_path: gemini

  gpt:
    enabled: true
    api_key: ${OPENAI_API_KEY}  # Expands from env var
    default_model: gpt-4o

  grok:
    enabled: true
    api_key: ${GROK_API_KEY}
    default_model: grok-2

defaults:
  auto_debate: true           # Automatically ask "any objections?" after responses
  consensus_timeout: 30       # Seconds to wait for all responses before consensus check
  model_timeout: 60           # Timeout per individual model
  retry_attempts: 3           # Retry failed requests
  retry_delay: 1000          # Milliseconds between retries
```

### Environment Variables

Set API keys in your shell:

```bash
export OPENAI_API_KEY="sk-..."
export GROK_API_KEY="..."
```

Or hardcode in config (less secure, but works):

```yaml
gpt:
  api_key: "sk-..."
```

### Database

Roundtable stores debates at `~/.local/share/roundtable/debates.db`. It persists:
- Debate metadata (name, creation time, status)
- All messages from all models
- Context files you've loaded
- Model status during debates

You can delete this to start fresh, but you'll lose debate history.

## Usage

### Starting a Debate

```bash
roundtable
```

You'll see:

```
ROUNDTABLE: New Debate                              3 models
┌─────────────────────────────────────────────────────────┬────────────────┐
│ DEBATE                                                  │ CONTEXT        │
│                                                          │                │
│ Welcome to Roundtable. Ctrl+Enter to start.              │ No files loaded │
│                                                          │ /context add... │
│                                                          ├────────────────┤
│                                                          │ MODELS         │
│                                                          │                │
│                                                          │ ● Claude       │
│                                                          │ ● GPT-4o       │
│                                                          │ ○ Gemini       │
│                                                          │                │
│ 1:New Debate │ [Alt+N: new]                   Ctrl+? │
└─────────────────────────────────────────────────────────┴────────────────┘
│ Message
│ Enter your prompt... (Ctrl+Enter to send)
└─────────────────────────────────────────────────────────────────────────
```

### Keybindings

#### Message Input

| Key | Action |
|-----|--------|
| `Ctrl+Enter` | Send message to all enabled models |
| `Ctrl+Space` | Pause/resume auto-debate |
| `Alt+P` | Disable pause overlay (just type normally) |

#### Navigation

| Key | Action |
|-----|--------|
| `Alt+1-9` | Switch to tab N |
| `Alt+[` | Previous tab |
| `Alt+]` | Next tab |
| `Alt+N` | New debate tab |
| `Alt+W` | Close current tab |
| `Tab` | Cycle focus: Input → Chat → Context → Models |
| `Shift+Tab` | Cycle focus backwards |

#### Help & View

| Key | Action |
|-----|--------|
| `F1` or `?` | Show keybindings overlay |
| `Alt+H` | Browse debate history |
| `Esc` | Close overlays, return focus to input |
| `Ctrl+C` or `Ctrl+Q` | Quit |

#### In History Browser

| Key | Action |
|-----|--------|
| `Up` / `K` | Select previous debate |
| `Down` / `J` | Select next debate |
| `Enter` | Open selected debate in new tab |
| `Esc` / `Q` | Close history browser |

### Slash Commands

Type these in the message input:

```
/help                    Show all commands
/new [name]              Create new debate tab
/rename [name]           Rename current debate
/context add <path>      Load file into shared context
/context remove <path>   Remove file from context
/context list            Show loaded files
/models                  Toggle which models are enabled
/consensus               Force consensus check now
/execute                 Tell Claude to implement agreed approach
/pause                   Pause auto-debate
/resume                  Resume auto-debate
/history                 Show past debates (picker)
/export                  Export debate transcript to markdown
```

Examples:

```
/context add src/api.ts
/context add docs/architecture.md
/new Performance Optimization
/rename "API Design v2"
```

### Colors & Indicators

Messages are color-coded by model:

- **Cyan** - Claude messages
- **Green** - GPT messages
- **Magenta** - Gemini messages
- **Orange** - Grok messages
- **Yellow** - System messages (consensus checks, auto-debate prompts)
- **Blue** - Your messages

Model status in the right sidebar:

- `●` Responding (streaming)
- `○` Idle/waiting
- `◌` Timed out
- `✗` Error

## Model Setup

### Claude CLI

Roundtable uses the Claude CLI (subprocess mode). Ensure it's already authenticated:

```bash
claude --help
# Should show: "You are not logged in. Run 'claude login' to authenticate."
# or display help if already logged in
```

If not authenticated:

```bash
claude login
# Follow the browser flow to authenticate with Anthropic
```

Roundtable calls: `claude --output-format stream-json` (for streaming JSON lines responses)

### Gemini CLI

Install and authenticate:

```bash
# Install (varies by platform)
# See: https://github.com/googleapis/gcloud-cli

gcloud auth login
gcloud auth application-default login
```

Roundtable calls: `gemini` (captures stdout streaming)

### OpenAI (GPT)

Set your API key:

```bash
export OPENAI_API_KEY="sk-..."
```

Then enable GPT in config:

```yaml
gpt:
  enabled: true
  api_key: ${OPENAI_API_KEY}
  default_model: gpt-4o
```

Roundtable connects directly to `https://api.openai.com/v1/chat/completions` with streaming.

### Grok (X.AI)

Set your API key:

```bash
export GROK_API_KEY="..."
```

Then enable Grok in config:

```yaml
grok:
  enabled: true
  api_key: ${GROK_API_KEY}
  default_model: grok-2
```

Roundtable connects to X.AI's API endpoint with streaming.

## Integrations

### Hermes (Session Lifecycle)

Roundtable emits Hermes events for debate outcomes:

```bash
# View debate events
curl localhost:5965/status

# Hermes automatically logs:
# - debate_started: when you create a new debate
# - consensus_reached: when all models agree
# - execution_complete: when Claude finishes implementing
```

Enable via:

```bash
python3 ~/Projects/scripts/hermes.py --server
# Roundtable will auto-emit events to port 5965
```

### Pensive Memory

Debate insights are logged to session memory:

```bash
# After a debate concludes, Roundtable logs:
python3 ~/Projects/scripts/claude-session-memory.py log "roundtable" "..."
```

This feeds into future session briefings so Claude remembers debate patterns across sessions.

### Voice Control

If voice-daemon is running:

```bash
# Say "hey computer, start a roundtable debate"
# Or use voice to interject with /consensus or other commands
```

Integration is optional; roundtable works fine keyboard-only.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Roundtable TUI                       │
│  (Bubbletea + Lipgloss)                                 │
└────────────┬────────────────────────────────────────────┘
             │
     ┌───────┴────────┬──────────────┬──────────────┐
     │                │              │              │
┌────▼──────┐  ┌──────▼────┐  ┌─────▼────┐  ┌─────▼────┐
│  Claude   │  │ Gemini    │  │    GPT   │  │   Grok   │
│   CLI     │  │   CLI     │  │   API    │  │   API    │
└────┬──────┘  └──────┬────┘  └─────┬────┘  └─────┬────┘
     │                │              │              │
┌────▼───────────────▼──────────────▼──────────────▼────┐
│            Orchestrator                               │
│  - Parallel seed dispatch                             │
│  - Streaming response aggregation                     │
│  - Timeout/retry logic                                │
│  - Consensus protocol                                 │
└────┬─────────────────────────────────────────────────┘
     │
┌────▼──────────────────────────────┐
│     SQLite Debate Storage          │
│  (~/.local/share/roundtable/)      │
│  - debates table                   │
│  - messages (unified stream)       │
│  - context_files                   │
│  - model_state                     │
└────────────────────────────────────┘
```

## FAQ

**Q: Will all models always agree?**

A: No. Different models have different training and optimization targets. Claude is trained to be helpful; GPT balances helpfulness with safety; Gemini has different data; Grok has different biases. Disagreement is common and valuable.

**Q: Can I execute code immediately if all models agree?**

A: No. Roundtable never auto-executes. Even after consensus, you must explicitly say `/execute` or confirm Claude's proposed implementation. The roundtable guides decision-making, but humans remain the executor.

**Q: What if I don't have all four models set up?**

A: Roundtable works with any combination. You can debate with just Claude and GPT, or just Claude and Gemini. Enable only what you have in config.

**Q: Can I load files into the debate?**

A: Yes. Use `/context add path/to/file.ts` to load a file into shared context. All models will see it when formulating responses. Great for architecture discussions where you want them all looking at the same code.

**Q: Can I pause the debate and interject?**

A: Yes. Type your message and press Ctrl+Enter. This adds your voice to the debate stream. Auto-debate pauses until you finish your interjection.

**Q: Does this work offline?**

A: Only if you use Claude/Gemini CLIs (which can run locally if you set them up). OpenAI and Grok APIs require internet. All debate data is stored locally in SQLite.

**Q: Can I export debate transcripts?**

A: Yes. Use `/export` to save the current debate as markdown. Useful for documentation or review.

**Q: What's the difference between consensus_timeout and model_timeout?**

A: `model_timeout` (60s default) is how long Roundtable waits for an individual model to respond. If Claude takes >60s, it times out. `consensus_timeout` (30s default) is how long the system waits for ALL models to respond before asking "any objections?" If one model is still responding after 30s, the consensus check runs anyway.

## Contributing

Issues and PRs welcome. Focus areas:

- Additional model backends (Anthropic models via API, Claude on local inference)
- Better consensus detection (detect true disagreement vs. alignment)
- Export formats (PDF, JSON, structured debate graphs)
- Voice command integration
- Better error recovery

## License

AGPL-3.0
