package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/scheiblingco/dnstui/internal/provider"
)

// ── GlobalSearch ──────────────────────────────────────────────────────────────

// searchResult is a matched DNS record with context about which provider/zone it lives in.
type searchResult struct {
	providerName string
	zoneName     string
	record       provider.Record
}

func (r searchResult) display() string {
	ttl := fmt.Sprintf("%d", r.record.TTL)
	if r.record.TTL == 0 {
		ttl = "auto"
	}
	return fmt.Sprintf("%-14s %-30s %-8s ttl:%-6s  %s  [%s]",
		r.providerName,
		r.record.Name,
		string(r.record.Type),
		ttl,
		r.record.Value,
		r.zoneName,
	)
}

// allZonesLoadedMsg carries the full cross-provider zone map after initial load.
type allZonesLoadedMsg struct {
	zones []provider.Zone
	prMap map[string]provider.Provider // zone.ID → provider
	err   error
}

// allRecordsLoadedMsg carries all records from a zone.
type allRecordsLoadedMsg struct {
	providerName string
	zoneName     string
	records      []provider.Record
	err          error
}

// GlobalSearch lets the user search across all records in all zones.
type GlobalSearch struct {
	providers []provider.Provider
	input     textinput.Model
	spinner   spinner.Model
	loading   bool
	loadMsg   string
	results   []searchResult
	cursor    int
	provMap   map[string]provider.Provider
}

// NewGlobalSearch creates the global search view.
func NewGlobalSearch(providers []provider.Provider) *GlobalSearch {
	ti := textinput.New()
	ti.Placeholder = "Search records across all zones…"
	ti.CharLimit = 128
	ti.Width = 60
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	return &GlobalSearch{
		providers: providers,
		input:     ti,
		spinner:   sp,
		provMap:   map[string]provider.Provider{},
	}
}

func (m *GlobalSearch) Init() tea.Cmd {
	m.loading = true
	m.loadMsg = "Loading zones…"
	return tea.Batch(m.spinner.Tick, loadAllZones(m.providers))
}

func (m *GlobalSearch) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return PopMsg{} }
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down":
			if m.cursor < len(m.results)-1 {
				m.cursor++
			}
			return m, nil
		}

	case allZonesLoadedMsg:
		if msg.err != nil {
			m.loading = false
			return m, func() tea.Msg { return ErrorMsg{Err: msg.err} }
		}
		m.provMap = msg.prMap
		m.loadMsg = fmt.Sprintf("Loading records from %d zones…", len(msg.zones))
		cmds := make([]tea.Cmd, len(msg.zones))
		for i, z := range msg.zones {
			cmds[i] = loadZoneRecords(msg.prMap[z.ID], z)
		}
		return m, tea.Batch(cmds...)

	case allRecordsLoadedMsg:
		if msg.err == nil {
			for _, r := range msg.records {
				m.results = append(m.results, searchResult{
					providerName: msg.providerName,
					zoneName:     msg.zoneName,
					record:       r,
				})
			}
		}
		// loading finishes naturally — all zone cmds fire in parallel
		m.loading = false

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *GlobalSearch) View() string {
	title := styleTitle.Render("Global Search")
	var sb strings.Builder
	sb.WriteString(title + "\n")
	sb.WriteString(m.input.View() + "\n\n")

	if m.loading {
		sb.WriteString(m.spinner.View() + " " + m.loadMsg + "\n")
		return sb.String()
	}

	query := strings.ToLower(strings.TrimSpace(m.input.Value()))
	matches := m.filtered(query)

	if query == "" {
		sb.WriteString(styleSubtitle.Render(fmt.Sprintf("%d total records indexed. Type to search.", len(m.results))) + "\n")
	} else if len(matches) == 0 {
		sb.WriteString(styleSubtitle.Render("No matches.") + "\n")
	} else {
		sb.WriteString(styleSubtitle.Render(fmt.Sprintf("%d match(es):", len(matches))) + "\n\n")
		for i, r := range matches {
			line := r.display()
			if i == m.cursor {
				sb.WriteString(styleSelected.Render("▶ "+line) + "\n")
			} else {
				sb.WriteString("  " + line + "\n")
			}
		}
	}

	sb.WriteString("\n" + styleHelp.Render("type to search  ↑↓: navigate  esc: back"))
	return sb.String()
}

// filtered returns results matching the query string.
func (m *GlobalSearch) filtered(query string) []searchResult {
	if query == "" {
		return nil
	}
	var out []searchResult
	for _, r := range m.results {
		if strings.Contains(strings.ToLower(r.record.Name), query) ||
			strings.Contains(strings.ToLower(r.record.Value), query) ||
			strings.Contains(strings.ToLower(string(r.record.Type)), query) ||
			strings.Contains(strings.ToLower(r.zoneName), query) ||
			strings.Contains(strings.ToLower(r.providerName), query) {
			out = append(out, r)
		}
	}
	return out
}

// loadAllZones fetches zones from all providers in parallel and aggregates results.
func loadAllZones(providers []provider.Provider) tea.Cmd {
	return func() tea.Msg {
		var allZones []provider.Zone
		prMap := map[string]provider.Provider{}

		for _, p := range providers {
			accounts, err := p.ListAccounts(context.Background())
			if err != nil {
				return allZonesLoadedMsg{err: err}
			}
			for _, acc := range accounts {
				zones, err := p.ListZones(context.Background(), acc.ID)
				if err != nil {
					return allZonesLoadedMsg{err: err}
				}
				for _, z := range zones {
					allZones = append(allZones, z)
					prMap[z.ID] = p
				}
			}
		}
		return allZonesLoadedMsg{zones: allZones, prMap: prMap}
	}
}

// loadZoneRecords fetches all records from a single zone.
func loadZoneRecords(p provider.Provider, z provider.Zone) tea.Cmd {
	return func() tea.Msg {
		records, err := p.ListRecords(context.Background(), z.ID)
		return allRecordsLoadedMsg{
			providerName: p.FriendlyName(),
			zoneName:     z.Name,
			records:      records,
			err:          err,
		}
	}
}
