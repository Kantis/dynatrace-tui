package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kantis/dynatrace-tui/internal/dql"
)

// openPickFilter starts the Ctrl-F flow from the editor. With no fragments
// configured the modal collapses into a one-line info message rather than
// showing an empty list.
func (m Model) openPickFilter() Model {
	if len(m.filters) == 0 {
		m.infoMsg = "no fragments — press Alt-3 to create one"
		m.state = stateIdle
		return m
	}
	m.modal = modalPickFilter
	m.pickFilterIdx = 0
	m.pickFilterQuery = ""
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
	matches := m.pickFilterMatches()
	switch msg.String() {
	case "esc":
		m.modal = modalNone
		return m, nil
	case "up":
		if m.pickFilterIdx > 0 {
			m.pickFilterIdx--
		}
		return m, nil
	case "down":
		if m.pickFilterIdx < len(matches)-1 {
			m.pickFilterIdx++
		}
		return m, nil
	case "enter":
		if len(matches) == 0 {
			return m, nil
		}
		// pickFilter may switch us into the resolve modal or close the
		// modal entirely (direct insert).
		next := m.pickFilter(m.filters[matches[m.pickFilterIdx]])
		return next, textinput.Blink
	case "backspace":
		if m.pickFilterQuery != "" {
			m.pickFilterQuery = m.pickFilterQuery[:len(m.pickFilterQuery)-1]
			m.pickFilterIdx = 0
		}
		return m, nil
	case "ctrl+u":
		m.pickFilterQuery = ""
		m.pickFilterIdx = 0
		return m, nil
	}

	// Treat any single printable rune as a search character. msg.Runes is
	// non-empty only for actual text input keys, so this naturally ignores
	// modifier-bearing keys we don't handle above.
	if len(msg.Runes) == 1 && msg.Runes[0] >= 0x20 {
		m.pickFilterQuery += string(msg.Runes)
		m.pickFilterIdx = 0
	}
	return m, nil
}

// pickFilterMatches returns the indices of m.filters that match the current
// fuzzy query, ordered best-match-first. With no query, returns all indices
// in their original order so the picker behaves like a plain list.
func (m Model) pickFilterMatches() []int {
	if strings.TrimSpace(m.pickFilterQuery) == "" {
		out := make([]int, len(m.filters))
		for i := range m.filters {
			out[i] = i
		}
		return out
	}
	type scored struct {
		idx, score int
	}
	var hits []scored
	for i, f := range m.filters {
		// Score against the name first; fall back to the template so users
		// can also find a fragment by something memorable in its body.
		nameOK, nameScore := fuzzyScore(m.pickFilterQuery, f.Name)
		tmplOK, tmplScore := fuzzyScore(m.pickFilterQuery, f.Template)
		switch {
		case nameOK && tmplOK:
			best := nameScore
			if tmplScore > best {
				best = tmplScore
			}
			// Slight bonus when the name itself matches — that's almost
			// always what the user is reaching for.
			if nameScore >= tmplScore {
				best += 5
			}
			hits = append(hits, scored{i, best})
		case nameOK:
			hits = append(hits, scored{i, nameScore + 5})
		case tmplOK:
			hits = append(hits, scored{i, tmplScore})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool {
		return hits[a].score > hits[b].score
	})
	out := make([]int, len(hits))
	for i, h := range hits {
		out[i] = h.idx
	}
	return out
}

// fuzzyScore reports whether needle is a (case-insensitive) subsequence of
// haystack and, if so, a quality score where higher is better. Bonuses for
// matches at word boundaries / starts / consecutive runs reward the kind of
// matches a human reader would consider "obvious".
func fuzzyScore(needle, haystack string) (bool, int) {
	if needle == "" {
		return true, 0
	}
	n := strings.ToLower(needle)
	h := strings.ToLower(haystack)
	score := 0
	consecutive := 0
	prevSep := true // treat string start like a word boundary
	ni := 0
	for hi := 0; hi < len(h) && ni < len(n); hi++ {
		c := h[hi]
		if c == n[ni] {
			if hi == 0 {
				score += 15
			}
			if prevSep {
				score += 10
			}
			if consecutive > 0 {
				score += 5
			}
			consecutive++
			ni++
		} else {
			consecutive = 0
		}
		prevSep = isFuzzySep(c)
	}
	if ni < len(n) {
		return false, 0
	}
	// Mild penalty for longer haystacks so a tight match beats a sparse one.
	score -= len(h) / 8
	return true, score
}

func isFuzzySep(c byte) bool {
	switch c {
	case ' ', '_', '-', '.', '/', '\t':
		return true
	}
	return false
}

// pickFilterTableWidth returns (nameColW, fragmentColW) for the picker table,
// scaled to the available terminal width. The name column is sized to the
// longest fragment name (capped) so it never dominates the row.
func (m Model) pickFilterTableWidth() (int, int) {
	// Modal chrome: 2 border chars + 4 padding chars = 6. A little breathing
	// room around the centred box keeps the table from crowding the edges.
	total := m.width - 10
	if total < 40 {
		total = 40
	}
	if total > 140 {
		total = 140
	}

	nameW := 0
	for _, f := range m.filters {
		if w := len(f.Name); w > nameW {
			nameW = w
		}
	}
	if nameW < len("Name") {
		nameW = len("Name")
	}
	if nameW > 28 {
		nameW = 28
	}
	// 3 chars between columns: " │ "
	fragW := total - nameW - 3
	if fragW < 20 {
		fragW = 20
	}
	return nameW, fragW
}

func (m Model) viewPickFilter() string {
	var b strings.Builder
	b.WriteString(paneTitleFocused.Render("Insert fragment"))
	b.WriteString("\n\n")
	if len(m.filters) == 0 {
		b.WriteString("(no fragments — press Alt-3 to create one)\n\n")
		b.WriteString(statusBar.Render("Esc close"))
		return m.renderModalOverlay(b.String())
	}

	matches := m.pickFilterMatches()
	nameW, fragW := m.pickFilterTableWidth()
	totalW := nameW + 3 + fragW

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	mutedStyle := lipgloss.NewStyle().Foreground(colorMuted)
	selStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(colorAccent)
	sepStyle := mutedStyle

	// Search input line (always visible — gives the cue that you can type).
	searchLabel := mutedStyle.Render("Search: ")
	cursor := lipgloss.NewStyle().Foreground(colorAccent).Render("▏")
	b.WriteString(searchLabel + m.pickFilterQuery + cursor + "\n\n")

	// Header row.
	b.WriteString(headerStyle.Render(padRight("Name", nameW)))
	b.WriteString(sepStyle.Render(" │ "))
	b.WriteString(headerStyle.Render(padRight("Fragment", fragW)))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(strings.Repeat("─", totalW)))
	b.WriteString("\n")

	if len(matches) == 0 {
		b.WriteString(mutedStyle.Render("(no matches)") + "\n\n")
		b.WriteString(statusBar.Render("Type to search · Backspace · Ctrl-U clear · Esc cancel"))
		return m.renderModalOverlay(b.String())
	}

	for i, idx := range matches {
		f := m.filters[idx]
		nameCell := padRight(truncate(f.Name, nameW), nameW)
		fragCell := padRight(truncate(f.Template, fragW), fragW)
		if i == m.pickFilterIdx {
			// Span the selection highlight across the full row width while
			// keeping the column gutter so the Fragment column still lines
			// up with the header.
			b.WriteString(selStyle.Render(nameCell + " │ " + fragCell))
		} else {
			b.WriteString(nameCell + sepStyle.Render(" │ ") + fragCell)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(statusBar.Render("Type to search · ↑/↓ select · Enter insert · Esc cancel"))
	return m.renderModalOverlay(b.String())
}

// padRight pads s with spaces on the right so its visible width is exactly w.
// Used to keep table cells aligned even when content is shorter than the
// column. If s is already wider than w it's returned untouched (truncate
// should have handled that upstream).
func padRight(s string, w int) string {
	vis := lipgloss.Width(s)
	if vis >= w {
		return s
	}
	return s + strings.Repeat(" ", w-vis)
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

// insertFilterIntoEditor appends the (already-substituted) fragment to the
// active editor body as a new pipe stage. The fragment is inserted verbatim
// — the user is responsible for any leading verb (`filter`, `sort`, etc.).
// Closes any open modal.
//
// The active editor depends on where the pick was triggered: from the
// Saved-searches edit form, the fragment lands in the saved-search body
// and the user stays in that view; otherwise it lands in the main query
// editor and the view flips back to viewQuery.
func (m Model) insertFilterIntoEditor(fragment string) Model {
	fragment = strings.TrimSpace(fragment)
	if m.currentView == viewSaved && m.savedMode == savedModeEditing {
		body := strings.TrimRight(m.savedEditBody.Value(), " \n")
		if body == "" {
			m.savedEditBody.SetValue(fragment)
		} else {
			m.savedEditBody.SetValue(body + "\n| " + fragment)
		}
		m.modal = modalNone
		m.savedEditFocus = savedEditFocusBody
		m.savedEditNameInput.Blur()
		m.savedEditBody.Focus()
		m.infoMsg = "inserted fragment"
		m.state = stateIdle
		return m
	}
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
	if m.detailFullscreen {
		m.detailFullscreen = false
		m.applyLayout()
	}
	m.infoMsg = "inserted fragment"
	m.state = stateIdle
	return m
}
