package tui

import (
	"strings"
	"testing"

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
	out := renderChart(grail.Records{rec}, 60, 12)

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
	out := renderChart(grail.Records{rec}, 40, 10)
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
	out := renderChart(grail.Records{rec}, 40, 10)
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
	out := renderChart(grail.Records{rec}, 40, 10)
	if !strings.Contains(out, "avg(latency)") {
		t.Errorf("expected series label, got:\n%s", out)
	}
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
