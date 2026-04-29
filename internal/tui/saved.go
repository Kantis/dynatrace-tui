package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/kantis/dynatrace-tui/internal/dql"
)

type SavedQuery struct {
	Name  string `yaml:"name"`
	Query string `yaml:"query"`
}

// savedFile is the on-disk shape of searches.yaml.
//
// `default` (optional) names the entry to auto-load and run when the TUI
// starts. An empty/missing value means "no default — start with the
// usual `from:now()-15m` editor body".
type savedFile struct {
	Default  string       `yaml:"default,omitempty"`
	Searches []SavedQuery `yaml:"searches"`
}

func savedQueriesPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "dynatrace-tui", "searches.yaml"), nil
}

// loadSavedQueries returns the saved entries and the name of the default
// (or "" if none). A missing file is treated as an empty list with no
// default — first-run users see the usual placeholder.
func loadSavedQueries() ([]SavedQuery, string, error) {
	p, err := savedQueriesPath()
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", err
	}
	var f savedFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, "", err
	}
	// Drop a stale default (entry was deleted out-of-band) so callers
	// can rely on `default` always pointing at a real entry.
	if f.Default != "" {
		found := false
		for _, q := range f.Searches {
			if q.Name == f.Default {
				found = true
				break
			}
		}
		if !found {
			f.Default = ""
		}
	}
	return f.Searches, f.Default, nil
}

func writeSavedQueries(qs []SavedQuery, defaultName string) error {
	p, err := savedQueriesPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(savedFile{Default: defaultName, Searches: qs})
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// --- Save modal -----------------------------------------------------------

func newSaveInput(defaultName string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "name (e.g. errors-last-hour)"
	ti.SetValue(defaultName)
	ti.Focus()
	ti.CharLimit = 64
	ti.Width = 40
	return ti
}

func (m Model) updateSaveQuery(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.saveInput.Value())
		if name == "" {
			return m, nil
		}
		query := strings.TrimSpace(m.editor.Value())
		if query == "" {
			m.modal = modalNone
			m.errMsg = "nothing to save (editor empty)"
			m.state = stateError
			return m, nil
		}
		// Replace existing entry with the same name.
		updated := false
		for i := range m.savedQueries {
			if m.savedQueries[i].Name == name {
				m.savedQueries[i].Query = query
				updated = true
				break
			}
		}
		if !updated {
			m.savedQueries = append(m.savedQueries, SavedQuery{Name: name, Query: query})
		}
		if err := writeSavedQueries(m.savedQueries, m.savedDefault); err != nil {
			m.errMsg = err.Error()
			m.state = stateError
		} else {
			m.infoMsg = "saved as " + name
			m.state = stateIdle
		}
		m.modal = modalNone
		return m, nil
	}
	var cmd tea.Cmd
	m.saveInput, cmd = m.saveInput.Update(msg)
	return m, cmd
}

func (m Model) viewSaveQuery() string {
	var b strings.Builder
	b.WriteString(paneTitleFocused.Render("Save query"))
	b.WriteString("\n\n")
	b.WriteString(m.saveInput.View())
	b.WriteString("\n\n")
	b.WriteString(statusBar.Render("Enter save · Esc cancel"))
	return m.renderModalOverlay(b.String())
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// --- Saved Searches view (Alt-2) ------------------------------------------

type savedSearchesMode int

const (
	savedModeList savedSearchesMode = iota
	savedModeEditing
)

type savedEditFocus int

const (
	savedEditFocusName savedEditFocus = iota
	savedEditFocusBody
)

// enterSavedView switches into the saved-searches view, clamping the cursor
// to a valid row and resetting to list mode.
func (m Model) enterSavedView() Model {
	m.currentView = viewSaved
	m.savedMode = savedModeList
	if m.savedListIdx < 0 {
		m.savedListIdx = 0
	}
	if m.savedListIdx >= len(m.savedQueries) && len(m.savedQueries) > 0 {
		m.savedListIdx = len(m.savedQueries) - 1
	}
	return m
}

func (m Model) updateSavedView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Filter pick/resolve modals can be opened from saved-edit mode (Ctrl-F).
	// Route them here so the modal owns the keys until it closes.
	switch m.modal {
	case modalPickFilter:
		next, cmd := m.updatePickFilter(msg)
		return next, cmd
	case modalResolveFilter:
		next, cmd := m.updateResolveFilter(msg)
		return next, cmd
	}
	if m.savedMode == savedModeEditing {
		return m.updateSavedEdit(msg)
	}
	return m.updateSavedList(msg)
}

func (m Model) updateSavedList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		return m, tea.Quit
	case "up", "k":
		if m.savedListIdx > 0 {
			m.savedListIdx--
		}
	case "down", "j":
		if m.savedListIdx < len(m.savedQueries)-1 {
			m.savedListIdx++
		}
	case "enter":
		if len(m.savedQueries) == 0 {
			return m, nil
		}
		sel := m.savedQueries[m.savedListIdx]
		m.editor.SetValue(dql.StripFetch(sel.Query))
		m.currentView = viewQuery
		return m.runQuery()
	case "d":
		if len(m.savedQueries) == 0 {
			return m, nil
		}
		removed := m.savedQueries[m.savedListIdx].Name
		m.savedQueries = append(m.savedQueries[:m.savedListIdx], m.savedQueries[m.savedListIdx+1:]...)
		if m.savedListIdx >= len(m.savedQueries) && m.savedListIdx > 0 {
			m.savedListIdx--
		}
		if m.savedDefault == removed {
			m.savedDefault = ""
		}
		if err := writeSavedQueries(m.savedQueries, m.savedDefault); err != nil {
			m.errMsg = err.Error()
			m.state = stateError
		}
	case "*":
		if len(m.savedQueries) == 0 {
			return m, nil
		}
		sel := m.savedQueries[m.savedListIdx].Name
		if m.savedDefault == sel {
			m.savedDefault = ""
			m.infoMsg = "default cleared"
		} else {
			m.savedDefault = sel
			m.infoMsg = "default → " + sel
		}
		m.state = stateIdle
		if err := writeSavedQueries(m.savedQueries, m.savedDefault); err != nil {
			m.errMsg = err.Error()
			m.state = stateError
		}
	case "e":
		if len(m.savedQueries) == 0 {
			return m, nil
		}
		sel := m.savedQueries[m.savedListIdx]
		ti := textinput.New()
		ti.Placeholder = "name"
		ti.SetValue(sel.Name)
		ti.CharLimit = 64
		ti.Width = 40
		ti.Focus()
		m.savedEditNameInput = ti
		body := NewEditor(m.savedEditBody.Vim())
		body.SetValue(dql.StripFetch(sel.Query))
		body.Blur()
		m.savedEditBody = body
		m.savedEditOriginalName = sel.Name
		m.savedEditFocus = savedEditFocusName
		m.savedMode = savedModeEditing
		m.errMsg = ""
		m.state = stateIdle
		// Apply layout sizing to the new editor.
		m.applyLayout()
		return m, textinput.Blink
	}
	return m, nil
}

func (m Model) updateSavedEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.savedMode = savedModeList
		m.errMsg = ""
		return m, nil
	case "tab":
		m = m.cycleSavedEditFocus(false)
		return m, nil
	case "shift+tab":
		m = m.cycleSavedEditFocus(true)
		return m, nil
	case "ctrl+f":
		// Inserts into the saved-search body just like Ctrl-F does in the
		// query editor; only meaningful when the body has focus, otherwise
		// the keypress would interrupt name editing.
		if m.savedEditFocus == savedEditFocusBody {
			return m.openPickFilter(), nil
		}
		return m, nil
	case "ctrl+s":
		name := strings.TrimSpace(m.savedEditNameInput.Value())
		if name == "" {
			m.errMsg = "name cannot be empty"
			m.state = stateError
			return m, nil
		}
		body := strings.TrimSpace(m.savedEditBody.Value())
		if body == "" {
			m.errMsg = "query cannot be empty"
			m.state = stateError
			return m, nil
		}
		// Collision check on rename.
		if name != m.savedEditOriginalName {
			for i, q := range m.savedQueries {
				if i == m.savedListIdx {
					continue
				}
				if q.Name == name {
					m.errMsg = "another saved search already uses that name"
					m.state = stateError
					return m, nil
				}
			}
		}
		m.savedQueries[m.savedListIdx] = SavedQuery{Name: name, Query: dql.PrependFetch(body)}
		// Carry the default marker through a rename.
		if m.savedDefault == m.savedEditOriginalName && name != m.savedEditOriginalName {
			m.savedDefault = name
		}
		if err := writeSavedQueries(m.savedQueries, m.savedDefault); err != nil {
			m.errMsg = err.Error()
			m.state = stateError
			return m, nil
		}
		m.infoMsg = "saved " + name
		m.errMsg = ""
		m.state = stateIdle
		m.savedMode = savedModeList
		return m, nil
	}

	// Route key to focused widget.
	var cmd tea.Cmd
	if m.savedEditFocus == savedEditFocusName {
		m.savedEditNameInput, cmd = m.savedEditNameInput.Update(msg)
	} else {
		m.savedEditBody, cmd = m.savedEditBody.Update(msg)
	}
	return m, cmd
}

func (m Model) cycleSavedEditFocus(reverse bool) Model {
	_ = reverse // only two fields, direction doesn't matter
	if m.savedEditFocus == savedEditFocusName {
		m.savedEditNameInput.Blur()
		m.savedEditFocus = savedEditFocusBody
		m.savedEditBody.Focus()
	} else {
		m.savedEditBody.Blur()
		m.savedEditFocus = savedEditFocusName
		m.savedEditNameInput.Focus()
	}
	return m
}

func (m Model) viewSavedSearches() string {
	switch m.modal {
	case modalPickFilter:
		return m.viewPickFilter()
	case modalResolveFilter:
		return m.viewResolveFilter()
	}
	var sections []string
	sections = append(sections, m.renderTabs())

	listBorder := paneBorder
	if m.savedMode == savedModeList {
		listBorder = paneBorderFocused
	}
	sections = append(sections, listBorder.Render(m.renderSavedList()))

	if m.savedMode == savedModeEditing {
		sections = append(sections, m.renderSavedEditForm()...)
	} else {
		sections = append(sections, paneTitle.Render("Preview"))
		sections = append(sections, paneBorder.Render(m.renderSavedPreview()))
	}

	sections = append(sections, m.savedStatusLine())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderSavedList() string {
	if len(m.savedQueries) == 0 {
		return "    (none — Alt-1 to query view, Ctrl-S there to save the current query)"
	}
	var b strings.Builder
	nameStyle := lipgloss.NewStyle().Bold(true)
	defaultStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	for i, q := range m.savedQueries {
		defaultMark := "  "
		if q.Name == m.savedDefault {
			defaultMark = defaultStyle.Render("★ ")
		}
		cursorMark := "  "
		if i == m.savedListIdx {
			cursorMark = "▶ "
		}
		querySnippet := truncate(q.Query, 60)
		line := fmt.Sprintf("%s — %s", nameStyle.Render(q.Name), querySnippet)
		if i == m.savedListIdx && m.savedMode == savedModeList {
			line = lipgloss.NewStyle().Foreground(colorAccent).Render(line)
		}
		b.WriteString(defaultMark + cursorMark + line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderSavedPreview() string {
	if len(m.savedQueries) == 0 {
		return ""
	}
	sel := m.savedQueries[m.savedListIdx]
	return dql.StripFetch(sel.Query)
}

func (m Model) renderSavedEditForm() []string {
	nameFocused := m.savedEditFocus == savedEditFocusName
	bodyFocused := m.savedEditFocus == savedEditFocusBody

	nameTitle := "Name"
	nameTitleStyle := paneTitle
	nameBorder := paneBorder
	if nameFocused {
		nameTitleStyle = paneTitleFocused
		nameBorder = paneBorderFocused
	}

	bodyTitle := fmt.Sprintf("Query [%s]", m.savedEditBody.Mode())
	bodyTitleStyle := paneTitle
	bodyBorder := paneBorder
	if bodyFocused {
		bodyTitleStyle = paneTitleFocused
		bodyBorder = paneBorderFocused
	}

	return []string{
		nameTitleStyle.Render(nameTitle),
		nameBorder.Render(m.savedEditNameInput.View()),
		bodyTitleStyle.Render(bodyTitle),
		bodyBorder.Render(m.savedEditBody.View()),
	}
}

func (m Model) savedStatusLine() string {
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
	if m.savedMode == savedModeEditing {
		right = "Tab switch · Ctrl-F insert filter · Ctrl-S save · Esc cancel"
	} else {
		right = "↑/↓ select · e edit · Enter run · * default · d delete · Alt-1 query"
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return statusBar.Render(left + strings.Repeat(" ", gap) + right)
}
