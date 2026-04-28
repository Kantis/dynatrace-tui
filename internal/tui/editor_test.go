package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// pressKey feeds a single key (named via tea.KeyMsg.String() semantics) into the
// editor and returns the updated copy. For runes, set Type=KeyRunes and Runes.
func pressKey(t *testing.T, e Editor, key string) Editor {
	t.Helper()
	var msg tea.KeyMsg
	switch key {
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	e, _ = e.Update(msg)
	return e
}

func pressKeys(t *testing.T, e Editor, keys ...string) Editor {
	t.Helper()
	for _, k := range keys {
		e = pressKey(t, e, k)
	}
	return e
}

func newEditorWithBuffer(t *testing.T, content string) Editor {
	t.Helper()
	e := NewEditor(true)
	e.SetValue(content)
	// Switch to normal mode for tests; SetValue itself doesn't change mode.
	e = pressKey(t, e, "esc")
	// Move to top of buffer.
	e = pressKeys(t, e, "g", "g")
	return e
}

func TestModeStartsInsert(t *testing.T) {
	e := NewEditor(true)
	if e.Mode() != modeInsert {
		t.Fatalf("expected initial mode INSERT, got %s", e.Mode())
	}
}

func TestEscEntersNormal(t *testing.T) {
	e := NewEditor(true)
	e = pressKey(t, e, "esc")
	if e.Mode() != modeNormal {
		t.Fatalf("expected mode NORMAL after Esc, got %s", e.Mode())
	}
}

func TestINormalEntersInsert(t *testing.T) {
	e := newEditorWithBuffer(t, "hello")
	e = pressKey(t, e, "i")
	if e.Mode() != modeInsert {
		t.Fatalf("expected INSERT after i, got %s", e.Mode())
	}
}

func TestDeleteLine(t *testing.T) {
	e := newEditorWithBuffer(t, "first\nsecond\nthird")
	// gg in newEditorWithBuffer moves to top → row 0
	e = pressKeys(t, e, "d", "d")
	if got := e.Value(); got != "second\nthird" {
		t.Errorf("after dd at row 0: got %q, want %q", got, "second\nthird")
	}
	// Cursor should now be on row 0 ("second")
	if row := e.ta.Line(); row != 0 {
		t.Errorf("cursor row after dd = %d, want 0", row)
	}
}

func TestDeleteLineLast(t *testing.T) {
	e := newEditorWithBuffer(t, "first\nsecond\nthird")
	e = pressKeys(t, e, "G")        // row 2
	e = pressKeys(t, e, "d", "d")   // delete last
	if got := e.Value(); got != "first\nsecond" {
		t.Errorf("after dd on last row: got %q, want %q", got, "first\nsecond")
	}
}

func TestYankAndPaste(t *testing.T) {
	e := newEditorWithBuffer(t, "alpha\nbeta\ngamma")
	e = pressKeys(t, e, "y", "y")          // yank "alpha"
	if e.register != "alpha" {
		t.Fatalf("register = %q, want alpha", e.register)
	}
	e = pressKey(t, e, "G")                // bottom (row 2)
	e = pressKey(t, e, "p")                // paste below
	want := "alpha\nbeta\ngamma\nalpha"
	if got := e.Value(); got != want {
		t.Errorf("after p at last row: got %q, want %q", got, want)
	}
}

func TestOpenLineBelow(t *testing.T) {
	e := newEditorWithBuffer(t, "one\ntwo")
	e = pressKey(t, e, "o") // opens below row 0, enters insert
	if e.Mode() != modeInsert {
		t.Fatalf("after o, mode = %s, want INSERT", e.Mode())
	}
	if got := e.Value(); got != "one\n\ntwo" {
		t.Errorf("after o on row 0: got %q, want %q", got, "one\n\ntwo")
	}
	// In insert mode now, type some text.
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if got := e.Value(); !strings.Contains(got, "X") {
		t.Errorf("after typing X: got %q, want it to contain X", got)
	}
}

func TestOpenLineAbove(t *testing.T) {
	e := newEditorWithBuffer(t, "one\ntwo")
	e = pressKey(t, e, "j") // move to row 1 ("two")
	e = pressKey(t, e, "O") // open above
	if got := e.Value(); got != "one\n\ntwo" {
		t.Errorf("after O on row 1: got %q, want %q", got, "one\n\ntwo")
	}
	if e.Mode() != modeInsert {
		t.Fatalf("after O, mode = %s, want INSERT", e.Mode())
	}
}

func TestPendingOperatorClearedOnMismatch(t *testing.T) {
	e := newEditorWithBuffer(t, "alpha\nbeta")
	e = pressKey(t, e, "d")           // pending d
	e = pressKey(t, e, "j")           // not d → clear, no edit
	if got := e.Value(); got != "alpha\nbeta" {
		t.Errorf("buffer changed unexpectedly: %q", got)
	}
	if e.pendingD {
		t.Errorf("pendingD still set after mismatch")
	}
}

func TestGotoTopBottom(t *testing.T) {
	e := newEditorWithBuffer(t, "a\nb\nc\nd\ne")
	e = pressKey(t, e, "G")
	if row := e.ta.Line(); row != 4 {
		t.Errorf("after G: row = %d, want 4", row)
	}
	e = pressKeys(t, e, "g", "g")
	if row := e.ta.Line(); row != 0 {
		t.Errorf("after gg: row = %d, want 0", row)
	}
}

func TestXDeletesChar(t *testing.T) {
	e := newEditorWithBuffer(t, "abc")
	e = pressKey(t, e, "x") // cursor at col 0 → delete 'a'
	if got := e.Value(); got != "bc" {
		t.Errorf("after x at col 0: got %q, want %q", got, "bc")
	}
}

// Word operators delegate boundary detection to bubbles/textarea, which
// jumps to end-of-current-word (not start-of-next-word). These tests pin
// that observed behavior so it doesn't drift if textarea changes.

func TestDeleteWordForward(t *testing.T) {
	e := newEditorWithBuffer(t, "alpha beta gamma")
	e = pressKeys(t, e, "d", "w")
	if got := e.Value(); got != " beta gamma" {
		t.Errorf("dw at col 0: got %q, want %q", got, " beta gamma")
	}
	if e.register != "alpha" {
		t.Errorf("register after dw: got %q, want %q", e.register, "alpha")
	}
}

func TestDeleteWordBackward(t *testing.T) {
	e := newEditorWithBuffer(t, "alpha beta")
	e = pressKey(t, e, "G") // bottom (single line — stays on row 0)
	e = pressKey(t, e, "$") // end of line
	e = pressKeys(t, e, "d", "b")
	if got := e.Value(); got != "alpha " {
		t.Errorf("db at end: got %q, want %q", got, "alpha ")
	}
	if e.register != "beta" {
		t.Errorf("register after db: got %q, want %q", e.register, "beta")
	}
}

func TestYankWordForwardLeavesBufferUnchanged(t *testing.T) {
	const buf = "alpha beta gamma"
	e := newEditorWithBuffer(t, buf)
	e = pressKeys(t, e, "y", "w")
	if got := e.Value(); got != buf {
		t.Errorf("yw modified buffer: got %q, want %q", got, buf)
	}
	if e.register != "alpha" {
		t.Errorf("register after yw: got %q, want %q", e.register, "alpha")
	}
	// And paste should restore that word.
	e = pressKey(t, e, "p")
	want := buf + "\nalpha"
	if got := e.Value(); got != want {
		t.Errorf("after yw then p: got %q, want %q", got, want)
	}
}

func TestYankWordBackwardLeavesBufferUnchanged(t *testing.T) {
	const buf = "alpha beta"
	e := newEditorWithBuffer(t, buf)
	e = pressKey(t, e, "$")
	e = pressKeys(t, e, "y", "b")
	if got := e.Value(); got != buf {
		t.Errorf("yb modified buffer: got %q, want %q", got, buf)
	}
	if e.register != "beta" {
		t.Errorf("register after yb: got %q, want %q", e.register, "beta")
	}
}

func TestDDeletesToLineEnd(t *testing.T) {
	e := newEditorWithBuffer(t, "alpha beta")
	// gg lands on row 0 col 0; advance one word to put cursor at col 5 (the space)
	e = pressKey(t, e, "w")
	e = pressKey(t, e, "D")
	if got := e.Value(); got != "alpha" {
		t.Errorf("D after w on \"alpha beta\": got %q, want %q", got, "alpha")
	}
	if e.register != " beta" {
		t.Errorf("register after D: got %q, want %q", e.register, " beta")
	}
}

func TestDDeletesToLineEndPreservesOtherLines(t *testing.T) {
	e := newEditorWithBuffer(t, "first line\nsecond")
	e = pressKey(t, e, "D")
	if got := e.Value(); got != "\nsecond" {
		t.Errorf("D at start of multiline: got %q, want %q", got, "\nsecond")
	}
}

func TestDIsUndoable(t *testing.T) {
	const buf = "alpha beta gamma"
	e := newEditorWithBuffer(t, buf)
	e = pressKey(t, e, "D")
	if e.Value() == buf {
		t.Fatalf("D didn't change buffer")
	}
	e = pressKey(t, e, "u")
	if got := e.Value(); got != buf {
		t.Errorf("undo after D: got %q, want %q", got, buf)
	}
}

func TestUndoRevertsDeleteLine(t *testing.T) {
	const buf = "alpha\nbeta\ngamma"
	e := newEditorWithBuffer(t, buf)
	e = pressKeys(t, e, "d", "d")
	if got := e.Value(); got != "beta\ngamma" {
		t.Fatalf("after dd: got %q", got)
	}
	e = pressKey(t, e, "u")
	if got := e.Value(); got != buf {
		t.Errorf("after u: got %q, want %q", got, buf)
	}
}

func TestUndoRevertsInsertSession(t *testing.T) {
	e := newEditorWithBuffer(t, "alpha")
	e = pressKey(t, e, "$") // end of line
	e = pressKey(t, e, "a") // append → insert mode
	// type some chars
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" beta")})
	if got := e.Value(); got != "alpha beta" {
		t.Fatalf("after typing: got %q", got)
	}
	e = pressKey(t, e, "esc")
	e = pressKey(t, e, "u")
	if got := e.Value(); got != "alpha" {
		t.Errorf("undo after insert session: got %q, want %q", got, "alpha")
	}
}

func TestUndoStacksMultipleEdits(t *testing.T) {
	e := newEditorWithBuffer(t, "alpha\nbeta\ngamma")
	e = pressKeys(t, e, "d", "d")             // → "beta\ngamma"
	e = pressKeys(t, e, "d", "d")             // → "gamma"
	if got := e.Value(); got != "gamma" {
		t.Fatalf("after two dds: got %q", got)
	}
	e = pressKey(t, e, "u")                   // → "beta\ngamma"
	if got := e.Value(); got != "beta\ngamma" {
		t.Errorf("first u: got %q", got)
	}
	e = pressKey(t, e, "u")                   // → "alpha\nbeta\ngamma"
	if got := e.Value(); got != "alpha\nbeta\ngamma" {
		t.Errorf("second u: got %q", got)
	}
}

func TestRedoReplaysUndoneEdit(t *testing.T) {
	e := newEditorWithBuffer(t, "alpha\nbeta")
	e = pressKeys(t, e, "d", "d") // → "beta"
	e = pressKey(t, e, "u")        // → "alpha\nbeta"
	if got := e.Value(); got != "alpha\nbeta" {
		t.Fatalf("after u: got %q", got)
	}
	e.Redo() // simulate global ctrl+r dispatch
	if got := e.Value(); got != "beta" {
		t.Errorf("after redo: got %q, want %q", got, "beta")
	}
}

func TestRedoClearedAfterNewEdit(t *testing.T) {
	e := newEditorWithBuffer(t, "alpha\nbeta\ngamma")
	e = pressKeys(t, e, "d", "d") // delete line → "beta\ngamma"
	e = pressKey(t, e, "u")        // → "alpha\nbeta\ngamma"
	e = pressKeys(t, e, "d", "d") // new edit → "beta\ngamma"; redo cleared
	if len(e.redoStack) != 0 {
		t.Errorf("redo stack should be cleared after new edit, has %d", len(e.redoStack))
	}
	e.Redo()
	if got := e.Value(); got != "beta\ngamma" {
		t.Errorf("redo on empty should be no-op, got %q", got)
	}
}

func TestUndoOnEmptyStackIsNoop(t *testing.T) {
	const buf = "alpha"
	e := newEditorWithBuffer(t, buf)
	e = pressKey(t, e, "u")
	if got := e.Value(); got != buf {
		t.Errorf("u on empty stack changed buffer: %q", got)
	}
}

func TestUndoRevertsOpenLine(t *testing.T) {
	const buf = "one\ntwo"
	e := newEditorWithBuffer(t, buf)
	e = pressKey(t, e, "o") // open below + insert
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("INSERTED")})
	e = pressKey(t, e, "esc")
	e = pressKey(t, e, "u")
	if got := e.Value(); got != buf {
		t.Errorf("undo after o+typing: got %q, want %q", got, buf)
	}
}

func TestPendingDClearedOnUnknownMotion(t *testing.T) {
	e := newEditorWithBuffer(t, "alpha")
	e = pressKey(t, e, "d")
	e = pressKey(t, e, "z") // unrecognized
	if e.pendingD {
		t.Errorf("pendingD still set after unknown motion")
	}
	if got := e.Value(); got != "alpha" {
		t.Errorf("buffer changed: %q", got)
	}
}
