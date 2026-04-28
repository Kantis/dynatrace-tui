package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHighlightDetailMatches_PlainText(t *testing.T) {
	out := highlightDetailMatches("foo bar foo", "foo")
	if !strings.Contains(out, matchHighlightOpen+"foo"+matchHighlightClose) {
		t.Errorf("expected highlight wrap around 'foo', got %q", out)
	}
	// Stripping the inserted ANSI must yield the original content.
	if got := ansi.Strip(out); got != "foo bar foo" {
		t.Errorf("strip(highlighted) = %q, want %q", got, "foo bar foo")
	}
	// Two matches, two open/close pairs.
	if got := strings.Count(out, matchHighlightOpen); got != 2 {
		t.Errorf("opens = %d, want 2", got)
	}
	if got := strings.Count(out, matchHighlightClose); got != 2 {
		t.Errorf("closes = %d, want 2", got)
	}
}

func TestHighlightDetailMatches_EmptyQuery(t *testing.T) {
	in := "anything"
	if got := highlightDetailMatches(in, ""); got != in {
		t.Errorf("empty query mutated content: %q", got)
	}
}

func TestHighlightDetailMatches_NoMatch(t *testing.T) {
	in := "alpha beta"
	if got := highlightDetailMatches(in, "gamma"); got != in {
		t.Errorf("non-matching query mutated content: %q", got)
	}
}

func TestHighlightDetailMatches_PreservesAnsiAndPlainText(t *testing.T) {
	// chroma-style fragmented match: \x1b[33mlog\x1b[0mlevel — the literal "log"
	// straddles an inline reset, so the highlight must be re-asserted after the
	// embedded ANSI to keep the background painted.
	in := "prefix \x1b[33mlog\x1b[0mlevel suffix"
	out := highlightDetailMatches(in, "loglevel")
	if got := ansi.Strip(out); got != "prefix loglevel suffix" {
		t.Errorf("strip(highlighted) = %q, want %q", got, "prefix loglevel suffix")
	}
	// Open should appear at the start of the match AND once more after the
	// embedded \x1b[0m reset inside the match.
	if got := strings.Count(out, matchHighlightOpen); got < 2 {
		t.Errorf("expected highlight to be re-asserted after inline ANSI, got %d opens in %q", got, out)
	}
}

func TestHighlightDetailMatches_MultilinePreservesNewlines(t *testing.T) {
	in := "foo\nbar\nfoobar"
	out := highlightDetailMatches(in, "foo")
	gotLines := strings.Split(ansi.Strip(out), "\n")
	wantLines := []string{"foo", "bar", "foobar"}
	if len(gotLines) != len(wantLines) {
		t.Fatalf("line count = %d, want %d", len(gotLines), len(wantLines))
	}
	for i := range wantLines {
		if gotLines[i] != wantLines[i] {
			t.Errorf("line %d: got %q, want %q", i, gotLines[i], wantLines[i])
		}
	}
}
