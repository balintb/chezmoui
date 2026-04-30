package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/balintb/chezmoui/internal/chezmoi"
	"github.com/balintb/chezmoui/internal/config"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

type viewState int

const (
	viewLoading viewState = iota
	viewList
	viewSideBySide
	viewRepoSetup
)

type tabID int

const (
	tabAll tabID = iota
	tabModified
	tabHelp
)

func (t tabID) name() string {
	switch t {
	case tabAll:
		return "All"
	case tabModified:
		return "Modified"
	case tabHelp:
		return "Help"
	}
	return ""
}

const tabCount = 3

type rowCategory int

const (
	catModified rowCategory = iota
	catAdded
	catDeleted
	catRun
	catClean
)

func (c rowCategory) title() string {
	switch c {
	case catModified:
		return "Modified"
	case catAdded:
		return "Will add"
	case catDeleted:
		return "Will delete"
	case catRun:
		return "Scripts"
	default:
		return "Clean"
	}
}

type row struct {
	target   string
	absolute string
	source   string
	status   chezmoi.StatusCode
	srcDrift chezmoi.StatusCode
	isDir    bool
}

func (r row) modified() bool {
	return r.status == chezmoi.StatusModified || r.srcDrift == chezmoi.StatusModified
}

func (r row) category() rowCategory {
	if r.modified() {
		return catModified
	}
	switch r.status {
	case chezmoi.StatusAdded:
		return catAdded
	case chezmoi.StatusDeleted:
		return catDeleted
	case chezmoi.StatusRun:
		return catRun
	}
	return catClean
}

func isSourceDir(sourceAbs string) bool {
	st, err := os.Lstat(sourceAbs)
	if err != nil {
		return false
	}
	return st.IsDir()
}

func isDirReadError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "is a directory")
}

func sortRows(rows []row) {
	sort.SliceStable(rows, func(i, j int) bool {
		ci, cj := rows[i].category(), rows[j].category()
		if ci != cj {
			return ci < cj
		}
		return rows[i].target < rows[j].target
	})
}

type Backend interface {
	Managed(ctx context.Context) ([]chezmoi.Entry, error)
	Status(ctx context.Context) ([]chezmoi.Status, error)
	Cat(ctx context.Context, path string) (string, error)
	ReAdd(ctx context.Context, paths ...string) error
	Apply(ctx context.Context, paths ...string) error
	SourcePath(ctx context.Context) (string, error)
	GitStatus(ctx context.Context) (chezmoi.GitStatus, error)
}

type Model struct {
	cli         Backend
	readFile    func(string) ([]byte, error)
	configStore *config.Store

	repoPath  string
	repoSetup repoSetupData

	activeTab   tabID
	previousTab tabID
	state       viewState

	rows        []row
	cursor      int
	selected    map[string]bool
	visibleIdxs []int

	vp              viewport.Model
	sideTarget      string
	sideLive        string
	sideLiveMissing bool
	sideDisplay     string
	sideAbs         string
	sideAdded       int
	sideRemoved     int
	sideLoose       int

	pendingPaths []string
	confirmMsg   string

	session sessionData

	width, height int
	status        string
	err           error
	loading       bool
}

func NewModel(cli Backend) Model {
	return Model{
		cli:         cli,
		readFile:    os.ReadFile,
		configStore: config.DefaultStore(),
		state:       viewLoading,
		selected:    map[string]bool{},
		loading:     true,
	}
}

func (m Model) WithConfigStore(s *config.Store) Model {
	m.configStore = s
	return m
}

func (m Model) WithReadFile(f func(string) ([]byte, error)) Model {
	m.readFile = f
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadEntriesCmd(m.cli), loadConfigCmd(m.configStore))
}

type entriesLoadedMsg struct{ rows []row }
type sideLoadedMsg struct {
	absolute    string
	displayPath string
	target      string
	live        string
	liveMissing bool
}
type reAddDoneMsg struct{ count int }
type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

func loadEntriesCmd(cli Backend) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		entries, err := cli.Managed(ctx)
		if err != nil {
			return errMsg{err}
		}
		statuses, err := cli.Status(ctx)
		if err != nil {
			return errMsg{err}
		}
		stByPath := map[string]chezmoi.Status{}
		for _, s := range statuses {
			stByPath[s.Path] = s
		}
		rows := make([]row, 0, len(entries))
		for _, e := range entries {
			r := row{
				target:   e.Target,
				absolute: e.Absolute,
				source:   e.SourceRelative,
				status:   chezmoi.StatusClean,
				srcDrift: chezmoi.StatusClean,
				isDir:    isSourceDir(e.SourceAbsolute),
			}
			if s, ok := stByPath[e.Target]; ok {
				r.status = s.Target
				r.srcDrift = s.Source
			}
			rows = append(rows, r)
		}
		sortRows(rows)
		return entriesLoadedMsg{rows: rows}
	}
}

func loadSideCmd(cli Backend, absPath, displayPath string, reader func(string) ([]byte, error)) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		target, err := cli.Cat(ctx, absPath)
		if err != nil {
			return errMsg{err}
		}
		var live []byte
		var liveMissing bool
		live, err = reader(absPath)
		switch {
		case err == nil:
		case os.IsNotExist(err), isDirReadError(err):
			liveMissing = true
		default:
			return errMsg{err}
		}
		return sideLoadedMsg{
			absolute: absPath, displayPath: displayPath,
			target: target, live: string(live), liveMissing: liveMissing,
		}
	}
}

func reAddCmd(cli Backend, paths []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := cli.ReAdd(ctx, paths...); err != nil {
			return errMsg{err}
		}
		return reAddDoneMsg{count: len(paths)}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		bw, bh := m.bodyDims()
		m.vp = viewport.New(bw, bh)
		if m.state == viewSideBySide {
			m.vp.SetContent(m.renderSidePanels())
		}
		return m, nil

	case entriesLoadedMsg:
		m.rows = msg.rows
		m.loading = false
		m.state = viewList
		m.recomputeVisible()
		if m.cursor >= len(m.visibleIdxs) {
			m.cursor = 0
		}
		return m, nil

	case sideLoadedMsg:
		m.sideDisplay = msg.displayPath
		m.sideAbs = msg.absolute
		m.sideTarget = msg.target
		m.sideLive = msg.live
		m.sideLiveMissing = msg.liveMissing
		rows := alignLines(msg.target, msg.live)
		m.sideAdded, m.sideRemoved, m.sideLoose = summarizeAlignment(rows)
		m.vp.SetContent(m.renderSidePanels())
		m.vp.GotoTop()
		m.state = viewSideBySide
		return m, nil

	case reAddDoneMsg:
		m.status = fmt.Sprintf("re-added %d file(s)", msg.count)
		m.selected = map[string]bool{}
		m.pendingPaths = nil
		m.confirmMsg = ""
		m.loading = true
		return m, loadEntriesCmd(m.cli)

	case errMsg:
		m.err = msg.err
		m.loading = false
		m.confirmMsg = ""
		m.pendingPaths = nil
		m.session.working = false
		return m, nil

	case configLoadedMsg:
		if msg.loadErr != nil {
			m.status = "config load: " + msg.loadErr.Error()
		}
		m.repoPath = msg.cfg.RepoPath
		if !msg.cfg.RepoConfirmed {
			return m, m.enterRepoSetup()
		}
		return m, nil

	case repoCandidatesMsg:
		m.repoSetup.candidates = msg.candidates
		m.repoSetup.cursor = 0
		return m, nil

	case configSavedMsg:
		if msg.err != nil {
			m.status = "config save failed: " + msg.err.Error()
		}
		return m, nil

	case sessionStartedMsg:
		m.session.queue = msg.queue
		m.session.cursor = 0
		m.session.sourceRepoPath = msg.sourceRepoPath
		m.session.gitBranch = msg.gitBranch
		m.session.gitChanged = msg.gitChanged
		if len(msg.queue) == 0 {
			m.session.state = sessionSummary
			return m, nil
		}
		m.session.state = sessionReview
		return m, m.loadCurrentSessionEntry()

	case sessionEntryLoadedMsg:
		if msg.cursor >= 0 && msg.cursor < len(m.session.queue) {
			m.session.queue[msg.cursor].target_contents = msg.target
			m.session.queue[msg.cursor].live_contents = msg.live
			m.session.queue[msg.cursor].liveMissing = msg.liveMissing
			m.session.queue[msg.cursor].added = msg.added
			m.session.queue[msg.cursor].removed = msg.removed
			m.session.queue[msg.cursor].loose = msg.loose
			m.session.queue[msg.cursor].loaded = true
		}
		if msg.cursor == m.session.cursor {
			m.vp.SetContent(m.renderSessionPanels())
			m.vp.GotoTop()
		}
		return m, nil

	case sessionDecisionDoneMsg:
		if msg.cursor >= 0 && msg.cursor < len(m.session.queue) {
			m.session.queue[msg.cursor].decision = msg.decision
			if msg.snapshotPath != "" {
				m.session.queue[msg.cursor].snapshotPath = msg.snapshotPath
			}
		}
		m.session.working = false
		return m, m.advanceSession()

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirmMsg != "" {
		switch {
		case key.Matches(msg, keys.Confirm):
			paths := m.pendingPaths
			m.confirmMsg = ""
			m.pendingPaths = nil
			m.loading = true
			return m, reAddCmd(m.cli, paths)
		case key.Matches(msg, keys.Cancel):
			m.confirmMsg = ""
			m.pendingPaths = nil
			m.status = "cancelled"
		}
		return m, nil
	}

	if m.session.state != sessionInactive {
		return m.handleSessionKey(msg)
	}
	if m.state == viewRepoSetup {
		return m.handleRepoSetupKey(msg)
	}

	switch {
	case key.Matches(msg, keys.NextTab):
		m.switchTab((m.activeTab + 1) % tabCount)
		return m, nil
	case key.Matches(msg, keys.PrevTab):
		m.switchTab((m.activeTab + tabCount - 1) % tabCount)
		return m, nil
	case key.Matches(msg, keys.OnlyMod):
		if m.activeTab == tabModified {
			m.switchTab(tabAll)
		} else {
			m.switchTab(tabModified)
		}
		return m, nil
	case key.Matches(msg, keys.Help):
		if m.activeTab == tabHelp {
			m.switchTab(m.previousTab)
		} else {
			m.previousTab = m.activeTab
			m.switchTab(tabHelp)
		}
		return m, nil
	}

	if m.activeTab == tabHelp {
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Cancel), key.Matches(msg, keys.Back):
			m.activeTab = m.previousTab
		}
		return m, nil
	}

	switch m.state {
	case viewSideBySide:
		return m.updateSide(msg)
	case viewList:
		return m.updateList(msg)
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, keys.Down):
		if m.cursor < len(m.visibleIdxs)-1 {
			m.cursor++
		}
	case key.Matches(msg, keys.PgUp):
		m.cursor -= 10
		if m.cursor < 0 {
			m.cursor = 0
		}
	case key.Matches(msg, keys.PgDown):
		m.cursor += 10
		if m.cursor > len(m.visibleIdxs)-1 {
			m.cursor = len(m.visibleIdxs) - 1
		}
	case key.Matches(msg, keys.Home):
		m.cursor = 0
	case key.Matches(msg, keys.End):
		m.cursor = max(0, len(m.visibleIdxs)-1)
	case key.Matches(msg, keys.Toggle):
		if r, ok := m.cursorRow(); ok {
			if m.selected[r.target] {
				delete(m.selected, r.target)
			} else {
				m.selected[r.target] = true
			}
		}
	case key.Matches(msg, keys.Refresh):
		m.loading = true
		m.err = nil
		return m, loadEntriesCmd(m.cli)
	case key.Matches(msg, keys.View):
		if r, ok := m.cursorRow(); ok {
			if r.isDir {
				m.status = fmt.Sprintf("directory — open a file inside %q to view its diff", r.target)
				return m, nil
			}
			m.status = "loading…"
			return m, loadSideCmd(m.cli, r.absolute, r.target, m.readFile)
		}
	case key.Matches(msg, keys.SessionStart):
		return m.startSession()
	case key.Matches(msg, keys.RelocateRepo):
		return m, m.enterRepoSetup()
	case key.Matches(msg, keys.ReAdd):
		paths := m.actionPaths()
		if len(paths) == 0 {
			m.status = "nothing to re-add"
			return m, nil
		}
		m.pendingPaths = paths
		m.confirmMsg = fmt.Sprintf("Re-add %d file(s)?", len(paths))
	}
	return m, nil
}

func (m Model) updateSide(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit), key.Matches(msg, keys.Back):
		m.state = viewList
		return m, nil
	case key.Matches(msg, keys.ReAdd):
		m.pendingPaths = []string{m.sideAbs}
		m.confirmMsg = fmt.Sprintf("Re-add %s?", m.sideDisplay)
		return m, nil
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionRelease {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.scrollWheel(-1)
	case tea.MouseButtonWheelDown:
		return m.scrollWheel(+1)
	case tea.MouseButtonLeft:
		for t := tabID(0); t < tabCount; t++ {
			if zone.Get(tabZoneID(t)).InBounds(msg) {
				if t == tabHelp && m.activeTab != tabHelp {
					m.previousTab = m.activeTab
				}
				m.switchTab(t)
				return m, nil
			}
		}
		if m.activeTab != tabHelp && m.state == viewList {
			for vi, idx := range m.visibleIdxs {
				if zone.Get(rowZoneID(idx)).InBounds(msg) {
					m.cursor = vi
					return m, nil
				}
			}
		}
	}
	return m, nil
}

func (m Model) scrollWheel(delta int) (tea.Model, tea.Cmd) {
	if m.activeTab == tabHelp {
		return m, nil
	}
	if m.state == viewSideBySide {
		for i := 0; i < 3; i++ {
			if delta < 0 {
				m.vp.LineUp(1)
			} else {
				m.vp.LineDown(1)
			}
		}
		return m, nil
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.visibleIdxs)-1 {
		m.cursor = max(0, len(m.visibleIdxs)-1)
	}
	return m, nil
}

func (m Model) onlyMod() bool { return m.activeTab == tabModified }

func (m Model) startSession() (Model, tea.Cmd) {
	m.session.state = sessionWelcome
	if m.session.snapshotDir == "" {
		m.session.snapshotDir = defaultSnapshotDir()
	}
	return m, nil
}

func (m *Model) loadCurrentSessionEntry() tea.Cmd {
	e, ok := m.session.entry()
	if !ok || e.loaded {
		if ok {
			m.vp.SetContent(m.renderSessionPanels())
			m.vp.GotoTop()
		}
		return nil
	}
	return loadSessionEntryCmd(m.cli, m.session.cursor, e.absolute, m.readFile)
}

func (m *Model) advanceSession() tea.Cmd {
	for i := m.session.cursor + 1; i < len(m.session.queue); i++ {
		if m.session.queue[i].decision == decisionPending {
			m.session.cursor = i
			return m.loadCurrentSessionEntry()
		}
	}
	for i := 0; i < len(m.session.queue); i++ {
		if m.session.queue[i].decision == decisionPending {
			m.session.cursor = i
			return m.loadCurrentSessionEntry()
		}
	}
	m.session.state = sessionSummary
	return nil
}

func (m Model) renderSessionPanels() string {
	e, ok := m.session.entry()
	if !ok {
		return ""
	}
	bw, _ := m.bodyDims()
	panelOuterW := (bw - 1) / 2
	panelInnerW := panelOuterW - 4
	if panelInnerW < sideGutterWidth+5 {
		panelInnerW = sideGutterWidth + 5
	}
	rows := alignLines(e.target_contents, e.live_contents)
	left, right := renderSideBySide(rows, panelInnerW)
	leftPanel := panelStyle.Width(panelOuterW).Render(strings.TrimRight(left, "\n"))
	rightPanel := panelStyle.Width(panelOuterW).Render(strings.TrimRight(right, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightPanel)
}

func (m Model) handleSessionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.session.state {
	case sessionWelcome:
		switch {
		case key.Matches(msg, keys.Confirm):
			if len(m.session.queue) == 0 {
				return m, startSessionCmd(m.cli, m.rows, m.session.snapshotDir)
			}
			m.session.state = sessionReview
			return m, m.loadCurrentSessionEntry()
		case key.Matches(msg, keys.Cancel), key.Matches(msg, keys.Back):
			m.session.state = sessionInactive
			return m, nil
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		}
		return m, nil

	case sessionReview:
		if m.session.working {
			return m, nil
		}
		switch {
		case key.Matches(msg, keys.KeepLive):
			e, ok := m.session.entry()
			if !ok {
				return m, nil
			}
			m.session.working = true
			return m, keepLiveCmd(m.cli, m.session.cursor, e.absolute)
		case key.Matches(msg, keys.Revert):
			e, ok := m.session.entry()
			if !ok {
				return m, nil
			}
			m.session.working = true
			return m, revertCmd(m.cli, m.session.cursor, e.absolute, m.session.snapshotDir, m.readFile)
		case key.Matches(msg, keys.SkipEntry):
			if e, ok := m.session.entry(); ok {
				m.session.queue[m.session.cursor].decision = decisionSkipped
				_ = e
			}
			return m, m.advanceSession()
		case key.Matches(msg, keys.BackEntry):
			if m.session.cursor > 0 {
				m.session.cursor--
				return m, m.loadCurrentSessionEntry()
			}
			return m, nil
		case key.Matches(msg, keys.Cancel), key.Matches(msg, keys.Back), key.Matches(msg, keys.Quit):
			m.session.state = sessionSummary
			return m, nil
		case key.Matches(msg, keys.PgUp):
			m.vp.HalfPageUp()
		case key.Matches(msg, keys.PgDown):
			m.vp.HalfPageDown()
		case key.Matches(msg, keys.Up):
			m.vp.LineUp(1)
		case key.Matches(msg, keys.Down):
			m.vp.LineDown(1)
		}
		return m, nil

	case sessionSummary:
		switch {
		case key.Matches(msg, keys.Confirm), key.Matches(msg, keys.Cancel), key.Matches(msg, keys.Back):
			m.session = sessionData{snapshotDir: m.session.snapshotDir}
			m.loading = true
			return m, loadEntriesCmd(m.cli)
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *Model) switchTab(t tabID) {
	m.activeTab = t
	m.recomputeVisible()
	if m.cursor >= len(m.visibleIdxs) {
		m.cursor = max(0, len(m.visibleIdxs)-1)
	}
}

func (m *Model) recomputeVisible() {
	m.visibleIdxs = m.visibleIdxs[:0]
	for i, r := range m.rows {
		if m.onlyMod() && !r.modified() {
			continue
		}
		m.visibleIdxs = append(m.visibleIdxs, i)
	}
}

func (m Model) cursorRow() (row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visibleIdxs) {
		return row{}, false
	}
	return m.rows[m.visibleIdxs[m.cursor]], true
}

func (m Model) actionPaths() []string {
	var paths []string
	if len(m.selected) > 0 {
		for _, r := range m.rows {
			if m.selected[r.target] && r.modified() {
				paths = append(paths, r.absolute)
			}
		}
		return paths
	}
	if r, ok := m.cursorRow(); ok && r.modified() {
		paths = append(paths, r.absolute)
	}
	return paths
}

func (m Model) categoryCounts() map[rowCategory]int {
	counts := map[rowCategory]int{}
	for _, r := range m.rows {
		counts[r.category()]++
	}
	return counts
}

func (m Model) bodyDims() (int, int) {
	w := m.width - 4
	h := m.height - 5
	if w < 20 {
		w = 20
	}
	if h < 5 {
		h = 5
	}
	return w, h
}

func tabZoneID(t tabID) string { return fmt.Sprintf("tab-%d", t) }
func rowZoneID(idx int) string { return fmt.Sprintf("row-%d", idx) }

func (m Model) alignTabsAndChip(tabs, chip string) string {
	bw, _ := m.bodyDims()
	lw := lipgloss.Width(tabs)
	rw := lipgloss.Width(chip)
	gap := bw - lw - rw
	if gap < 1 {
		return tabs
	}
	return tabs + strings.Repeat(" ", gap) + chip
}

func (m Model) View() string {
	if m.width == 0 {
		return "chezmoui"
	}
	body := m.viewBody()
	var chrome string
	switch {
	case m.state == viewRepoSetup:
		chrome = body
	case m.session.state != sessionInactive:
		chrome = lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("⚡ Sync session")+"  "+m.sessionFooter(),
			body,
		)
	default:
		topRow := m.viewTabBar()
		if chip := m.repoChip(); chip != "" {
			topRow = m.alignTabsAndChip(topRow, chip)
		}
		chrome = lipgloss.JoinVertical(lipgloss.Left, topRow, body)
	}
	framed := windowStyle.Width(m.width - 2).Render(chrome)
	if m.confirmMsg != "" {
		framed = overlay(framed, m.viewConfirmModal(), m.width, m.height)
	}
	return zone.Scan(framed)
}

func (m Model) viewTabBar() string {
	var parts []string
	for t := tabID(0); t < tabCount; t++ {
		label := t.name()
		if t == tabModified {
			counts := m.categoryCounts()
			if n := counts[catModified]; n > 0 {
				label = fmt.Sprintf("%s (%d)", label, n)
			}
		}
		var styled string
		if t == m.activeTab {
			styled = tabActiveStyle.Render(label)
		} else {
			styled = tabInactiveStyle.Render(label)
		}
		parts = append(parts, zone.Mark(tabZoneID(t), styled))
	}
	return strings.Join(parts, tabSeparatorStyle.Render(" │ "))
}

func (m Model) viewBody() string {
	switch {
	case m.state == viewRepoSetup:
		return m.viewRepoSetup()
	case m.loading:
		return mutedStyle.Render("loading…")
	case m.err != nil:
		return errorStyle.Render("error: "+m.err.Error()) + "\n\n" +
			mutedStyle.Render("press R to retry · q to quit")
	case m.session.state == sessionWelcome:
		return m.viewSessionWelcome()
	case m.session.state == sessionReview:
		return m.viewSessionReview()
	case m.session.state == sessionSummary:
		return m.viewSessionSummary()
	case m.activeTab == tabHelp:
		return m.viewHelpPage()
	case m.state == viewSideBySide:
		return m.viewSidePane()
	default:
		return m.viewListPane()
	}
}

func (m Model) viewListPane() string {
	counts := m.categoryCounts()
	chips := []string{
		statusChip("●", "modified", counts[catModified], badgeModified),
		statusChip("+", "add", counts[catAdded], badgeAdded),
		statusChip("−", "delete", counts[catDeleted], badgeDeleted),
		statusChip("·", "clean", counts[catClean], badgeClean),
	}
	title := mutedStyle.Render(strings.Join(chips, "  "))

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")

	_, bh := m.bodyDims()
	listHeight := bh - 4
	if listHeight < 5 {
		listHeight = 5
	}

	if len(m.visibleIdxs) == 0 {
		b.WriteString(mutedStyle.Render("  (no entries)"))
	} else {
		b.WriteString(m.renderListWindow(listHeight))
	}

	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(mutedStyle.Render(m.status))
	} else if r, ok := m.cursorRow(); ok {
		b.WriteString(mutedStyle.Render(r.absolute))
	}
	b.WriteString("\n")
	b.WriteString(m.contextHelp())
	return b.String()
}

func (m Model) renderListWindow(height int) string {
	type renderable struct {
		header bool
		text   string
		rowVi  int
	}
	bw, _ := m.bodyDims()
	var items []renderable
	prevCat := rowCategory(-1)
	for vi, idx := range m.visibleIdxs {
		r := m.rows[idx]
		c := r.category()
		if c != prevCat {
			items = append(items, renderable{header: true, text: sectionHeaderStyle.Render(c.title())})
			prevCat = c
		}
		isCursor := vi == m.cursor
		line := formatRow(r, m.selected[r.target], isCursor, bw)
		line = zone.Mark(rowZoneID(idx), line)
		items = append(items, renderable{text: line, rowVi: vi})
	}

	cursorPos := 0
	for i, it := range items {
		if !it.header && it.rowVi == m.cursor {
			cursorPos = i
			break
		}
	}
	start := 0
	if cursorPos >= height {
		start = cursorPos - height + 1
	}
	end := start + height
	if end > len(items) {
		end = len(items)
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		b.WriteString(items[i].text)
		b.WriteString("\n")
	}
	return b.String()
}

func formatRow(r row, selected, cursor bool, width int) string {
	withCursor := func(s lipgloss.Style) lipgloss.Style {
		if cursor {
			return s.Background(colorSelBg).Bold(true)
		}
		return s
	}
	plainBg := lipgloss.NewStyle()
	if cursor {
		plainBg = plainBg.Background(colorSelBg)
	}

	var ch string
	var glyphStyle lipgloss.Style
	switch r.category() {
	case catModified:
		ch, glyphStyle = "●", badgeModified
	case catAdded:
		ch, glyphStyle = "+", badgeAdded
	case catDeleted:
		ch, glyphStyle = "−", badgeDeleted
	case catRun:
		ch, glyphStyle = "↻", badgeRun
	default:
		ch, glyphStyle = "·", badgeClean
	}

	var markPart string
	if selected {
		markPart = withCursor(selectedMark).Render("✓")
	} else {
		markPart = plainBg.Render(" ")
	}
	glyphPart := withCursor(glyphStyle).Render(ch)

	target := r.target
	if r.isDir {
		target += "/"
	}
	var targetStyle lipgloss.Style
	if r.category() == catClean {
		targetStyle = mutedStyle
	} else {
		targetStyle = lipgloss.NewStyle()
	}
	targetPart := withCursor(targetStyle).Render(target)

	line := plainBg.Render("  ") + markPart + plainBg.Render(" ") +
		glyphPart + plainBg.Render("  ") + targetPart

	if cursor && width > 0 {
		visible := lipgloss.Width(line)
		if visible < width {
			line += plainBg.Render(strings.Repeat(" ", width-visible))
		}
	}
	return line
}

func statusChip(glyph, label string, count int, style lipgloss.Style) string {
	return fmt.Sprintf("%s %d %s", style.Render(glyph), count, label)
}

func (m Model) contextHelp() string {
	switch m.state {
	case viewSideBySide:
		return mutedStyle.Render("↑/↓ scroll · r re-add · esc back · ? help · q quit")
	default:
		return mutedStyle.Render("enter view · space select · tab switch · r re-add · S session · ? help · q quit")
	}
}

func (m Model) renderSidePanels() string {
	bw, _ := m.bodyDims()
	panelOuterW := (bw - 1) / 2
	panelInnerW := panelOuterW - 4
	if panelInnerW < sideGutterWidth+5 {
		panelInnerW = sideGutterWidth + 5
	}
	rows := alignLines(m.sideTarget, m.sideLive)
	left, right := renderSideBySide(rows, panelInnerW)
	leftPanel := panelStyle.Width(panelOuterW).Render(strings.TrimRight(left, "\n"))
	rightPanel := panelStyle.Width(panelOuterW).Render(strings.TrimRight(right, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightPanel)
}

func (m Model) viewSidePane() string {
	leftLabel := "target (chezmoi)"
	rightLabel := "live (filesystem)"
	if m.sideLiveMissing {
		rightLabel = "live (missing)"
	}
	summary := fmt.Sprintf("%s  %s",
		addStyle.Render(fmt.Sprintf("+%d", m.sideAdded)),
		delStyle.Render(fmt.Sprintf("−%d", m.sideRemoved)),
	)
	if m.sideLoose > 0 {
		summary += "  " + looseLineStyle.Render(fmt.Sprintf(" ≈%d ", m.sideLoose))
	}
	headerLines := []string{
		titleStyle.Render(m.sideDisplay) + "  " + summary,
	}
	if m.sideAdded == 0 && m.sideRemoved == 0 && m.sideLoose > 0 {
		headerLines = append(headerLines,
			looseLineStyle.Render(" ≈ logically identical — only case/whitespace differs "),
		)
	}
	headerLines = append(headerLines, mutedStyle.Render(leftLabel+"  vs  "+rightLabel))
	header := strings.Join(headerLines, "\n")
	return lipgloss.JoinVertical(lipgloss.Left, header, m.vp.View(), m.contextHelp())
}

func (m Model) viewConfirmModal() string {
	body := titleStyle.Render(m.confirmMsg) + "\n"
	if len(m.pendingPaths) > 1 {
		body += "\n"
		shown := m.pendingPaths
		const maxShow = 8
		if len(shown) > maxShow {
			shown = shown[:maxShow]
		}
		for _, p := range shown {
			body += "  " + mutedStyle.Render("• ") + p + "\n"
		}
		if len(m.pendingPaths) > maxShow {
			body += "  " + mutedStyle.Render(fmt.Sprintf("(+%d more)", len(m.pendingPaths)-maxShow)) + "\n"
		}
	}
	body += "\n" + statusStyle.Render("[y]") + " confirm   " + statusStyle.Render("[n/esc]") + " cancel"
	return modalStyle.Render(body)
}

func (m Model) viewHelpPage() string {
	rows := [][2]string{
		{"Movement", ""},
		{"  ↑ / k", "up"},
		{"  ↓ / j", "down"},
		{"  pgup / ctrl+u", "page up"},
		{"  pgdn / ctrl+d", "page down"},
		{"  g / G", "top / bottom"},
		{"  mouse wheel", "scroll list / diff"},
		{"Navigation", ""},
		{"  tab / shift+tab", "next / prev tab"},
		{"  click tab", "switch tab"},
		{"  m", "jump to Modified tab"},
		{"  ?", "toggle Help tab"},
		{"  esc", "back / leave help"},
		{"Selection", ""},
		{"  space", "select / deselect"},
		{"  click row", "move cursor"},
		{"Actions", ""},
		{"  enter / d / s", "side-by-side diff"},
		{"  r", "re-add (live → source)"},
		{"  R", "refresh"},
		{"  S", "start sync session"},
		{"Sync session", ""},
		{"  k", "keep live (re-add)"},
		{"  v", "revert to source (with backup)"},
		{"  s", "skip"},
		{"  ← / h", "previous entry"},
		{"  esc", "end session"},
		{"Other", ""},
		{"  q / ctrl+c", "quit"},
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Keybindings") + "\n\n")
	for _, r := range rows {
		if r[1] == "" {
			b.WriteString(sectionHeaderStyle.Render(r[0]) + "\n")
			continue
		}
		b.WriteString(fmt.Sprintf("%-22s %s\n", r[0], mutedStyle.Render(r[1])))
	}
	return b.String()
}

func overlay(_, content string, width, height int) string {
	if width <= 0 || height <= 0 {
		return content
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
