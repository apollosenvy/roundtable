// internal/ui/app.go
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"

	"roundtable/internal/commands"
	"roundtable/internal/config"
	"roundtable/internal/consensus"
	ctxloader "roundtable/internal/context"
	"roundtable/internal/db"
	"roundtable/internal/hermes"
	"roundtable/internal/memory"
	"roundtable/internal/models"
	"roundtable/internal/orchestrator"
	"roundtable/internal/pensive"
)

// Package-level program reference for async message sending
var program *tea.Program

// SetProgram sets the tea.Program reference for async operations
func SetProgram(p *tea.Program) {
	program = p
}

// Program returns the tea.Program reference
func Program() *tea.Program {
	return program
}

// Message types for async model responses
type modelResponseMsg struct {
	modelID   string
	content   string
	done      bool
	err       error
	isTimeout bool // True if error was due to timeout
}

type allModelsDoneMsg struct{}

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

	// Orchestrator
	orchestrator *orchestrator.Orchestrator
	cancelDebate context.CancelFunc

	// Streaming state - tracks partial messages being built
	// map[modelID]messageIndex - which message in debate.Messages is being streamed to
	streamingMsgs map[string]int

	// View mode state (normal, history browser, etc.)
	viewMode     ViewMode
	historyState *HistoryState

	// Hermes event client
	hermes *hermes.Client

	// Pensive memory bridge
	pensive *pensive.Bridge

	// Session memory client for learning tracking
	memory *memory.Client
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

	// Create orchestrator with timeout and retry settings from config
	timeout := time.Duration(cfg.Defaults.ModelTimeout) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	retryAttempts := cfg.Defaults.RetryAttempts
	if retryAttempts == 0 {
		retryAttempts = 3
	}
	retryDelay := time.Duration(cfg.Defaults.RetryDelay) * time.Millisecond
	if retryDelay == 0 {
		retryDelay = time.Second
	}
	orch := orchestrator.NewWithRetry(registry, timeout, retryAttempts, retryDelay)

	// Create Hermes client for event tracking
	hermesClient := hermes.NewClient()

	// Create Pensive memory bridge
	pensiveBridge := pensive.NewBridge()

	// Create session memory client for learning tracking
	memoryClient := memory.NewClient()

	// Text input
	ta := textarea.New()
	ta.Placeholder = "Enter your prompt... (Ctrl+Enter to send)"
	ta.Focus()
	ta.CharLimit = 8192
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	// Load existing debates from database or create initial debate
	var debates []*Debate
	if store != nil {
		debates = loadDebatesFromStore(store)
	}

	// If no debates loaded, create a new one
	if len(debates) == 0 {
		debateID := uuid.New().String()[:8]
		firstDebate := NewDebate(debateID, "New Debate")
		debates = []*Debate{firstDebate}

		// Persist the new debate to database
		if store != nil {
			store.CreateDebate(debateID, "New Debate", "")
		}
	}

	return Model{
		config:        cfg,
		store:         store,
		registry:      registry,
		orchestrator:  orch,
		input:         ta,
		debates:       debates,
		activeTab:     0,
		focus:         FocusInput,
		streamingMsgs: make(map[string]int),
		viewMode:      ViewNormal,
		historyState:  NewHistoryState(),
		hermes:        hermesClient,
		pensive:       pensiveBridge,
		memory:        memoryClient,
	}
}

// loadDebatesFromStore loads existing debates and their messages from the database
func loadDebatesFromStore(store *db.Store) []*Debate {
	dbDebates, err := store.ListDebates()
	if err != nil {
		return nil
	}

	var debates []*Debate
	for _, dbDebate := range dbDebates {
		// Only load active debates
		if dbDebate.Status != "active" {
			continue
		}

		debate := NewDebate(dbDebate.ID, dbDebate.Name)
		debate.ProjectPath = dbDebate.ProjectPath

		// Load messages for this debate
		messages, err := store.GetMessages(dbDebate.ID)
		if err == nil {
			for _, msg := range messages {
				debate.Messages = append(debate.Messages, DebateMessage{
					Source:    msg.Source,
					Content:   msg.Content,
					Timestamp: msg.CreatedAt,
				})
			}
		}

		// Load context files for this debate
		contextFiles, err := store.GetContextFiles(dbDebate.ID)
		if err == nil {
			for _, cf := range contextFiles {
				debate.ContextFiles[cf.Path] = cf.Content
			}
		}

		debates = append(debates, debate)
	}

	return debates
}

// saveMessage persists a message to the database
func (m *Model) saveMessage(debateID, source, content, msgType string) {
	if m.store != nil {
		m.store.AddMessage(debateID, source, content, msgType)
	}
}

// saveContextFile persists a context file to the database
func (m *Model) saveContextFile(debateID, path, content string) {
	if m.store != nil {
		m.store.AddContextFile(debateID, path, content)
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

	// Handle history view mode separately
	if m.viewMode == ViewHistory {
		return m.updateHistoryView(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			// Cancel any ongoing model requests before quitting
			if m.cancelDebate != nil {
				m.cancelDebate()
			}
			return m, tea.Quit

		case "alt+h":
			// Open history browser
			m.viewMode = ViewHistory
			if m.historyState == nil {
				m.historyState = NewHistoryState()
			}
			m.historyState.SetMaxHeight(m.height)
			m.historyState.LoadDebates(m.store)
			return m, nil

		case "ctrl+enter":
			// Send message or execute command
			input := strings.TrimSpace(m.input.Value())
			if input == "" {
				return m, nil
			}

			// Check if this is a slash command
			if cmd := commands.Parse(input); cmd != nil {
				m.input.Reset()
				return m.handleCommand(cmd)
			}

			// Not a command - send as prompt to models
			if m.activeDebate() != nil {
				debate := m.activeDebate()
				debate.AddMessage("user", input)

				// Persist user message to database
				m.saveMessage(debate.ID, "user", input, "user")

				m.input.Reset()
				// Clear streaming state for new round
				m.streamingMsgs = make(map[string]int)
				m.updateChatView()
				// Dispatch to all models in parallel
				return m, m.dispatchToModels(input)
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
		case "alt+4":
			m.switchTab(3)
		case "alt+5":
			m.switchTab(4)
		case "alt+6":
			m.switchTab(5)
		case "alt+7":
			m.switchTab(6)
		case "alt+8":
			m.switchTab(7)
		case "alt+9":
			m.switchTab(8)
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

	case modelResponseMsg:
		debate := m.activeDebate()
		if debate == nil {
			return m, nil
		}

		if msg.err != nil {
			// Add error message with proper error styling
			errContent := msg.err.Error()

			// Set model status based on error type
			if msg.isTimeout {
				debate.UpdateModelStatus(msg.modelID, models.StatusTimeout)
				debate.AddErrorMessage(msg.modelID, errContent, true)
			} else {
				debate.UpdateModelStatus(msg.modelID, models.StatusError)
				debate.AddErrorMessage(msg.modelID, errContent, false)
			}

			// Persist error to database
			m.saveMessage(debate.ID, msg.modelID, "[ERROR] "+errContent, "system")
		} else if msg.content != "" {
			// Check if we're streaming to an existing message or starting a new one
			if idx, ok := m.streamingMsgs[msg.modelID]; ok && idx < len(debate.Messages) {
				// Append to existing streaming message
				debate.Messages[idx].Content += msg.content
			} else {
				// Start a new message
				debate.AddMessage(msg.modelID, msg.content)
				m.streamingMsgs[msg.modelID] = len(debate.Messages) - 1
			}
			debate.ModelStatus[msg.modelID] = models.StatusResponding
		}

		if msg.done {
			// Finalize the message - save complete content to database
			if idx, ok := m.streamingMsgs[msg.modelID]; ok && idx < len(debate.Messages) {
				finalContent := debate.Messages[idx].Content
				m.saveMessage(debate.ID, msg.modelID, finalContent, "model")
				delete(m.streamingMsgs, msg.modelID)
			}
			debate.ModelStatus[msg.modelID] = models.StatusIdle
		}

		m.updateChatView()
		return m, nil

	case allModelsDoneMsg:
		debate := m.activeDebate()
		if debate != nil {
			// Check for consensus among model responses
			consensusResult := m.checkDebateConsensus(debate)

			var systemMsg string
			if consensusResult.HasConsensus {
				systemMsg = fmt.Sprintf("CONSENSUS REACHED: %d models agree (no objections). Ready for execution.", consensusResult.AgreeCount)

				// Build consensus description for storage
				consensusText := fmt.Sprintf("Agreement target: %s", consensusResult.AgreementTarget)
				if len(consensusResult.Additions) > 0 {
					consensusText += fmt.Sprintf(" with %d additions", len(consensusResult.Additions))
				}

				// Emit Hermes consensus event
				if m.hermes != nil {
					m.hermes.ConsensusReached(debate.ID, consensusText)
				}

				// Update debate status in database
				if m.store != nil {
					m.store.UpdateDebateStatus(debate.ID, "resolved", consensusText)
				}

				// Store debate to Pensive for future reference
				m.storeDebateToPensive(debate, consensusText)

				// Log learnings to session memory for future Claude sessions
				m.logDebateLearnings(debate)
			} else {
				systemMsg = "All models have responded. Any objections or additions?"
				if consensusResult.ObjectCount > 0 {
					systemMsg = fmt.Sprintf("All models have responded. %d objection(s) raised - consensus not reached.", consensusResult.ObjectCount)

					// Log failed approaches even without full consensus
					m.logDebateLearnings(debate)
				}
			}

			debate.AddMessage("system", systemMsg)
			m.saveMessage(debate.ID, "system", systemMsg, "system")
			m.updateChatView()
		}
		return m, nil
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
	debateName := fmt.Sprintf("Debate %d", len(m.debates)+1)
	debate := NewDebate(debateID, debateName)
	m.debates = append(m.debates, debate)
	m.activeTab = len(m.debates) - 1

	// Persist new debate to database
	if m.store != nil {
		m.store.CreateDebate(debateID, debateName, "")
	}

	// Emit Hermes event for debate started
	if m.hermes != nil {
		m.hermes.DebateStarted(debateID, debateName, m.registry.Count())
	}

	m.updateChatView()
}

// createTabWithContext creates a new debate tab and queries Pensive for relevant context
func (m *Model) createTabWithContext(topic string) {
	debateID := uuid.New().String()[:8]
	debateName := fmt.Sprintf("Debate %d", len(m.debates)+1)
	debate := NewDebate(debateID, debateName)

	// Query Pensive for relevant past debates
	if pensiveContext := m.queryPensiveContext(topic); pensiveContext != "" {
		// Add context as a system message at the start of the debate
		debate.AddMessage("system", fmt.Sprintf("Relevant context from past debates:\n%s", pensiveContext))
	}

	m.debates = append(m.debates, debate)
	m.activeTab = len(m.debates) - 1

	// Persist new debate to database
	if m.store != nil {
		m.store.CreateDebate(debateID, debateName, "")
		// Also persist the context message if any
		if len(debate.Messages) > 0 {
			m.store.AddMessage(debateID, "system", debate.Messages[0].Content, "system")
		}
	}

	// Emit Hermes event for debate started
	if m.hermes != nil {
		m.hermes.DebateStarted(debateID, debateName, m.registry.Count())
	}

	m.updateChatView()
}

func (m *Model) closeTab(idx int) {
	if idx < 0 || idx >= len(m.debates) || len(m.debates) <= 1 {
		return
	}

	// Mark debate as abandoned in database before removing from memory
	closedDebate := m.debates[idx]
	if m.store != nil && closedDebate != nil {
		m.store.UpdateDebateStatus(closedDebate.ID, "abandoned", "")

		// Store abandoned debates to Pensive too - we can learn from incomplete discussions
		// Only if there was meaningful discussion (more than just the initial prompt)
		if len(closedDebate.Messages) > 2 {
			m.storeDebateToPensive(closedDebate, "")
		}
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

	if m.viewMode == ViewHistory && m.historyState != nil {
		return m.historyState.Render(m.width, m.height)
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
	left := TitleStyle.Render("ROUNDTABLE")

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

	bar := strings.Join(tabs, DimStyle.Render("|"))
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
			content.WriteString(DimStyle.Render("* " + path))
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
		mstyle := ModelStyle(info.ID)

		name := info.Name
		if status == models.StatusResponding {
			name += "..."
		}

		content.WriteString(fmt.Sprintf("%s %s\n", indicator, mstyle.Render(name)))
	}

	return style.Width(15).Height(m.height - 10).Render(content.String())
}

func (m Model) renderStatusBar() string {
	debate := m.activeDebate()

	// Debate status
	status := StatusOK.Render("* READY")
	if debate != nil && debate.Paused {
		status = StatusWarn.Render("* PAUSED")
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

	separator := DimStyle.Render(strings.Repeat("-", m.width))
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

// renderHelp is defined in help.go

// updateHistoryView handles input when in history view mode
func (m Model) updateHistoryView(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			return m, tea.Quit

		case "esc", "q":
			// Close history view
			m.viewMode = ViewNormal
			return m, nil

		case "up", "k":
			if m.historyState != nil {
				m.historyState.Up()
			}
			return m, nil

		case "down", "j":
			if m.historyState != nil {
				m.historyState.Down()
			}
			return m, nil

		case "enter":
			// Resume the selected debate
			if m.historyState != nil {
				selected := m.historyState.Selected()
				if selected != nil {
					debate, err := ResumeDebate(m.store, selected.ID)
					if err == nil && debate != nil {
						// Check if this debate is already open
						for i, d := range m.debates {
							if d.ID == debate.ID {
								// Switch to existing tab
								m.activeTab = i
								m.viewMode = ViewNormal
								m.updateChatView()
								return m, nil
							}
						}

						// Add as new tab
						m.debates = append(m.debates, debate)
						m.activeTab = len(m.debates) - 1
						m.updateChatView()
					}
				}
			}
			m.viewMode = ViewNormal
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.historyState != nil {
			m.historyState.SetMaxHeight(m.height)
		}
	}

	return m, nil
}

// dispatchToModels sends the prompt to all enabled models in parallel
// It creates a goroutine that reads from the orchestrator's response channel
// and forwards messages to the tea.Program via Send()
func (m *Model) dispatchToModels(prompt string) tea.Cmd {
	return func() tea.Msg {
		debate := m.activeDebate()
		if debate == nil || m.orchestrator == nil {
			return nil
		}

		// Create cancellable context
		ctx, cancel := context.WithCancel(context.Background())
		m.cancelDebate = cancel

		// On first message in debate, query Pensive for relevant context
		// Count non-system, non-user messages to determine if this is the first prompt
		modelMsgCount := 0
		for _, msg := range debate.Messages {
			if msg.Source != "system" && msg.Source != "user" {
				modelMsgCount++
			}
		}

		// If this is the first prompt (no model responses yet), inject Pensive context
		if modelMsgCount == 0 && m.pensive != nil {
			if pensiveContext := m.queryPensiveContext(prompt); pensiveContext != "" {
				// Add context as a system message before the models respond
				contextMsg := fmt.Sprintf("Context from relevant past debates:\n%s", pensiveContext)
				debate.AddMessage("system", contextMsg)
				m.saveMessage(debate.ID, "system", contextMsg, "system")
				m.updateChatView()
			}
		}

		// Convert debate messages to model messages format
		var history []models.Message
		for _, msg := range debate.Messages {
			history = append(history, models.Message{
				Source:    msg.Source,
				Content:   msg.Content,
				Timestamp: msg.Timestamp,
			})
		}

		// Start parallel model requests
		responses := m.orchestrator.ParallelSeed(ctx, history, prompt)

		// Forward responses to the UI as tea messages
		go func() {
			for resp := range responses {
				if program != nil {
					program.Send(modelResponseMsg{
						modelID:   resp.ModelID,
						content:   resp.Content,
						done:      resp.Done,
						err:       resp.Error,
						isTimeout: resp.IsTimeout,
					})
				}
			}
			// Signal that all models are done
			if program != nil {
				program.Send(allModelsDoneMsg{})
			}
		}()

		return nil
	}
}

// checkDebateConsensus analyzes the most recent round of model responses
// and returns consensus analysis results
func (m *Model) checkDebateConsensus(debate *Debate) consensus.ConsensusResult {
	if debate == nil || len(debate.Messages) == 0 {
		return consensus.ConsensusResult{}
	}

	// Find the most recent user message and collect model responses after it
	positions := make(map[string]consensus.ParsedPosition)

	// Iterate backwards to find the last user message
	lastUserIdx := -1
	for i := len(debate.Messages) - 1; i >= 0; i-- {
		if debate.Messages[i].Source == "user" {
			lastUserIdx = i
			break
		}
	}

	if lastUserIdx == -1 {
		return consensus.ConsensusResult{}
	}

	// Collect model responses after the last user message
	for i := lastUserIdx + 1; i < len(debate.Messages); i++ {
		msg := debate.Messages[i]
		// Skip system messages and user messages
		if msg.Source == "system" || msg.Source == "user" {
			continue
		}
		// This is a model response
		parsed := consensus.ParseResponse(msg.Content)
		positions[msg.Source] = parsed
	}

	return consensus.AnalyzeConsensus(positions)
}

// EmitExecutionComplete emits a Hermes event when execution is complete.
// This should be called after Claude (or other executor) completes a task.
func (m *Model) EmitExecutionComplete(debateID string, success bool, result string) {
	if m.hermes != nil {
		m.hermes.ExecutionComplete(debateID, success, result)
	}

	// Also track execution outcome in Pensive for learning
	if m.pensive != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.pensive.UpdateExecutionOutcome(ctx, debateID, success, result)
	}
}

// storeDebateToPensive stores a completed debate to Pensive memory
func (m *Model) storeDebateToPensive(debate *Debate, consensusText string) {
	if m.pensive == nil || m.store == nil || debate == nil {
		return
	}

	// Get the debate from the store for full metadata
	dbDebate, err := m.store.GetDebate(debate.ID)
	if err != nil {
		return
	}

	// Get all messages
	messages, err := m.store.GetMessages(debate.ID)
	if err != nil {
		return
	}

	// Update consensus if provided
	if consensusText != "" {
		dbDebate.Consensus = consensusText
	}

	// Store asynchronously to not block the UI
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := m.pensive.StoreDebate(ctx, dbDebate, messages); err != nil {
			// Silent failure - this is fire-and-forget
			// The file fallback will catch it
		}
	}()
}

// queryPensiveContext queries Pensive for relevant past debates and formats them as context
func (m *Model) queryPensiveContext(topic string) string {
	if m.pensive == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	debates, err := m.pensive.QueryRelevantDebates(ctx, topic, 3)
	if err != nil || len(debates) == 0 {
		return ""
	}

	return m.pensive.FormatContextForDebate(debates)
}

// handleCommand processes a parsed slash command and returns the updated model
func (m Model) handleCommand(cmd commands.Command) (tea.Model, tea.Cmd) {
	debate := m.activeDebate()

	switch c := cmd.(type) {
	case commands.Help:
		m.showHelp = true
		return m, nil

	case commands.NewDebate:
		name := c.Name
		if name == "" {
			name = fmt.Sprintf("Debate %d", len(m.debates)+1)
		}
		debateID := uuid.New().String()[:8]
		newDebate := NewDebate(debateID, name)
		m.debates = append(m.debates, newDebate)
		m.activeTab = len(m.debates) - 1

		if m.store != nil {
			m.store.CreateDebate(debateID, name, "")
		}
		if m.hermes != nil {
			m.hermes.DebateStarted(debateID, name, m.registry.Count())
		}
		m.updateChatView()
		return m, nil

	case commands.CloseDebate:
		m.closeTab(m.activeTab)
		return m, nil

	case commands.RenameDebate:
		if debate != nil && c.Name != "" {
			debate.Name = c.Name
			if m.store != nil {
				m.store.UpdateDebateName(debate.ID, c.Name)
			}
		}
		return m, nil

	case commands.AddContext:
		if debate == nil {
			return m, nil
		}
		// Load the file/directory content
		content, err := ctxloader.LoadContext(c.Path)
		if err != nil {
			debate.AddMessage("system", fmt.Sprintf("Failed to load context: %v", err))
		} else {
			debate.ContextFiles[c.Path] = content
			m.saveContextFile(debate.ID, c.Path, content)
			debate.AddMessage("system", fmt.Sprintf("Added context: %s", c.Path))
		}
		m.updateChatView()
		return m, nil

	case commands.RemoveContext:
		if debate != nil {
			delete(debate.ContextFiles, c.Path)
			if m.store != nil {
				m.store.RemoveContextFile(debate.ID, c.Path)
			}
			debate.AddMessage("system", fmt.Sprintf("Removed context: %s", c.Path))
			m.updateChatView()
		}
		return m, nil

	case commands.ListContext:
		if debate != nil {
			var files []string
			for path := range debate.ContextFiles {
				files = append(files, path)
			}
			if len(files) == 0 {
				debate.AddMessage("system", "No context files loaded")
			} else {
				debate.AddMessage("system", "Context files:\n- "+strings.Join(files, "\n- "))
			}
			m.updateChatView()
		}
		return m, nil

	case commands.ToggleModels:
		m.focus = FocusModels
		return m, nil

	case commands.ForceConsensus:
		if debate != nil {
			// Dispatch consensus check to all models
			m.streamingMsgs = make(map[string]int)
			return m, m.dispatchConsensusCheck()
		}
		return m, nil

	case commands.Execute:
		if debate == nil {
			return m, nil
		}
		// Check for consensus before allowing execution
		consensusResult := m.checkDebateConsensus(debate)
		if !consensusResult.HasConsensus {
			debate.AddMessage("system", "Cannot execute: consensus not reached. Use /consensus to check positions.")
			m.updateChatView()
			return m, nil
		}
		// Send execution request to Claude (the only executor)
		debate.AddMessage("system", "Execution requested. Sending to Claude for implementation...")
		m.updateChatView()
		return m, m.dispatchExecutionToClaude()

	case commands.Pause:
		if debate != nil {
			debate.Paused = true
			debate.AddMessage("system", "Debate paused. Use /resume to continue.")
			m.updateChatView()
		}
		return m, nil

	case commands.Resume:
		if debate != nil {
			debate.Paused = false
			debate.AddMessage("system", "Debate resumed.")
			m.updateChatView()
		}
		return m, nil

	case commands.ShowHistory:
		m.viewMode = ViewHistory
		if m.historyState == nil {
			m.historyState = NewHistoryState()
		}
		m.historyState.SetMaxHeight(m.height)
		m.historyState.LoadDebates(m.store)
		return m, nil

	case commands.Export:
		if debate != nil {
			// Export is handled elsewhere - just notify
			debate.AddMessage("system", "Export functionality: use /export to save debate to markdown (feature pending full implementation)")
			m.updateChatView()
		}
		return m, nil

	case commands.ParseError:
		if debate != nil {
			debate.AddMessage("system", fmt.Sprintf("Command error: %s\n\n%s", c.Message, commands.HelpText()))
			m.updateChatView()
		}
		return m, nil
	}

	return m, nil
}

// dispatchConsensusCheck sends the consensus prompt to all models
func (m *Model) dispatchConsensusCheck() tea.Cmd {
	return func() tea.Msg {
		debate := m.activeDebate()
		if debate == nil || m.orchestrator == nil {
			return nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		m.cancelDebate = cancel

		// Convert debate messages to model messages format
		var history []models.Message
		for _, msg := range debate.Messages {
			history = append(history, models.Message{
				Source:    msg.Source,
				Content:   msg.Content,
				Timestamp: msg.Timestamp,
			})
		}

		// Use orchestrator's consensus prompt
		responses := m.orchestrator.ConsensusPrompt(ctx, history)

		// Forward responses to the UI
		go func() {
			for resp := range responses {
				if program != nil {
					program.Send(modelResponseMsg{
						modelID:   resp.ModelID,
						content:   resp.Content,
						done:      resp.Done,
						err:       resp.Error,
						isTimeout: resp.IsTimeout,
					})
				}
			}
			if program != nil {
				program.Send(allModelsDoneMsg{})
			}
		}()

		return nil
	}
}

// dispatchExecutionToClaude sends the execution request to Claude only
func (m *Model) dispatchExecutionToClaude() tea.Cmd {
	return func() tea.Msg {
		debate := m.activeDebate()
		if debate == nil || m.orchestrator == nil {
			return nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		m.cancelDebate = cancel

		// Build execution prompt with context from debate
		executionPrompt := `Based on the consensus reached in this debate, please implement the agreed-upon approach.

You have execution capabilities. The other models provided advisory input, but you are the executor.

Summarize what you're about to do, then proceed with implementation. If you need user confirmation for destructive operations, ask first.`

		// Convert debate messages to model messages format
		var history []models.Message
		for _, msg := range debate.Messages {
			history = append(history, models.Message{
				Source:    msg.Source,
				Content:   msg.Content,
				Timestamp: msg.Timestamp,
			})
		}

		// Send only to Claude
		responses := m.orchestrator.SendToModel(ctx, "claude", history, executionPrompt)

		// Forward responses to the UI
		go func() {
			for resp := range responses {
				if program != nil {
					program.Send(modelResponseMsg{
						modelID:   resp.ModelID,
						content:   resp.Content,
						done:      resp.Done,
						err:       resp.Error,
						isTimeout: resp.IsTimeout,
					})
				}
			}
			if program != nil {
				program.Send(allModelsDoneMsg{})
			}
		}()

		return nil
	}
}

// logDebateLearnings extracts and logs learnings from a completed debate
func (m *Model) logDebateLearnings(debate *Debate) {
	if m.memory == nil || debate == nil {
		return
	}

	// Convert debate messages to memory format
	var memoryMessages []memory.DebateMessage
	for _, msg := range debate.Messages {
		memoryMessages = append(memoryMessages, memory.DebateMessage{
			Source:    msg.Source,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		})
	}

	// Extract learnings asynchronously
	go func() {
		learnings := m.memory.ExtractLearnings(debate.ID, debate.Name, memoryMessages)
		m.memory.LogLearnings(learnings)
	}()
}
