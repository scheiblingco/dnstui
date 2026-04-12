package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/scheiblingco/dnstui/internal/provider"
)

// ── GlobalSearch ──────────────────────────────────────────────────────────────

// GlobalSearch is a modal search overlay (opened with Ctrl+K from any screen)
// that lets the user filter and navigate to accounts and domains across all
// providers. The entry list is populated from the startup search cache.
type GlobalSearch struct {
	entries   []provider.SearchEntry
	input     textinput.Model
	cursor    int
	lastQuery string
}

// NewGlobalSearch creates the global search modal with pre-cached entries.
func NewGlobalSearch(entries []provider.SearchEntry) *GlobalSearch {
	ti := textinput.New()
	ti.Placeholder = "Search accounts and domains…"
	ti.CharLimit = 128
	ti.Width = 60
	ti.Focus()

	return &GlobalSearch{
		entries: entries,
		input:   ti,
	}
}

func (m *GlobalSearch) Init() tea.Cmd {
	return textinput.Blink
}

func (m *GlobalSearch) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+k":
			return m, func() tea.Msg { return PopMsg{} }

		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down":
			matches := m.filtered(strings.ToLower(strings.TrimSpace(m.input.Value())))
			if m.cursor < len(matches)-1 {
				m.cursor++
			}
			return m, nil

		case "enter":
			matches := m.filtered(strings.ToLower(strings.TrimSpace(m.input.Value())))
			if m.cursor >= 0 && m.cursor < len(matches) {
				return m, navigate(matches[m.cursor])
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)

	// Reset the cursor whenever the query changes.
	query := strings.ToLower(strings.TrimSpace(m.input.Value()))
	if query != m.lastQuery {
		m.cursor = 0
		m.lastQuery = query
	}

	return m, cmd
}

func (m *GlobalSearch) View() string {
	title := styleTitle.Render("Global Search")
	var sb strings.Builder
	sb.WriteString(title + "\n")
	sb.WriteString(m.input.View() + "\n\n")

	query := strings.ToLower(strings.TrimSpace(m.input.Value()))
	matches := m.filtered(query)

	switch {
	case len(m.entries) == 0:
		sb.WriteString(styleSubtitle.Render("Cache is loading, please try again in a moment.") + "\n")
	case query == "":
		sb.WriteString(styleSubtitle.Render(fmt.Sprintf("%d entries indexed. Type to filter.", len(m.entries))) + "\n\n")
		for i, e := range m.entries {
			if i == m.cursor {
				sb.WriteString(styleSelected.Render("▶ "+e.Label) + "\n")
			} else {
				sb.WriteString("  " + e.Label + "\n")
			}
		}
	case len(matches) == 0:
		sb.WriteString(styleSubtitle.Render("No matches.") + "\n")
	default:
		sb.WriteString(styleSubtitle.Render(fmt.Sprintf("%d match(es):", len(matches))) + "\n\n")
		for i, e := range matches {
			if i == m.cursor {
				sb.WriteString(styleSelected.Render("▶ "+e.Label) + "\n")
			} else {
				sb.WriteString("  " + e.Label + "\n")
			}
		}
	}

	sb.WriteString("\n" + styleHelp.Render("type to filter  ↑↓: navigate  enter: open  esc/ctrl+k: close"))
	return sb.String()
}

// filtered returns entries whose label contains the query string.
// When query is empty the full entry list is returned.
func (m *GlobalSearch) filtered(query string) []provider.SearchEntry {
	if query == "" {
		return m.entries
	}
	var out []provider.SearchEntry
	for _, e := range m.entries {
		if strings.Contains(strings.ToLower(e.Label), query) {
			out = append(out, e)
		}
	}
	return out
}

// navigate returns the tea.Cmd that pushes the appropriate view for the selected entry:
// account entries open the ZoneList; domain entries open the RecordList.
func navigate(e provider.SearchEntry) tea.Cmd {
	switch e.Kind {
	case provider.SearchEntryKindAccount:
		view := NewZoneList(e.Provider, []provider.Account{e.Account})
		return func() tea.Msg { return PushMsg{Model: view} }
	case provider.SearchEntryKindDomain:
		view := NewRecordList(e.Provider, e.Zone)
		return func() tea.Msg { return PushMsg{Model: view} }
	}
	return nil
}
