package cmd

import "testing"

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
			got, err := applyTimeframe(tc.dql, tc.tf)
			if err != nil {
				t.Fatalf("applyTimeframe: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestApplyTimeframeRejectsInvalid(t *testing.T) {
	if _, err := applyTimeframe("fetch logs", "30m"); err == nil {
		t.Fatal("expected error for invalid timeframe")
	}
}
