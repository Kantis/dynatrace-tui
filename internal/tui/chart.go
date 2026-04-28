package tui

import (
	"fmt"
	"math"
	"strconv"
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

// nudgeChartTimeframe rewrites the editor's from:/to: clauses and stages the
// new range as "pending" — the chart re-renders with the pending values
// highlighted but does not re-run the query; the user re-runs explicitly
// (Ctrl-G). Subsequent nudges accumulate against the pending values.
// Returns handled=false when the chart has no usable timeframe.
func (m Model) nudgeChartTimeframe(endpoint chartEndpoint, delta time.Duration) (Model, tea.Cmd, bool) {
	if len(m.chartRecords) == 0 {
		return m, nil, false
	}
	startStr, endStr := pickTimeframe(m.chartRecords[0])
	chartFrom, fromOK := parseISOTime(startStr)
	chartTo, toOK := parseISOTime(endStr)
	if !fromOK || !toOK {
		return m, nil, false
	}

	from := m.chartPendingFrom
	if from.IsZero() {
		from = chartFrom
	}
	to := m.chartPendingTo
	if to.IsZero() {
		to = chartTo
	}

	if endpoint == nudgeFrom {
		from = from.Add(delta)
	} else {
		to = to.Add(delta)
	}

	m.chartPendingFrom = from
	m.chartPendingTo = to

	// Swap so the materialised DQL always has from < to. The pending values
	// in the model stay in the user's nudged order — only what we write to
	// the editor (and what the next run will use) is normalised.
	dqlFrom, dqlTo := from, to
	if dqlFrom.After(dqlTo) {
		dqlFrom, dqlTo = dqlTo, dqlFrom
	}
	newDQL := dql.SubstituteAbsolute(dql.PrependFetch(m.editor.Value()), dqlFrom, dqlTo, true)
	m.editor.SetValue(dql.StripFetch(newDQL))

	m.setDetailContent(renderChart(m.chartRecords, m.detail.Width, m.detail.Height, m.chartPendingFrom, m.chartPendingTo))
	m.infoMsg = "pending range — Ctrl-G to apply"
	m.state = stateIdle
	return m, nil, true
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
//
// pendingFrom/pendingTo are non-zero when the user has nudged the timeframe
// but not yet re-run the query — those positions are drawn as highlighted
// vertical markers in the bar grid and axis, and listed in a footer line, so
// the user can see where the staged from/to fall against the current data.
func renderChart(records grail.Records, width, height int, pendingFrom, pendingTo time.Time) string {
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

	chartFrom, _ := parseISOTime(start)
	chartTo, _ := parseISOTime(end)
	hasPending := !pendingFrom.IsZero() || !pendingTo.IsZero()

	if width < 20 {
		width = 20
	}
	if height < 6 {
		height = 6
	}
	// header + axis + time-label + footer lines = 4; pending footer adds one.
	reserved := 4
	if hasPending {
		reserved = 5
	}
	bodyH := height - reserved
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
	pendingFromCol := timeToCol(pendingFrom, chartFrom, chartTo, len(series))
	pendingToCol := timeToCol(pendingTo, chartFrom, chartTo, len(series))

	var b strings.Builder
	b.WriteString(fmt.Sprintf("max %s · total %s · interval %s · %d buckets",
		fmtCount(max), fmtCount(total), prettyInterval(interval), len(counts)))
	b.WriteByte('\n')
	for _, row := range grid {
		b.WriteString(renderRowWithMarkers(row, bar, pendingNudgeStyle, pendingFromCol, pendingToCol))
		b.WriteByte('\n')
	}
	axisRow := []rune(strings.Repeat("─", len(series)))
	b.WriteString(renderRowWithMarkers(axisRow, lipgloss.NewStyle(), pendingNudgeStyle, pendingFromCol, pendingToCol))
	b.WriteByte('\n')
	b.WriteString(spaceBetween(shortTime(start), shortTime(end), len(series)))
	b.WriteByte('\n')
	if hasPending {
		b.WriteString(statusBar.Render("pending: " + pendingFooter(pendingFrom, pendingTo)))
		b.WriteByte('\n')
	}
	b.WriteString(statusBar.Render("series: " + label))
	return b.String()
}

// timeToCol maps a wall-clock time onto a chart column index in [0, ncols).
// Returns -1 when the inputs don't form a valid window or t is unset.
func timeToCol(t, start, end time.Time, ncols int) int {
	if t.IsZero() || start.IsZero() || end.IsZero() || ncols <= 0 || !end.After(start) {
		return -1
	}
	pos := float64(t.Sub(start)) / float64(end.Sub(start)) * float64(ncols)
	col := int(math.Round(pos))
	if col < 0 {
		col = 0
	}
	if col >= ncols {
		col = ncols - 1
	}
	return col
}

// renderRowWithMarkers paints `row` with `base` style, switching to `marker`
// at the columns listed in `markerCols` (negative entries are ignored). The
// row is emitted in contiguous same-style segments to keep the ANSI overhead
// proportional to the number of marker columns, not the row length.
func renderRowWithMarkers(row []rune, base, marker lipgloss.Style, markerCols ...int) string {
	if len(row) == 0 {
		return ""
	}
	isMarker := func(j int) bool {
		for _, c := range markerCols {
			if c == j {
				return true
			}
		}
		return false
	}
	var b strings.Builder
	j := 0
	for j < len(row) {
		startJ := j
		m := isMarker(j)
		for j < len(row) && isMarker(j) == m {
			j++
		}
		seg := string(row[startJ:j])
		if m {
			b.WriteString(marker.Render(seg))
		} else {
			b.WriteString(base.Render(seg))
		}
	}
	return b.String()
}

// pendingFooter renders "from <X> · to <Y>" with each non-zero value
// highlighted. Skips the part for whichever endpoint is unset.
func pendingFooter(pendingFrom, pendingTo time.Time) string {
	parts := []string{}
	if !pendingFrom.IsZero() {
		parts = append(parts, "from "+pendingNudgeStyle.Render(pendingFrom.Local().Format("Jan 2 15:04")))
	}
	if !pendingTo.IsZero() {
		parts = append(parts, "to "+pendingNudgeStyle.Render(pendingTo.Local().Format("Jan 2 15:04")))
	}
	return strings.Join(parts, " · ")
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

// spaceBetween joins start and end with whitespace so the rendered visual width
// matches `width`. It uses lipgloss.Width so styled (ANSI-coloured) labels are
// measured by visible glyph count, not by raw byte length.
func spaceBetween(start, end string, width int) string {
	gap := width - lipgloss.Width(start) - lipgloss.Width(end)
	if gap < 1 {
		return start + "  " + end
	}
	return start + strings.Repeat(" ", gap) + end
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
// duration ("1h", "5m", "30s", "500ms"). Grail returns it variously as an
// ISO-8601 string ("PT1M"), a Go-style duration string ("2m0s"), a numeric
// string of nanoseconds ("120000000000"), or a number — all handled here.
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
		if n, err := strconv.ParseInt(x, 10, 64); err == nil {
			return formatDuration(time.Duration(n))
		}
		if d, err := time.ParseDuration(x); err == nil {
			return formatDuration(d)
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
	const day = 24 * time.Hour
	switch {
	case d%day == 0:
		return fmt.Sprintf("%dd", int64(d/day))
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
