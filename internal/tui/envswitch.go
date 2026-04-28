package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) updateSwitchEnv(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
		return m, nil
	case "up", "k":
		if m.envSwitchIdx > 0 {
			m.envSwitchIdx--
		}
	case "down", "j":
		if m.envSwitchIdx < len(m.envNames)-1 {
			m.envSwitchIdx++
		}
	case "enter":
		if len(m.envNames) == 0 {
			m.modal = modalNone
			return m, nil
		}
		picked := m.envNames[m.envSwitchIdx]
		if picked == m.envName {
			m.modal = modalNone
			return m, nil
		}
		// Cancel any running query before swapping the client.
		if m.state == stateRunning {
			m = m.cancelRunning()
		}
		client, err := m.makeClient(picked)
		if err != nil {
			m.errMsg = err.Error()
			m.state = stateError
			m.modal = modalNone
			return m, nil
		}
		m.client = client
		m.envName = picked
		// Stale results belong to the previous environment.
		m.records = nil
		m.rowCount = 0
		m.chartRecords = nil
		m.populateTable()
		m.detailKind = detailRecord
		m.detail.SetContent("")
		if m.focus == focusDetail {
			m.focus = focusEditor
			m.editor.Focus()
		}
		m.modal = modalNone
		m.errMsg = ""
		m.infoMsg = "switched to " + picked
		m.state = stateIdle
		return m, nil
	}
	return m, nil
}

func (m Model) viewSwitchEnv() string {
	var b strings.Builder
	b.WriteString(paneTitleFocused.Render("Switch environment"))
	b.WriteString("\n\n")
	if len(m.envNames) == 0 {
		b.WriteString(statusBar.Render("(none configured)"))
		b.WriteString("\n\n")
		b.WriteString(statusBar.Render("Esc close"))
		return m.renderModalOverlay(b.String())
	}
	for i, name := range m.envNames {
		prefix := "  "
		line := lipgloss.NewStyle().Bold(true).Render(name)
		if name == m.envName {
			line += "  (current)"
		}
		if i == m.envSwitchIdx {
			prefix = "▶ "
			line = lipgloss.NewStyle().Foreground(colorAccent).Render(line)
		}
		b.WriteString(prefix + line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(statusBar.Render("↑/↓ select · Enter switch · Esc cancel"))
	return m.renderModalOverlay(b.String())
}
