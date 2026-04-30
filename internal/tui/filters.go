package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/kantis/dynatrace-tui/internal/dql"
)

// SavedFilter is a reusable DQL fragment with optional `$placeholder`
// substitution. The `Template` is stored *without* a leading ` | ` —
// the picker prepends one when appending the fragment to a non-empty editor.
type SavedFilter struct {
	Name        string              `yaml:"name"`
	Template    string              `yaml:"template"`
	Suggestions map[string][]string `yaml:"suggestions,omitempty"`
}

type filtersFile struct {
	Filters []SavedFilter `yaml:"fragments"`
}

func savedFiltersPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "dynatrace-tui", "fragments.yaml"), nil
}

func loadSavedFilters() ([]SavedFilter, error) {
	p, err := savedFiltersPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f filtersFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return f.Filters, nil
}

func writeSavedFilters(fs []SavedFilter) error {
	p, err := savedFiltersPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(filtersFile{Filters: fs})
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// normalizeTemplate trims surrounding whitespace and any leading pipe so the
// template is stored as a clean fragment. The picker re-adds ` | ` when
// appending to a non-empty editor body. The user is responsible for any
// leading verb (`filter`, `sort`, etc.) — fragments are inserted verbatim.
func normalizeTemplate(s string) string {
	s = strings.TrimSpace(s)
	for strings.HasPrefix(s, "|") {
		s = strings.TrimLeft(s[1:], " \t")
	}
	return s
}

// --- Fragments view (Alt-3) -----------------------------------------------

type filtersMode int

const (
	filtersModeList filtersMode = iota
	filtersModeEditing
)

func (m Model) enterFiltersView() Model {
	m.currentView = viewFilters
	m.filtersMode = filtersModeList
	if m.filtersListIdx < 0 {
		m.filtersListIdx = 0
	}
	if m.filtersListIdx >= len(m.filters) && len(m.filters) > 0 {
		m.filtersListIdx = len(m.filters) - 1
	}
	return m
}

func (m Model) updateFiltersView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtersMode == filtersModeEditing {
		return m.updateFiltersEdit(msg)
	}
	return m.updateFiltersList(msg)
}

func (m Model) updateFiltersList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.filtersListIdx > 0 {
			m.filtersListIdx--
		}
	case "down", "j":
		if m.filtersListIdx < len(m.filters)-1 {
			m.filtersListIdx++
		}
	case "n":
		return m.enterFiltersEdit(true), textinput.Blink
	case "e":
		if len(m.filters) == 0 {
			return m, nil
		}
		return m.enterFiltersEdit(false), textinput.Blink
	case "d":
		if len(m.filters) == 0 {
			return m, nil
		}
		m.filters = append(m.filters[:m.filtersListIdx], m.filters[m.filtersListIdx+1:]...)
		if m.filtersListIdx >= len(m.filters) && m.filtersListIdx > 0 {
			m.filtersListIdx--
		}
		if err := writeSavedFilters(m.filters); err != nil {
			m.errMsg = err.Error()
			m.state = stateError
		}
	case "enter":
		if len(m.filters) == 0 {
			return m, nil
		}
		// Insertion uses the same flow as Ctrl-F from the editor: switch
		// back to the query view, then either insert directly or open the
		// resolve modal.
		m.currentView = viewQuery
		next := m.pickFilter(m.filters[m.filtersListIdx])
		return next, nil
	}
	return m, nil
}

// enterFiltersEdit opens the edit form. If isNew is true a blank entry is
// drafted and only committed on Ctrl-S; the filters slice is *not* mutated up
// front. Cancelling (Esc) drops the draft.
func (m Model) enterFiltersEdit(isNew bool) Model {
	var src SavedFilter
	if isNew {
		src = SavedFilter{Name: "", Template: "", Suggestions: map[string][]string{}}
		m.filterEditOriginalName = ""
	} else {
		src = m.filters[m.filtersListIdx]
		m.filterEditOriginalName = src.Name
	}
	m.filterEditIsNew = isNew

	name := textinput.New()
	name.Placeholder = "fragment name (e.g. by-service)"
	name.SetValue(src.Name)
	name.CharLimit = 64
	name.Width = 40
	name.Focus()
	m.filterEditNameInput = name

	tmpl := textarea.New()
	tmpl.ShowLineNumbers = false
	tmpl.CharLimit = 0
	tmpl.Placeholder = `filter loglevel == "$level"`
	tmpl.SetValue(src.Template)
	tmpl.Blur()
	m.filterEditTemplate = tmpl

	// Seed the by-name suggestion store from the source filter so existing
	// values are preserved across template edits.
	m.filterEditValuesByName = map[string]string{}
	for k, vs := range src.Suggestions {
		m.filterEditValuesByName[k] = strings.Join(vs, "\n")
	}

	m.filterEditFocus = 0
	m.filtersMode = filtersModeEditing
	m.refreshFilterEditPlaceholders()
	m.applyLayout()
	m.errMsg = ""
	m.state = stateIdle
	return m
}

// refreshFilterEditPlaceholders rebuilds the per-placeholder suggestion
// textareas from the current template, preserving any previously-typed
// suggestion values keyed by placeholder name.
func (m *Model) refreshFilterEditPlaceholders() {
	names := dql.Placeholders(m.filterEditTemplate.Value())

	// Snapshot current textarea values so any in-flight edits aren't lost
	// when the placeholder set changes.
	for i, n := range m.filterEditPlaceholders {
		if i < len(m.filterEditSuggestions) {
			m.filterEditValuesByName[n] = m.filterEditSuggestions[i].Value()
		}
	}

	tas := make([]textarea.Model, len(names))
	for i, n := range names {
		ta := textarea.New()
		ta.ShowLineNumbers = false
		ta.CharLimit = 0
		ta.Placeholder = "one suggestion per line"
		if v, ok := m.filterEditValuesByName[n]; ok {
			ta.SetValue(v)
		}
		ta.Blur()
		tas[i] = ta
	}
	m.filterEditPlaceholders = names
	m.filterEditSuggestions = tas

	if m.width > 0 {
		innerW := m.width - 2
		if innerW < 20 {
			innerW = 20
		}
		m.layoutFilterEdit(innerW)
	}
}

// layoutFilterEdit distributes the available vertical space across the
// template and suggestion textareas. The template stays small when there
// are placeholders (it's typically a single-line predicate) so the
// suggestion lists soak up the remaining height; with no placeholders the
// template gets the full residual area.
func (m *Model) layoutFilterEdit(innerW int) {
	listLines := len(m.filters)
	if listLines < 1 {
		listLines = 1
	}
	// Cap the list height so a long list doesn't push the form off-screen.
	if listLines > 5 {
		listLines = 5
	}
	n := len(m.filterEditPlaceholders)

	// Chrome line accounting:
	//   tabs(1) + list+borders(listLines+2)
	//   + name title(1) + name input area(3, includes border)
	//   + template title(1) + template borders(2)
	//   + status(1) + per-placeholder (title 1 + borders 2)
	chrome := 1 + listLines + 2 + 1 + 3 + 1 + 2 + 1 + n*3
	remaining := m.height - chrome
	if remaining < 6 {
		remaining = 6
	}

	var templateH, sugH int
	if n == 0 {
		templateH = remaining
		sugH = 3
	} else {
		templateH = 3
		sugH = (remaining - templateH) / n
		if sugH < 3 {
			sugH = 3
		}
	}

	m.filterEditTemplate.SetWidth(innerW)
	m.filterEditTemplate.SetHeight(templateH)
	for i := range m.filterEditSuggestions {
		m.filterEditSuggestions[i].SetWidth(innerW)
		m.filterEditSuggestions[i].SetHeight(sugH)
	}
}

func (m *Model) filterEditFocusCount() int {
	return 2 + len(m.filterEditPlaceholders)
}

func (m Model) cycleFilterEditFocus(reverse bool) Model {
	count := m.filterEditFocusCount()
	if count == 0 {
		return m
	}
	// Snapshot suggestion values before potentially recomputing the layout
	// (only matters when leaving the template field).
	if m.filterEditFocus == 1 {
		m.refreshFilterEditPlaceholders()
		count = m.filterEditFocusCount() // may have changed
	}

	// Blur current.
	m.blurFilterEditFocus()

	if reverse {
		m.filterEditFocus = (m.filterEditFocus - 1 + count) % count
	} else {
		m.filterEditFocus = (m.filterEditFocus + 1) % count
	}

	m.focusFilterEdit()
	return m
}

func (m *Model) blurFilterEditFocus() {
	switch {
	case m.filterEditFocus == 0:
		m.filterEditNameInput.Blur()
	case m.filterEditFocus == 1:
		m.filterEditTemplate.Blur()
	default:
		idx := m.filterEditFocus - 2
		if idx >= 0 && idx < len(m.filterEditSuggestions) {
			m.filterEditSuggestions[idx].Blur()
		}
	}
}

func (m *Model) focusFilterEdit() {
	switch {
	case m.filterEditFocus == 0:
		m.filterEditNameInput.Focus()
	case m.filterEditFocus == 1:
		m.filterEditTemplate.Focus()
	default:
		idx := m.filterEditFocus - 2
		if idx >= 0 && idx < len(m.filterEditSuggestions) {
			m.filterEditSuggestions[idx].Focus()
		}
	}
}

func (m Model) updateFiltersEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtersMode = filtersModeList
		m.errMsg = ""
		return m, nil
	case "tab":
		return m.cycleFilterEditFocus(false), nil
	case "shift+tab":
		return m.cycleFilterEditFocus(true), nil
	case "ctrl+s":
		return m.saveFilterEdit()
	}

	// Route key to focused widget.
	var cmd tea.Cmd
	switch {
	case m.filterEditFocus == 0:
		m.filterEditNameInput, cmd = m.filterEditNameInput.Update(msg)
	case m.filterEditFocus == 1:
		m.filterEditTemplate, cmd = m.filterEditTemplate.Update(msg)
	default:
		idx := m.filterEditFocus - 2
		if idx >= 0 && idx < len(m.filterEditSuggestions) {
			m.filterEditSuggestions[idx], cmd = m.filterEditSuggestions[idx].Update(msg)
		}
	}
	return m, cmd
}

func (m Model) saveFilterEdit() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.filterEditNameInput.Value())
	if name == "" {
		m.errMsg = "name cannot be empty"
		m.state = stateError
		return m, nil
	}
	tmpl := normalizeTemplate(m.filterEditTemplate.Value())
	if tmpl == "" {
		m.errMsg = "template cannot be empty"
		m.state = stateError
		return m, nil
	}

	// Sync current suggestion values into the by-name map (covers the
	// focused field which hasn't been snapshotted by tab cycling yet).
	for i, n := range m.filterEditPlaceholders {
		if i < len(m.filterEditSuggestions) {
			m.filterEditValuesByName[n] = m.filterEditSuggestions[i].Value()
		}
	}

	// Recompute placeholders from the (normalised) saved template — the
	// user may have edited it without tab-cycling out, so the in-memory
	// placeholder list could be stale.
	placeholders := dql.Placeholders(tmpl)
	suggestions := map[string][]string{}
	for _, n := range placeholders {
		raw, ok := m.filterEditValuesByName[n]
		if !ok {
			continue
		}
		var items []string
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				items = append(items, line)
			}
		}
		if len(items) > 0 {
			suggestions[n] = items
		}
	}

	// Collision check on rename / new.
	for i, f := range m.filters {
		if !m.filterEditIsNew && i == m.filtersListIdx {
			continue
		}
		if f.Name == name {
			m.errMsg = "another fragment already uses that name"
			m.state = stateError
			return m, nil
		}
	}

	saved := SavedFilter{Name: name, Template: tmpl, Suggestions: suggestions}
	if m.filterEditIsNew {
		m.filters = append(m.filters, saved)
		m.filtersListIdx = len(m.filters) - 1
	} else {
		m.filters[m.filtersListIdx] = saved
	}

	if err := writeSavedFilters(m.filters); err != nil {
		m.errMsg = err.Error()
		m.state = stateError
		return m, nil
	}
	m.infoMsg = "saved fragment " + name
	m.errMsg = ""
	m.state = stateIdle
	m.filtersMode = filtersModeList
	return m, nil
}

// --- View rendering -------------------------------------------------------

func (m Model) viewFilters() string {
	var sections []string
	sections = append(sections, m.renderTabs())

	listBorder := paneBorder
	if m.filtersMode == filtersModeList {
		listBorder = paneBorderFocused
	}
	sections = append(sections, listBorder.Render(m.renderFiltersList()))

	if m.filtersMode == filtersModeEditing {
		sections = append(sections, m.renderFilterEditForm()...)
	} else {
		sections = append(sections, paneTitle.Render("Preview"))
		sections = append(sections, paneBorder.Render(m.renderFilterPreview()))
	}

	sections = append(sections, m.filtersStatusLine())
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderFiltersList() string {
	if len(m.filters) == 0 {
		return "    (none — press n to create one)"
	}
	var b strings.Builder
	nameStyle := lipgloss.NewStyle().Bold(true)
	for i, f := range m.filters {
		cursor := "  "
		if i == m.filtersListIdx {
			cursor = "▶ "
		}
		line := fmt.Sprintf("%s — %s", nameStyle.Render(f.Name), truncate(f.Template, 60))
		if i == m.filtersListIdx && m.filtersMode == filtersModeList {
			line = lipgloss.NewStyle().Foreground(colorAccent).Render(line)
		}
		b.WriteString(cursor + line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderFilterPreview() string {
	if len(m.filters) == 0 {
		return ""
	}
	f := m.filters[m.filtersListIdx]
	var b strings.Builder
	b.WriteString(f.Template)
	if len(f.Suggestions) > 0 {
		b.WriteString("\n\n")
		mutedStyle := lipgloss.NewStyle().Foreground(colorMuted)
		for _, n := range dql.Placeholders(f.Template) {
			items := f.Suggestions[n]
			if len(items) == 0 {
				continue
			}
			b.WriteString(mutedStyle.Render("$" + n + ": " + strings.Join(items, ", ")))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderFilterEditForm() []string {
	out := []string{}

	titleFor := func(focused bool, title string) lipgloss.Style {
		if focused {
			return paneTitleFocused
		}
		_ = title
		return paneTitle
	}
	borderFor := func(focused bool) lipgloss.Style {
		if focused {
			return paneBorderFocused
		}
		return paneBorder
	}

	out = append(out, titleFor(m.filterEditFocus == 0, "Name").Render("Name"))
	out = append(out, borderFor(m.filterEditFocus == 0).Render(m.filterEditNameInput.View()))
	out = append(out, titleFor(m.filterEditFocus == 1, "Template").Render("Template"))
	out = append(out, borderFor(m.filterEditFocus == 1).Render(m.filterEditTemplate.View()))

	for i, n := range m.filterEditPlaceholders {
		focused := m.filterEditFocus == 2+i
		title := "Suggestions: $" + n
		out = append(out, titleFor(focused, title).Render(title))
		out = append(out, borderFor(focused).Render(m.filterEditSuggestions[i].View()))
	}
	return out
}

func (m Model) filtersStatusLine() string {
	left := ""
	switch m.state {
	case stateError:
		left = errorText.Render("error: " + m.errMsg)
	case stateIdle:
		if m.infoMsg != "" {
			left = okText.Render(m.infoMsg)
		}
	}
	var right string
	if m.filtersMode == filtersModeEditing {
		right = "Tab switch · Ctrl-S save · Esc cancel"
	} else {
		right = "↑/↓ select · n new · e edit · Enter insert · d delete · Alt-1 query"
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return statusBar.Render(left + strings.Repeat(" ", gap) + right)
}
