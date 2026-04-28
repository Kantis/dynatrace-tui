package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// setDetailContent updates the viewport and remembers the raw content so the
// `/` search can scan it without going through the rendered viewport view. If
// a search query is still active (e.g. after a window resize re-renders the
// detail), matches are recomputed and the visible content gets highlighted.
func (m *Model) setDetailContent(s string) {
	m.detailRawContent = s
	rendered := s
	if q := m.detailSearchQuery; q != "" {
		m.detailSearchMatches = findDetailMatches(s, q)
		rendered = highlightDetailMatches(s, q)
	} else {
		m.detailSearchMatches = nil
	}
	m.detailSearchIdx = 0
	m.detail.SetContent(rendered)
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
		m.detail.SetContent(highlightDetailMatches(m.detailRawContent, query))
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

// SGR codes used to wrap each match. Yellow background + black foreground is
// the conventional `/`-search highlight. The codes are emitted twice: once at
// the start of the match, and again after any inline ANSI sequence within the
// match — chroma's per-token resets would otherwise drop the background
// mid-match. Close uses a full reset since we don't track chroma's prior style.
const (
	matchHighlightOpen  = "\x1b[48;5;226;30m"
	matchHighlightClose = "\x1b[0m"
)

// highlightDetailMatches returns content with each occurrence of query wrapped
// in ANSI background-highlight codes. Search runs on the ANSI-stripped form so
// the chroma highlighting in content doesn't fragment matches.
func highlightDetailMatches(content, query string) string {
	if query == "" || content == "" {
		return content
	}
	var out strings.Builder
	out.Grow(len(content) + 32)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		out.WriteString(highlightLineMatches(line, query))
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// highlightLineMatches injects highlight codes around every occurrence of
// query in line. ANSI sequences are passed through unchanged; whenever a
// match contains an inline escape, the highlight is re-asserted afterward so
// chroma's resets don't drop it.
func highlightLineMatches(line, query string) string {
	if !strings.Contains(ansi.Strip(line), query) {
		return line
	}

	type segment struct {
		text string
		ansi bool
	}
	var segs []segment
	for i := 0; i < len(line); {
		if line[i] == 0x1b && i+1 < len(line) && line[i+1] == '[' {
			j := i + 2
			for j < len(line) && !isCSIFinal(line[j]) {
				j++
			}
			if j < len(line) {
				j++
			}
			segs = append(segs, segment{line[i:j], true})
			i = j
			continue
		}
		start := i
		for i < len(line) && line[i] != 0x1b {
			i++
		}
		segs = append(segs, segment{line[start:i], false})
	}

	var plainBuf strings.Builder
	for _, s := range segs {
		if !s.ansi {
			plainBuf.WriteString(s.text)
		}
	}
	plain := plainBuf.String()

	type span struct{ start, end int }
	var matches []span
	cursor := 0
	for cursor <= len(plain) {
		idx := strings.Index(plain[cursor:], query)
		if idx < 0 {
			break
		}
		s := cursor + idx
		e := s + len(query)
		matches = append(matches, span{s, e})
		cursor = e
	}
	if len(matches) == 0 {
		return line
	}

	var b strings.Builder
	b.Grow(len(line) + len(matches)*16)
	plainCursor := 0
	matchI := 0
	inMatch := false
	for _, s := range segs {
		if s.ansi {
			b.WriteString(s.text)
			if inMatch {
				b.WriteString(matchHighlightOpen)
			}
			continue
		}
		for k := 0; k < len(s.text); k++ {
			if !inMatch && matchI < len(matches) && plainCursor == matches[matchI].start {
				b.WriteString(matchHighlightOpen)
				inMatch = true
			}
			b.WriteByte(s.text[k])
			plainCursor++
			if inMatch && plainCursor == matches[matchI].end {
				b.WriteString(matchHighlightClose)
				inMatch = false
				matchI++
			}
		}
	}
	if inMatch {
		b.WriteString(matchHighlightClose)
	}
	return b.String()
}

func isCSIFinal(b byte) bool {
	return b >= 0x40 && b <= 0x7e
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
