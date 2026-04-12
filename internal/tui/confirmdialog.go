package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ── ConfirmDialog ─────────────────────────────────────────────────────────────

// ConfirmDialog is a modal overlay for destructive actions.
// It executes confirmCmd when the user presses y/Y/enter, and discards cleanly
// on n/N/esc.
type ConfirmDialog struct {
	message    string
	confirmCmd tea.Cmd
	selected   bool // true = yes, false = no
}

// NewConfirmDialog creates a confirm/cancel dialog.
// confirmCmd is the tea.Cmd that should run on confirmation.
func NewConfirmDialog(message string, confirmCmd tea.Cmd) *ConfirmDialog {
	return &ConfirmDialog{
		message:    message,
		confirmCmd: confirmCmd,
		selected:   false,
	}
}

func (m *ConfirmDialog) Init() tea.Cmd { return nil }

func (m *ConfirmDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch strings.ToLower(msg.String()) {
		case "y", "enter":
			// Confirm: pop dialog, then run the action.
			return m, tea.Batch(
				func() tea.Msg { return PopMsg{} },
				m.confirmCmd,
			)
		case "n", "esc":
			return m, func() tea.Msg { return PopMsg{} }
		case "left", "right", "h", "l":
			m.selected = !m.selected
		}
	}
	return m, nil
}

func (m *ConfirmDialog) View() string {
	yes := "[ Yes ]"
	no := "[ No  ]"
	if m.selected {
		yes = styleSelected.Render("[ Yes ]")
	} else {
		no = styleSelected.Render("[ No  ]")
	}

	box := styleBorder.Render(
		styleTitle.Render("Confirm") + "\n\n" +
			m.message + "\n\n" +
			yes + "   " + no + "\n\n" +
			styleHelp.Render("y/enter: confirm  n/esc: cancel  ←→: toggle"),
	)
	return "\n" + box
}
