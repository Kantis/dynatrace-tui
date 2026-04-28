package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kantis/dynatrace-tui/internal/dql"
)

func (m *Model) prepareTemplate() bool {
	names := dql.Placeholders(m.editor.Value())
	if len(names) == 0 {
		m.infoMsg = "no $placeholders in query"
		m.state = stateIdle
		return false
	}
	m.templateNames = names
	m.templateInputs = make([]textinput.Model, len(names))
	for i, n := range names {
		ti := textinput.New()
		ti.Placeholder = "value for $" + n
		ti.CharLimit = 256
		ti.Width = 40
		if i == 0 {
			ti.Focus()
		}
		m.templateInputs[i] = ti
	}
	m.templateIdx = 0
	return true
}

func (m Model) updateTemplate(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
		m.templateInputs = nil
		m.templateNames = nil
		return m, nil
	case "tab", "down":
		return m.cycleTemplate(false), nil
	case "shift+tab", "up":
		return m.cycleTemplate(true), nil
	case "enter":
		if m.templateIdx < len(m.templateInputs)-1 {
			return m.cycleTemplate(false), nil
		}
		// Submit
		values := map[string]string{}
		for i, n := range m.templateNames {
			values[n] = m.templateInputs[i].Value()
		}
		m.editor.SetValue(dql.Substitute(m.editor.Value(), values))
		m.modal = modalNone
		m.templateInputs = nil
		m.templateNames = nil
		m.infoMsg = "template substituted"
		m.state = stateIdle
		return m, nil
	}
	var cmd tea.Cmd
	m.templateInputs[m.templateIdx], cmd = m.templateInputs[m.templateIdx].Update(msg)
	return m, cmd
}

func (m Model) cycleTemplate(reverse bool) Model {
	if len(m.templateInputs) == 0 {
		return m
	}
	m.templateInputs[m.templateIdx].Blur()
	if reverse {
		m.templateIdx = (m.templateIdx - 1 + len(m.templateInputs)) % len(m.templateInputs)
	} else {
		m.templateIdx = (m.templateIdx + 1) % len(m.templateInputs)
	}
	m.templateInputs[m.templateIdx].Focus()
	return m
}

func (m Model) viewTemplate() string {
	var b strings.Builder
	b.WriteString(paneTitleFocused.Render("Template parameters"))
	b.WriteString("\n\n")
	for i, ti := range m.templateInputs {
		label := lipgloss.NewStyle().Bold(true).Render("$" + m.templateNames[i])
		b.WriteString(label + "\n")
		b.WriteString(ti.View() + "\n\n")
	}
	b.WriteString(statusBar.Render("Tab next · Enter submit (on last) · Esc cancel"))
	return m.renderModalOverlay(b.String())
}
