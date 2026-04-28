package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kantis/dynatrace-tui/internal/dql"
)

const (
	tfFocusFrom = 0
	tfFocusTo   = 1
)

const timeDisplayLayout = "2006-01-02 15:04:05"

type timePick struct {
	label string
	value string
}

func newTimeInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 64
	ti.Width = 28
	return ti
}

func fromPicks(opened time.Time) []timePick {
	hourStart := opened.Truncate(time.Hour).Format(timeDisplayLayout)
	return []timePick{
		{label: "now()-15m", value: "now()-15m"},
		{label: "now()-1h", value: "now()-1h"},
		{label: "now()-6h", value: "now()-6h"},
		{label: "now()-24h", value: "now()-24h"},
		{label: hourStart, value: hourStart},
	}
}

func toPicks(opened time.Time) []timePick {
	openedStr := opened.Format(timeDisplayLayout)
	return []timePick{
		{label: "now()  (= " + openedStr + ")", value: openedStr},
	}
}

func (m Model) updateTimeRange(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
		return m, nil
	case "tab":
		return m.cycleTimeFocus(false), textinput.Blink
	case "shift+tab":
		return m.cycleTimeFocus(true), textinput.Blink
	case "ctrl+@", "ctrl+enter", "alt+enter":
		return m.applyTimeRange()
	}

	// Nudge keys take precedence over typed input. Datetime entry uses only
	// digits and symbols, so the letters below are free.
	if delta, ok := nudgeDelta(msg.String()); ok {
		input := m.focusedInput()
		if input != nil {
			nudge(input, m.timeRangeOpened, delta)
		}
		return m, nil
	}

	picks := m.focusedPicks()
	idx := m.focusedPickIdx()

	switch msg.String() {
	case "up", "k":
		if *idx > 0 {
			*idx--
		}
		return m, nil
	case "down", "j":
		if *idx < len(picks)-1 {
			*idx++
		}
		return m, nil
	case "enter":
		if len(picks) > 0 && *idx >= 0 && *idx < len(picks) {
			input := m.focusedInput()
			input.SetValue(picks[*idx].value)
			input.SetCursor(len(picks[*idx].value))
		}
		return m, nil
	}

	// Forward to focused textinput.
	var cmd tea.Cmd
	if m.timeRangeFocus == tfFocusFrom {
		m.timeFromInput, cmd = m.timeFromInput.Update(msg)
	} else {
		m.timeToInput, cmd = m.timeToInput.Update(msg)
	}
	return m, cmd
}

func (m *Model) focusedInput() *textinput.Model {
	if m.timeRangeFocus == tfFocusFrom {
		return &m.timeFromInput
	}
	return &m.timeToInput
}

func (m Model) focusedPicks() []timePick {
	if m.timeRangeFocus == tfFocusFrom {
		return fromPicks(m.timeRangeOpened)
	}
	return toPicks(m.timeRangeOpened)
}

func (m *Model) focusedPickIdx() *int {
	if m.timeRangeFocus == tfFocusFrom {
		return &m.timeFromIdx
	}
	return &m.timeToIdx
}

func (m Model) cycleTimeFocus(reverse bool) Model {
	const n = 2
	if reverse {
		m.timeRangeFocus = (m.timeRangeFocus + n - 1) % n
	} else {
		m.timeRangeFocus = (m.timeRangeFocus + 1) % n
	}
	m.refreshTimeFocus()
	return m
}

func (m *Model) refreshTimeFocus() {
	m.timeFromInput.Blur()
	m.timeToInput.Blur()
	switch m.timeRangeFocus {
	case tfFocusFrom:
		m.timeFromInput.Focus()
	case tfFocusTo:
		m.timeToInput.Focus()
	}
}

// nudgeDelta maps single-key nudge bindings to a duration delta. Lowercase
// advances time, uppercase rewinds.
func nudgeDelta(key string) (time.Duration, bool) {
	switch key {
	case "h":
		return time.Hour, true
	case "H":
		return -time.Hour, true
	case "m":
		return time.Minute, true
	case "M":
		return -time.Minute, true
	case "s":
		return time.Second, true
	case "S":
		return -time.Second, true
	case "d":
		return 24 * time.Hour, true
	case "D":
		return -24 * time.Hour, true
	}
	return 0, false
}

func nudge(input *textinput.Model, opened time.Time, delta time.Duration) {
	t := resolveValue(input.Value(), opened)
	t = t.Add(delta)
	s := t.Format(timeDisplayLayout)
	input.SetValue(s)
	input.SetCursor(len(s))
}

var relativeOffsetRE = regexp.MustCompile(`^now\(\)\s*-\s*(\d+)([smhdw])$`)

// resolveValue turns a field's string value into an absolute time. Empty input
// resolves to opened so a fresh nudge starts from "now". Unparseable input
// also snaps to opened so the user can recover with another keystroke.
func resolveValue(s string, opened time.Time) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return opened
	}
	if d, ok := parseRelativeOffset(s); ok {
		return opened.Add(-d)
	}
	t, err := dql.ParseFlexibleTime(s, false)
	if err != nil {
		return opened
	}
	return t
}

func parseRelativeOffset(s string) (time.Duration, bool) {
	m := relativeOffsetRE.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	var unit time.Duration
	switch m[2] {
	case "s":
		unit = time.Second
	case "m":
		unit = time.Minute
	case "h":
		unit = time.Hour
	case "d":
		unit = 24 * time.Hour
	case "w":
		unit = 7 * 24 * time.Hour
	default:
		return 0, false
	}
	var n int
	if _, err := fmt.Sscanf(m[1], "%d", &n); err != nil {
		return 0, false
	}
	return time.Duration(n) * unit, true
}

// matchCanonicalRelative returns the timeframe ("15m", "1h", ...) when s is
// `now()-<tf>` for a tf in dql.ValidTimeframes — letting applyTimeRange keep
// the relative form in the query instead of resolving to absolute.
func matchCanonicalRelative(s string) (string, bool) {
	m := relativeOffsetRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", false
	}
	tf := m[1] + m[2]
	if dql.IsValidTimeframe(tf) {
		return tf, true
	}
	return "", false
}

func (m Model) applyTimeRange() (Model, tea.Cmd) {
	fromStr := strings.TrimSpace(m.timeFromInput.Value())
	toStr := strings.TrimSpace(m.timeToInput.Value())

	if fromStr == "" {
		m.errMsg = "from time is required"
		m.state = stateError
		m.modal = modalNone
		return m, nil
	}

	// Fast path: canonical relative From + empty To → keep `from:now()-Xy`.
	if toStr == "" {
		if tf, ok := matchCanonicalRelative(fromStr); ok {
			newDQL, err := dql.SubstituteTimeframe(dql.PrependFetch(m.editor.Value()), tf)
			if err != nil {
				m.errMsg = err.Error()
				m.state = stateError
				m.modal = modalNone
				return m, nil
			}
			m.editor.SetValue(dql.StripFetch(newDQL))
			m.modal = modalNone
			m.infoMsg = "applied timeframe " + tf
			m.state = stateIdle
			if strings.TrimSpace(m.editor.Value()) != "" {
				model, cmd := m.runQuery()
				return model.(Model), cmd
			}
			return m, nil
		}
	}

	// Absolute path.
	fromT := resolveValue(fromStr, m.timeRangeOpened)
	hasTo := toStr != ""
	var toT time.Time
	if hasTo {
		toT = resolveValue(toStr, m.timeRangeOpened)
	}
	newDQL := dql.SubstituteAbsolute(dql.PrependFetch(m.editor.Value()), fromT, toT, hasTo)
	m.editor.SetValue(dql.StripFetch(newDQL))
	m.modal = modalNone
	if hasTo {
		m.infoMsg = "applied absolute range"
	} else {
		m.infoMsg = "applied absolute from"
	}
	m.state = stateIdle
	if strings.TrimSpace(m.editor.Value()) != "" {
		model, cmd := m.runQuery()
		return model.(Model), cmd
	}
	return m, nil
}

func (m Model) viewTimeRange() string {
	var b strings.Builder
	b.WriteString(paneTitleFocused.Render("Time range"))
	b.WriteString("\n\n")

	fromCol := renderTimeColumn("From", fromPicks(m.timeRangeOpened), m.timeFromIdx, m.timeFromInput, m.timeRangeFocus == tfFocusFrom)
	toCol := renderTimeColumn("To", toPicks(m.timeRangeOpened), m.timeToIdx, m.timeToInput, m.timeRangeFocus == tfFocusTo)

	colSep := "    "
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, fromCol, colSep, toCol))
	b.WriteString("\n\n")

	b.WriteString(statusBar.Render("Tab switch · j/k pick · Enter apply pick · h/H ±1h · m/M ±1m · s/S ±1s · d/D ±1d · Ctrl-Enter run · Esc cancel"))
	return m.renderModalOverlay(b.String())
}

func renderTimeColumn(label string, picks []timePick, idx int, input textinput.Model, focused bool) string {
	var b strings.Builder
	b.WriteString(sectionHeader(label, focused))
	b.WriteString("\n")
	for i, p := range picks {
		prefix := "  "
		line := p.label
		if focused && i == idx {
			prefix = "▶ "
			line = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(line)
		}
		b.WriteString("  " + prefix + line + "\n")
	}
	b.WriteString("  " + strings.Repeat("─", 28) + "\n")
	b.WriteString("  " + input.View())
	return b.String()
}

func sectionHeader(label string, focused bool) string {
	style := lipgloss.NewStyle().Bold(true)
	if focused {
		style = style.Foreground(colorAccent)
		return style.Render("▶ " + label)
	}
	return style.Render("  " + label)
}
