package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kantis/dynatrace-tui/internal/dql"
	"github.com/kantis/dynatrace-tui/internal/grail"
)

type focus int

const (
	focusEditor focus = iota
	focusResults
	focusDetail
)

type view int

const (
	viewQuery view = iota
	viewSaved
)

type runState int

const (
	stateIdle runState = iota
	stateRunning
	stateError
)

type detailKind int

const (
	detailRecord detailKind = iota
	detailChart
)

type Model struct {
	client     *grail.Client
	envName    string
	envNames   []string
	makeClient func(name string) (*grail.Client, error)

	width, height int
	focus         focus
	currentView   view

	editor  Editor
	table   Table
	detail  viewport.Model
	spinner spinner.Model

	columns []string
	records grail.Records

	state    runState
	errMsg   string
	infoMsg  string
	rowCount int

	queryToken string
	cancel     context.CancelFunc

	pendingChart  bool
	detailKind    detailKind
	chartRecords  grail.Records

	// Modal state
	modal          modalKind
	timeRangeIdx   int
	timeRangeFocus int // 0 = preset list, 1 = from input, 2 = to input
	timeFromInput  textinput.Model
	timeToInput    textinput.Model
	saveInput      textinput.Model
	savedQueries   []SavedQuery
	savedListIdx   int
	// Saved searches view (Alt-2)
	savedMode             savedSearchesMode
	savedEditNameInput    textinput.Model
	savedEditBody         Editor
	savedEditOriginalName string
	savedEditFocus        savedEditFocus
	templateNames  []string
	templateInputs []textinput.Model
	templateIdx    int
	exportIdx      int
	envSwitchIdx   int
}

func New(client *grail.Client, envName string, envNames []string, makeClient func(string) (*grail.Client, error)) Model {
	ed := NewEditor()
	ed.SetValue("from:now()-15m")

	t := newTable()
	t.SetColumns([]tableColumn{{Title: "(no results)", Width: 40}})

	vp := viewport.New(0, 0)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	saved, _ := loadSavedQueries() // missing file → empty list

	// Pre-initialise the saved-search edit body so applyLayout's SetWidth/Height
	// calls are safe even before the user enters edit mode (zero-value textarea
	// panics on SetWidth).
	editBody := NewEditor()
	editBody.Blur()

	return Model{
		client:        client,
		envName:       envName,
		envNames:      envNames,
		makeClient:    makeClient,
		focus:         focusEditor,
		currentView:   viewQuery,
		editor:        ed,
		table:         t,
		detail:        vp,
		spinner:       sp,
		state:         stateIdle,
		infoMsg:       "ready — Alt-Enter run · Ctrl-G chart · Ctrl-T timerange · Alt-2 saved · Ctrl-S save · Ctrl-P params · Ctrl-X export · Ctrl-E env · q quit",
		savedQueries:  saved,
		savedEditBody: editBody,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.applyLayout()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		if m.state == stateRunning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case executeMsg:
		if msg.err != nil {
			return m.endWithError(msg.err), nil
		}
		if msg.resp.State == grail.StateRunning {
			m.queryToken = msg.resp.RequestToken
			ctx, cancel := context.WithCancel(context.Background())
			m.cancel = cancel
			return m, pollCmd(ctx, m.client, msg.resp.RequestToken)
		}
		return m.applyResult(msg.resp), nil

	case pollMsg:
		if msg.err != nil {
			if strings.Contains(msg.err.Error(), "context canceled") {
				m.state = stateIdle
				m.infoMsg = "cancelled"
				if m.queryToken != "" {
					tok := m.queryToken
					m.queryToken = ""
					return m, cancelCmd(m.client, tok)
				}
				return m, nil
			}
			return m.endWithError(msg.err), nil
		}
		return m.applyResult(msg.resp), nil

	case cancelDoneMsg:
		// no-op; UI already updated when ctx was cancelled
		return m, nil
	}

	// Default: route to focused widget
	return m.routeToFocus(msg)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// View switching is global (works in either view) and pre-empts other handlers.
	switch msg.String() {
	case "alt+1":
		m.currentView = viewQuery
		return m, nil
	case "alt+2":
		return m.enterSavedView(), nil
	}

	// Saved Searches view has its own dispatch and ignores modals.
	if m.currentView == viewSaved {
		return m.updateSavedView(msg)
	}

	// If a modal is open, route to its handler first.
	if m.modal != modalNone {
		switch m.modal {
		case modalTimeRange:
			return m.updateTimeRange(msg)
		case modalSaveQuery:
			return m.updateSaveQuery(msg)
		case modalTemplate:
			return m.updateTemplate(msg)
		case modalExport:
			return m.updateExport(msg)
		case modalSwitchEnv:
			return m.updateSwitchEnv(msg)
		}
	}

	switch msg.String() {
	case "ctrl+c":
		if m.state == stateRunning {
			return m.cancelRunning(), nil
		}
		return m, tea.Quit
	case "tab":
		return m.cycleFocus(false), nil
	case "shift+tab":
		return m.cycleFocus(true), nil
	case "ctrl+@", "ctrl+enter", "alt+enter":
		// Run query. ctrl+enter is only distinguishable on terminals with an
		// enhanced keyboard protocol; alt+enter and ctrl+space (ctrl+@) cover
		// the rest. Enter alone still inserts a newline in the editor.
		if m.state != stateRunning {
			return m.runQuery()
		}
		return m, nil
	case "ctrl+r":
		// Vim-style redo. Works in either mode since it's intercepted globally.
		if m.focus == focusEditor {
			m.editor.Redo()
		}
		return m, nil
	case "ctrl+g":
		if m.state != stateRunning {
			return m.runChart()
		}
		return m, nil
	case "ctrl+t":
		m.modal = modalTimeRange
		m.timeRangeIdx = 0
		m.timeRangeFocus = 0
		m.timeFromInput = newTimeInput("e.g. 2026-04-28 09:00")
		m.timeToInput = newTimeInput("e.g. 2026-04-28 17:00 (empty = now)")
		return m, nil
	case "ctrl+s":
		m.modal = modalSaveQuery
		m.saveInput = newSaveInput("")
		return m, textinput.Blink
	case "ctrl+o":
		return m.enterSavedView(), nil
	case "ctrl+p":
		if m.prepareTemplate() {
			m.modal = modalTemplate
			return m, textinput.Blink
		}
		return m, nil
	case "ctrl+x":
		if len(m.records) == 0 {
			m.errMsg = "no records to export"
			m.state = stateError
			return m, nil
		}
		m.modal = modalExport
		m.exportIdx = 0
		return m, nil
	case "ctrl+e":
		if len(m.envNames) <= 1 {
			m.infoMsg = "only one environment configured"
			m.state = stateIdle
			return m, nil
		}
		m.modal = modalSwitchEnv
		m.envSwitchIdx = 0
		for i, n := range m.envNames {
			if n == m.envName {
				m.envSwitchIdx = i
				break
			}
		}
		return m, nil
	case "esc":
		if m.focus == focusDetail {
			m.focus = focusResults
			return m, nil
		}
		if m.state == stateRunning {
			return m.cancelRunning(), nil
		}
	}

	// Quit on q only if not editing text
	if m.focus != focusEditor && msg.String() == "q" {
		return m, tea.Quit
	}

	// Enter in results: open detail
	if m.focus == focusResults && msg.String() == "enter" && len(m.records) > 0 {
		m.openDetail()
		return m, nil
	}

	return m.routeToFocus(msg)
}

func (m Model) routeToFocus(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focus {
	case focusEditor:
		m.editor, cmd = m.editor.Update(msg)
	case focusResults:
		m.table, cmd = m.table.Update(msg)
	case focusDetail:
		m.detail, cmd = m.detail.Update(msg)
	}
	return m, cmd
}

func (m Model) cycleFocus(reverse bool) Model {
	order := []focus{focusEditor, focusResults}
	if len(m.records) > 0 && m.focus == focusDetail {
		// keep detail in cycle if we're already in it
		order = []focus{focusEditor, focusResults, focusDetail}
	}
	idx := 0
	for i, f := range order {
		if f == m.focus {
			idx = i
			break
		}
	}
	if reverse {
		idx = (idx - 1 + len(order)) % len(order)
	} else {
		idx = (idx + 1) % len(order)
	}
	m.focus = order[idx]
	switch m.focus {
	case focusEditor:
		m.editor.Focus()
		m.table.Blur()
	case focusResults:
		m.editor.Blur()
		m.table.Focus()
	case focusDetail:
		m.editor.Blur()
		m.table.Blur()
	}
	return m
}

func (m Model) runQuery() (tea.Model, tea.Cmd) {
	body := strings.TrimSpace(m.editor.Value())
	if body == "" {
		m.errMsg = "query is empty"
		m.state = stateError
		return m, nil
	}
	m.pendingChart = false
	return m.startQuery(dql.PrependFetch(body), "running…")
}

func (m Model) runChart() (tea.Model, tea.Cmd) {
	body := strings.TrimSpace(m.editor.Value())
	if body == "" {
		m.errMsg = "query is empty"
		m.state = stateError
		return m, nil
	}
	chartDQL, interval, err := dql.MakeTimeseries(dql.PrependFetch(body))
	if err != nil {
		m.errMsg = err.Error()
		m.state = stateError
		return m, nil
	}
	m.pendingChart = true
	return m.startQuery(chartDQL, "charting (interval "+interval+")…")
}

func (m Model) startQuery(query, infoMsg string) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.state = stateRunning
	m.errMsg = ""
	m.infoMsg = infoMsg
	m.records = nil
	m.rowCount = 0
	m.applyLayout()

	return m, tea.Batch(executeCmd(ctx, m.client, query), m.spinner.Tick)
}

func (m Model) cancelRunning() Model {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	return m
}

func (m Model) endWithError(err error) Model {
	m.state = stateError
	m.errMsg = err.Error()
	m.infoMsg = ""
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	return m
}

func (m Model) applyResult(resp *grail.Response) Model {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	switch resp.State {
	case grail.StateSucceeded:
		records := grail.Records{}
		if resp.Result != nil {
			records = resp.Result.Records
		}
		m.state = stateIdle
		m.errMsg = ""
		if m.pendingChart {
			m.pendingChart = false
			m.chartRecords = records
			m.records = nil
			m.rowCount = 0
			m.populateTable()
			m.detailKind = detailChart
			m.detail.SetContent(renderChart(records, m.detail.Width, m.detail.Height))
			m.detail.GotoTop()
			m.focus = focusDetail
			m.editor.Blur()
			m.table.Blur()
			m.infoMsg = "chart ready (Esc to close)"
			return m
		}
		m.records = records
		m.rowCount = len(m.records)
		m.infoMsg = fmt.Sprintf("%d records", m.rowCount)
		m.populateTable()
		if m.rowCount > 0 {
			m.focus = focusResults
			m.editor.Blur()
			m.table.Focus()
		}
	case grail.StateFailed:
		msg := "query failed"
		if resp.Error != nil {
			msg = resp.Error.Message
		}
		m.state = stateError
		m.errMsg = msg
	case grail.StateCancelled:
		m.state = stateIdle
		m.infoMsg = "cancelled"
	}
	return m
}

// populateTable picks a sensible column set from the records and pushes rows in.
//
// Order is significant: bubbles/table.SetColumns triggers UpdateViewport, which
// iterates each row's cells and indexes m.cols[i]. If the new column count is
// smaller than the previous row's cell count (e.g. an empty result follows a
// populated one), it panics with "index out of range". Clearing rows before
// changing columns avoids that.
func (m *Model) populateTable() {
	innerWidth := m.width - 2
	if innerWidth < 40 {
		innerWidth = 40
	}

	if len(m.records) == 0 {
		m.table.SetRows(nil)
		m.table.SetColumns([]tableColumn{{Title: "(no records)", Width: innerWidth}})
		m.columns = nil
		return
	}

	cols := pickColumns(m.records)
	m.columns = cols

	tableCols := make([]tableColumn, len(cols))
	widths := distributeWidths(cols, innerWidth)
	for i, c := range cols {
		tableCols[i] = tableColumn{Title: c, Width: widths[i]}
	}

	rows := make([]tableRow, len(m.records))
	for i, rec := range m.records {
		row := make(tableRow, len(cols))
		for j, c := range cols {
			cell := stringCell(rec[c])
			if c == "timestamp" {
				cell = formatTimestampCell(cell)
			}
			row[j] = highlightJSONCell(cell)
		}
		rows[i] = row
	}

	m.table.SetColumns(tableCols)
	m.table.SetRows(rows)
}

func (m *Model) openDetail() {
	if len(m.records) == 0 {
		return
	}
	cur := m.table.Cursor()
	if cur < 0 || cur >= len(m.records) {
		cur = 0
	}
	rec := m.records[cur]
	m.detailKind = detailRecord
	m.detail.SetContent(renderRecordDetail(rec))
	m.detail.GotoTop()
	m.focus = focusDetail
}

// Layout

func (m *Model) applyLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	editorH := 8
	statusH := 1
	// Subtractions account for: editor inner (editorH), status bar (statusH),
	// 2 pane titles, and 4 border lines (top+bottom of each pane).
	resultsH := m.height - editorH - statusH - 2 - 4
	if resultsH < 5 {
		resultsH = 5
	}
	innerW := m.width - 2
	if innerW < 20 {
		innerW = 20
	}
	m.editor.SetWidth(innerW)
	m.editor.SetHeight(editorH)
	m.table.SetWidth(innerW)
	m.table.SetHeight(resultsH)
	m.detail.Width = innerW
	m.detail.Height = resultsH
	m.savedEditBody.SetWidth(innerW)
	m.savedEditBody.SetHeight(editorH)
	if len(m.records) > 0 {
		m.populateTable()
	}
	if m.detailKind == detailChart && len(m.chartRecords) > 0 {
		m.detail.SetContent(renderChart(m.chartRecords, m.detail.Width, m.detail.Height))
	}
}

// View rendering

func (m Model) View() string {
	if m.width == 0 {
		return "initializing…"
	}

	if m.currentView == viewSaved {
		return m.viewSavedSearches()
	}

	if m.modal != modalNone {
		switch m.modal {
		case modalTimeRange:
			return m.viewTimeRange()
		case modalSaveQuery:
			return m.viewSaveQuery()
		case modalTemplate:
			return m.viewTemplate()
		case modalExport:
			return m.viewExport()
		case modalSwitchEnv:
			return m.viewSwitchEnv()
		}
	}

	var sections []string

	editorTitle := fmt.Sprintf("Query%s [%s]", m.envSuffix(), m.editor.Mode())
	editorBorder := paneBorder
	editorTitleStyle := paneTitle
	if m.focus == focusEditor {
		editorBorder = paneBorderFocused
		editorTitleStyle = paneTitleFocused
	}
	sections = append(sections, editorTitleStyle.Render(editorTitle))
	sections = append(sections, editorBorder.Render(m.editor.View()))

	if m.focus == focusDetail {
		title := "Detail (Esc to close)"
		if m.detailKind == detailChart {
			title = "Chart (Esc to close)"
		}
		sections = append(sections, paneTitleFocused.Render(title))
		sections = append(sections, paneBorderFocused.Render(m.detail.View()))
	} else {
		title := "Results" + m.envSuffix()
		if m.rowCount > 0 {
			title = fmt.Sprintf("Results%s (%d)", m.envSuffix(), m.rowCount)
		}
		border := paneBorder
		titleStyle := paneTitle
		if m.focus == focusResults {
			border = paneBorderFocused
			titleStyle = paneTitleFocused
		}
		sections = append(sections, titleStyle.Render(title))
		sections = append(sections, border.Render(m.table.View()))
	}

	sections = append(sections, m.statusLine())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// envSuffix renders ` [<envName>]` for use as a pane-title suffix, or "" if
// no env name is set (e.g. the legacy single-env config path).
func (m Model) envSuffix() string {
	if m.envName == "" {
		return ""
	}
	return " [" + m.envName + "]"
}

func (m Model) statusLine() string {
	left := ""
	switch m.state {
	case stateRunning:
		left = m.spinner.View() + " running (Esc/Ctrl-C to cancel)"
	case stateError:
		left = errorText.Render("error: " + m.errMsg)
	case stateIdle:
		if m.infoMsg != "" {
			left = okText.Render(m.infoMsg)
		}
	}
	right := "Alt-Enter run · Ctrl-G chart · Alt-1/2 view · q quit"
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return statusBar.Render(left + strings.Repeat(" ", gap) + right)
}

// Helpers

var preferredColumns = []string{"timestamp", "loglevel", "status", "content"}

func pickColumns(records grail.Records) []string {
	if len(records) == 0 {
		return []string{"(empty)"}
	}
	keys := map[string]bool{}
	for _, r := range records {
		for k := range r {
			keys[k] = true
		}
	}
	var out []string
	for _, c := range preferredColumns {
		if keys[c] {
			out = append(out, c)
			delete(keys, c)
		}
	}
	if len(out) >= 4 {
		return out
	}
	rest := make([]string, 0, len(keys))
	for k := range keys {
		if strings.HasPrefix(k, "dt.") || strings.HasPrefix(k, "k8s.") || strings.HasPrefix(k, "host.") {
			continue // skip noisy attribute fields by default
		}
		rest = append(rest, k)
	}
	sort.Strings(rest)
	for _, k := range rest {
		if len(out) >= 4 {
			break
		}
		out = append(out, k)
	}
	if len(out) == 0 {
		out = []string{"(no fields)"}
	}
	return out
}

func distributeWidths(cols []string, total int) []int {
	if len(cols) == 0 {
		return nil
	}
	widths := make([]int, len(cols))
	// Fixed widths for known short columns.
	remaining := total
	for i, c := range cols {
		switch c {
		case "timestamp":
			widths[i] = 25
		case "loglevel", "status":
			widths[i] = 8
		}
		remaining -= widths[i]
	}
	// Distribute remainder across unset columns.
	unset := 0
	for _, w := range widths {
		if w == 0 {
			unset++
		}
	}
	if unset > 0 && remaining > 0 {
		share := remaining / unset
		if share < 8 {
			share = 8
		}
		for i, w := range widths {
			if w == 0 {
				widths[i] = share
			}
		}
	}
	return widths
}

func stringCell(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.ReplaceAll(x, "\n", " ")
	case float64:
		return fmt.Sprintf("%g", x)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// formatTimestampCell renders RFC3339 timestamps as `YYYY-MM-DD HH:MM:SS.mmm`
// in the user's local timezone (millisecond precision, space separator).
// Non-timestamp strings pass through.
func formatTimestampCell(s string) string {
	if s == "" {
		return s
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.Local().Format("2006-01-02 15:04:05.000")
}
