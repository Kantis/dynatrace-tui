package dql

import (
	"fmt"
	"regexp"
	"strings"
)

var ValidTimeframes = []string{"15m", "1h", "6h", "24h"}

func IsValidTimeframe(tf string) bool {
	for _, v := range ValidTimeframes {
		if v == tf {
			return true
		}
	}
	return false
}

// ApplyTimeframe injects `from:now()-<tf>` into a DQL query. If the query
// already contains "from:" it is returned unchanged. For queries that begin
// with `fetch <table>` the clause is injected after the table name; otherwise
// a `| filter timestamp > now()-<tf>` is appended.
func ApplyTimeframe(dql, tf string) (string, error) {
	if tf == "" {
		return dql, nil
	}
	if !IsValidTimeframe(tf) {
		return "", fmt.Errorf("invalid timeframe %q (allowed: %s)", tf, strings.Join(ValidTimeframes, ", "))
	}
	if strings.Contains(dql, "from:") {
		return dql, nil
	}
	trimmed := strings.TrimSpace(dql)
	if strings.HasPrefix(trimmed, "fetch ") {
		head, tail := trimmed, ""
		if i := strings.IndexAny(trimmed[len("fetch "):], ",|"); i >= 0 {
			head = strings.TrimRight(trimmed[:len("fetch ")+i], " \t")
			tail = trimmed[len("fetch ")+i:]
		}
		injected := head + ", from:now()-" + tf
		if tail != "" {
			injected += " " + strings.TrimLeft(tail, " \t")
		}
		return injected, nil
	}
	return trimmed + " | filter timestamp > now()-" + tf, nil
}

var placeholderRE = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// Placeholders returns the unique `$name` identifiers in dql in first-occurrence order.
func Placeholders(dql string) []string {
	matches := placeholderRE.FindAllStringSubmatch(dql, -1)
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

// Substitute replaces `$name` occurrences with the provided values.
func Substitute(dql string, values map[string]string) string {
	return placeholderRE.ReplaceAllStringFunc(dql, func(m string) string {
		name := m[1:]
		if v, ok := values[name]; ok {
			return v
		}
		return m
	})
}
