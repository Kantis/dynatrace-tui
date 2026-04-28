package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kantis/dynatrace-tui/internal/dql"
	"github.com/kantis/dynatrace-tui/internal/grail"
)

// chartEndpoint identifies which side of the chart timeframe a nudge applies to.
type chartEndpoint int

const (
	nudgeFrom chartEndpoint = iota
	nudgeTo
)

// chartNudgeDelta maps a key to a (delta, endpoint) pair. Lowercase keys push
// `from` forward; uppercase keys pull `to` backward — both narrow the window
// toward the middle so the user can zoom in around an interesting feature.
func chartNudgeDelta(key string) (time.Duration, chartEndpoint, bool) {
	switch key {
	case "h":
		return time.Hour, nudgeFrom, true
	case "H":
		return -time.Hour, nudgeTo, true
	case "m":
		return time.Minute, nudgeFrom, true
	case "M":
		return -time.Minute, nudgeTo, true
	case "s":
		return time.Second, nudgeFrom, true
	case "S":
		return -time.Second, nudgeTo, true
	case "d":
		return 24 * time.Hour, nudgeFrom, true
	case "D":
		return -24 * time.Hour, nudgeTo, true
	}
	return 0, 0, false
}

// nudgeChartTimeframe rewrites the editor's from:/to: clauses based on the
// chart's current timeframe and re-runs the chart. Returns handled=false when
// the chart has no usable timeframe or the nudge would invert the window.
func (m Model) nudgeChartTimeframe(endpoint chartEndpoint, delta time.Duration) (Model, tea.Cmd, bool) {
	if len(m.chartRecords) == 0 {
		return m, nil, false
	}
	startStr, endStr := pickTimeframe(m.chartRecords[0])
	from, fromOK := parseISOTime(startStr)
	to, toOK := parseISOTime(endStr)
	if !fromOK || !toOK {
		return m, nil, false
	}

	if endpoint == nudgeFrom {
		from = from.Add(delta)
	} else {
		to = to.Add(delta)
	}
	if !from.Before(to) {
		m.infoMsg = "nudge would invert the time range"
		m.state = stateIdle
		return m, nil, true
	}

	newDQL := dql.SubstituteAbsolute(dql.PrependFetch(m.editor.Value()), from, to, true)
	m.editor.SetValue(dql.StripFetch(newDQL))

	next, cmd := m.runChart()
	return next.(Model), cmd, true
}

func parseISOTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// makeTimeseries returns one record per series; the first is what we chart.
// The numeric column is an array of length N (one per bucket); `timeframe` is
// `{start, end}`; `interval` is either an ISO-8601 string ("PT1M") or a
// numeric nanosecond count.
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
	interval := rec["interval"]

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

// prettyInterval renders the makeTimeseries interval field as a compact human
// duration ("1h", "5m", "30s", "500ms"). Grail sometimes returns it as an
// ISO-8601 string ("PT1M"), sometimes as a nanosecond number — both are handled.
func prettyInterval(v any) string {
	switch x := v.(type) {
	case nil:
		return "?"
	case string:
		if x == "" {
			return "?"
		}
		if strings.HasPrefix(x, "PT") {
			return strings.ToLower(x[2:])
		}
		return x
	case float64:
		return formatDuration(time.Duration(int64(x)))
	case int64:
		return formatDuration(time.Duration(x))
	case int:
		return formatDuration(time.Duration(x))
	}
	return "?"
}

// formatDuration prints d using the largest exact unit (h/m/s/ms). Falls back
// to time.Duration's default String() form when no clean unit fits.
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "?"
	}
	switch {
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", int64(d/time.Hour))
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", int64(d/time.Minute))
	case d%time.Second == 0:
		return fmt.Sprintf("%ds", int64(d/time.Second))
	case d%time.Millisecond == 0:
		return fmt.Sprintf("%dms", int64(d/time.Millisecond))
	}
	return d.String()
}

func fmtCount(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e12 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%g", v)
}
