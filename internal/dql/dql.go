package dql

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var ValidTimeframes = []string{"15m", "1h", "6h", "24h"}

// TimeframePlaceholder is the literal token users can write in their DQL to
// mark where the time-range picker should substitute. Reserved — Placeholders
// excludes it so the parameter template (Ctrl-P) doesn't prompt for it.
const TimeframePlaceholder = "$timeframe"

var (
	timeframeRE = regexp.MustCompile(`now\(\)\s*-\s*\d+[smhdwMy]`)
)

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

var fromTimeframeRE = regexp.MustCompile(`from:\s*now\(\)\s*-\s*(\d+[smhd])`)
var trailingLimitRE = regexp.MustCompile(`(?i)\|\s*limit\s+\d+\s*$`)

// MakeTimeseries wraps a query with `| makeTimeseries count=count(), interval:X`
// so the result is a count-over-time series suitable for charting. A trailing
// `| limit N` is dropped, since limiting the input rows would distort the
// counts. The chosen interval is derived from the query's `from:now()-<tf>`
// clause; if no timeframe is present, a 1m fallback is used.
func MakeTimeseries(query string) (string, string, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "", "", fmt.Errorf("query is empty")
	}
	q = trailingLimitRE.ReplaceAllString(q, "")
	q = strings.TrimRight(q, " \t\n|")
	interval := IntervalFor(extractTimeframe(query))
	return q + " | makeTimeseries count=count(), interval:" + interval, interval, nil
}

func extractTimeframe(q string) string {
	m := fromTimeframeRE.FindStringSubmatch(q)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// IntervalFor returns a chart bucket size suitable for the given timeframe.
// The aim is roughly 30-90 buckets, which fits comfortably on a wide terminal
// and matches the granularity Grail will accept without complaining.
func IntervalFor(tf string) string {
	switch tf {
	case "15m":
		return "30s"
	case "1h":
		return "1m"
	case "6h":
		return "5m"
	case "24h":
		return "15m"
	}
	return "1m"
}

var placeholderRE = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

var reservedPlaceholders = map[string]bool{
	"timeframe": true, // handled by the time-range picker, not the param form
	"from":      true,
	"to":        true,
}

// Placeholders returns the unique `$name` identifiers in dql in first-occurrence
// order, excluding reserved names that other features handle.
func Placeholders(dql string) []string {
	matches := placeholderRE.FindAllStringSubmatch(dql, -1)
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		if reservedPlaceholders[m[1]] || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	return out
}

// SubstituteTimeframe applies the chosen preset to a DQL query. It tries
// strategies in order:
//
//  1. If the query contains the literal `$timeframe` placeholder, replace
//     every occurrence with `now()-<tf>`.
//  2. Else if the query already has one or more `now()-<duration>` clauses
//     (the typical from:now()-1h shape), swap them for `now()-<tf>`.
//  3. Else if the query has an existing `from:<anything>` clause (typically
//     an absolute timestamp like `from:"2026-04-28T06:12:59Z"`), rewrite that
//     clause to `from:now()-<tf>`. Any `to:` clause is left in place.
//  4. Else fall back to ApplyTimeframe, which injects a `from:` clause into
//     a `fetch <table>` query or appends a filter for non-fetch queries.
func SubstituteTimeframe(query, tf string) (string, error) {
	if !IsValidTimeframe(tf) {
		return "", fmt.Errorf("invalid timeframe %q (allowed: %s)", tf, strings.Join(ValidTimeframes, ", "))
	}
	replacement := "now()-" + tf
	if strings.Contains(query, TimeframePlaceholder) {
		return strings.ReplaceAll(query, TimeframePlaceholder, replacement), nil
	}
	if timeframeRE.MatchString(query) {
		return timeframeRE.ReplaceAllString(query, replacement), nil
	}
	if fromClauseRE.MatchString(query) {
		return fromClauseRE.ReplaceAllString(query, "from:"+replacement), nil
	}
	return ApplyTimeframe(query, tf)
}

// PrependFetch turns the editor's user-typed body into a full DQL query by
// adding `fetch logs` (and the right separator) to the front. If the body
// already starts with a `fetch ...` clause it's returned unchanged so users
// can still query non-logs tables explicitly.
func PrependFetch(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "fetch logs"
	}
	if strings.HasPrefix(trimmed, "fetch ") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "|") {
		return "fetch logs " + trimmed
	}
	return "fetch logs, " + trimmed
}

// StripFetch is the inverse of PrependFetch: removes a leading `fetch logs[,]`
// so a stored or imported query becomes the body the editor displays. Queries
// that don't start with `fetch logs` pass through unchanged.
func StripFetch(s string) string {
	trimmed := strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(trimmed, "fetch logs, "):
		return trimmed[len("fetch logs, "):]
	case strings.HasPrefix(trimmed, "fetch logs |"):
		return trimmed[len("fetch logs "):]
	case strings.HasPrefix(trimmed, "fetch logs ,"):
		return strings.TrimLeft(trimmed[len("fetch logs ,"):], " ")
	case trimmed == "fetch logs":
		return ""
	}
	return trimmed
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

// --- Absolute time-range support -----------------------------------------

const (
	FromPlaceholder = "$from"
	ToPlaceholder   = "$to"
)

var (
	fromClauseRE = regexp.MustCompile(`from\s*:\s*[^,|\s]+`)
	toClauseRE   = regexp.MustCompile(`to\s*:\s*[^,|\s]+`)
)

// flexibleLayouts is tried in order. Date-only inputs become start-of-day for
// from values and end-of-day (23:59:59) for to values.
var flexibleLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
}

// ParseFlexibleTime accepts a few common datetime spellings and returns a UTC
// time. If isEnd is true and the input is date-only, the returned time is
// 23:59:59 of that day (so a single-day pick covers the whole day).
func ParseFlexibleTime(s string, isEnd bool) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	for _, layout := range flexibleLayouts {
		t, err := time.Parse(layout, s)
		if err != nil {
			continue
		}
		if layout == "2006-01-02" && isEnd {
			t = t.Add(24*time.Hour - time.Second)
		}
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %q (try YYYY-MM-DD, YYYY-MM-DD HH:MM, or full ISO)", s)
}

// FormatForDQL renders t as a DQL-quoted ISO 8601 timestamp.
func FormatForDQL(t time.Time) string {
	return `"` + t.UTC().Format(time.RFC3339) + `"`
}

// SubstituteAbsolute applies an absolute time range to a query.
//
// For each of from / to, in order:
//  1. Replace the `$from` / `$to` placeholder if present.
//  2. Else rewrite an existing `from:<expr>` / `to:<expr>` clause.
//  3. Else inject a clause into a leading `fetch <table>`. When both bounds
//     need injection, they are added together so we don't end up with
//     interleaved commas.
//
// If hasTo is false, existing `to:` clauses and `$to` placeholders are left
// untouched so a "from-only" pick doesn't disturb a query that already
// constrains its end.
func SubstituteAbsolute(query string, from, to time.Time, hasTo bool) string {
	fromVal := FormatForDQL(from)
	out := query

	// Step 1: placeholders.
	fromDone := false
	toDone := !hasTo
	if strings.Contains(out, FromPlaceholder) {
		out = strings.ReplaceAll(out, FromPlaceholder, fromVal)
		fromDone = true
	}
	if hasTo && strings.Contains(out, ToPlaceholder) {
		out = strings.ReplaceAll(out, ToPlaceholder, FormatForDQL(to))
		toDone = true
	}

	// Step 2: rewrite existing clauses.
	if !fromDone && fromClauseRE.MatchString(out) {
		out = fromClauseRE.ReplaceAllString(out, "from:"+fromVal)
		fromDone = true
	}
	if !toDone && toClauseRE.MatchString(out) {
		out = toClauseRE.ReplaceAllString(out, "to:"+FormatForDQL(to))
		toDone = true
	}

	// Step 3: inject anything still missing.
	switch {
	case !fromDone && !toDone:
		return injectFetchClauses(out, "from:"+fromVal+", to:"+FormatForDQL(to))
	case !fromDone:
		return injectFetchClauses(out, "from:"+fromVal)
	case !toDone:
		// from is in place — append to: alongside it instead of injecting
		// separately, so we don't push the to-clause to the front of fetch.
		return fromClauseRE.ReplaceAllStringFunc(out, func(s string) string {
			return s + ", to:" + FormatForDQL(to)
		})
	}
	return out
}

// injectFetchClauses inserts the given clauses string after `fetch <table>`.
// Returns the query unchanged if it doesn't start with `fetch `.
func injectFetchClauses(query, clauses string) string {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(trimmed, "fetch ") {
		return query
	}
	head, tail := trimmed, ""
	if i := strings.IndexAny(trimmed[len("fetch "):], ",|"); i >= 0 {
		head = strings.TrimRight(trimmed[:len("fetch ")+i], " \t")
		tail = trimmed[len("fetch ")+i:]
	}
	injected := head + ", " + clauses
	if tail != "" {
		injected += " " + strings.TrimLeft(tail, " \t")
	}
	return injected
}
