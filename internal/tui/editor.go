package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type vimMode int

const (
	modeInsert vimMode = iota
	modeNormal
)

func (m vimMode) String() string {
	if m == modeNormal {
		return "NORMAL"
	}
	return "INSERT"
}

// Editor is a vim-style modal wrapper around bubbles/textarea.
//
// Supported normal-mode keys:
//
//	motions:    h j k l, w b, 0 $, gg G
//	edits:      x, D (delete to end of line), dd, yy, p, dw, db, yw, yb
//	undo/redo:  u (undo) — Ctrl+R (redo) is intercepted at the app level
//	            so it works in either mode; an insert session counts as
//	            one undo step.
//	→ insert:   i (here), I (line start), a (right), A (line end),
//	            o (open below), O (open above)
//	Esc returns to normal mode from insert.
//
// Operator-pending state is single-character; entering anything other
// than the expected completion clears it (e.g. d followed by anything
// but d is dropped — only `dd` is supported).
type Editor struct {
	ta   textarea.Model
	mode vimMode

	pendingD bool
	pendingY bool
	pendingG bool

	register string // last yanked / deleted line

	undoStack []undoSnapshot
	redoStack []undoSnapshot
}

type undoSnapshot struct {
	value string
	row   int
	col   int
}

const maxUndoDepth = 200

// pushUndo captures the current buffer + cursor onto the undo stack. Call
// before any edit (insert/delete/paste) and before entering insert mode so a
// whole insert session collapses into one undo step. Pushing a new edit
// invalidates any pending redo history.
func (e *Editor) pushUndo() {
	e.undoStack = append(e.undoStack, e.snapshot())
	if len(e.undoStack) > maxUndoDepth {
		e.undoStack = e.undoStack[len(e.undoStack)-maxUndoDepth:]
	}
	e.redoStack = nil
}

func (e Editor) snapshot() undoSnapshot {
	row, col := e.cursorPos()
	return undoSnapshot{value: e.ta.Value(), row: row, col: col}
}

func (e *Editor) undo() {
	n := len(e.undoStack)
	if n == 0 {
		return
	}
	// Capture current state for redo before reverting.
	e.redoStack = append(e.redoStack, e.snapshot())
	snap := e.undoStack[n-1]
	e.undoStack = e.undoStack[:n-1]
	e.ta.SetValue(snap.value)
	e.setCursor(snap.row, snap.col)
}

func (e *Editor) redo() {
	n := len(e.redoStack)
	if n == 0 {
		return
	}
	e.undoStack = append(e.undoStack, e.snapshot())
	snap := e.redoStack[n-1]
	e.redoStack = e.redoStack[:n-1]
	e.ta.SetValue(snap.value)
	e.setCursor(snap.row, snap.col)
}

// Redo is called by the app-level handler (Ctrl+R is global so it works in
// both insert and normal mode).
func (e *Editor) Redo() { e.redo() }

func NewEditor() Editor {
	ta := textarea.New()
	ta.ShowLineNumbers = true
	ta.CharLimit = 0
	ta.SetHeight(8)
	ta.Placeholder = "fetch logs, from:now()-15m | limit 50"
	ta.Focus()
	return Editor{ta: ta, mode: modeInsert}
}

func (e Editor) Mode() vimMode { return e.mode }

func (e Editor) Value() string  { return e.ta.Value() }
func (e Editor) Focused() bool  { return e.ta.Focused() }
func (e Editor) View() string   { return e.ta.View() }
func (e Editor) Cursor() string { return "" } // placeholder if needed

func (e *Editor) SetValue(s string)   { e.ta.SetValue(s) }
func (e *Editor) SetWidth(w int)      { e.ta.SetWidth(w) }
func (e *Editor) SetHeight(h int)     { e.ta.SetHeight(h) }
func (e *Editor) Focus() tea.Cmd      { return e.ta.Focus() }
func (e *Editor) Blur()               { e.ta.Blur() }

func (e Editor) Update(msg tea.Msg) (Editor, tea.Cmd) {
	keyMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		var cmd tea.Cmd
		e.ta, cmd = e.ta.Update(msg)
		return e, cmd
	}

	if e.mode == modeInsert {
		if keyMsg.Type == tea.KeyEsc {
			e.mode = modeNormal
			return e, nil
		}
		var cmd tea.Cmd
		e.ta, cmd = e.ta.Update(msg)
		return e, cmd
	}

	// Normal mode
	return e.updateNormal(keyMsg)
}

func (e Editor) updateNormal(msg tea.KeyMsg) (Editor, tea.Cmd) {
	s := msg.String()

	// Resolve any pending two-key operators first.
	switch {
	case e.pendingG:
		e.pendingG = false
		if s == "g" {
			e.gotoTop()
		}
		return e, nil
	case e.pendingD:
		e.pendingD = false
		switch s {
		case "d":
			e.pushUndo()
			e.deleteLine()
		case "w":
			e.pushUndo()
			e.wordOp(true, true)
		case "b":
			e.pushUndo()
			e.wordOp(false, true)
		}
		return e, nil
	case e.pendingY:
		e.pendingY = false
		switch s {
		case "y":
			e.yankLine()
		case "w":
			e.wordOp(true, false)
		case "b":
			e.wordOp(false, false)
		}
		return e, nil
	}

	switch s {
	// Mode transitions — snapshot before so the whole insert session
	// collapses into one undo step.
	case "i":
		e.pushUndo()
		e.mode = modeInsert
	case "I":
		e.pushUndo()
		e.send(tea.KeyMsg{Type: tea.KeyHome})
		e.mode = modeInsert
	case "a":
		e.pushUndo()
		e.send(tea.KeyMsg{Type: tea.KeyRight})
		e.mode = modeInsert
	case "A":
		e.pushUndo()
		e.send(tea.KeyMsg{Type: tea.KeyEnd})
		e.mode = modeInsert
	case "o":
		e.pushUndo()
		e.openLineBelow()
		e.mode = modeInsert
	case "O":
		e.pushUndo()
		e.openLineAbove()
		e.mode = modeInsert

	// Motions
	case "h", "left":
		e.send(tea.KeyMsg{Type: tea.KeyLeft})
	case "j", "down":
		e.send(tea.KeyMsg{Type: tea.KeyDown})
	case "k", "up":
		e.send(tea.KeyMsg{Type: tea.KeyUp})
	case "l", "right":
		e.send(tea.KeyMsg{Type: tea.KeyRight})
	case "0", "home":
		e.send(tea.KeyMsg{Type: tea.KeyHome})
	case "$", "end":
		e.send(tea.KeyMsg{Type: tea.KeyEnd})
	case "w":
		e.send(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	case "b":
		e.send(tea.KeyMsg{Type: tea.KeyLeft, Alt: true})
	case "g":
		e.pendingG = true
	case "G":
		e.gotoBottom()

	// Edits
	case "x":
		e.pushUndo()
		e.send(tea.KeyMsg{Type: tea.KeyDelete})
	case "D":
		e.pushUndo()
		e.deleteToLineEnd()
	case "d":
		e.pendingD = true
	case "y":
		e.pendingY = true
	case "p":
		e.pushUndo()
		e.pasteLine()
	case "u":
		e.undo()
	}

	return e, nil
}

func (e *Editor) send(msg tea.KeyMsg) {
	e.ta, _ = e.ta.Update(msg)
}

// --- buffer manipulation helpers ---

// setRow moves the cursor to the start of `target` (clamped). It assumes the
// caller has just called SetValue, which leaves the cursor at the end of input.
func (e *Editor) setRow(target, lastRow int) {
	if target < 0 {
		target = 0
	}
	if target > lastRow {
		target = lastRow
	}
	for e.ta.Line() > target {
		e.ta.CursorUp()
	}
	for e.ta.Line() < target {
		e.ta.CursorDown()
	}
	e.ta.SetCursor(0)
}

func (e *Editor) deleteLine() {
	row := e.ta.Line()
	lines := strings.Split(e.ta.Value(), "\n")
	if row < 0 || row >= len(lines) {
		return
	}
	e.register = lines[row]
	lines = append(lines[:row], lines[row+1:]...)
	if len(lines) == 0 {
		lines = []string{""}
	}
	e.ta.SetValue(strings.Join(lines, "\n"))
	e.setRow(row, len(lines)-1)
}

func (e *Editor) yankLine() {
	row := e.ta.Line()
	lines := strings.Split(e.ta.Value(), "\n")
	if row < 0 || row >= len(lines) {
		return
	}
	e.register = lines[row]
}

func (e *Editor) pasteLine() {
	if e.register == "" {
		return
	}
	row := e.ta.Line()
	lines := strings.Split(e.ta.Value(), "\n")
	if row >= len(lines) {
		row = len(lines) - 1
	}
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:row+1]...)
	newLines = append(newLines, e.register)
	newLines = append(newLines, lines[row+1:]...)
	e.ta.SetValue(strings.Join(newLines, "\n"))
	e.setRow(row+1, len(newLines)-1)
}

func (e *Editor) openLineBelow() {
	row := e.ta.Line()
	lines := strings.Split(e.ta.Value(), "\n")
	if row >= len(lines) {
		row = len(lines) - 1
	}
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:row+1]...)
	newLines = append(newLines, "")
	newLines = append(newLines, lines[row+1:]...)
	e.ta.SetValue(strings.Join(newLines, "\n"))
	e.setRow(row+1, len(newLines)-1)
}

func (e *Editor) openLineAbove() {
	row := e.ta.Line()
	lines := strings.Split(e.ta.Value(), "\n")
	if row < 0 {
		row = 0
	}
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:row]...)
	newLines = append(newLines, "")
	newLines = append(newLines, lines[row:]...)
	e.ta.SetValue(strings.Join(newLines, "\n"))
	e.setRow(row, len(newLines)-1)
}

func (e *Editor) gotoTop() {
	e.ta.SetCursor(0)
	for e.ta.Line() > 0 {
		e.ta.CursorUp()
	}
	e.ta.SetCursor(0)
}

// deleteToLineEnd deletes from the cursor to the end of the current line and
// captures the removed text in the register.
func (e *Editor) deleteToLineEnd() {
	row, col := e.cursorPos()
	lines := strings.Split(e.ta.Value(), "\n")
	if row >= 0 && row < len(lines) {
		e.register = runeSlice(lines[row], col, -1)
	}
	e.send(tea.KeyMsg{Type: tea.KeyCtrlK})
}

func (e *Editor) gotoBottom() {
	last := strings.Count(e.ta.Value(), "\n")
	for e.ta.Line() < last {
		e.ta.CursorDown()
	}
	e.ta.SetCursor(0)
}

// cursorPos returns the (source row, source column) of the cursor.
func (e Editor) cursorPos() (int, int) {
	row := e.ta.Line()
	info := e.ta.LineInfo()
	return row, info.StartColumn + info.CharOffset
}

// setCursor moves the cursor to (row, col), clamping as necessary. Assumes
// caller has already ensured the buffer contains row.
func (e *Editor) setCursor(row, col int) {
	for e.ta.Line() > row {
		e.ta.CursorUp()
	}
	for e.ta.Line() < row {
		e.ta.CursorDown()
	}
	e.ta.SetCursor(col)
}

// wordOp implements dw/db/yw/yb. It uses textarea's own word-jump (alt+left/
// alt+right) to find the boundary, captures the range to the register, then
// either restores the cursor (yank) or synthesises alt+delete / alt+backspace
// to perform the deletion (delete).
func (e *Editor) wordOp(forward, deleting bool) {
	r1, c1 := e.cursorPos()
	if forward {
		e.send(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	} else {
		e.send(tea.KeyMsg{Type: tea.KeyLeft, Alt: true})
	}
	r2, c2 := e.cursorPos()
	if r1 == r2 && c1 == c2 {
		return // no-op (already at edge of buffer)
	}

	rA, cA, rB, cB := r1, c1, r2, c2
	if !forward {
		rA, cA, rB, cB = r2, c2, r1, c1
	}
	e.register = extractRange(e.ta.Value(), rA, cA, rB, cB)

	e.setCursor(r1, c1)
	if deleting {
		if forward {
			e.send(tea.KeyMsg{Type: tea.KeyDelete, Alt: true})
		} else {
			e.send(tea.KeyMsg{Type: tea.KeyBackspace, Alt: true})
		}
	}
}

// extractRange returns the substring of s between (rA, cA) inclusive and
// (rB, cB) exclusive, treating columns as rune indices.
func extractRange(s string, rA, cA, rB, cB int) string {
	lines := strings.Split(s, "\n")
	if rA < 0 || rA >= len(lines) || rB < 0 || rB >= len(lines) {
		return ""
	}
	if rA == rB {
		return runeSlice(lines[rA], cA, cB)
	}
	var b strings.Builder
	b.WriteString(runeSlice(lines[rA], cA, -1))
	b.WriteByte('\n')
	for r := rA + 1; r < rB; r++ {
		b.WriteString(lines[r])
		b.WriteByte('\n')
	}
	b.WriteString(runeSlice(lines[rB], 0, cB))
	return b.String()
}

func runeSlice(s string, lo, hi int) string {
	rs := []rune(s)
	if lo < 0 {
		lo = 0
	}
	if hi < 0 || hi > len(rs) {
		hi = len(rs)
	}
	if lo > hi {
		return ""
	}
	return string(rs[lo:hi])
}
