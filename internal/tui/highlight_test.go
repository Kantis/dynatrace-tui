package tui

import (
	"reflect"
	"strings"
	"testing"
)

func TestExpandJSONStrings_Object(t *testing.T) {
	in := map[string]any{
		"timestamp": "2026-04-28T12:00:00Z",
		"content":   `{"event":"login","user":"emil"}`,
	}
	got := expandJSONStrings(in).(map[string]any)
	want := map[string]any{
		"timestamp": "2026-04-28T12:00:00Z",
		"content": map[string]any{
			"event": "login",
			"user":  "emil",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestExpandJSONStrings_Array(t *testing.T) {
	in := map[string]any{"x": `[1,2,3]`}
	got := expandJSONStrings(in).(map[string]any)
	arr, ok := got["x"].([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", got["x"])
	}
	if len(arr) != 3 {
		t.Errorf("array length = %d, want 3", len(arr))
	}
}

func TestExpandJSONStrings_NestedExpansion(t *testing.T) {
	in := map[string]any{
		"outer": `{"inner":"{\"deep\":42}"}`,
	}
	got := expandJSONStrings(in).(map[string]any)
	outer := got["outer"].(map[string]any)
	inner, ok := outer["inner"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested object after recursive expansion, got %#v", outer["inner"])
	}
	if inner["deep"] != float64(42) {
		t.Errorf("deep = %v, want 42", inner["deep"])
	}
}

func TestExpandJSONStrings_LeavesPrimitivesAsStrings(t *testing.T) {
	// "123" is valid JSON (a number) but expanding would hide its string-ness;
	// only objects/arrays are expanded.
	in := map[string]any{"port": "123", "active": "true"}
	got := expandJSONStrings(in).(map[string]any)
	if got["port"] != "123" {
		t.Errorf(`got["port"] = %v, want "123"`, got["port"])
	}
	if got["active"] != "true" {
		t.Errorf(`got["active"] = %v, want "true"`, got["active"])
	}
}

func TestExpandJSONStrings_LeavesNonJSONStringsAlone(t *testing.T) {
	in := map[string]any{"msg": "hello world", "blank": "", "looks-like": "{not json"}
	got := expandJSONStrings(in).(map[string]any)
	if got["msg"] != "hello world" {
		t.Errorf("msg changed: %v", got["msg"])
	}
	if got["blank"] != "" {
		t.Errorf("blank changed: %v", got["blank"])
	}
	if got["looks-like"] != "{not json" {
		t.Errorf("invalid JSON should pass through unchanged: %v", got["looks-like"])
	}
}

func TestHighlightJSONAddsAnsiCodes(t *testing.T) {
	in := `{"a": 1, "b": "x"}`
	got := highlightJSON(in)
	if got == in {
		t.Errorf("highlightJSON returned input unchanged")
	}
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("expected ANSI escape codes in output")
	}
}

func TestHighlightJSONCell_HighlightsObjectsAndArrays(t *testing.T) {
	cases := []string{
		`{"a":1}`,
		`[1,2,3]`,
		`  {"event":"login"}  `, // tolerates surrounding whitespace
	}
	for _, in := range cases {
		got := highlightJSONCell(in)
		if !strings.Contains(got, "\x1b[") {
			t.Errorf("highlightJSONCell(%q) = %q, want ANSI codes", in, got)
		}
	}
}

func TestHighlightJSONCell_LeavesNonJSONAlone(t *testing.T) {
	cases := []string{
		"",
		"plain log line",
		"{not json",
		`"123"`,    // JSON primitive — we only highlight objects/arrays
		"INFO",
	}
	for _, in := range cases {
		if got := highlightJSONCell(in); got != in {
			t.Errorf("highlightJSONCell(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestRenderRecordDetailIncludesExpandedAndAnsi(t *testing.T) {
	rec := map[string]any{
		"timestamp": "2026-04-28T12:00:00Z",
		"content":   `{"event":"login"}`,
	}
	out := renderRecordDetail(rec, 0)
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI codes in detail output")
	}
	// The expanded content should appear as a nested object — i.e. the
	// pretty-printed JSON contains "event" as a key (not inside a quoted
	// string with escapes).
	if !strings.Contains(out, `"event"`) {
		t.Errorf("expected expanded inner key to appear in output")
	}
	if strings.Contains(out, `\"event\"`) {
		t.Errorf("found escaped JSON — expansion didn't run")
	}
}

func TestRenderRecordDetailWrapsLongLines(t *testing.T) {
	rec := map[string]any{
		"long": strings.Repeat("x", 200),
	}
	const width = 40
	out := renderRecordDetail(rec, width)
	// Strip ANSI before measuring so chroma's color codes don't inflate widths.
	for _, line := range strings.Split(out, "\n") {
		if w := lipglossWidth(line); w > width {
			t.Errorf("line width %d exceeds wrap limit %d: %q", w, width, line)
		}
	}
}

// lipglossWidth measures the on-screen width of an ANSI-styled string by
// stripping escapes and counting runes. Avoids pulling lipgloss into a test.
func lipglossWidth(s string) int {
	stripped := stripAnsi(s)
	return len([]rune(stripped))
}

func stripAnsi(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if r == 0x1b {
			in = true
			continue
		}
		if in {
			if r == 'm' {
				in = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
