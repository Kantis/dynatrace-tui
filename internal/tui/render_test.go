package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestViewFitsHeight asserts the layout never emits more lines than the
// terminal height. Regression: an earlier applyLayout subtracted only
// border rows and forgot the pane-title rows, overflowing by 2 lines and
// scrolling the editor title off the alt screen.
func TestViewFitsHeight(t *testing.T) {
	cases := []struct{ w, h int }{
		{80, 24},
		{120, 30},
		{100, 40},
	}
	for _, tc := range cases {
		m := New(nil, "", nil, nil, false)
		m, _ = applyMsg(m, tea.WindowSizeMsg{Width: tc.w, Height: tc.h})
		out := m.View()
		lines := strings.Count(out, "\n") + 1
		if lines > tc.h {
			t.Errorf("View() at %dx%d emitted %d lines, want ≤ %d", tc.w, tc.h, lines, tc.h)
		}
	}
}

func applyMsg(m Model, msg tea.Msg) (Model, tea.Cmd) {
	tm, cmd := m.Update(msg)
	return tm.(Model), cmd
}

// TestPopulateTableHandlesEmptyAfterPopulated reproduces the panic that
// happened when a query returning zero records followed a query that
// returned records: the table still held the populated rows when SetColumns
// installed the placeholder "(empty)" column, and bubbles/table indexed
// past it.
func TestPopulateTableHandlesEmptyAfterPopulated(t *testing.T) {
	m := New(nil, "", nil, nil, false)
	m, _ = applyMsg(m, tea.WindowSizeMsg{Width: 100, Height: 30})

	// First "query": multiple records with multiple fields → multi-column table.
	m.records = []map[string]any{
		{"timestamp": "2026-04-28T12:00:00Z", "loglevel": "INFO", "content": "hello"},
		{"timestamp": "2026-04-28T12:00:01Z", "loglevel": "ERROR", "content": "boom"},
	}
	m.populateTable()

	// Second "query": zero records. Used to panic in renderRow.
	m.records = nil
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("populateTable panicked on empty records after populated: %v", r)
		}
	}()
	m.populateTable()
	// Force a render to invoke the table's UpdateViewport path.
	_ = m.View()
}
