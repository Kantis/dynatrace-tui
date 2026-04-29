package dql

import (
	"reflect"
	"testing"
	"time"
)

func TestApplyTimeframe(t *testing.T) {
	cases := []struct {
		name string
		dql  string
		tf   string
		want string
	}{
		{"empty tf is identity", "fetch logs | limit 5", "", "fetch logs | limit 5"},
		{"injects into fetch with pipe", "fetch logs | limit 5", "1h", "fetch logs, from:now()-1h | limit 5"},
		{"injects into fetch with no tail", "fetch logs", "15m", "fetch logs, from:now()-15m"},
		{"skips when from: already present", "fetch logs, from:now()-1d", "1h", "fetch logs, from:now()-1d"},
		{"appends filter for non-fetch start", "timeseries count() | limit 5", "6h", "timeseries count() | limit 5 | filter timestamp > now()-6h"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ApplyTimeframe(tc.dql, tc.tf)
			if err != nil {
				t.Fatalf("ApplyTimeframe: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestApplyTimeframeRejectsInvalid(t *testing.T) {
	if _, err := ApplyTimeframe("fetch logs", "30m"); err == nil {
		t.Fatal("expected error for invalid timeframe")
	}
}

func TestPlaceholders(t *testing.T) {
	got := Placeholders("fetch logs | filter service==$service and level==$level and again $service")
	want := []string{"service", "level"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Placeholders = %v, want %v", got, want)
	}
}

func TestSubstitute(t *testing.T) {
	got := Substitute(`filter service=="$service" and level=="$level"`, map[string]string{
		"service": `"checkout"`,
		"level":   `"ERROR"`,
	})
	want := `filter service==""checkout"" and level==""ERROR""`
	if got != want {
		t.Errorf("Substitute got %q want %q", got, want)
	}
}

func TestEnsureDefaultSort(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain fetch", "fetch logs", "fetch logs | sort timestamp desc"},
		{"with limit", "fetch logs | limit 5", "fetch logs | limit 5 | sort timestamp desc"},
		{"trailing pipe and whitespace", "fetch logs |  ", "fetch logs | sort timestamp desc"},
		{"already sorted", "fetch logs | sort timestamp asc", "fetch logs | sort timestamp asc"},
		{"sort with extra whitespace", "fetch logs |  sort foo desc", "fetch logs |  sort foo desc"},
		{"sort with mixed case", "fetch logs | Sort timestamp desc", "fetch logs | Sort timestamp desc"},
		{"summarize skips", "fetch logs | summarize count(), by:loglevel", "fetch logs | summarize count(), by:loglevel"},
		{"makeTimeseries skips", "fetch logs | makeTimeseries count=count(), interval:1m", "fetch logs | makeTimeseries count=count(), interval:1m"},
		{"fieldsSummary skips", "fetch logs | fieldsSummary", "fetch logs | fieldsSummary"},
		{"fields does not skip", "fetch logs | fields timestamp, content", "fetch logs | fields timestamp, content | sort timestamp desc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EnsureDefaultSort(c.in); got != c.want {
				t.Errorf("EnsureDefaultSort(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSubstituteTimeframe_PlaceholderTakesPrecedence(t *testing.T) {
	got, err := SubstituteTimeframe("fetch logs, from:$timeframe | limit 5", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if got != "fetch logs, from:now()-1h | limit 5" {
		t.Errorf("got %q", got)
	}
}

func TestSubstituteTimeframe_PlaceholderMultipleOccurrences(t *testing.T) {
	got, err := SubstituteTimeframe("from:$timeframe to:$timeframe", "6h")
	if err != nil {
		t.Fatal(err)
	}
	if got != "from:now()-6h to:now()-6h" {
		t.Errorf("got %q", got)
	}
}

func TestSubstituteTimeframe_RewritesExistingNowClause(t *testing.T) {
	got, err := SubstituteTimeframe("fetch logs, from:now()-15m | limit 5", "24h")
	if err != nil {
		t.Fatal(err)
	}
	if got != "fetch logs, from:now()-24h | limit 5" {
		t.Errorf("got %q", got)
	}
}

func TestSubstituteTimeframe_RewritesAllNowClauses(t *testing.T) {
	// Two now() clauses → both updated.
	got, err := SubstituteTimeframe("fetch logs, from:now()-30m to:now()-1m", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if got != "fetch logs, from:now()-1h to:now()-1h" {
		t.Errorf("got %q", got)
	}
}

func TestSubstituteTimeframe_RewritesAbsoluteFromClause(t *testing.T) {
	got, err := SubstituteTimeframe(`fetch logs, from:"2026-04-28T06:12:59Z", to:"2026-04-28T14:12:59Z"`, "15m")
	if err != nil {
		t.Fatal(err)
	}
	want := `fetch logs, from:now()-15m, to:"2026-04-28T14:12:59Z"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubstituteTimeframe_RewritesAbsoluteFromClauseWithoutTo(t *testing.T) {
	got, err := SubstituteTimeframe(`fetch logs, from:"2026-04-28T06:12:59Z" | limit 5`, "1h")
	if err != nil {
		t.Fatal(err)
	}
	want := `fetch logs, from:now()-1h | limit 5`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubstituteTimeframe_FallsBackToApplyTimeframe(t *testing.T) {
	// No placeholder, no existing now() — should inject like ApplyTimeframe does.
	got, err := SubstituteTimeframe("fetch logs | limit 5", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if got != "fetch logs, from:now()-1h | limit 5" {
		t.Errorf("got %q", got)
	}
}

func TestPlaceholdersExcludesTimeframe(t *testing.T) {
	got := Placeholders("from:$timeframe | filter service==$service")
	want := []string{"service"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Placeholders = %v, want %v", got, want)
	}
}

func TestPlaceholdersExcludesFromTo(t *testing.T) {
	got := Placeholders("from:$from, to:$to | filter service==$service")
	want := []string{"service"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Placeholders = %v, want %v", got, want)
	}
}

func TestParseFlexibleTime(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		isEnd bool
		want  string // RFC3339 expected
	}{
		{"date start", "2026-04-28", false, "2026-04-28T00:00:00Z"},
		{"date end", "2026-04-28", true, "2026-04-28T23:59:59Z"},
		{"date+time space", "2026-04-28 09:00", false, "2026-04-28T09:00:00Z"},
		{"date+time T", "2026-04-28T09:00:00", false, "2026-04-28T09:00:00Z"},
		{"rfc3339 utc", "2026-04-28T09:00:00Z", false, "2026-04-28T09:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseFlexibleTime(tc.in, tc.isEnd)
			if err != nil {
				t.Fatal(err)
			}
			if got.Format(time.RFC3339) != tc.want {
				t.Errorf("got %q, want %q", got.Format(time.RFC3339), tc.want)
			}
		})
	}
}

func TestParseFlexibleTimeRejectsGarbage(t *testing.T) {
	if _, err := ParseFlexibleTime("not a time", false); err == nil {
		t.Error("expected error")
	}
	if _, err := ParseFlexibleTime("", false); err == nil {
		t.Error("expected error for empty string")
	}
}

func TestSubstituteAbsolute_PlaceholdersBoth(t *testing.T) {
	from := mustTime(t, "2026-04-28T09:00:00Z")
	to := mustTime(t, "2026-04-28T17:00:00Z")
	got := SubstituteAbsolute(`fetch logs, from:$from, to:$to | limit 5`, from, to, true)
	want := `fetch logs, from:"2026-04-28T09:00:00Z", to:"2026-04-28T17:00:00Z" | limit 5`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubstituteAbsolute_RewritesExistingClauses(t *testing.T) {
	from := mustTime(t, "2026-04-28T09:00:00Z")
	to := mustTime(t, "2026-04-28T17:00:00Z")
	got := SubstituteAbsolute(`fetch logs, from:now()-1h, to:now() | limit 5`, from, to, true)
	want := `fetch logs, from:"2026-04-28T09:00:00Z", to:"2026-04-28T17:00:00Z" | limit 5`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubstituteAbsolute_InjectsWhenAbsent(t *testing.T) {
	from := mustTime(t, "2026-04-28T09:00:00Z")
	to := mustTime(t, "2026-04-28T17:00:00Z")
	got := SubstituteAbsolute(`fetch logs | limit 5`, from, to, true)
	want := `fetch logs, from:"2026-04-28T09:00:00Z", to:"2026-04-28T17:00:00Z" | limit 5`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubstituteAbsolute_FromOnlyPreservesExistingTo(t *testing.T) {
	from := mustTime(t, "2026-04-28T09:00:00Z")
	got := SubstituteAbsolute(`fetch logs, from:now()-1h, to:now()-30m | limit 5`, from, time.Time{}, false)
	want := `fetch logs, from:"2026-04-28T09:00:00Z", to:now()-30m | limit 5`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubstituteAbsolute_AddsToWhenInjectingBoth(t *testing.T) {
	from := mustTime(t, "2026-04-28T09:00:00Z")
	to := mustTime(t, "2026-04-28T17:00:00Z")
	// Existing from: but no to: — inject the to alongside.
	got := SubstituteAbsolute(`fetch logs, from:now()-1h | limit 5`, from, to, true)
	want := `fetch logs, from:"2026-04-28T09:00:00Z", to:"2026-04-28T17:00:00Z" | limit 5`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrependFetch(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "fetch logs"},
		{"  ", "fetch logs"},
		{"from:now()-1h", "fetch logs, from:now()-1h"},
		{"| filter level == \"ERROR\"", "fetch logs | filter level == \"ERROR\""},
		{"fetch events", "fetch events"},
		{"fetch logs, from:now()-1h", "fetch logs, from:now()-1h"},
	}
	for _, tc := range cases {
		if got := PrependFetch(tc.in); got != tc.want {
			t.Errorf("PrependFetch(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStripFetch(t *testing.T) {
	cases := []struct{ in, want string }{
		{"fetch logs, from:now()-1h", "from:now()-1h"},
		{"fetch logs | filter level == \"ERROR\"", "| filter level == \"ERROR\""},
		{"fetch logs", ""},
		{"fetch events, from:now()-1h", "fetch events, from:now()-1h"},
		{"timeseries count() | limit 5", "timeseries count() | limit 5"},
	}
	for _, tc := range cases {
		if got := StripFetch(tc.in); got != tc.want {
			t.Errorf("StripFetch(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPrependStripRoundTrip(t *testing.T) {
	bodies := []string{
		"from:now()-1h",
		"| filter level == \"ERROR\"",
		"from:now()-1h | filter k8s.namespace.name == \"casino\"",
	}
	for _, body := range bodies {
		full := PrependFetch(body)
		round := StripFetch(full)
		if round != body {
			t.Errorf("round-trip %q → %q → %q", body, full, round)
		}
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSubstituteUnknownLeftAlone(t *testing.T) {
	got := Substitute(`filter service=="$service" and host=="$host"`, map[string]string{"service": "x"})
	want := `filter service=="x" and host=="$host"`
	if got != want {
		t.Errorf("Substitute got %q want %q", got, want)
	}
}

func TestMakeTimeseries(t *testing.T) {
	cases := []struct {
		name         string
		dql          string
		wantQuery    string
		wantInterval string
	}{
		{
			"basic with 1h timeframe picks 1m interval",
			"fetch logs, from:now()-1h",
			"fetch logs, from:now()-1h | makeTimeseries count=count(), interval:1m",
			"1m",
		},
		{
			"strips trailing limit",
			"fetch logs, from:now()-1h | filter loglevel==\"ERROR\" | limit 50",
			"fetch logs, from:now()-1h | filter loglevel==\"ERROR\" | makeTimeseries count=count(), interval:1m",
			"1m",
		},
		{
			"24h timeframe → 15m interval",
			"fetch logs, from:now()-24h | filter status==\"ERROR\"",
			"fetch logs, from:now()-24h | filter status==\"ERROR\" | makeTimeseries count=count(), interval:15m",
			"15m",
		},
		{
			"15m timeframe → 30s interval",
			"fetch logs, from:now()-15m",
			"fetch logs, from:now()-15m | makeTimeseries count=count(), interval:30s",
			"30s",
		},
		{
			"no timeframe falls back to 1m",
			"fetch logs",
			"fetch logs | makeTimeseries count=count(), interval:1m",
			"1m",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, interval, err := MakeTimeseries(tc.dql)
			if err != nil {
				t.Fatalf("MakeTimeseries: %v", err)
			}
			if got != tc.wantQuery {
				t.Errorf("query: got %q want %q", got, tc.wantQuery)
			}
			if interval != tc.wantInterval {
				t.Errorf("interval: got %q want %q", interval, tc.wantInterval)
			}
		})
	}
}

func TestMakeTimeseriesEmpty(t *testing.T) {
	if _, _, err := MakeTimeseries("   "); err == nil {
		t.Fatal("expected error for empty query")
	}
}
