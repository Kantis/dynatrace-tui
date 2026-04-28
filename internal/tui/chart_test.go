package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/kantis/dynatrace-tui/internal/grail"
)

func TestRenderChartHasBars(t *testing.T) {
	rec := map[string]any{
		"count":    []any{0.0, 3.0, 7.0, 1.0, 12.0, 0.0, 5.0},
		"interval": "PT1M",
		"timeframe": map[string]any{
			"start": "2026-04-28T12:00:00.000Z",
			"end":   "2026-04-28T12:07:00.000Z",
		},
	}
	out := renderChart(grail.Records{rec}, 60, 12, time.Time{}, time.Time{}, nudgeFrom, false)

	if !strings.Contains(out, "█") {
		t.Errorf("expected at least one full block in chart output, got:\n%s", out)
	}
	if !strings.Contains(out, "interval 1m") {
		t.Errorf("expected interval label in header, got:\n%s", out)
	}
	if !strings.Contains(out, "max 12") {
		t.Errorf("expected max label, got:\n%s", out)
	}
}

func TestRenderChartHandlesNilSamples(t *testing.T) {
	rec := map[string]any{
		"count":    []any{nil, 1.0, nil, 2.0},
		"interval": "PT30S",
		"timeframe": map[string]any{
			"start": "2026-04-28T12:00:00Z",
			"end":   "2026-04-28T12:02:00Z",
		},
	}
	out := renderChart(grail.Records{rec}, 40, 10, time.Time{}, time.Time{}, nudgeFrom, false)
	if !strings.Contains(out, "max 2") {
		t.Errorf("nil samples should be treated as 0; got:\n%s", out)
	}
}

func TestRenderChartHandlesAllZero(t *testing.T) {
	rec := map[string]any{
		"count":    []any{0.0, 0.0, 0.0},
		"interval": "PT1M",
		"timeframe": map[string]any{
			"start": "2026-04-28T12:00:00Z",
			"end":   "2026-04-28T12:03:00Z",
		},
	}
	out := renderChart(grail.Records{rec}, 40, 10, time.Time{}, time.Time{}, nudgeFrom, false)
	if strings.Contains(out, "█") {
		t.Errorf("all-zero series should render no bars; got:\n%s", out)
	}
}

func TestRenderChartFallsBackToFirstNumericSeries(t *testing.T) {
	rec := map[string]any{
		"avg(latency)": []any{10.0, 20.0, 30.0},
		"interval":     "PT1M",
		"timeframe": map[string]any{
			"start": "2026-04-28T12:00:00Z",
			"end":   "2026-04-28T12:03:00Z",
		},
	}
	out := renderChart(grail.Records{rec}, 40, 10, time.Time{}, time.Time{}, nudgeFrom, false)
	if !strings.Contains(out, "avg(latency)") {
		t.Errorf("expected series label, got:\n%s", out)
	}
}

func TestPrettyInterval(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"ISO hour", "PT1H", "1h"},
		{"ISO minute", "PT5M", "5m"},
		{"ISO seconds", "PT30S", "30s"},
		{"nanoseconds float (1m)", float64(60_000_000_000), "1m"},
		{"nanoseconds float (5s)", float64(5_000_000_000), "5s"},
		{"nanoseconds float (1h)", float64(3_600_000_000_000), "1h"},
		{"nanoseconds float (1d)", float64(86_400_000_000_000), "1d"},
		{"nanoseconds string (2m)", "120000000000", "2m"},
		{"nanoseconds string (1h)", "3600000000000", "1h"},
		{"go duration string", "2m0s", "2m"},
		{"empty string", "", "?"},
		{"nil", nil, "?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := prettyInterval(tc.in)
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestRenderChartFormatsNumericInterval(t *testing.T) {
	rec := map[string]any{
		"count":    []any{1.0, 2.0, 3.0},
		"interval": float64(60_000_000_000), // 1 minute as nanoseconds
		"timeframe": map[string]any{
			"start": "2026-04-28T12:00:00Z",
			"end":   "2026-04-28T12:03:00Z",
		},
	}
	out := renderChart(grail.Records{rec}, 40, 10, time.Time{}, time.Time{}, nudgeFrom, false)
	if !strings.Contains(out, "interval 1m") {
		t.Errorf("expected numeric nanosecond interval to render as 1m, got:\n%s", out)
	}
}

func TestRenderChartShowsPendingFooter(t *testing.T) {
	chartStart := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	chartEnd := chartStart.Add(time.Hour)
	rec := map[string]any{
		"count":    repeatFloats(1.0, 60),
		"interval": "PT1M",
		"timeframe": map[string]any{
			"start": chartStart.Format(time.RFC3339),
			"end":   chartEnd.Format(time.RFC3339),
		},
	}
	pending := chartStart.Add(30 * time.Minute)
	out := renderChart(grail.Records{rec}, 80, 14, pending, time.Time{}, nudgeFrom, false)

	if !strings.Contains(out, "pending: from") {
		t.Errorf("expected 'pending: from' footer when pending is set, got:\n%s", out)
	}
	if !strings.Contains(out, pending.Local().Format("Jan 2 15:04")) {
		t.Errorf("expected pending value %q in output, got:\n%s",
			pending.Local().Format("Jan 2 15:04"), out)
	}
	if !strings.Contains(out, chartStart.Local().Format("Jan 2 15:04")) {
		t.Errorf("expected original from-label to remain, got:\n%s", out)
	}
	if !strings.Contains(out, chartEnd.Local().Format("Jan 2 15:04")) {
		t.Errorf("expected original to-label to remain, got:\n%s", out)
	}
}

func TestRangeFooterShowsBothEndpoints(t *testing.T) {
	from := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	fromText := from.Local().Format("Jan 2 15:04")
	toText := to.Local().Format("Jan 2 15:04")

	out := rangeFooter(from, to, false, true)
	if !strings.Contains(out, fromText) {
		t.Errorf("missing from value %q in:\n%s", fromText, out)
	}
	if !strings.Contains(out, toText) {
		t.Errorf("missing to value %q in:\n%s", toText, out)
	}
	if !strings.Contains(out, " · to ") {
		t.Errorf("missing separator in:\n%s", out)
	}
}

func TestChartFocusMarkerCols(t *testing.T) {
	chartFrom := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	chartTo := chartFrom.Add(time.Hour)
	mid := chartFrom.Add(30 * time.Minute)
	const ncols = 60

	cases := []struct {
		name             string
		focus            chartEndpoint
		pFrom, pTo       time.Time
		wantFocus, wantOther int
	}{
		{"focus from, no pending → focus on first col, no other", nudgeFrom, time.Time{}, time.Time{}, 0, -1},
		{"focus to, no pending → focus on last col, no other", nudgeTo, time.Time{}, time.Time{}, ncols - 1, -1},
		{"focus from with pending from → focus on pending col", nudgeFrom, mid, time.Time{}, 30, -1},
		{"focus to with pending to → focus on pending col", nudgeTo, time.Time{}, mid, 30, -1},
		{"focus from with both pending → focus on from, other on to", nudgeFrom, mid, chartTo.Add(-time.Minute), 30, ncols - 1},
		{"focus to with both pending → focus on to, other on from", nudgeTo, chartFrom.Add(time.Minute), mid, 30, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, o := chartFocusMarkerCols(tc.focus, tc.pFrom, tc.pTo, chartFrom, chartTo, ncols)
			if f != tc.wantFocus {
				t.Errorf("focused: got %d want %d", f, tc.wantFocus)
			}
			if o != tc.wantOther {
				t.Errorf("other: got %d want %d", o, tc.wantOther)
			}
		})
	}
}

// renderChart should always include the focus footer (pending: when nudged,
// range: otherwise), so the user sees the focus indicator before nudging.
func TestRenderChartShowsRangeFooterWithoutPending(t *testing.T) {
	chartStart := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	chartEnd := chartStart.Add(time.Hour)
	rec := map[string]any{
		"count":    repeatFloats(1.0, 60),
		"interval": "PT1M",
		"timeframe": map[string]any{
			"start": chartStart.Format(time.RFC3339),
			"end":   chartEnd.Format(time.RFC3339),
		},
	}
	out := renderChart(grail.Records{rec}, 80, 14, time.Time{}, time.Time{}, nudgeFrom, false)
	if !strings.Contains(out, "range:") {
		t.Errorf("expected 'range:' footer when no pending, got:\n%s", out)
	}
	if !strings.Contains(out, "from "+chartStart.Local().Format("Jan 2 15:04")) {
		t.Errorf("expected from value in footer, got:\n%s", out)
	}
}

func TestTimeToCol(t *testing.T) {
	start := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	cases := []struct {
		name  string
		t     time.Time
		ncols int
		want  int
	}{
		{"midpoint of 60", start.Add(30 * time.Minute), 60, 30},
		{"start clamps to 0", start, 60, 0},
		{"end clamps to ncols-1", end, 60, 59},
		{"before start clamps to 0", start.Add(-time.Hour), 60, 0},
		{"after end clamps to last", end.Add(time.Hour), 60, 59},
		{"zero time returns -1", time.Time{}, 60, -1},
		{"ncols 0 returns -1", start.Add(30 * time.Minute), 0, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := timeToCol(tc.t, start, end, tc.ncols)
			if got != tc.want {
				t.Errorf("got %d want %d", got, tc.want)
			}
		})
	}
}

func repeatFloats(v float64, n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func TestDownsample(t *testing.T) {
	// 8 values into 4 columns → averages over pairs.
	got := downsample([]float64{1, 1, 3, 3, 5, 5, 7, 7}, 4)
	want := []float64{1, 3, 5, 7}
	if len(got) != len(want) {
		t.Fatalf("len got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("col %d: got %v want %v", i, got[i], want[i])
		}
	}
}
