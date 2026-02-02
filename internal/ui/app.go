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

func (m Model) renderHelp() string {
	help := `
ROUNDTABLE HELP

NAVIGATION
  Tab / Shift+Tab    Cycle focus between panes
  Alt+1-9            Switch to tab N
  Alt+[ / Alt+]      Previous / Next tab
  Esc                Return to input

ACTIONS
  Ctrl+Enter         Send message to all models
  Ctrl+Space         Pause / Resume auto-debate
  Ctrl+E             Execute (after consensus)

TABS
  Alt+N              New debate
  Alt+W              Close current tab

COMMANDS
  /help              Show this help
  /new [name]        New debate
  /close             Close debate
  /context add PATH  Load file into context
  /context list      List context files
  /models            Toggle model picker
  /consensus         Force consensus check
  /execute           Execute agreed approach
  /pause             Pause auto-debate
  /resume            Resume auto-debate
  /history           Browse past debates
  /export            Export to markdown

Press F1 or ? to toggle this help
`
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Cyan).
			Padding(1, 2).
			Render(help))
}
