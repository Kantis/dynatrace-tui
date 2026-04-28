package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Table is a minimal ANSI-aware replacement for bubbles/table.
//
// bubbles/table v1.0.0 truncates cells with runewidth.Truncate, which counts
// SGR escape bytes as visible characters and will cut in the middle of an
// escape sequence — corrupting any cell that carries inline ANSI styling
// (e.g. chroma-highlighted JSON). We render cells through ansi.Truncate so
// escape sequences stay intact.
type Table struct {
	cols    []tableColumn
	rows    []tableRow
	cursor  int
	yOffset int
	width   int
	height  int
	focus   bool

	headerStyle   lipgloss.Style
	cellStyle     lipgloss.Style
	selectedStyle lipgloss.Style
	separator     lipgloss.Style
}

type tableColumn struct {
	Title string
	Width int
}

type tableRow []string

func newTable() Table {
	return Table{
		headerStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			Padding(0, 1),
		cellStyle: lipgloss.NewStyle().Padding(0, 1),
		selectedStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("231")).
			Background(colorAccent).
			Bold(true).
			Padding(0, 1),
		separator: lipgloss.NewStyle().Foreground(colorMuted),
	}
}

func (t *Table) SetColumns(c []tableColumn) {
	t.cols = c
	t.clampCursor()
	t.clampOffset()
}

func (t *Table) SetRows(r []tableRow) {
	t.rows = r
	t.clampCursor()
	t.clampOffset()
}

func (t *Table) SetWidth(w int)  { t.width = w; t.clampOffset() }
func (t *Table) SetHeight(h int) { t.height = h; t.clampOffset() }
func (t *Table) Focus()          { t.focus = true }
func (t *Table) Blur()           { t.focus = false }
func (t Table) Cursor() int      { return t.cursor }

func (t *Table) clampCursor() {
	if t.cursor < 0 {
		t.cursor = 0
	}
	if last := len(t.rows) - 1; t.cursor > last {
		if last < 0 {
			t.cursor = 0
		} else {
			t.cursor = last
		}
	}
}

// visibleRows is the number of body rows that fit under the header + separator.
func (t Table) visibleRows() int {
	n := t.height - 2
	if n < 1 {
		return 1
	}
	return n
}

func (t *Table) clampOffset() {
	visible := t.visibleRows()
	if len(t.rows) == 0 {
		t.yOffset = 0
		return
	}
	maxOffset := len(t.rows) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if t.cursor < t.yOffset {
		t.yOffset = t.cursor
	}
	if t.cursor >= t.yOffset+visible {
		t.yOffset = t.cursor - visible + 1
	}
	if t.yOffset > maxOffset {
		t.yOffset = maxOffset
	}
	if t.yOffset < 0 {
		t.yOffset = 0
	}
}

func (t *Table) MoveUp(n int)   { t.cursor -= n; t.clampCursor(); t.clampOffset() }
func (t *Table) MoveDown(n int) { t.cursor += n; t.clampCursor(); t.clampOffset() }

func (t Table) Update(msg tea.Msg) (Table, tea.Cmd) {
	if !t.focus {
		return t, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return t, nil
	}
	switch keyMsg.String() {
	case "up", "k":
		t.MoveUp(1)
	case "down", "j":
		t.MoveDown(1)
	case "pgup", "b":
		t.MoveUp(t.visibleRows())
	case "pgdown", "f", " ":
		t.MoveDown(t.visibleRows())
	case "ctrl+u":
		t.MoveUp(max(1, t.visibleRows()/2))
	case "ctrl+d":
		t.MoveDown(max(1, t.visibleRows()/2))
	case "home", "g":
		t.cursor = 0
		t.clampOffset()
	case "end", "G":
		t.cursor = max(0, len(t.rows)-1)
		t.clampOffset()
	}
	return t, nil
}

func (t Table) View() string {
	if len(t.cols) == 0 {
		return ""
	}
	visible := t.visibleRows()
	lines := make([]string, 0, visible+2)
	lines = append(lines, t.renderHeader(), t.renderSeparator())

	end := t.yOffset + visible
	if end > len(t.rows) {
		end = len(t.rows)
	}
	for i := t.yOffset; i < end; i++ {
		lines = append(lines, t.renderRow(i))
	}
	for len(lines)-2 < visible {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (t Table) renderHeader() string {
	cells := make([]string, 0, len(t.cols))
	for _, c := range t.cols {
		if c.Width <= 0 {
			continue
		}
		innerW := c.Width - 2 // 1ch padding on each side
		if innerW < 1 {
			innerW = 1
		}
		title := ansi.Truncate(c.Title, innerW, "…")
		cells = append(cells,
			t.headerStyle.Width(c.Width).MaxWidth(c.Width).Inline(true).Render(title))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}

func (t Table) renderSeparator() string {
	total := 0
	for _, c := range t.cols {
		if c.Width > 0 {
			total += c.Width
		}
	}
	return t.separator.Render(strings.Repeat("─", total))
}

func (t Table) renderRow(idx int) string {
	row := t.rows[idx]
	isSelected := idx == t.cursor && t.focus
	cells := make([]string, 0, len(t.cols))
	for i, col := range t.cols {
		if col.Width <= 0 {
			continue
		}
		var value string
		if i < len(row) {
			value = row[i]
		}
		// On the selected row, drop inline ANSI from the cell content so the
		// selection background isn't punched out by per-token resets from
		// chroma-highlighted JSON.
		if isSelected {
			value = ansi.Strip(value)
		}
		innerW := col.Width - 2
		if innerW < 1 {
			innerW = 1
		}
		value = ansi.Truncate(value, innerW, "…")
		style := t.cellStyle
		if isSelected {
			style = t.selectedStyle
		}
		cells = append(cells,
			style.Width(col.Width).MaxWidth(col.Width).Inline(true).Render(value))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}
