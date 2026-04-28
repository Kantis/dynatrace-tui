package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kantis/dynatrace-tui/internal/dql"
)

func (m Model) updateTimeRange(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
		return m, nil
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
		newDQL, err := dql.ApplyTimeframe(m.editor.Value(), tf)
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

func (m Model) viewTimeRange() string {
	var b strings.Builder
	b.WriteString(paneTitleFocused.Render("Time range"))
	b.WriteString("\n\n")
	for i, tf := range dql.ValidTimeframes {
		prefix := "  "
		line := tf
		if i == m.timeRangeIdx {
			prefix = "▶ "
			line = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(line)
		}
		b.WriteString(prefix + line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(statusBar.Render("↑/↓ select · Enter apply · Esc cancel"))
	return m.renderModalOverlay(b.String())
}
