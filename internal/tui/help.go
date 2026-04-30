package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type helpEntry struct {
	keys, desc string
}

type helpSection struct {
	title   string
	entries []helpEntry
}

var helpSections = []helpSection{
	{"Global", []helpEntry{
		{"Alt-Enter / Ctrl-Space", "run query"},
		{"Ctrl-G", "chart timeseries"},
		{"Ctrl-T", "time range picker"},
		{"Ctrl-S", "save current query"},
		{"Ctrl-O", "open saved searches"},
		{"Ctrl-F", "insert fragment"},
		{"Ctrl-P", "fill $param templates"},
		{"Ctrl-X", "export results"},
		{"Ctrl-E", "switch environment"},
		{"Ctrl-B", "open in browser (Notebooks)"},
		{"Alt-1 / Alt-2 / Alt-3", "switch view (Query / Saved / Fragments)"},
		{"Tab / Shift-Tab", "cycle focus"},
		{"?", "this help"},
		{"Esc", "cancel running query / close detail"},
		{"Ctrl-C", "cancel query / quit"},
		{"q", "quit (when not editing)"},
	}},
	{"Editor — INSERT mode", []helpEntry{
		{"<text>", "type DQL"},
		{"Esc", "→ NORMAL mode"},
		{"Ctrl-R", "redo (works in either mode)"},
	}},
	{"Editor — NORMAL mode", []helpEntry{
		{"h j k l", "move cursor"},
		{"w / b", "next / prev word"},
		{"0 / $", "line start / end"},
		{"gg / G", "top / bottom of buffer"},
		{"i I a A", "insert here / line start / right / line end"},
		{"o / O", "open line below / above"},
		{"x", "delete char"},
		{"D", "delete to end of line"},
		{"dd / dw / db", "delete line / word fwd / word back"},
		{"yy / yw / yb", "yank line / word fwd / word back"},
		{"p", "paste"},
		{"u", "undo"},
	}},
	{"Results table", []helpEntry{
		{"↑ / ↓ (or k / j)", "move cursor"},
		{"Enter", "open record detail"},
	}},
	{"Detail viewport", []helpEntry{
		{"/", "search in content"},
		{"n / N", "next / previous match"},
		{"gg / G", "top / bottom"},
		{"q / Esc", "close detail"},
	}},
	{"Chart view (nudge timeframe)", []helpEntry{
		{"Tab", "toggle from / to focus (focused endpoint blinks)"},
		{"h / m / s / d", "advance focused endpoint by 1h / 1m / 1s / 1d"},
		{"H / M / S / D", "rewind focused endpoint by 1h / 1m / 1s / 1d"},
		{"Enter / Ctrl-G", "re-run with staged range"},
	}},
	{"Saved searches view (Alt-2)", []helpEntry{
		{"↑ / ↓ (or k / j)", "move cursor"},
		{"Enter", "load and run"},
		{"e", "edit name + body in place"},
		{"*", "toggle as default (auto-run on startup)"},
		{"d", "delete entry"},
	}},
	{"Fragments view (Alt-3)", []helpEntry{
		{"↑ / ↓ (or k / j)", "move cursor"},
		{"Enter", "insert fragment into editor (resolve placeholders)"},
		{"n", "new fragment"},
		{"e", "edit fragment (name, template, suggestions)"},
		{"d", "delete fragment"},
		{"Tab (in edit)", "cycle name → template → suggestion fields"},
		{"Ctrl-S (in edit)", "save"},
	}},
}

func (m Model) updateHelp(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "?", "enter", " ":
		m.modal = modalNone
	}
	return m, nil
}

func (m Model) viewHelp() string {
	keyStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	titleStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Underline(true)

	// Width the key column to the longest key in the legend so the
	// description columns line up.
	keyW := 0
	for _, sec := range helpSections {
		for _, e := range sec.entries {
			if w := lipgloss.Width(e.keys); w > keyW {
				keyW = w
			}
		}
	}

	renderSection := func(sec helpSection) string {
		var b strings.Builder
		b.WriteString(titleStyle.Render(sec.title))
		b.WriteString("\n")
		for _, e := range sec.entries {
			pad := strings.Repeat(" ", keyW-lipgloss.Width(e.keys))
			b.WriteString(keyStyle.Render(e.keys) + pad + "  " + e.desc + "\n")
		}
		return strings.TrimRight(b.String(), "\n")
	}

	// Two-column layout: left column gets Global + Results + Detail +
	// Chart; right column gets editor sections + view-specific shortcuts.
	leftIdx := []int{0, 3, 4, 5}
	rightIdx := []int{1, 2, 6, 7}

	left := make([]string, 0, len(leftIdx))
	for _, i := range leftIdx {
		left = append(left, renderSection(helpSections[i]))
	}
	right := make([]string, 0, len(rightIdx))
	for _, i := range rightIdx {
		right = append(right, renderSection(helpSections[i]))
	}

	leftCol := strings.Join(left, "\n\n")
	rightCol := strings.Join(right, "\n\n")
	gap := lipgloss.NewStyle().Width(4).Render(" ")
	cols := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, gap, rightCol)

	var b strings.Builder
	b.WriteString(paneTitleFocused.Render("Keyboard shortcuts"))
	b.WriteString("\n\n")
	b.WriteString(cols)
	b.WriteString("\n\n")
	b.WriteString(statusBar.Render("Esc / q / ? close"))
	return m.renderModalOverlay(b.String())
}
