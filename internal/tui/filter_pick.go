package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kantis/dynatrace-tui/internal/dql"
)

// openPickFilter starts the Ctrl-F flow from the editor. With no filters
// configured the modal collapses into a one-line info message rather than
// showing an empty list.
func (m Model) openPickFilter() Model {
	if len(m.filters) == 0 {
		m.infoMsg = "no favorite filters — press Alt-3 to create one"
		m.state = stateIdle
		return m
	}
	m.modal = modalPickFilter
	m.pickFilterIdx = 0
	return m
}

// pickFilter is the post-selection branch shared between the Ctrl-F modal
// and the Alt-3 list (Enter on a row): if the filter has placeholders, open
// the resolve modal; otherwise insert directly.
func (m Model) pickFilter(f SavedFilter) Model {
	names := dql.Placeholders(f.Template)
	if len(names) == 0 {
		return m.insertFilterIntoEditor(f.Template)
	}

	inputs := make([]textinput.Model, len(names))
	for i, n := range names {
		ti := textinput.New()
		ti.Placeholder = "$" + n + " (blank = leave literal)"
		ti.CharLimit = 256
		ti.Width = 40
		if i == 0 {
			ti.Focus()
		}
		inputs[i] = ti
	}
	m.resolveFilter = f
	m.resolveNames = names
	m.resolveInputs = inputs
	m.resolveSugIdx = make([]int, len(names))
	for i := range m.resolveSugIdx {
		m.resolveSugIdx[i] = -1 // -1 = nothing selected; user is free-typing
	}
	m.resolveFocus = 0
	m.modal = modalResolveFilter
	return m
}

func (m Model) updatePickFilter(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
		return m, nil
	case "up", "k":
		if m.pickFilterIdx > 0 {
			m.pickFilterIdx--
		}
		return m, nil
	case "down", "j":
		if m.pickFilterIdx < len(m.filters)-1 {
			m.pickFilterIdx++
		}
		return m, nil
	case "enter":
		if len(m.filters) == 0 {
			m.modal = modalNone
			return m, nil
		}
		// pickFilter may switch us into the resolve modal or close the
		// modal entirely (direct insert).
		next := m.pickFilter(m.filters[m.pickFilterIdx])
		return next, textinput.Blink
	}
	return m, nil
}

func (m Model) viewPickFilter() string {
	var b strings.Builder
	b.WriteString(paneTitleFocused.Render("Insert favorite filter"))
	b.WriteString("\n\n")
	if len(m.filters) == 0 {
		b.WriteString("(no filters — press Alt-3 to create one)\n\n")
		b.WriteString(statusBar.Render("Esc close"))
		return m.renderModalOverlay(b.String())
	}
	nameStyle := lipgloss.NewStyle().Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(colorMuted)
	for i, f := range m.filters {
		prefix := "  "
		line := nameStyle.Render(f.Name) + "  " + mutedStyle.Render(truncate(f.Template, 50))
		if i == m.pickFilterIdx {
			prefix = "▶ "
			line = lipgloss.NewStyle().Foreground(colorAccent).Render(f.Name) + "  " + mutedStyle.Render(truncate(f.Template, 50))
		}
		b.WriteString(prefix + line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(statusBar.Render("↑/↓ select · Enter insert · Esc cancel"))
	return m.renderModalOverlay(b.String())
}

// --- Resolve filter modal -------------------------------------------------

func (m Model) updateResolveFilter(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
		m.resolveInputs = nil
		m.resolveNames = nil
		m.resolveSugIdx = nil
		return m, nil
	case "tab":
		return m.cycleResolveFocus(false), nil
	case "shift+tab":
		return m.cycleResolveFocus(true), nil
	case "up":
		return m.cycleResolveSuggestion(-1), nil
	case "down":
		return m.cycleResolveSuggestion(+1), nil
	case "enter", "ctrl+enter":
		return m.confirmResolveFilter()
	}

	// Free typing on the focused input clears the suggestion-cycle pointer
	// for that placeholder so subsequent ↑/↓ starts from the top again.
	if m.resolveFocus < len(m.resolveSugIdx) {
		m.resolveSugIdx[m.resolveFocus] = -1
	}
	var cmd tea.Cmd
	m.resolveInputs[m.resolveFocus], cmd = m.resolveInputs[m.resolveFocus].Update(msg)
	return m, cmd
}

func (m Model) cycleResolveFocus(reverse bool) Model {
	if len(m.resolveInputs) == 0 {
		return m
	}
	m.resolveInputs[m.resolveFocus].Blur()
	if reverse {
		m.resolveFocus = (m.resolveFocus - 1 + len(m.resolveInputs)) % len(m.resolveInputs)
	} else {
		m.resolveFocus = (m.resolveFocus + 1) % len(m.resolveInputs)
	}
	m.resolveInputs[m.resolveFocus].Focus()
	return m
}

// cycleResolveSuggestion moves through the suggestion list for the focused
// placeholder, setting the input value to the picked entry. dir is +1 / -1.
func (m Model) cycleResolveSuggestion(dir int) Model {
	if m.resolveFocus >= len(m.resolveNames) {
		return m
	}
	name := m.resolveNames[m.resolveFocus]
	suggestions := m.resolveFilter.Suggestions[name]
	if len(suggestions) == 0 {
		return m
	}
	idx := m.resolveSugIdx[m.resolveFocus] + dir
	if idx < 0 {
		idx = len(suggestions) - 1
	}
	if idx >= len(suggestions) {
		idx = 0
	}
	m.resolveSugIdx[m.resolveFocus] = idx
	m.resolveInputs[m.resolveFocus].SetValue(suggestions[idx])
	m.resolveInputs[m.resolveFocus].CursorEnd()
	return m
}

func (m Model) confirmResolveFilter() (Model, tea.Cmd) {
	values := map[string]string{}
	for i, n := range m.resolveNames {
		v := strings.TrimSpace(m.resolveInputs[i].Value())
		if v != "" {
			values[n] = v
		}
	}
	substituted := dql.Substitute(m.resolveFilter.Template, values)
	m = m.insertFilterIntoEditor(substituted)
	m.resolveInputs = nil
	m.resolveNames = nil
	m.resolveSugIdx = nil
	return m, nil
}

func (m Model) viewResolveFilter() string {
	var b strings.Builder
	b.WriteString(paneTitleFocused.Render("Resolve placeholders — " + m.resolveFilter.Name))
	b.WriteString("\n\n")

	labelStyle := lipgloss.NewStyle().Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(colorMuted)
	highlightStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	for i, name := range m.resolveNames {
		b.WriteString(labelStyle.Render("$" + name))
		b.WriteString("\n")
		b.WriteString(m.resolveInputs[i].View())
		b.WriteString("\n")
		suggestions := m.resolveFilter.Suggestions[name]
		if len(suggestions) > 0 {
			selected := -1
			if i < len(m.resolveSugIdx) {
				selected = m.resolveSugIdx[i]
			}
			for j, s := range suggestions {
				prefix := "    "
				line := mutedStyle.Render("• " + s)
				if j == selected {
					prefix = "  ▶ "
					line = highlightStyle.Render("• " + s)
				}
				b.WriteString(prefix + line + "\n")
			}
		} else {
			b.WriteString(mutedStyle.Render("    (no suggestions)") + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(statusBar.Render("Tab next · ↑/↓ pick suggestion · Enter insert · Esc cancel"))
	return m.renderModalOverlay(b.String())
}

// insertFilterIntoEditor appends the (already-substituted) filter fragment
// to the current editor body as a new pipe stage. The leading `filter ` is
// added automatically when the fragment doesn't already start with one, so
// users can keep their templates as bare predicates. Closes any open modal
// and switches back to the query view so focus lands on the editor.
func (m Model) insertFilterIntoEditor(fragment string) Model {
	fragment = applyFilterPrefix(fragment)
	body := strings.TrimRight(m.editor.Value(), " \n")
	if body == "" {
		m.editor.SetValue(fragment)
	} else {
		m.editor.SetValue(body + "\n| " + fragment)
	}
	m.modal = modalNone
	m.currentView = viewQuery
	m.focus = focusEditor
	m.editor.Focus()
	m.infoMsg = "inserted filter"
	m.state = stateIdle
	return m
}

// applyFilterPrefix prepends `filter ` to a template fragment unless one is
// already present. Trims surrounding whitespace; case-sensitive to match DQL.
func applyFilterPrefix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, "filter ") || strings.HasPrefix(s, "filter\t") {
		return s
	}
	return "filter " + s
}
