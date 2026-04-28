package tui

import (
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
)

var openedRef = time.Date(2026, 4, 28, 13, 42, 17, 0, time.UTC)

func TestResolveValue(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want time.Time
	}{
		{"empty snaps to opened", "", openedRef},
		{"unparseable snaps to opened", "garbage", openedRef},
		{"relative 15m", "now()-15m", openedRef.Add(-15 * time.Minute)},
		{"relative 1h", "now()-1h", openedRef.Add(-time.Hour)},
		{"relative 24h", "now()-24h", openedRef.Add(-24 * time.Hour)},
		{"relative 30s", "now()-30s", openedRef.Add(-30 * time.Second)},
		{"relative 2d", "now()-2d", openedRef.Add(-48 * time.Hour)},
		{"absolute datetime", "2026-04-28 09:00", time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC)},
		{"absolute date+time+seconds", "2026-04-28 09:00:00", time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveValue(tc.in, openedRef)
			if !got.Equal(tc.want) {
				t.Errorf("got %s, want %s", got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}

func TestNudgeFromRelative(t *testing.T) {
	in := textinput.New()
	in.SetValue("now()-15m")
	nudge(&in, openedRef, time.Hour)
	want := openedRef.Add(-15*time.Minute + time.Hour).Format(timeDisplayLayout)
	if got := in.Value(); got != want {
		t.Errorf("nudge from relative: got %q want %q", got, want)
	}
}

func TestNudgeFromEmpty(t *testing.T) {
	in := textinput.New()
	nudge(&in, openedRef, -time.Hour)
	want := openedRef.Add(-time.Hour).Format(timeDisplayLayout)
	if got := in.Value(); got != want {
		t.Errorf("nudge from empty: got %q want %q", got, want)
	}
}

func TestNudgeChained(t *testing.T) {
	in := textinput.New()
	in.SetValue("now()-15m")
	nudge(&in, openedRef, time.Hour)
	nudge(&in, openedRef, time.Minute)
	nudge(&in, openedRef, -time.Second)
	want := openedRef.Add(-15*time.Minute + time.Hour + time.Minute - time.Second).Format(timeDisplayLayout)
	if got := in.Value(); got != want {
		t.Errorf("chained nudge: got %q want %q", got, want)
	}
}

func TestNudgeDelta(t *testing.T) {
	cases := []struct {
		key  string
		want time.Duration
		ok   bool
	}{
		{"h", time.Hour, true},
		{"H", -time.Hour, true},
		{"m", time.Minute, true},
		{"M", -time.Minute, true},
		{"s", time.Second, true},
		{"S", -time.Second, true},
		{"d", 24 * time.Hour, true},
		{"D", -24 * time.Hour, true},
		{"x", 0, false},
		{"", 0, false},
		{"enter", 0, false},
	}
	for _, tc := range cases {
		got, ok := nudgeDelta(tc.key)
		if ok != tc.ok || got != tc.want {
			t.Errorf("nudgeDelta(%q) = (%v,%v), want (%v,%v)", tc.key, got, ok, tc.want, tc.ok)
		}
	}
}

func TestMatchCanonicalRelative(t *testing.T) {
	cases := []struct {
		in     string
		wantTf string
		wantOk bool
	}{
		{"now()-15m", "15m", true},
		{"now()-1h", "1h", true},
		{"now()-6h", "6h", true},
		{"now()-24h", "24h", true},
		{"  now()-1h  ", "1h", true},
		{"now()-30m", "", false}, // not in ValidTimeframes
		{"now()-2h", "", false},
		{"now()-1d", "", false},
		{"2026-04-28 09:00:00", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		gotTf, gotOk := matchCanonicalRelative(tc.in)
		if gotTf != tc.wantTf || gotOk != tc.wantOk {
			t.Errorf("matchCanonicalRelative(%q) = (%q,%v), want (%q,%v)", tc.in, gotTf, gotOk, tc.wantTf, tc.wantOk)
		}
	}
}

func TestFromPicks(t *testing.T) {
	picks := fromPicks(openedRef)
	if len(picks) != 5 {
		t.Fatalf("len = %d, want 5", len(picks))
	}
	wantValues := []string{"now()-15m", "now()-1h", "now()-6h", "now()-24h", "2026-04-28 13:00:00"}
	for i, want := range wantValues {
		if picks[i].value != want {
			t.Errorf("picks[%d].value = %q, want %q", i, picks[i].value, want)
		}
	}
}

func TestToPicks(t *testing.T) {
	picks := toPicks(openedRef)
	if len(picks) != 1 {
		t.Fatalf("len = %d, want 1", len(picks))
	}
	if picks[0].value != "2026-04-28 13:42:17" {
		t.Errorf("picks[0].value = %q, want %q", picks[0].value, "2026-04-28 13:42:17")
	}
}
