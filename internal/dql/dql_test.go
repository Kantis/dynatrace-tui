package dql

import (
	"reflect"
	"testing"
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

func TestSubstituteUnknownLeftAlone(t *testing.T) {
	got := Substitute(`filter service=="$service" and host=="$host"`, map[string]string{"service": "x"})
	want := `filter service=="x" and host=="$host"`
	if got != want {
		t.Errorf("Substitute got %q want %q", got, want)
	}
}
