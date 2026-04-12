package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/scheiblingco/dnstui/internal/provider"
)

// ── Record-type selector ──────────────────────────────────────────────────────

var allRecordTypes = []provider.RecordType{
	provider.RecordTypeA, provider.RecordTypeAAAA, provider.RecordTypeCNAME,
	provider.RecordTypeMX, provider.RecordTypeTXT, provider.RecordTypeNS,
	provider.RecordTypeSRV, provider.RecordTypeCAA, provider.RecordTypePTR,
	provider.RecordTypeTLSA, provider.RecordTypeSSHFP, provider.RecordTypeNAPTR,
}

// ── Sub-field system ──────────────────────────────────────────────────────────
//
// Each record type has an ordered list of subFieldDefs that replace the generic
// single "Value" input.  The "key" field maps the value to provider.Record:
//   "value"      → Record.Value
//   "priority"   → Record.Priority
//   "extra:NAME" → Record.Extra["NAME"] (int or string based on kind)

type subFieldDef struct {
	key         string // "value" | "priority" | "extra:NAME"
	label       string
	placeholder string
	kind        string // "text" | "int"
	required    bool
}

type subFieldInst struct {
	def   subFieldDef
	input textinput.Model
}

// typeSubFields defines structured sub-fields for complex record types.
// Types not in this map get a single generic "Value" text field.
var typeSubFields = map[provider.RecordType][]subFieldDef{
	provider.RecordTypeMX: {
		{key: "priority", label: "Priority ", placeholder: "10", kind: "int", required: false},
		{key: "value", label: "Exchange ", placeholder: "mail.example.com", kind: "text", required: true},
	},
	provider.RecordTypeSRV: {
		{key: "priority", label: "Priority ", placeholder: "10", kind: "int", required: true},
		{key: "extra:weight", label: "Weight   ", placeholder: "0", kind: "int", required: true},
		{key: "extra:port", label: "Port     ", placeholder: "443", kind: "int", required: true},
		{key: "value", label: "Target   ", placeholder: "_service.example.com", kind: "text", required: true},
	},
	provider.RecordTypeCAA: {
		{key: "extra:caa_flags", label: "Flags    ", placeholder: "0", kind: "int", required: true},
		{key: "extra:caa_tag", label: "Tag      ", placeholder: "issue / issuewild / iodef", kind: "text", required: true},
		{key: "value", label: "Value    ", placeholder: "letsencrypt.org", kind: "text", required: true},
	},
	provider.RecordTypeTLSA: {
		{key: "extra:tlsa_usage", label: "Usage    ", placeholder: "0=PKIX-CA  1=PKIX-EE  2=DANE-TA  3=DANE-EE", kind: "int", required: true},
		{key: "extra:tlsa_selector", label: "Selector ", placeholder: "0=Full cert  1=SubjectPublicKeyInfo", kind: "int", required: true},
		{key: "extra:tlsa_matching", label: "Match    ", placeholder: "0=Exact  1=SHA-256  2=SHA-512", kind: "int", required: true},
		{key: "value", label: "Cert Data", placeholder: "hex-encoded certificate/key data", kind: "text", required: true},
	},
	provider.RecordTypeSSHFP: {
		{key: "extra:sshfp_algorithm", label: "Algorithm", placeholder: "1=RSA  2=DSA  3=ECDSA  4=Ed25519", kind: "int", required: true},
		{key: "extra:sshfp_fp_type", label: "FP Type  ", placeholder: "1=SHA-1  2=SHA-256", kind: "int", required: true},
		{key: "value", label: "Fingerprint", placeholder: "hex-encoded fingerprint", kind: "text", required: true},
	},
	provider.RecordTypeNAPTR: {
		{key: "priority", label: "Order    ", placeholder: "100", kind: "int", required: true},
		{key: "extra:naptr_pref", label: "Preference", placeholder: "10", kind: "int", required: true},
		{key: "extra:naptr_flags", label: "Flags    ", placeholder: "S / A / U / P", kind: "text", required: false},
		{key: "extra:naptr_service", label: "Service  ", placeholder: "SIP+D2U", kind: "text", required: true},
		{key: "extra:naptr_regexp", label: "Regexp   ", placeholder: "!^.*$!sip:info@example.com!i", kind: "text", required: false},
		{key: "value", label: "Replacement", placeholder: ".", kind: "text", required: true},
	},
}

// valuePlaceholderFor returns a type-appropriate placeholder for the generic Value field.
func valuePlaceholderFor(t provider.RecordType) string {
	switch t {
	case provider.RecordTypeA:
		return "1.2.3.4"
	case provider.RecordTypeAAAA:
		return "2001:db8::1"
	case provider.RecordTypeCNAME:
		return "target.example.com"
	case provider.RecordTypeTXT:
		return `"v=spf1 ~all"`
	case provider.RecordTypeNS:
		return "ns1.example.com"
	case provider.RecordTypePTR:
		return "host.example.com"
	default:
		return ""
	}
}

// ── Provider-specific extras ──────────────────────────────────────────────────

type extraFieldDef struct {
	key   string // matches provider.Record.Extra key
	label string
	kind  string // "bool" | "text"
}

var providerExtraFields = map[string]map[provider.RecordType][]extraFieldDef{
	"cloudflare": {
		provider.RecordTypeA:     {{key: "proxied", label: "Proxied (CDN)", kind: "bool"}},
		provider.RecordTypeAAAA:  {{key: "proxied", label: "Proxied (CDN)", kind: "bool"}},
		provider.RecordTypeCNAME: {{key: "proxied", label: "Proxied (CDN)", kind: "bool"}},
	},
}

type activeExtra struct {
	def     extraFieldDef
	boolVal bool
	textVal string
	input   textinput.Model
}

// ── Focus constants ───────────────────────────────────────────────────────────

const (
	focusName = 0
	focusType = 1
	focusTTL  = 2

// sub-fields start at 3
)

// ── RecordForm ────────────────────────────────────────────────────────────────

type RecordForm struct {
	prov     provider.Provider
	zone     provider.Zone
	original provider.Record
	isEdit   bool

	nameInput textinput.Model
	ttlInput  textinput.Model

	typeIdx   int
	subFields []subFieldInst
	extras    []activeExtra

	focused    int
	submitting bool
	err        string
}

func NewRecordForm(p provider.Provider, z provider.Zone, rec provider.Record, isEdit bool) *RecordForm {
	mk := func(placeholder string) textinput.Model {
		ti := textinput.New()
		ti.CharLimit = 255
		ti.Placeholder = placeholder
		return ti
	}

	m := &RecordForm{
		prov:      p,
		zone:      z,
		original:  rec,
		isEdit:    isEdit,
		nameInput: mk("www.example.com"),
		ttlInput:  mk("300  (0 = automatic)"),
	}

	if isEdit {
		for i, t := range allRecordTypes {
			if t == rec.Type {
				m.typeIdx = i
				break
			}
		}
		m.nameInput.SetValue(rec.Name)
		m.ttlInput.SetValue(fmt.Sprintf("%d", rec.TTL))
	}

	m.rebuildSubFields(rec)
	m.rebuildExtras(rec)
	m.nameInput.Focus()
	return m
}

func (m *RecordForm) currentType() provider.RecordType { return allRecordTypes[m.typeIdx] }

func (m *RecordForm) subFieldsStart() int { return 3 }
func (m *RecordForm) extrasStart() int    { return 3 + len(m.subFields) }
func (m *RecordForm) totalFields() int    { return m.extrasStart() + len(m.extras) }

// rebuildSubFields recreates the ordered sub-fields for the current record type,
// populating initial values from sourceRec.
func (m *RecordForm) rebuildSubFields(sourceRec provider.Record) {
	defs, hasStructured := typeSubFields[m.currentType()]
	if !hasStructured {
		defs = []subFieldDef{
			{key: "value", label: "Value   ", placeholder: valuePlaceholderFor(m.currentType()), kind: "text", required: true},
		}
	}

	fields := make([]subFieldInst, len(defs))
	for i, def := range defs {
		ti := textinput.New()
		ti.CharLimit = 512
		ti.Placeholder = def.placeholder

		switch def.key {
		case "value":
			if sourceRec.Value != "" {
				ti.SetValue(sourceRec.Value)
			}
		case "priority":
			if sourceRec.Priority > 0 {
				ti.SetValue(strconv.Itoa(sourceRec.Priority))
			}
		default:
			if strings.HasPrefix(def.key, "extra:") {
				k := strings.TrimPrefix(def.key, "extra:")
				if v, ok := sourceRec.Extra[k]; ok {
					switch vv := v.(type) {
					case int:
						ti.SetValue(strconv.Itoa(vv))
					case float64:
						ti.SetValue(strconv.Itoa(int(vv)))
					case string:
						ti.SetValue(vv)
					}
				}
			}
		}
		fields[i] = subFieldInst{def: def, input: ti}
	}
	m.subFields = fields
}

// rebuildExtras recreates provider-specific extra fields for the current type.
func (m *RecordForm) rebuildExtras(sourceRec provider.Record) {
	defs, ok := providerExtraFields[m.prov.ProviderName()][m.currentType()]
	if !ok {
		m.extras = nil
		return
	}
	newExtras := make([]activeExtra, 0, len(defs))
	for _, def := range defs {
		ae := activeExtra{def: def}
		switch def.kind {
		case "bool":
			if v, ok := sourceRec.Extra[def.key].(bool); ok {
				ae.boolVal = v
			} else {
				for _, prev := range m.extras {
					if prev.def.key == def.key {
						ae.boolVal = prev.boolVal
					}
				}
			}
		case "text":
			ti := textinput.New()
			ti.CharLimit = 255
			if v, ok := sourceRec.Extra[def.key].(string); ok {
				ti.SetValue(v)
			}
			ae.input = ti
		}
		newExtras = append(newExtras, ae)
	}
	m.extras = newExtras
}

// focusField updates the focused index and moves text-input focus accordingly.
func (m *RecordForm) focusField(idx int) {
	m.nameInput.Blur()
	m.ttlInput.Blur()
	for i := range m.subFields {
		m.subFields[i].input.Blur()
	}
	for i := range m.extras {
		if m.extras[i].def.kind == "text" {
			m.extras[i].input.Blur()
		}
	}

	m.focused = idx
	switch idx {
	case focusName:
		m.nameInput.Focus()
	case focusType:
	// no text input
	case focusTTL:
		m.ttlInput.Focus()
	default:
		si := idx - m.subFieldsStart()
		if si >= 0 && si < len(m.subFields) {
			m.subFields[si].input.Focus()
			return
		}
		ei := idx - m.extrasStart()
		if ei >= 0 && ei < len(m.extras) && m.extras[ei].def.kind == "text" {
			m.extras[ei].input.Focus()
		}
	}
}

func (m *RecordForm) Init() tea.Cmd { return textinput.Blink }

func (m *RecordForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return PopMsg{} }

		case "ctrl+s":
			return m.submit()

		case "enter":
			if m.focused == focusType {
				m.focusField((m.focused + 1) % m.totalFields())
				return m, textinput.Blink
			}
			if m.focused >= m.extrasStart() {
				ei := m.focused - m.extrasStart()
				if ei < len(m.extras) && m.extras[ei].def.kind == "bool" {
					m.extras[ei].boolVal = !m.extras[ei].boolVal
					return m, nil
				}
			}
			if m.focused >= m.totalFields()-1 {
				return m.submit()
			}
			m.focusField(m.focused + 1)
			return m, textinput.Blink

		case "tab", "down":
			m.focusField((m.focused + 1) % m.totalFields())
			return m, textinput.Blink

		case "shift+tab", "up":
			m.focusField((m.focused + m.totalFields() - 1) % m.totalFields())
			return m, textinput.Blink

		case "left":
			if m.focused == focusType {
				m.typeIdx = (m.typeIdx + len(allRecordTypes) - 1) % len(allRecordTypes)
				m.rebuildSubFields(provider.Record{Extra: map[string]any{}})
				m.rebuildExtras(provider.Record{Extra: map[string]any{}})
				return m, nil
			}
			if m.focused >= m.extrasStart() {
				ei := m.focused - m.extrasStart()
				if ei < len(m.extras) && m.extras[ei].def.kind == "bool" {
					m.extras[ei].boolVal = false
					return m, nil
				}
			}

		case "right":
			if m.focused == focusType {
				m.typeIdx = (m.typeIdx + 1) % len(allRecordTypes)
				m.rebuildSubFields(provider.Record{Extra: map[string]any{}})
				m.rebuildExtras(provider.Record{Extra: map[string]any{}})
				return m, nil
			}
			if m.focused >= m.extrasStart() {
				ei := m.focused - m.extrasStart()
				if ei < len(m.extras) && m.extras[ei].def.kind == "bool" {
					m.extras[ei].boolVal = true
					return m, nil
				}
			}

		case " ":
			if m.focused >= m.extrasStart() {
				ei := m.focused - m.extrasStart()
				if ei < len(m.extras) && m.extras[ei].def.kind == "bool" {
					m.extras[ei].boolVal = !m.extras[ei].boolVal
					return m, nil
				}
			}
		}

	case RecordSavedMsg:
		m.submitting = false
		if msg.Err != nil {
			m.err = msg.Err.Error()
			return m, nil
		}
		saved := msg
		return m, func() tea.Msg {
			return PopMsg{FollowUp: func() tea.Msg { return saved }}
		}
	}

	var cmd tea.Cmd
	switch m.focused {
	case focusName:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case focusType:
	// type selector handles its own keys above
	case focusTTL:
		m.ttlInput, cmd = m.ttlInput.Update(msg)
	default:
		si := m.focused - m.subFieldsStart()
		if si >= 0 && si < len(m.subFields) {
			m.subFields[si].input, cmd = m.subFields[si].input.Update(msg)
		} else {
			ei := m.focused - m.extrasStart()
			if ei >= 0 && ei < len(m.extras) && m.extras[ei].def.kind == "text" {
				m.extras[ei].input, cmd = m.extras[ei].input.Update(msg)
			}
		}
	}
	return m, cmd
}

func (m *RecordForm) submit() (tea.Model, tea.Cmd) {
	rec, valErr := m.buildRecord()
	if valErr != "" {
		m.err = valErr
		return m, nil
	}
	m.err = ""
	m.submitting = true

	var saveCmd tea.Cmd
	if m.isEdit {
		orig := m.original
		saveCmd = func() tea.Msg {
			updated, err := m.prov.UpdateRecord(context.Background(), m.zone.ID, orig.ID, rec)
			return RecordSavedMsg{Record: updated, Err: err}
		}
	} else {
		saveCmd = func() tea.Msg {
			created, err := m.prov.CreateRecord(context.Background(), m.zone.ID, rec)
			return RecordSavedMsg{Record: created, Err: err}
		}
	}
	return m, saveCmd
}

func (m *RecordForm) buildRecord() (provider.Record, string) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		return provider.Record{}, "Name is required"
	}

	ttlStr := strings.TrimSpace(m.ttlInput.Value())
	ttl := 0
	if ttlStr != "" {
		n, err := strconv.Atoi(ttlStr)
		if err != nil || n < 0 {
			return provider.Record{}, "TTL must be a non-negative integer (0 = automatic)"
		}
		ttl = n
	}

	rec := provider.Record{
		Name:  name,
		Type:  m.currentType(),
		TTL:   ttl,
		Extra: map[string]any{},
	}

	// Process sub-fields.
	for _, sf := range m.subFields {
		raw := strings.TrimSpace(sf.input.Value())
		if raw == "" {
			if sf.def.required {
				return provider.Record{}, strings.TrimSpace(sf.def.label) + " is required"
			}
			continue
		}
		switch sf.def.key {
		case "value":
			rec.Value = raw
		case "priority":
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				return provider.Record{}, strings.TrimSpace(sf.def.label) + " must be a non-negative integer"
			}
			rec.Priority = n
		default:
			if strings.HasPrefix(sf.def.key, "extra:") {
				k := strings.TrimPrefix(sf.def.key, "extra:")
				if sf.def.kind == "int" {
					n, err := strconv.Atoi(raw)
					if err != nil || n < 0 {
						return provider.Record{}, strings.TrimSpace(sf.def.label) + " must be a non-negative integer"
					}
					rec.Extra[k] = n
				} else {
					rec.Extra[k] = raw
				}
			}
		}
	}

	// Process provider extras.
	for _, ae := range m.extras {
		switch ae.def.kind {
		case "bool":
			rec.Extra[ae.def.key] = ae.boolVal
		case "text":
			if v := strings.TrimSpace(ae.input.Value()); v != "" {
				rec.Extra[ae.def.key] = v
			}
		}
	}

	return rec, ""
}

// ── Rendering ─────────────────────────────────────────────────────────────────

var (
	styleLabelActive = lipgloss.NewStyle().Foreground(colorSelected).Bold(true)
	styleLabelMuted  = lipgloss.NewStyle().Foreground(colorMuted)
	styleToggleOn    = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	styleToggleOff   = lipgloss.NewStyle().Foreground(colorMuted)
)

func (m *RecordForm) View() string {
	action := "Add Record"
	if m.isEdit {
		action = "Edit Record"
	}

	var sb strings.Builder
	sb.WriteString(styleTitle.Render(m.zone.Name+" — "+action) + "\n\n")

	writeField := func(focusIdx int, label, value string) {
		var lbl string
		if m.focused == focusIdx {
			lbl = styleLabelActive.Render(label + ": ")
		} else {
			lbl = styleLabelMuted.Render(label + ": ")
		}
		sb.WriteString(lbl + value + "\n")
	}

	writeField(focusName, "Name    ", m.nameInput.View())
	writeField(focusType, "Type    ", renderTypeSelector(m.typeIdx, m.focused == focusType))
	writeField(focusTTL, "TTL     ", m.ttlInput.View())

	// Sub-fields (replace the old single Value + Priority fields).
	_, isStructured := typeSubFields[m.currentType()]
	if isStructured && len(m.subFields) > 0 {
		sb.WriteString("\n" + styleSubtitle.Render("── "+string(m.currentType())+" fields ──") + "\n")
	}
	for i, sf := range m.subFields {
		writeField(m.subFieldsStart()+i, sf.def.label, sf.input.View())
	}

	// Provider extras.
	if len(m.extras) > 0 {
		sb.WriteString("\n" + styleSubtitle.Render("── "+m.prov.ProviderName()+" options ──") + "\n")
		for i, ae := range m.extras {
			fi := m.extrasStart() + i
			active := m.focused == fi
			var lbl string
			if active {
				lbl = styleLabelActive.Render(ae.def.label + ": ")
			} else {
				lbl = styleLabelMuted.Render(ae.def.label + ": ")
			}
			var val string
			switch ae.def.kind {
			case "bool":
				val = renderBoolToggle(ae.boolVal, active)
			case "text":
				val = ae.input.View()
			}
			sb.WriteString(lbl + val + "\n")
		}
	}

	if m.err != "" {
		sb.WriteString("\n" + styleError.Render("✖ "+m.err) + "\n")
	}

	sb.WriteString("\n")
	if m.submitting {
		sb.WriteString(styleSubtitle.Render("Saving…"))
	} else {
		hints := []string{"tab/↑↓: navigate", "ctrl+s: save", "esc: cancel"}
		if m.focused == focusType {
			hints = append([]string{"←/→: change type"}, hints...)
		}
		sb.WriteString(styleHelp.Render(strings.Join(hints, "  ")))
	}

	return sb.String()
}

func renderTypeSelector(idx int, active bool) string {
	cur := string(allRecordTypes[idx])
	if active {
		return styleLabelActive.Render("← ") +
			styleSelected.Render(fmt.Sprintf("%-6s", cur)) +
			styleLabelActive.Render(" →")
	}
	return styleLabelMuted.Render("  ") +
		styleSubtitle.Render(fmt.Sprintf("%-6s", cur)) +
		styleLabelMuted.Render("  ")
}

func renderBoolToggle(val bool, active bool) string {
	on := "[ ✔ Yes ]"
	off := "[ ✘ No  ]"
	if val {
		if active {
			return styleToggleOn.Render(on) + "  " + styleLabelMuted.Render(off)
		}
		return styleToggleOn.Render(on) + "  " + styleToggleOff.Render(off)
	}
	if active {
		return styleToggleOff.Render(on) + "  " + styleLabelActive.Render(off)
	}
	return styleToggleOff.Render(on) + "  " + styleToggleOff.Render(off)
}
