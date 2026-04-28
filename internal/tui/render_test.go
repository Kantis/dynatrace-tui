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
		m := New(nil)
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
