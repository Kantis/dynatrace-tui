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
	viewFilters
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

	columns     []string
	records     grail.Records
	recordOrder []string // column order from Grail metadata; nil falls back to heuristic

	state    runState
	errMsg   string
	infoMsg  string
	rowCount int

	queryToken string
	cancel     context.CancelFunc

	pendingChart   bool
	detailKind     detailKind
	chartRecords   grail.Records
	currentRecord  map[string]any // record currently shown in the detail viewport, kept so layout changes can re-wrap
	detailPendingG bool           // vim `gg` in the detail viewport

	// Chart-view nudge: from/to staged by the user but not yet re-run. Zero
	// means "no pending value for that endpoint". Cleared whenever the chart
	// successfully re-runs.
	chartPendingFrom time.Time
	chartPendingTo   time.Time

	// Chart-view focus: which endpoint Tab-toggle currently targets, and
	// whether the blink highlight is currently visible. The blink toggles
	// via chartBlinkMsg ticks while the chart detail is focused.
	chartFocusEndpoint chartEndpoint
	chartFocusBlinkOn  bool

	// Detail viewport search (`/`)
	detailRawContent    string
	detailSearchInput   textinput.Model
	detailSearchQuery   string
	detailSearchMatches []int // line indices of matches in raw content
	detailSearchIdx     int
	detailSearchActive  bool

	// Modal state
	modal           modalKind
	timeRangeFocus  int // 0 = From column, 1 = To column
	timeFromIdx     int // selected quick-pick row in From column
	timeToIdx       int // selected quick-pick row in To column
	timeFromInput   textinput.Model
	timeToInput     textinput.Model
	timeRangeOpened time.Time // captured at modal open; used for "now"/"start of hour" picks and for resolving relatives during nudging
	saveInput      textinput.Model
	savedQueries   []SavedQuery
	savedDefault   string // name of the saved search auto-loaded on startup
	savedListIdx   int
	pendingAutoRun bool // run the editor body on first event after startup
	silentRun      bool // suppress the post-result focus shift to the results table (used by the startup auto-run)
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

	// Favorite filters (Alt-3)
	filters         []SavedFilter
	filtersListIdx  int
	filtersMode     filtersMode
	filterEditIsNew         bool
	filterEditOriginalName  string
	filterEditNameInput     textinput.Model
	filterEditTemplate      textarea.Model
	filterEditPlaceholders  []string
	filterEditSuggestions   []textarea.Model
	filterEditValuesByName  map[string]string
	filterEditFocus         int

	// Pick-filter modal (Ctrl-F)
	pickFilterIdx int

	// Resolve-filter modal
	resolveFilter SavedFilter
	resolveNames  []string
	resolveInputs []textinput.Model
	resolveSugIdx []int // current suggestion index per placeholder, -1 if none picked
	resolveFocus  int
}

func New(client *grail.Client, envName string, envNames []string, makeClient func(string) (*grail.Client, error), vimMode bool) Model {
	ed := NewEditor(vimMode)
	ed.SetValue("from:now()-15m")

	t := newTable()
	t.SetColumns([]tableColumn{{Title: "(no results)", Width: 40}})

	vp := viewport.New(0, 0)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	saved, defaultName, _ := loadSavedQueries() // missing file → empty list
	filters, _ := loadSavedFilters()            // missing file → empty list

	// Pre-initialise the saved-search edit body so applyLayout's SetWidth/Height
	// calls are safe even before the user enters edit mode (zero-value textarea
	// panics on SetWidth).
	editBody := NewEditor(vimMode)
	editBody.Blur()

	infoMsg := "ready — Alt-Enter run · Ctrl-G chart · Ctrl-T timerange · Alt-2 saved · Alt-3 filters · Ctrl-F insert filter · Ctrl-S save · Ctrl-P params · Ctrl-X export · Ctrl-E env · q quit"
	autoRun := false
	// If a default saved search exists and resolves to a non-empty body,
	// preload the editor with it and queue an auto-run for after Init().
	if defaultName != "" {
		for _, q := range saved {
			if q.Name == defaultName {
				body := dql.StripFetch(q.Query)
				if strings.TrimSpace(body) != "" {
					ed.SetValue(body)
					autoRun = true
					infoMsg = "running default: " + defaultName
				}
				break
			}
		}
	}

	return Model{
		client:         client,
		envName:        envName,
		envNames:       envNames,
		makeClient:     makeClient,
		focus:          focusEditor,
		currentView:    viewQuery,
		editor:         ed,
		table:          t,
		detail:         vp,
		spinner:        sp,
		state:          stateIdle,
		infoMsg:        infoMsg,
		savedQueries:   saved,
		savedDefault:   defaultName,
		savedEditBody:  editBody,
		filters:        filters,
		pendingAutoRun: autoRun,
	}
}

// autoRunMsg is dispatched once from Init() when a default saved search
// is configured, so Update() can call runQuery() (which mutates the
// model and needs to issue a tea.Cmd).
type autoRunMsg struct{}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink}
	if m.pendingAutoRun {
		cmds = append(cmds, func() tea.Msg { return autoRunMsg{} })
	}
	return tea.Batch(cmds...)
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
		return m.applyResult(msg.resp)

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
		return m.applyResult(msg.resp)

	case cancelDoneMsg:
		// no-op; UI already updated when ctx was cancelled
		return m, nil

	case chartBlinkMsg:
		if m.focus != focusDetail || m.detailKind != detailChart {
			return m, nil
		}
		// Stop ticking when nothing is staged — the ticker resumes on the
		// next nudge. This keeps the chart static until the user actually
		// starts nudging.
		if m.chartPendingFrom.IsZero() && m.chartPendingTo.IsZero() {
			return m, nil
		}
		m.chartFocusBlinkOn = !m.chartFocusBlinkOn
		m.setDetailContent(renderChart(m.chartRecords, m.detail.Width, m.detail.Height,
			m.chartPendingFrom, m.chartPendingTo, m.chartFocusEndpoint, m.chartFocusBlinkOn))
		return m, chartBlinkCmd()

	case autoRunMsg:
		if !m.pendingAutoRun {
			return m, nil
		}
		m.pendingAutoRun = false
		m.silentRun = true
		return m.runQuery()
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
	case "alt+3":
		return m.enterFiltersView(), nil
	}

	// Saved Searches view has its own dispatch and ignores modals.
	if m.currentView == viewSaved {
		return m.updateSavedView(msg)
	}
	if m.currentView == viewFilters {
		return m.updateFiltersView(msg)
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
		case modalHelp:
			next, cmd := m.updateHelp(msg)
			return next, cmd
		case modalPickFilter:
			next, cmd := m.updatePickFilter(msg)
			return next, cmd
		case modalResolveFilter:
			next, cmd := m.updateResolveFilter(msg)
			return next, cmd
		}
	}

	// `?` opens the shortcut legend. Suppressed when the editor is focused
	// and accepting literal input — i.e. vim is off (no modal) or vim is
	// in insert mode — so the character can land in the query.
	editorTyping := m.focus == focusEditor && (!m.editor.Vim() || m.editor.Mode() == modeInsert)
	if msg.String() == "?" && !editorTyping {
		m.modal = modalHelp
		return m, nil
	}

	// Chart detail view intercepts Tab (toggle from/to focus) and Enter
	// (re-run with the staged range) before the global handlers can claim
	// them for focus cycling / query execution.
	if m.focus == focusDetail && m.detailKind == detailChart {
		switch msg.String() {
		case "tab", "shift+tab":
			if m.chartFocusEndpoint == nudgeFrom {
				m.chartFocusEndpoint = nudgeTo
			} else {
				m.chartFocusEndpoint = nudgeFrom
			}
			m.chartFocusBlinkOn = true
			m.setDetailContent(renderChart(m.chartRecords, m.detail.Width, m.detail.Height,
				m.chartPendingFrom, m.chartPendingTo, m.chartFocusEndpoint, m.chartFocusBlinkOn))
			return m, nil
		case "enter":
			if m.state != stateRunning && (!m.chartPendingFrom.IsZero() || !m.chartPendingTo.IsZero()) {
				return m.runChart()
			}
			return m, nil
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
		m.timeRangeOpened = time.Now()
		m.timeRangeFocus = 0
		m.timeFromIdx = 0
		m.timeToIdx = 0
		m.timeFromInput = newTimeInput("")
		m.timeFromInput.SetValue("now()-15m")
		m.timeToInput = newTimeInput("(empty = open-ended)")
		m.refreshTimeFocus()
		return m, textinput.Blink
	case "ctrl+s":
		m.modal = modalSaveQuery
		m.saveInput = newSaveInput("")
		return m, textinput.Blink
	case "ctrl+o":
		return m.enterSavedView(), nil
	case "ctrl+f":
		if m.state != stateRunning {
			return m.openPickFilter(), nil
		}
		return m, nil
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

	// Detail viewport: search prompt, vim-style nav, and `q` to close.
	if m.focus == focusDetail {
		if m.detailSearchActive {
			next, cmd := m.updateDetailSearchInput(msg)
			return next, cmd
		}
		// Chart-only: h/H/m/M/s/S/d/D nudge the focused endpoint (Tab
		// toggles which is focused). Gated on stateIdle so we don't stack
		// queries while one is in flight.
		if m.detailKind == detailChart && m.state != stateRunning {
			if delta, ok := chartNudgeDelta(msg.String()); ok {
				m.detailPendingG = false
				next, cmd, handled := m.nudgeChartTimeframe(delta)
				if handled {
					return next, cmd
				}
			}
		}
		switch msg.String() {
		case "/":
			m.detailPendingG = false
			return m.startDetailSearch(), textinput.Blink
		case "n":
			m.detailPendingG = false
			return m.nextDetailMatch(), nil
		case "N":
			m.detailPendingG = false
			return m.prevDetailMatch(), nil
		case "q":
			m.detailPendingG = false
			m.focus = focusResults
			return m, nil
		case "G":
			m.detailPendingG = false
			m.detail.GotoBottom()
			return m, nil
		case "g":
			if m.detailPendingG {
				m.detailPendingG = false
				m.detail.GotoTop()
				return m, nil
			}
			m.detailPendingG = true
			return m, nil
		}
		// Any other key cancels a pending `g`.
		m.detailPendingG = false
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
	m.recordOrder = nil
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

func (m Model) applyResult(resp *grail.Response) (Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	silent := m.silentRun
	m.silentRun = false
	switch resp.State {
	case grail.StateSucceeded:
		records := grail.Records{}
		var order []string
		if resp.Result != nil {
			records = resp.Result.Records
			order = resp.Result.FieldOrder()
		}
		m.state = stateIdle
		m.errMsg = ""
		if m.pendingChart {
			m.pendingChart = false
			m.chartRecords = records
			m.chartPendingFrom = time.Time{}
			m.chartPendingTo = time.Time{}
			m.records = nil
			m.recordOrder = nil
			m.rowCount = 0
			m.populateTable()
			m.detailKind = detailChart
			m.chartFocusEndpoint = nudgeFrom
			m.chartFocusBlinkOn = true
			m.setDetailContent(renderChart(records, m.detail.Width, m.detail.Height,
				m.chartPendingFrom, m.chartPendingTo, m.chartFocusEndpoint, m.chartFocusBlinkOn))
			m.detail.GotoTop()
			m.focus = focusDetail
			m.editor.Blur()
			m.table.Blur()
			m.infoMsg = "chart ready — Tab switch · h/m/s/d nudge · Enter run · Esc close"
			return m, nil
		}
		m.records = records
		m.recordOrder = order
		m.rowCount = len(m.records)
		m.infoMsg = fmt.Sprintf("%d records", m.rowCount)
		m.populateTable()
		if m.rowCount > 0 && !silent {
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
	return m, nil
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

	cols := pickColumns(m.records, m.recordOrder)
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
	m.currentRecord = rec
	m.setDetailContent(renderRecordDetail(rec, m.detail.Width))
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
	tabsH := 1
	// Subtractions account for: tabs row (tabsH), editor inner (editorH),
	// status bar (statusH), 1 pane title (results — the editor's title
	// moved to the tabs row), and 4 border lines (top+bottom of each pane).
	resultsH := m.height - tabsH - editorH - statusH - 1 - 4
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
	if m.filtersMode == filtersModeEditing {
		m.layoutFilterEdit(innerW)
	}
	m.populateTable()
	if m.detailKind == detailChart && len(m.chartRecords) > 0 {
		m.setDetailContent(renderChart(m.chartRecords, m.detail.Width, m.detail.Height,
			m.chartPendingFrom, m.chartPendingTo, m.chartFocusEndpoint, m.chartFocusBlinkOn))
	}
	if m.detailKind == detailRecord && m.currentRecord != nil {
		m.setDetailContent(renderRecordDetail(m.currentRecord, m.detail.Width))
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
	if m.currentView == viewFilters {
		return m.viewFilters()
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
		case modalHelp:
			return m.viewHelp()
		case modalPickFilter:
			return m.viewPickFilter()
		case modalResolveFilter:
			return m.viewResolveFilter()
		}
	}

	var sections []string
	sections = append(sections, m.renderTabs())

	editorBorder := paneBorder
	if m.focus == focusEditor {
		editorBorder = paneBorderFocused
	}
	sections = append(sections, editorBorder.Render(m.editor.View()))

	if m.focus == focusDetail {
		title := "Detail (Esc to close)"
		if m.detailKind == detailChart {
			title = "Chart (Esc to close)"
		}
		sections = append(sections, paneTitleFocused.Render(title))
		sections = append(sections, paneBorderFocused.Render(m.detail.View()))
	} else {
		title := "Results"
		if m.rowCount > 0 {
			title = fmt.Sprintf("Results (%d)", m.rowCount)
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

// renderTabs draws the three view tabs in one row, highlighting the active
// view. Right-aligned context (env name, vim mode) lives on the same row
// since the per-view title would otherwise duplicate the tab name.
func (m Model) renderTabs() string {
	labels := []struct {
		v    view
		text string
	}{
		{viewQuery, "(1) Query"},
		{viewSaved, "(2) Saved searches"},
		{viewFilters, "(3) Filters"},
	}
	parts := make([]string, len(labels))
	for i, l := range labels {
		if l.v == m.currentView {
			parts[i] = paneTitleFocused.Render(l.text)
		} else {
			parts[i] = paneTitle.Render(l.text)
		}
	}
	left := strings.Join(parts, " ")

	var rightParts []string
	if m.envName != "" {
		rightParts = append(rightParts, "env: "+m.envName)
	}
	if mode := m.activeEditorMode(); mode != "" {
		rightParts = append(rightParts, mode)
	}
	right := ""
	if len(rightParts) > 0 {
		right = paneTitle.Render(strings.Join(rightParts, " · "))
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// activeEditorMode returns the vim mode string of whichever editor is
// currently in focus, or "" when no editor is (or when vim mode is
// disabled). This drives the mode chip shown on the tabs row so users
// always see INSERT/NORMAL when typing, regardless of which view's
// editor is active.
func (m Model) activeEditorMode() string {
	switch {
	case m.currentView == viewQuery && m.focus == focusEditor:
		if !m.editor.Vim() {
			return ""
		}
		return m.editor.Mode().String()
	case m.currentView == viewSaved && m.savedMode == savedModeEditing && m.savedEditFocus == savedEditFocusBody:
		if !m.savedEditBody.Vim() {
			return ""
		}
		return m.savedEditBody.Mode().String()
	}
	return ""
}

func (m Model) statusLine() string {
	// While typing into the detail search prompt, the status line *is* the
	// search prompt — overrides everything else.
	if m.focus == focusDetail && m.detailSearchActive {
		return statusBar.Render(m.detailSearchInput.View())
	}
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
	right := "Alt-Enter run · Ctrl-G chart · Alt-1/2/3 view · ? help · q quit"
	if m.focus == focusDetail {
		if s := m.detailSearchStatus(); s != "" {
			right = s
		} else {
			right = "/ search · gg/G top/bottom · q close"
		}
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return statusBar.Render(left + strings.Repeat(" ", gap) + right)
}

// Helpers

var preferredColumns = []string{"timestamp", "loglevel", "status", "content"}

// pickColumns returns the table columns to render. When `order` is non-empty
// (Grail returned column metadata) it's used verbatim — that's the user's
// authored projection, so we trust it. Otherwise we fall back to a heuristic:
// preferred columns first, then alphabetical, with attribute fields
// (dt.*/k8s.*/host.*) deprioritised so raw `fetch logs` doesn't drown the
// table in noisy keys.
func pickColumns(records grail.Records, order []string) []string {
	if len(records) == 0 {
		return []string{"(empty)"}
	}
	if len(order) > 0 {
		return order
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
	noisy := make([]string, 0)
	for k := range keys {
		if strings.HasPrefix(k, "dt.") || strings.HasPrefix(k, "k8s.") || strings.HasPrefix(k, "host.") {
			noisy = append(noisy, k)
			continue
		}
		rest = append(rest, k)
	}
	sort.Strings(rest)
	sort.Strings(noisy)
	for _, k := range rest {
		if len(out) >= 4 {
			break
		}
		out = append(out, k)
	}
	for _, k := range noisy {
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
