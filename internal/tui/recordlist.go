package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/scheiblingco/dnstui/internal/provider"
)

// ── RecordList ────────────────────────────────────────────────────────────────

var tableStyle = lipgloss.NewStyle().
	Border(lipgloss.NormalBorder()).
	BorderForeground(colorPrimary)

// RecordList shows the DNS records for a single zone.
type RecordList struct {
	prov    provider.Provider
	zone    provider.Zone
	records []provider.Record
	table   table.Model
	loading bool
	spinner spinner.Model
	filter  string
}

// NewRecordList creates the record-table view for the given zone.
func NewRecordList(p provider.Provider, z provider.Zone) *RecordList {
	cols := []table.Column{
		{Title: "Name", Width: 32},
		{Title: "Type", Width: 8},
		{Title: "TTL", Width: 8},
		{Title: "Value", Width: 40},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	ts := table.DefaultStyles()
	ts.Header = ts.Header.BorderStyle(lipgloss.NormalBorder()).BorderForeground(colorPrimary).Bold(true)
	ts.Selected = ts.Selected.Foreground(colorSelected).Bold(true)
	t.SetStyles(ts)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	return &RecordList{
		prov:    p,
		zone:    z,
		table:   t,
		spinner: sp,
	}
}

func (m *RecordList) Init() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, loadRecords(m.prov, m.zone.ID))
}

func (m *RecordList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.table.SetWidth(msg.Width - 4)
		m.table.SetHeight(msg.Height - 6)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return PopMsg{} }
		case "r":
			// Refresh.
			m.loading = true
			return m, tea.Batch(m.spinner.Tick, loadRecords(m.prov, m.zone.ID))
		case "n":
			// New record.
			form := NewRecordForm(m.prov, m.zone, provider.Record{}, false)
			return m, func() tea.Msg { return PushMsg{Model: form} }
		case "e":
			// Edit selected record.
			if len(m.records) == 0 {
				return m, nil
			}
			idx := m.table.Cursor()
			if idx < 0 || idx >= len(m.records) {
				return m, nil
			}
			form := NewRecordForm(m.prov, m.zone, m.records[idx], true)
			return m, func() tea.Msg { return PushMsg{Model: form} }
		case "d", "delete":
			// Delete selected record — push confirm dialog.
			if len(m.records) == 0 {
				return m, nil
			}
			idx := m.table.Cursor()
			if idx < 0 || idx >= len(m.records) {
				return m, nil
			}
			rec := m.records[idx]
			dlg := NewConfirmDialog(
				fmt.Sprintf("Delete %s record '%s'?", rec.Type, rec.Name),
				deleteRecord(m.prov, m.zone.ID, rec.ID),
			)
			return m, func() tea.Msg { return PushMsg{Model: dlg} }
		}

	case RecordsLoadedMsg:
		m.loading = false
		if msg.Err != nil {
			return m, func() tea.Msg { return ErrorMsg{Err: msg.Err} }
		}
		m.records = msg.Records
		m.table.SetRows(recordsToRows(msg.Records, m.filter))
		return m, nil

	case RecordSavedMsg:
		if msg.Err != nil {
			return m, func() tea.Msg { return ErrorMsg{Err: msg.Err} }
		}
		m.loading = true
		return m, tea.Batch(
			m.spinner.Tick,
			loadRecords(m.prov, m.zone.ID),
			func() tea.Msg { return StatusMsg{Text: "Record saved."} },
		)

	case RecordDeletedMsg:
		if msg.Err != nil {
			return m, func() tea.Msg { return ErrorMsg{Err: msg.Err} }
		}
		m.loading = true
		return m, tea.Batch(
			m.spinner.Tick,
			loadRecords(m.prov, m.zone.ID),
			func() tea.Msg { return StatusMsg{Text: "Record deleted."} },
		)

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *RecordList) View() string {
	title := styleTitle.Render(m.zone.Name + " — Records")

	if m.loading {
		return title + "\n" + m.spinner.View() + " Loading records…"
	}

	body := tableStyle.Render(m.table.View())
	help := styleHelp.Render("↑↓: navigate  n: new  e: edit  d: delete  r: refresh  /: search  esc: back")
	return title + "\n" + body + "\n" + help
}

// recordsToRows converts provider records to table rows, applying a filter.
func recordsToRows(records []provider.Record, filter string) []table.Row {
	rows := make([]table.Row, 0, len(records))
	for _, r := range records {
		if filter != "" {
			low := strings.ToLower
			if !strings.Contains(low(r.Name), low(filter)) &&
				!strings.Contains(low(string(r.Type)), low(filter)) &&
				!strings.Contains(low(r.Value), low(filter)) {
				continue
			}
		}
		ttlStr := fmt.Sprintf("%d", r.TTL)
		if r.TTL == 0 {
			ttlStr = "auto"
		}
		rows = append(rows, table.Row{r.Name, string(r.Type), ttlStr, formatRecordValue(r)})
	}
	return rows
}

// formatRecordValue returns a human-readable summary of a record's value,
// showing individual sub-fields for complex types like SRV, CAA, TLSA, SSHFP, NAPTR.
func formatRecordValue(r provider.Record) string {
	intExtra := func(key string) int {
		if v, ok := r.Extra[key]; ok {
			switch vv := v.(type) {
			case int:
				return vv
			case float64:
				return int(vv)
			}
		}
		return 0
	}
	strExtra := func(key string) string {
		if v, ok := r.Extra[key].(string); ok {
			return v
		}
		return ""
	}

	switch r.Type {
	case provider.RecordTypeSRV:
		// "prio=10 w=0 port=443 target.example.com"
		return fmt.Sprintf("prio=%d w=%d port=%d %s",
			r.Priority, intExtra("weight"), intExtra("port"), r.Value)
	case provider.RecordTypeCAA:
		// "0 issue letsencrypt.org"
		return fmt.Sprintf("%d %s %s",
			intExtra("caa_flags"), strExtra("caa_tag"), r.Value)
	case provider.RecordTypeTLSA:
		// "usage=3 sel=1 match=1 cert=abc123…"
		cert := r.Value
		if len(cert) > 16 {
			cert = cert[:16] + "…"
		}
		return fmt.Sprintf("usage=%d sel=%d match=%d %s",
			intExtra("tlsa_usage"), intExtra("tlsa_selector"), intExtra("tlsa_matching"), cert)
	case provider.RecordTypeSSHFP:
		// "alg=1 type=2 fp=abc123…"
		fp := r.Value
		if len(fp) > 20 {
			fp = fp[:20] + "…"
		}
		return fmt.Sprintf("alg=%d type=%d %s",
			intExtra("sshfp_algorithm"), intExtra("sshfp_fp_type"), fp)
	case provider.RecordTypeNAPTR:
		// "order=100 pref=10 S SIP+D2U replacement"
		return fmt.Sprintf("order=%d pref=%d %q %s %s",
			r.Priority, intExtra("naptr_pref"),
			strExtra("naptr_flags"), strExtra("naptr_service"), r.Value)
	case provider.RecordTypeMX:
		return fmt.Sprintf("[%d] %s", r.Priority, r.Value)
	default:
		return r.Value
	}
}

func loadRecords(p provider.Provider, zoneID string) tea.Cmd {
	return func() tea.Msg {
		records, err := p.ListRecords(context.Background(), zoneID)
		return RecordsLoadedMsg{ZoneID: zoneID, Records: records, Err: err}
	}
}

func deleteRecord(p provider.Provider, zoneID, recordID string) tea.Cmd {
	return func() tea.Msg {
		err := p.DeleteRecord(context.Background(), zoneID, recordID)
		return RecordDeletedMsg{Err: err}
	}
}
