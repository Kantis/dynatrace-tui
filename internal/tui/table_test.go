package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// rowFor renders a single row to a string, focused so the cursor row gets the
// selection style. Helper for the tests below.
func rowFor(t *testing.T, cols []tableColumn, row tableRow, selected bool) string {
	t.Helper()
	tbl := newTable()
	tbl.SetColumns(cols)
	tbl.SetRows([]tableRow{row})
	tbl.SetWidth(sumWidths(cols))
	tbl.SetHeight(4)
	if selected {
		tbl.Focus()
	}
	out := tbl.View()
	// View returns header + separator + body lines; pull line 3 (the row).
	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(lines))
	}
	return lines[2]
}

func sumWidths(cols []tableColumn) int {
	w := 0
	for _, c := range cols {
		w += c.Width
	}
	return w
}

// TestTableRowKeepsHighlightedJSONIntact confirms a long, ANSI-highlighted
// cell isn't truncated mid-escape — the cell's visible width must equal the
// allotted column width and no orphan SGR fragments may leak through.
func TestTableRowKeepsHighlightedJSONIntact(t *testing.T) {
	long := `{"event":"login","user":"emil","attempts":42,"meta":"some long trailing text"}`
	highlighted := highlightJSONCell(long)
	if !strings.Contains(highlighted, "\x1b[") {
		t.Fatalf("test setup: expected highlighted JSON to contain ANSI codes")
	}

	cols := []tableColumn{{Title: "content", Width: 30}}
	rendered := rowFor(t, cols, tableRow{highlighted}, false)

	// Stripping ANSI should leave the cell at exactly the column's visible width.
	plain := ansi.Strip(rendered)
	if got := ansi.StringWidth(plain); got != cols[0].Width {
		t.Errorf("visible width = %d, want %d (rendered=%q stripped=%q)", got, cols[0].Width, rendered, plain)
	}

	// Any ANSI escape that appears must be a well-formed SGR sequence
	// (\x1b[...m). A truncated escape (e.g. \x1b[38;5 with no terminator)
	// would not match the trailing 'm'.
	sgr := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	idx := 0
	for {
		i := strings.Index(rendered[idx:], "\x1b")
		if i < 0 {
			break
		}
		i += idx
		// A complete SGR starting at i must match.
		if loc := sgr.FindStringIndex(rendered[i:]); loc == nil || loc[0] != 0 {
			t.Errorf("malformed ANSI escape at offset %d in %q", i, rendered)
			break
		}
		idx = i + 1
	}
}

// TestTableSelectedRowHasNoAnsiBleed verifies that when a row with ANSI
// content is selected, the cell content is stripped before rendering — so
// the selection background covers the whole cell instead of being broken up
// by inline color resets.
func TestTableSelectedRowHasNoAnsiBleed(t *testing.T) {
	cols := []tableColumn{{Title: "content", Width: 30}}
	highlighted := highlightJSONCell(`{"a":1,"b":"x"}`)

	rendered := rowFor(t, cols, tableRow{highlighted}, true)

	// The whole rendered cell must be wrapped by the selection style. We can't
	// pin the exact escape sequence, but we can check that no per-token reset
	// (\x1b[0m) appears in the middle of the content — if it did, the
	// selection background would be punched out for the rest of the cell.
	plain := ansi.Strip(rendered)
	if got := ansi.StringWidth(plain); got != cols[0].Width {
		t.Errorf("visible width = %d, want %d", got, cols[0].Width)
	}
	// The plain text shouldn't contain chroma's typical token boundaries
	// (which would be visible if ANSI were left in and somehow malformed).
	// More importantly, the rendered output should contain a single style
	// span — easiest check: the count of SGR opening sequences shouldn't
	// scale with the number of JSON tokens.
	sgrOpens := strings.Count(rendered, "\x1b[")
	if sgrOpens > 4 {
		t.Errorf("selected row has too many ANSI runs (%d) — chroma codes likely leaked through; rendered=%q",
			sgrOpens, rendered)
	}
}

// TestTableViewSurvivesEmptyRowsAfterPopulated mirrors the regression that
// bubbles/table had with index-out-of-range when columns shrink under the
// rows. Our Table accesses row[i] only if i < len(row), but pin the
// behavior so future refactors don't reintroduce it.
func TestTableViewSurvivesEmptyRowsAfterPopulated(t *testing.T) {
	tbl := newTable()
	tbl.SetWidth(80)
	tbl.SetHeight(10)
	tbl.SetColumns([]tableColumn{
		{Title: "a", Width: 10},
		{Title: "b", Width: 10},
		{Title: "c", Width: 10},
	})
	tbl.SetRows([]tableRow{
		{"1", "2", "3"},
		{"4", "5", "6"},
	})
	_ = tbl.View()

	// Now shrink to a single placeholder column with no rows.
	tbl.SetRows(nil)
	tbl.SetColumns([]tableColumn{{Title: "(empty)", Width: 30}})
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("View panicked after rows shrunk: %v", r)
		}
	}()
	_ = tbl.View()
}

// TestTableCursorMovementClampsToRowCount makes sure pgdown/end don't push
// the cursor past the last row.
func TestTableCursorMovementClampsToRowCount(t *testing.T) {
	tbl := newTable()
	tbl.SetWidth(40)
	tbl.SetHeight(5)
	tbl.SetColumns([]tableColumn{{Title: "x", Width: 40}})
	rows := make([]tableRow, 3)
	for i := range rows {
		rows[i] = tableRow{"row"}
	}
	tbl.SetRows(rows)
	tbl.Focus()

	tbl.MoveDown(100)
	if got := tbl.Cursor(); got != 2 {
		t.Errorf("after MoveDown(100): cursor = %d, want 2", got)
	}
	tbl.MoveUp(100)
	if got := tbl.Cursor(); got != 0 {
		t.Errorf("after MoveUp(100): cursor = %d, want 0", got)
	}
}
