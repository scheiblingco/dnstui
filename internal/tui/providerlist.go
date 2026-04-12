package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/scheiblingco/dnstui/internal/provider"
)

// ── ProviderList ─────────────────────────────────────────────────────────────

// providerItem wraps a provider.Provider for the bubbles/list component.
type providerItem struct {
	p provider.Provider
}

func (i providerItem) Title() string       { return i.p.FriendlyName() }
func (i providerItem) Description() string { return i.p.ProviderName() }
func (i providerItem) FilterValue() string { return i.p.FriendlyName() }

// ProviderList is the root view showing all configured provider accounts.
type ProviderList struct {
	providers []provider.Provider
	list      list.Model
	loading   bool
	spinner   spinner.Model
}

// NewProviderList creates the initial provider-selection view.
func NewProviderList(providers []provider.Provider) *ProviderList {
	items := make([]list.Item, len(providers))
	for i, p := range providers {
		items[i] = providerItem{p}
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "DNSTUI — Providers"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = styleTitle
	l.Styles.TitleBar = lipgloss.NewStyle()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	return &ProviderList{
		providers: providers,
		list:      l,
		spinner:   sp,
	}
}

func (m *ProviderList) Init() tea.Cmd {
	return nil
}

func (m *ProviderList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ":
			if m.loading {
				return m, nil
			}
			item, ok := m.list.SelectedItem().(providerItem)
			if !ok {
				return m, nil
			}
			m.loading = true
			prov := item.p
			return m, tea.Batch(
				m.spinner.Tick,
				loadAccounts(prov),
			)

		case "q":
			return m, tea.Quit

		case "/":
			// Handled by bubbles list filtering — let it fall through.
		}

	case AccountsLoadedMsg:
		m.loading = false
		if msg.Err != nil {
			return m, func() tea.Msg { return ErrorMsg{Err: msg.Err} }
		}
		item, ok := m.list.SelectedItem().(providerItem)
		if !ok {
			return m, nil
		}
		if len(msg.Accounts) == 1 {
			// Single account — skip the selection step.
			zoneView := NewZoneList(item.p, msg.Accounts)
			return m, func() tea.Msg { return PushMsg{Model: zoneView} }
		}
		// Multiple accounts — let the user pick one.
		accView := NewAccountList(item.p, msg.Accounts)
		return m, func() tea.Msg { return PushMsg{Model: accView} }

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *ProviderList) View() string {
	if m.loading {
		return m.list.View() + "\n" + m.spinner.View() + " Loading accounts…"
	}
	return m.list.View() + "\n" + styleHelp.Render("enter: select  /: filter  q: quit")
}

// loadAccounts fetches accounts from a provider asynchronously.
func loadAccounts(p provider.Provider) tea.Cmd {
	return func() tea.Msg {
		accounts, err := p.ListAccounts(context.Background())
		return AccountsLoadedMsg{Accounts: accounts, Err: err}
	}
}

// ── AccountList ───────────────────────────────────────────────────────────────

type accountItem struct{ a provider.Account }

func (i accountItem) Title() string       { return i.a.Name }
func (i accountItem) Description() string { return "id: " + i.a.ID }
func (i accountItem) FilterValue() string { return i.a.Name }

// AccountList is shown when a provider returns more than one account, letting
// the user pick which account to browse.
type AccountList struct {
	prov     provider.Provider
	accounts []provider.Account
	list     list.Model
}

// NewAccountList creates the account-selection view.
func NewAccountList(p provider.Provider, accounts []provider.Account) *AccountList {
	items := make([]list.Item, len(accounts))
	for i, a := range accounts {
		items[i] = accountItem{a}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = fmt.Sprintf("%s — Accounts", p.FriendlyName())
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = styleTitle
	l.Styles.TitleBar = lipgloss.NewStyle()
	return &AccountList{prov: p, accounts: accounts, list: l}
}

func (m *AccountList) Init() tea.Cmd { return nil }

func (m *AccountList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return PopMsg{} }

		case "enter", " ":
			item, ok := m.list.SelectedItem().(accountItem)
			if !ok {
				return m, nil
			}
			zoneView := NewZoneList(m.prov, []provider.Account{item.a})
			return m, func() tea.Msg { return PushMsg{Model: zoneView} }

		case "q":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *AccountList) View() string {
	return m.list.View() + "\n" + styleHelp.Render("enter: select  /: filter  esc: back  q: quit")
}

// ── ZoneList ─────────────────────────────────────────────────────────────────

type zoneItem struct{ z provider.Zone }

func (i zoneItem) Title() string       { return i.z.Name }
func (i zoneItem) Description() string { return "id: " + i.z.ID }
func (i zoneItem) FilterValue() string { return i.z.Name }

// ZoneList shows zones for the selected provider account.
type ZoneList struct {
	prov     provider.Provider
	accounts []provider.Account
	accIdx   int
	list     list.Model
	loading  bool
	spinner  spinner.Model
}

// NewZoneList creates the zone-selection view.
func NewZoneList(p provider.Provider, accounts []provider.Account) *ZoneList {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = styleTitle

	zl := &ZoneList{
		prov:     p,
		accounts: accounts,
		list:     l,
		spinner:  sp,
	}
	if len(accounts) > 0 {
		zl.list.Title = fmt.Sprintf("%s — Zones (%s)", p.FriendlyName(), accounts[0].Name)
	} else {
		zl.list.Title = p.FriendlyName() + " — Zones"
	}
	return zl
}

func (m *ZoneList) Init() tea.Cmd {
	accountID := ""
	if len(m.accounts) > 0 {
		accountID = m.accounts[m.accIdx].ID
	}
	m.loading = true
	return tea.Batch(m.spinner.Tick, loadZones(m.prov, accountID))
}

func (m *ZoneList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return PopMsg{} }

		case "enter", " ":
			if m.loading {
				return m, nil
			}
			item, ok := m.list.SelectedItem().(zoneItem)
			if !ok {
				return m, nil
			}
			rl := NewRecordList(m.prov, item.z)
			return m, func() tea.Msg { return PushMsg{Model: rl} }

		case "tab":
			// Cycle to next account if there are multiple.
			if len(m.accounts) > 1 {
				m.accIdx = (m.accIdx + 1) % len(m.accounts)
				m.loading = true
				m.list.Title = fmt.Sprintf("%s — Zones (%s)", m.prov.FriendlyName(), m.accounts[m.accIdx].Name)
				return m, tea.Batch(m.spinner.Tick, loadZones(m.prov, m.accounts[m.accIdx].ID))
			}
		}

	case ZonesLoadedMsg:
		m.loading = false
		if msg.Err != nil {
			return m, func() tea.Msg { return ErrorMsg{Err: msg.Err} }
		}
		items := make([]list.Item, len(msg.Zones))
		for i, z := range msg.Zones {
			items[i] = zoneItem{z}
		}
		m.list.SetItems(items)
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *ZoneList) View() string {
	var sb strings.Builder
	sb.WriteString(m.list.View())
	if m.loading {
		sb.WriteString("\n" + m.spinner.View() + " Loading zones…")
	} else {
		help := "enter: open  /: filter  esc: back  q: quit"
		if len(m.accounts) > 1 {
			help += "  tab: next account"
		}
		sb.WriteString("\n" + styleHelp.Render(help))
	}
	return sb.String()
}

func loadZones(p provider.Provider, accountID string) tea.Cmd {
	return func() tea.Msg {
		zones, err := p.ListZones(context.Background(), accountID)
		return ZonesLoadedMsg{AccountID: accountID, Zones: zones, Err: err}
	}
}
