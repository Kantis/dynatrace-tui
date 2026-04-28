package tui

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kantis/dynatrace-tui/internal/grail"
)

type exportOption struct {
	label  string
	format string // "json" or "csv"
	scope  string // "all" or "current"
}

var exportOptions = []exportOption{
	{"All records  → JSON", "json", "all"},
	{"All records  → CSV", "csv", "all"},
	{"Current row  → JSON", "json", "current"},
	{"Current row  → CSV", "csv", "current"},
}

func (m Model) updateExport(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
		return m, nil
	case "up", "k":
		if m.exportIdx > 0 {
			m.exportIdx--
		}
	case "down", "j":
		if m.exportIdx < len(exportOptions)-1 {
			m.exportIdx++
		}
	case "enter":
		opt := exportOptions[m.exportIdx]
		records := m.records
		if opt.scope == "current" {
			cur := m.table.Cursor()
			if cur < 0 || cur >= len(m.records) {
				m.errMsg = "no row selected"
				m.state = stateError
				m.modal = modalNone
				return m, nil
			}
			records = grail.Records{m.records[cur]}
		}
		path, err := writeExport(records, opt.format)
		if err != nil {
			m.errMsg = err.Error()
			m.state = stateError
		} else {
			m.infoMsg = "exported to " + path
			m.state = stateIdle
		}
		m.modal = modalNone
		return m, nil
	}
	return m, nil
}

func (m Model) viewExport() string {
	var b strings.Builder
	b.WriteString(paneTitleFocused.Render("Export"))
	b.WriteString("\n\n")
	for i, opt := range exportOptions {
		prefix := "  "
		line := opt.label
		if i == m.exportIdx {
			prefix = "▶ "
			line = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(line)
		}
		b.WriteString(prefix + line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(statusBar.Render("↑/↓ select · Enter export · Esc cancel"))
	return m.renderModalOverlay(b.String())
}

func writeExport(records grail.Records, format string) (string, error) {
	if len(records) == 0 {
		return "", fmt.Errorf("no records to export")
	}
	stamp := time.Now().Format("2006-01-02T15-04-05")
	name := fmt.Sprintf("dttui-export-%s.%s", stamp, format)
	f, err := os.Create(name)
	if err != nil {
		return "", err
	}
	defer f.Close()

	switch format {
	case "json":
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(records); err != nil {
			return "", err
		}
	case "csv":
		if err := writeCSV(f, records); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported format %q", format)
	}
	return name, nil
}

func writeCSV(f *os.File, records grail.Records) error {
	keys := map[string]bool{}
	for _, r := range records {
		for k := range r {
			keys[k] = true
		}
	}
	headers := make([]string, 0, len(keys))
	for k := range keys {
		headers = append(headers, k)
	}
	sort.Strings(headers)

	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write(headers); err != nil {
		return err
	}
	for _, r := range records {
		row := make([]string, len(headers))
		for i, h := range headers {
			row[i] = stringCell(r[h])
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}
