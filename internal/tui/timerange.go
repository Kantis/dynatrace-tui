package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kantis/dynatrace-tui/internal/dql"
)

const (
	tfFocusPresets = 0
	tfFocusFrom    = 1
	tfFocusTo      = 2
)

func newTimeInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 64
	ti.Width = 40
	return ti
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
	}

	switch m.timeRangeFocus {
	case tfFocusPresets:
		return m.updateTimePresets(msg)
	case tfFocusFrom, tfFocusTo:
		if msg.String() == "enter" {
			return m.applyAbsoluteRange()
		}
		var cmd tea.Cmd
		if m.timeRangeFocus == tfFocusFrom {
			m.timeFromInput, cmd = m.timeFromInput.Update(msg)
		} else {
			m.timeToInput, cmd = m.timeToInput.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m Model) updateTimePresets(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.timeRangeIdx > 0 {
			m.timeRangeIdx--
		}
	case "down", "j":
		if m.timeRangeIdx < len(dql.ValidTimeframes)-1 {
			m.timeRangeIdx++
		}
	case "enter":
		tf := dql.ValidTimeframes[m.timeRangeIdx]
		newDQL, err := dql.SubstituteTimeframe(m.editor.Value(), tf)
		if err != nil {
			m.errMsg = err.Error()
			m.state = stateError
			m.modal = modalNone
			return m, nil
		}
		m.editor.SetValue(newDQL)
		m.modal = modalNone
		m.infoMsg = "applied timeframe " + tf
		m.state = stateIdle
		return m, nil
	}
	return m, nil
}

func (m Model) cycleTimeFocus(reverse bool) Model {
	const n = 3
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

func (m Model) applyAbsoluteRange() (Model, tea.Cmd) {
	fromStr := strings.TrimSpace(m.timeFromInput.Value())
	toStr := strings.TrimSpace(m.timeToInput.Value())

	if fromStr == "" {
		m.errMsg = "from time is required"
		m.state = stateError
		m.modal = modalNone
		return m, nil
	}
	fromT, err := dql.ParseFlexibleTime(fromStr, false)
	if err != nil {
		m.errMsg = "from: " + err.Error()
		m.state = stateError
		m.modal = modalNone
		return m, nil
	}

	hasTo := toStr != ""
	var toT = fromT // unused when hasTo is false
	if hasTo {
		toT, err = dql.ParseFlexibleTime(toStr, true)
		if err != nil {
			m.errMsg = "to: " + err.Error()
			m.state = stateError
			m.modal = modalNone
			return m, nil
		}
	}

	newDQL := dql.SubstituteAbsolute(m.editor.Value(), fromT, toT, hasTo)
	m.editor.SetValue(newDQL)
	m.modal = modalNone
	if hasTo {
		m.infoMsg = "applied absolute range"
	} else {
		m.infoMsg = "applied absolute from"
	}
	m.state = stateIdle
	return m, nil
}

func (m Model) viewTimeRange() string {
	var b strings.Builder
	b.WriteString(paneTitleFocused.Render("Time range"))
	b.WriteString("\n\n")

	// Presets section
	b.WriteString(sectionHeader("Presets", m.timeRangeFocus == tfFocusPresets))
	b.WriteString("\n")
	for i, tf := range dql.ValidTimeframes {
		prefix := "  "
		line := tf
		if m.timeRangeFocus == tfFocusPresets && i == m.timeRangeIdx {
			prefix = "▶ "
			line = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(line)
		}
		b.WriteString("  " + prefix + line + "\n")
	}
	b.WriteString("\n")

	// From / To section
	b.WriteString(sectionHeader("From", m.timeRangeFocus == tfFocusFrom))
	b.WriteString("\n  ")
	b.WriteString(m.timeFromInput.View())
	b.WriteString("\n\n")

	b.WriteString(sectionHeader("To  (empty = now)", m.timeRangeFocus == tfFocusTo))
	b.WriteString("\n  ")
	b.WriteString(m.timeToInput.View())
	b.WriteString("\n\n")

	b.WriteString(statusBar.Render("Tab switch · Enter apply · Esc cancel"))
	return m.renderModalOverlay(b.String())
}

func sectionHeader(label string, focused bool) string {
	style := lipgloss.NewStyle().Bold(true)
	if focused {
		style = style.Foreground(colorAccent)
		return style.Render("▶ " + label)
	}
	return style.Render("  " + label)
}
