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
)

type SavedQuery struct {
	Name  string `yaml:"name"`
	Query string `yaml:"query"`
}

type savedFile struct {
	Searches []SavedQuery `yaml:"searches"`
}

func savedQueriesPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "dynatrace-tui", "searches.yaml"), nil
}

func loadSavedQueries() ([]SavedQuery, error) {
	p, err := savedQueriesPath()
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
	var f savedFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return f.Searches, nil
}

func writeSavedQueries(qs []SavedQuery) error {
	p, err := savedQueriesPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(savedFile{Searches: qs})
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
		if err := writeSavedQueries(m.savedQueries); err != nil {
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

// --- Load modal -----------------------------------------------------------

func (m Model) updateLoadQuery(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
		return m, nil
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
			m.modal = modalNone
			return m, nil
		}
		sel := m.savedQueries[m.savedListIdx]
		m.editor.SetValue(sel.Query)
		m.modal = modalNone
		m.infoMsg = "loaded " + sel.Name
		m.state = stateIdle
		return m, nil
	case "d":
		if len(m.savedQueries) == 0 {
			return m, nil
		}
		m.savedQueries = append(m.savedQueries[:m.savedListIdx], m.savedQueries[m.savedListIdx+1:]...)
		if m.savedListIdx >= len(m.savedQueries) && m.savedListIdx > 0 {
			m.savedListIdx--
		}
		if err := writeSavedQueries(m.savedQueries); err != nil {
			m.errMsg = err.Error()
			m.state = stateError
		}
		return m, nil
	}
	return m, nil
}

func (m Model) viewLoadQuery() string {
	var b strings.Builder
	b.WriteString(paneTitleFocused.Render("Saved searches"))
	b.WriteString("\n\n")
	if len(m.savedQueries) == 0 {
		b.WriteString(statusBar.Render("(none — Ctrl-S in editor to save)"))
		b.WriteString("\n\n")
		b.WriteString(statusBar.Render("Esc close"))
		return m.renderModalOverlay(b.String())
	}
	for i, q := range m.savedQueries {
		prefix := "  "
		nameStyle := lipgloss.NewStyle().Bold(true)
		querySnippet := truncate(q.Query, 60)
		line := fmt.Sprintf("%s — %s", nameStyle.Render(q.Name), querySnippet)
		if i == m.savedListIdx {
			prefix = "▶ "
			line = lipgloss.NewStyle().Foreground(colorAccent).Render(line)
		}
		b.WriteString(prefix + line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(statusBar.Render("↑/↓ select · Enter load · d delete · Esc close"))
	return m.renderModalOverlay(b.String())
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
