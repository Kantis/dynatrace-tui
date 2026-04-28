package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/kantis/dynatrace-tui/internal/grail"
)

// makeTimeseries returns one record per series; the first is what we chart.
// The numeric column is an array of length N (one per bucket); `timeframe` is
// `{start, end}`; `interval` is an ISO-8601 duration like "PT1M".
func renderChart(records grail.Records, width, height int) string {
	if len(records) == 0 {
		return chartHint("no time series data — try a query that returns rows")
	}
	rec := records[0]

	counts, label, ok := pickSeries(rec)
	if !ok {
		return chartHint("no numeric series in result")
	}
	start, end := pickTimeframe(rec)
	interval, _ := rec["interval"].(string)

	if width < 20 {
		width = 20
	}
	if height < 6 {
		height = 6
	}
	bodyH := height - 4 // header line, axis line, time-label line, footer line
	if bodyH < 3 {
		bodyH = 3
	}

	cols := width
	if cols > len(counts) {
		cols = len(counts)
	}
	series := downsample(counts, cols)

	var max, total float64
	for _, v := range counts {
		total += v
		if v > max {
			max = v
		}
	}

	grid := make([][]rune, bodyH)
	for i := range grid {
		grid[i] = make([]rune, len(series))
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}
	if max > 0 {
		for j, v := range series {
			eighths := int(math.Round(v / max * float64(bodyH*8)))
			full := eighths / 8
			rem := eighths % 8
			for r := 0; r < bodyH; r++ {
				row := bodyH - 1 - r
				switch {
				case r < full:
					grid[row][j] = '█'
				case r == full && rem > 0:
					grid[row][j] = blocks[rem]
				}
			}
		}
	}

	bar := lipgloss.NewStyle().Foreground(colorAccent)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("max %s · total %s · interval %s · %d buckets",
		fmtCount(max), fmtCount(total), prettyInterval(interval), len(counts)))
	b.WriteByte('\n')
	for _, row := range grid {
		b.WriteString(bar.Render(string(row)))
		b.WriteByte('\n')
	}
	b.WriteString(strings.Repeat("─", len(series)))
	b.WriteByte('\n')
	b.WriteString(axisLabel(start, end, len(series)))
	b.WriteByte('\n')
	b.WriteString(statusBar.Render("series: " + label))
	return b.String()
}

func chartHint(msg string) string {
	return statusBar.Render(msg)
}

var blocks = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇'}

func pickSeries(rec map[string]any) ([]float64, string, bool) {
	for _, k := range []string{"count", "count()"} {
		if v, ok := rec[k]; ok {
			if s, ok := toFloatSlice(v); ok {
				return s, k, true
			}
		}
	}
	for k, v := range rec {
		if k == "timeframe" || k == "interval" {
			continue
		}
		if s, ok := toFloatSlice(v); ok {
			return s, k, true
		}
	}
	return nil, "", false
}

func toFloatSlice(v any) ([]float64, bool) {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return nil, false
	}
	out := make([]float64, len(arr))
	for i, x := range arr {
		switch n := x.(type) {
		case float64:
			out[i] = n
		case nil:
			out[i] = 0
		default:
			return nil, false
		}
	}
	return out, true
}

func pickTimeframe(rec map[string]any) (string, string) {
	tf, ok := rec["timeframe"].(map[string]any)
	if !ok {
		return "", ""
	}
	start, _ := tf["start"].(string)
	end, _ := tf["end"].(string)
	return start, end
}

func downsample(s []float64, cols int) []float64 {
	if cols >= len(s) || cols <= 0 {
		return s
	}
	out := make([]float64, cols)
	for i := 0; i < cols; i++ {
		a := i * len(s) / cols
		b := (i + 1) * len(s) / cols
		if b <= a {
			b = a + 1
		}
		var sum float64
		for _, v := range s[a:b] {
			sum += v
		}
		out[i] = sum / float64(b-a)
	}
	return out
}

func axisLabel(start, end string, width int) string {
	s := shortTime(start)
	e := shortTime(end)
	gap := width - len(s) - len(e)
	if gap < 1 {
		return s + "  " + e
	}
	return s + strings.Repeat(" ", gap) + e
}

func shortTime(s string) string {
	if s == "" {
		return "?"
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Local().Format("Jan 2 15:04")
		}
	}
	return s
}

func prettyInterval(iso string) string {
	if strings.HasPrefix(iso, "PT") {
		return strings.ToLower(iso[2:])
	}
	if iso == "" {
		return "?"
	}
	return iso
}

func fmtCount(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e12 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%g", v)
}
