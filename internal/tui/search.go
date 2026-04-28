package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// setDetailContent updates the viewport and remembers the raw content so the
// `/` search can scan it without going through the rendered viewport view.
func (m *Model) setDetailContent(s string) {
	m.detailRawContent = s
	m.detail.SetContent(s)
	// New content invalidates the previous match list.
	m.detailSearchMatches = nil
	m.detailSearchIdx = 0
}

func newDetailSearchInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = "search"
	ti.CharLimit = 256
	ti.Focus()
	return ti
}

// startDetailSearch opens the search input. Pre-fills with the previous query
// so re-opening `/` is a quick "edit and resubmit" path.
func (m Model) startDetailSearch() Model {
	m.detailSearchInput = newDetailSearchInput()
	m.detailSearchInput.SetValue(m.detailSearchQuery)
	m.detailSearchInput.CursorEnd()
	m.detailSearchActive = true
	return m
}

// updateDetailSearchInput routes a keystroke to the search input. Enter
// commits the query; Esc cancels.
func (m Model) updateDetailSearchInput(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.detailSearchActive = false
		m.detailSearchInput.Blur()
		return m, nil
	case "enter":
		query := strings.TrimSpace(m.detailSearchInput.Value())
		m.detailSearchActive = false
		m.detailSearchInput.Blur()
		m.detailSearchQuery = query
		m.detailSearchMatches = findDetailMatches(m.detailRawContent, query)
		m.detailSearchIdx = 0
		if len(m.detailSearchMatches) > 0 {
			m.scrollDetailToMatch(0)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.detailSearchInput, cmd = m.detailSearchInput.Update(msg)
	return m, cmd
}

// nextDetailMatch / prevDetailMatch jump to the next/previous match, wrapping
// around either end. No-ops when there are no matches.
func (m Model) nextDetailMatch() Model {
	if len(m.detailSearchMatches) == 0 {
		return m
	}
	m.detailSearchIdx = (m.detailSearchIdx + 1) % len(m.detailSearchMatches)
	m.scrollDetailToMatch(m.detailSearchIdx)
	return m
}

func (m Model) prevDetailMatch() Model {
	if len(m.detailSearchMatches) == 0 {
		return m
	}
	m.detailSearchIdx = (m.detailSearchIdx - 1 + len(m.detailSearchMatches)) % len(m.detailSearchMatches)
	m.scrollDetailToMatch(m.detailSearchIdx)
	return m
}

// scrollDetailToMatch puts the matched line a few rows below the top of the
// viewport so context above the hit stays visible.
func (m *Model) scrollDetailToMatch(idx int) {
	if idx < 0 || idx >= len(m.detailSearchMatches) {
		return
	}
	line := m.detailSearchMatches[idx]
	target := line - 2
	if target < 0 {
		target = 0
	}
	m.detail.YOffset = target
}

// findDetailMatches scans the raw content (post-ANSI strip) for case-sensitive
// substring hits and returns the line indices where each hit starts. Empty
// queries return nil.
func findDetailMatches(content, query string) []int {
	if query == "" || content == "" {
		return nil
	}
	var out []int
	for i, line := range strings.Split(content, "\n") {
		plain := ansi.Strip(line)
		if strings.Contains(plain, query) {
			out = append(out, i)
		}
	}
	return out
}

// detailSearchStatus returns the right-aligned status fragment shown when a
// search is active or has results. Empty when there's nothing to say.
func (m Model) detailSearchStatus() string {
	if m.detailSearchQuery == "" {
		return ""
	}
	if len(m.detailSearchMatches) == 0 {
		return fmt.Sprintf("/%s · no matches", m.detailSearchQuery)
	}
	return fmt.Sprintf("/%s · %d/%d · n next · N prev", m.detailSearchQuery, m.detailSearchIdx+1, len(m.detailSearchMatches))
}
