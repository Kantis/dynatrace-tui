package tui

import (
	"encoding/json"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

// expandJSONStrings walks v and replaces any string value that itself contains
// valid JSON (an object or array) with the parsed value. It recurses into the
// expanded result so deeply nested stringified JSON unwraps fully. Numbers,
// booleans, and bare strings inside JSON strings are left alone — only `{...}`
// and `[...]` shapes are expanded, since otherwise we'd hide that something
// like "123" was actually a string.
func expandJSONStrings(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = expandJSONStrings(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = expandJSONStrings(val)
		}
		return out
	case string:
		trimmed := strings.TrimSpace(x)
		if trimmed == "" {
			return x
		}
		switch trimmed[0] {
		case '{', '[':
		default:
			return x
		}
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return x
		}
		return expandJSONStrings(parsed)
	default:
		return x
	}
}

// highlightJSON applies syntax highlighting to a JSON document using chroma's
// 256-color terminal formatter. Falls back to the unstyled string if chroma
// can't process it for any reason.
func highlightJSON(s string) string {
	lexer := lexers.Get("json")
	if lexer == nil {
		return s
	}
	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}
	formatter := formatters.Get("terminal256")
	if formatter == nil {
		return s
	}
	iter, err := lexer.Tokenise(nil, s)
	if err != nil {
		return s
	}
	var buf strings.Builder
	if err := formatter.Format(&buf, style, iter); err != nil {
		return s
	}
	return buf.String()
}

// renderRecordDetail produces the highlighted, JSON-expanded view of a single
// record for the detail viewport. When width > 0 the output is hard-wrapped
// to that width so long values stay visible without horizontal scroll. ANSI
// escapes from chroma are preserved across the wrap.
//
// When `simplified` is true and the expanded record carries a structured `msg`
// value, only that value is rendered — the rest of the record (timestamps,
// host attributes, etc.) is hidden so the user can focus on the payload.
// Falls back to the full record when there's no structured msg.
func renderRecordDetail(rec map[string]any, width int, simplified bool) string {
	expanded := expandJSONStrings(rec)
	if simplified {
		if expandedMap, ok := expanded.(map[string]any); ok {
			if msg, has := expandedMap["msg"]; has && isStructured(msg) {
				return renderJSONValue(msg, width)
			}
		}
	}
	return renderJSONValue(expanded, width)
}

// recordHasStructuredMsg reports whether the simplified rendering would kick
// in for rec. Used by the title bar to label the pane accurately.
func recordHasStructuredMsg(rec map[string]any) bool {
	expanded := expandJSONStrings(rec)
	expandedMap, ok := expanded.(map[string]any)
	if !ok {
		return false
	}
	msg, has := expandedMap["msg"]
	return has && isStructured(msg)
}

func renderJSONValue(v any, width int) string {
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err.Error()
	}
	out := highlightJSON(string(pretty))
	if width > 0 {
		out = ansi.Hardwrap(out, width, false)
	}
	return out
}

// isStructured reports whether v is a JSON object or array — i.e. has nested
// shape worth viewing on its own.
func isStructured(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}

// highlightJSONCell returns s with chroma highlighting if it parses as a JSON
// object or array; otherwise s is returned unchanged. Used for inline table
// cells, where adding ANSI codes only makes sense if the value is JSON.
func highlightJSONCell(s string) string {
	t := strings.TrimSpace(s)
	if len(t) < 2 {
		return s
	}
	if t[0] != '{' && t[0] != '[' {
		return s
	}
	var v any
	if err := json.Unmarshal([]byte(t), &v); err != nil {
		return s
	}
	return highlightJSON(s)
}
