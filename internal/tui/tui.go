// Package tui implements the full terminal UI for dnstui using Bubble Tea.
//
// Navigation follows a model-stack pattern: each view pushes onto the stack
// when the user drills down, and pops when they press Esc. The root model
// dispatches messages and rendering to whichever view is on top.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/scheiblingco/dnstui/internal/provider"
)

// ── Shared colours / styles ──────────────────────────────────────────────────

var (
	colorPrimary  = lipgloss.Color("63")  // medium slate blue
	colorMuted    = lipgloss.Color("240") // dim grey
	colorSuccess  = lipgloss.Color("10")  // bright green
	colorWarning  = lipgloss.Color("214") // orange
	colorDanger   = lipgloss.Color("196") // red
	colorSelected = lipgloss.Color("229") // pale yellow

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			MarginBottom(1)

	styleSubtitle = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(0, 1)

	styleHelp = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1)

	styleError = lipgloss.NewStyle().
			Foreground(colorDanger).
			Bold(true)

	styleSuccess = lipgloss.NewStyle().
			Foreground(colorSuccess)

	styleDanger = lipgloss.NewStyle().
			Foreground(colorDanger).
			Bold(true)

	styleSelected = lipgloss.NewStyle().
			Foreground(colorSelected).
			Bold(true)
)

// ── Messages ─────────────────────────────────────────────────────────────────

// PopMsg signals the current view should be removed and control returned to the
// view beneath it. FollowUp, if non-nil, is dispatched to the new top view
// immediately after the pop (used to deliver results to the parent view).
type PopMsg struct{ FollowUp tea.Cmd }

// PushMsg signals that a new view model should be pushed on top of the stack.
type PushMsg struct{ Model tea.Model }

// ErrorMsg carries an error to be displayed in the status bar.
type ErrorMsg struct{ Err error }

// StatusMsg carries a transient success message.
type StatusMsg struct{ Text string }

// AccountsLoadedMsg delivers ListAccounts results.
type AccountsLoadedMsg struct {
	ProviderIdx int
	Accounts    []provider.Account
	Err         error
}

// ZonesLoadedMsg delivers ListZones results.
type ZonesLoadedMsg struct {
	AccountID string
	Zones     []provider.Zone
	Err       error
}

// RecordsLoadedMsg delivers ListRecords results.
type RecordsLoadedMsg struct {
	ZoneID  string
	Records []provider.Record
	Err     error
}

// RecordSavedMsg signals a create or update completed.
type RecordSavedMsg struct {
	Record provider.Record
	Err    error
}

// RecordDeletedMsg signals a delete completed.
type RecordDeletedMsg struct{ Err error }

// ── Root model ────────────────────────────────────────────────────────────────

// Model is the root Bubble Tea model. It owns the view stack and a shared
// status/error line across the bottom.
type Model struct {
	stack      []tea.Model
	statusText string
	errorText  string
	width      int
	height     int
}

// New creates the root TUI model with the provider list as the initial view.
func New(providers []provider.Provider) Model {
	m := Model{}
	m.push(NewProviderList(providers))
	return m
}

func (m *Model) push(child tea.Model) {
	m.stack = append(m.stack, child)
}

func (m *Model) pop() {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
	}
}

func (m Model) top() tea.Model {
	return m.stack[len(m.stack)-1]
}

// Init initialises the top view.
func (m Model) Init() tea.Cmd {
	return m.top().Init()
}

// Update handles messages and delegates to the top view.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Propagate to top view with reduced height to leave room for status bar.
		inner := tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height - 2}
		updated, cmd := m.top().Update(inner)
		m.stack[len(m.stack)-1] = updated
		return m, cmd

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case PopMsg:
		m.errorText = ""
		m.statusText = ""
		m.pop()
		if msg.FollowUp != nil {
			return m, msg.FollowUp
		}
		return m, nil

	case PushMsg:
		m.push(msg.Model)
		initCmd := msg.Model.Init()
		// Immediately size the new view with the stored dimensions so it renders
		// correctly without waiting for the next terminal resize event.
		if m.width > 0 && m.height > 0 {
			sized, sizeCmd := msg.Model.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height - 2})
			m.stack[len(m.stack)-1] = sized
			return m, tea.Batch(initCmd, sizeCmd)
		}
		return m, initCmd

	case ErrorMsg:
		if msg.Err != nil {
			m.errorText = msg.Err.Error()
		} else {
			m.errorText = ""
		}
		m.statusText = ""
		return m, nil

	case StatusMsg:
		m.statusText = msg.Text
		m.errorText = ""
		return m, nil
	}

	// Delegate to the top view.
	updated, cmd := m.top().Update(msg)
	m.stack[len(m.stack)-1] = updated
	return m, cmd
}

// View renders the top view plus the shared status bar.
func (m Model) View() string {
	content := m.top().View()

	statusLine := ""
	if m.errorText != "" {
		statusLine = styleError.Render("✖ " + m.errorText)
	} else if m.statusText != "" {
		statusLine = styleSuccess.Render("✔ " + m.statusText)
	}

	if statusLine == "" {
		return content
	}
	return content + "\n" + statusLine
}

// Run starts the Bubble Tea program with the given providers.
func Run(providers []provider.Provider) error {
	m := New(providers)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// RunWithSearch starts the TUI with the GlobalSearch view as the first screen.
func RunWithSearch(providers []provider.Provider) error {
	m := Model{}
	m.push(NewGlobalSearch(providers))
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
