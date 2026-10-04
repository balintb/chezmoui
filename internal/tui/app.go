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
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	tabUnmanaged
	tabIgnored
	tabDoctor
	tabHelp
)

// tabs lists the tab order shown in the tab bar.
var tabs = []tabID{tabAll, tabModified, tabUnmanaged, tabIgnored, tabDoctor, tabHelp}

// tabCount is the number of tabs; kept for callers that iterate the enum.
const tabCount = int(tabHelp) + 1

func (t tabID) name() string {
	switch t {
	case tabAll:
		return "All"
	case tabModified:
		return "Modified"
	case tabUnmanaged:
		return "Unmanaged"
	case tabIgnored:
		return "Ignored"
	case tabDoctor:
		return "Doctor"
	case tabHelp:
		return "Help"
	}
	return ""
}

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
	target    string
	absolute  string
	source    string
	sourceAbs string
	status    chezmoi.StatusCode
	srcDrift  chezmoi.StatusCode
	isDir     bool
	attrs     []chezmoi.Attribute
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

// Lister enumerates the managed targets and their drift status.
type Lister interface {
	Managed(ctx context.Context) ([]chezmoi.Entry, error)
	Status(ctx context.Context) ([]chezmoi.Status, error)
}

// Reader loads target contents for diffing.
type Reader interface {
	Cat(ctx context.Context, path string) (string, error)
}

// Differ renders a unified diff for a target path.
type Differ interface {
	Diff(ctx context.Context, path string, reverse bool) (string, error)
}

// Mutator changes the source state and/or the destination state.
type Mutator interface {
	ReAdd(ctx context.Context, paths ...string) error
	Apply(ctx context.Context, paths ...string) error
	Add(ctx context.Context, paths ...string) error
}

// Enumer lists destination paths chezmoi does not manage or has ignored.
type Enumer interface {
	Unmanaged(ctx context.Context) ([]string, error)
	Ignored(ctx context.Context) ([]string, error)
}

// Diagnoser runs chezmoi self-diagnostics.
type Diagnoser interface {
	Doctor(ctx context.Context) ([]chezmoi.DoctorCheck, error)
}

// RepoInfo reports source-repository locations and git state.
type RepoInfo interface {
	SourcePath(ctx context.Context) (string, error)
	GitStatus(ctx context.Context) (chezmoi.GitStatus, error)
}

// Backend is the composite of all chezmoi operations used by the TUI.
type Backend interface {
	Lister
	Reader
	Differ
	Mutator
	RepoInfo
	Enumer
	Diagnoser
}

type Model struct {
	cli         Backend
	readFile    func(string) ([]byte, error)
	writeFile   func(string, []byte, os.FileMode) error
	configStore *config.Store
	now         func() time.Time
	cacheDir    string

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

	sideUnified bool
	unifiedDiff string
	hunks       []int
	hunkCursor  int
	sideWrap    bool
	sideHScroll int
	filtering   bool
	filter      string
	filterInput textinput.Model

	undo []undoEntry

	unmanaged       []string
	unmanagedSel    map[string]bool
	unmanagedCursor int
	ignored         []string
	ignoredCursor   int
	doctor          []chezmoi.DoctorCheck
	doctorCursor    int
	inspectLoaded   map[tabID]bool

	pendingPaths []string
	confirmMsg   string
	pendingOp    pendingOp
	pendingUndo  undoEntry

	session sessionData

	width, height int
	status        string
	err           error
	retry         tea.Cmd
	loading       bool
}

func NewModel(cli Backend) Model {
	return Model{
		cli:           cli,
		readFile:      os.ReadFile,
		writeFile:     writeFileAtomic,
		now:           time.Now,
		configStore:   config.DefaultStore(),
		state:         viewLoading,
		selected:      map[string]bool{},
		unmanagedSel:  map[string]bool{},
		inspectLoaded: map[tabID]bool{},
		filterInput:   newFilterInput(),
		loading:       true,
	}
}

// pendingOp identifies which action the confirm modal will run.
type pendingOp int

const (
	opReAdd pendingOp = iota
	opApply
	opUndo
	opAdd
)

func newFilterInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = "filter: "
	ti.Placeholder = "substring of target path"
	ti.CharLimit = 256
	return ti
}

func (m Model) WithConfigStore(s *config.Store) Model {
	m.configStore = s
	return m
}

// WithWriteFile overrides the file writer used to restore undo snapshots.
func (m Model) WithWriteFile(f func(string, []byte, os.FileMode) error) Model {
	m.writeFile = f
	return m
}

func (m Model) WithReadFile(f func(string) ([]byte, error)) Model {
	m.readFile = f
	return m
}

// WithNow overrides the clock used for snapshot timestamps. Tests inject a fixed clock to keep filenames deterministic.
func (m Model) WithNow(now func() time.Time) Model {
	m.now = now
	return m
}

// WithCacheDir overrides the base cache directory used for default snapshot storage. An empty value falls back to the OS user cache directory.
func (m Model) WithCacheDir(dir string) Model {
	m.cacheDir = dir
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
	unified     string
}
type reAddDoneMsg struct {
	count int
	undo  []undoEntry
}
type applyDoneMsg struct {
	count int
	undo  []undoEntry
}
type errMsg struct {
	err   error
	retry tea.Cmd
}

func (e errMsg) Error() string { return e.err.Error() }

func loadEntriesCmd(cli Lister) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		entries, err := cli.Managed(ctx)
		if err != nil {
			return errMsg{err: opError{Op: "list managed targets", Err: err}, retry: loadEntriesCmd(cli)}
		}
		statuses, err := cli.Status(ctx)
		if err != nil {
			return errMsg{err: opError{Op: "read status", Err: err}, retry: loadEntriesCmd(cli)}
		}
		stByPath := map[string]chezmoi.Status{}
		for _, s := range statuses {
			stByPath[s.Path] = s
		}
		rows := make([]row, 0, len(entries))
		for _, e := range entries {
			r := row{
				target:    e.Target,
				absolute:  e.Absolute,
				source:    e.SourceRelative,
				sourceAbs: e.SourceAbsolute,
				status:    chezmoi.StatusClean,
				srcDrift:  chezmoi.StatusClean,
				isDir:     isSourceDir(e.SourceAbsolute),
				attrs:     e.Attributes,
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

func loadSideCmd(cli Reader, differ Differ, absPath, displayPath string, reader func(string) ([]byte, error)) tea.Cmd {
	return func() tea.Msg {
		retry := loadSideCmd(cli, differ, absPath, displayPath, reader)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		target, err := cli.Cat(ctx, absPath)
		if err != nil {
			return errMsg{err: opError{Op: "load target contents", Target: absPath, Err: err}, retry: retry}
		}
		var live []byte
		var liveMissing bool
		live, err = reader(absPath)
		switch {
		case err == nil:
		case os.IsNotExist(err), isDirReadError(err):
			liveMissing = true
		default:
			return errMsg{err: opError{Op: "read live file", Target: absPath, Err: err}, retry: retry}
		}
		unified := ""
		if differ != nil {
			if d, derr := differ.Diff(ctx, absPath, false); derr == nil {
				unified = d
			}
		}
		return sideLoadedMsg{
			absolute: absPath, displayPath: displayPath,
			target: target, live: string(live), liveMissing: liveMissing, unified: unified,
		}
	}
}

func reAddCmd(cli Mutator, paths []string) tea.Cmd {
	return reAddCmdUndo(cli, paths, nil)
}

func reAddCmdUndo(cli Mutator, paths []string, undo []undoEntry) tea.Cmd {
	return func() tea.Msg {
		retry := reAddCmdUndo(cli, paths, undo)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := cli.ReAdd(ctx, paths...); err != nil {
			return errMsg{err: opError{Op: "re-add", Target: strings.Join(paths, ", "), Err: err}, retry: retry}
		}
		return reAddDoneMsg{count: len(paths), undo: undo}
	}
}

func applyCmd(cli Mutator, paths []string) tea.Cmd {
	return applyCmdUndo(cli, paths, nil)
}

func applyCmdUndo(cli Mutator, paths []string, undo []undoEntry) tea.Cmd {
	return func() tea.Msg {
		retry := applyCmdUndo(cli, paths, undo)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := cli.Apply(ctx, paths...); err != nil {
			return errMsg{err: opError{Op: "apply", Target: strings.Join(paths, ", "), Err: err}, retry: retry}
		}
		return applyDoneMsg{count: len(paths), undo: undo}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		bw, bh := m.bodyDims()
		m.vp = viewport.New(bw, bh)
		switch {
		case m.state == viewSideBySide:
			m.vp.SetContent(m.renderDiffBody())
		case m.session.state == sessionReview:
			m.vp.SetContent(m.renderSessionPanels())
		}
		return m, nil

	case entriesLoadedMsg:
		m.rows = msg.rows
		m.loading = false
		m.err = nil
		m.retry = nil
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
		m.unifiedDiff = msg.unified
		parsed := parseUnified(msg.unified)
		m.hunks = hunkOffsets(parsed)
		m.hunkCursor = -1
		rows := alignLines(msg.target, msg.live)
		m.sideAdded, m.sideRemoved, m.sideLoose = summarizeAlignment(rows)
		m.vp.SetContent(m.renderDiffBody())
		m.vp.GotoTop()
		m.state = viewSideBySide
		m.err = nil
		m.retry = nil
		m.loading = false
		return m, nil

	case reAddDoneMsg:
		m.pushUndo(msg.undo)
		m.status = fmt.Sprintf("re-added %d file(s) · u to undo", msg.count)
		m.selected = map[string]bool{}
		m.pendingPaths = nil
		m.confirmMsg = ""
		m.err = nil
		m.retry = nil
		m.loading = true
		return m, loadEntriesCmd(m.cli)

	case applyDoneMsg:
		m.pushUndo(msg.undo)
		m.status = fmt.Sprintf("applied %d file(s) · u to undo", msg.count)
		m.selected = map[string]bool{}
		m.pendingPaths = nil
		m.confirmMsg = ""
		m.err = nil
		m.retry = nil
		m.loading = true
		return m, loadEntriesCmd(m.cli)

	case undoDoneMsg:
		if msg.hasPush {
			m.undo = append(m.undo, msg.pushed)
		}
		m.pendingPaths = nil
		m.pendingOp = opUndo
		m.confirmMsg = ""
		m.err = nil
		m.retry = nil
		m.status = "restored " + msg.restored + " · u to redo"
		m.loading = true
		return m, loadEntriesCmd(m.cli)

	case unmanagedLoadedMsg:
		m.unmanaged = msg.paths
		m.unmanagedCursor = 0
		m.unmanagedSel = map[string]bool{}
		m.err = nil
		m.retry = nil
		return m, nil

	case ignoredLoadedMsg:
		m.ignored = msg.paths
		m.ignoredCursor = 0
		m.err = nil
		m.retry = nil
		return m, nil

	case doctorLoadedMsg:
		m.doctor = msg.checks
		m.doctorCursor = 0
		m.err = nil
		m.retry = nil
		return m, nil

	case addDoneMsg:
		m.status = fmt.Sprintf("added %d file(s) to source", msg.count)
		m.unmanagedSel = map[string]bool{}
		m.pendingPaths = nil
		m.pendingOp = opReAdd
		m.confirmMsg = ""
		m.err = nil
		m.retry = nil
		m.inspectLoaded[tabUnmanaged] = false
		m.loading = true
		return m, tea.Batch(loadEntriesCmd(m.cli), loadUnmanagedCmd(m.cli))

	case errMsg:
		m.err = msg.err
		m.retry = msg.retry
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
		m.err = nil
		m.retry = nil
		return m, nil

	case sessionDecisionDoneMsg:
		if msg.cursor >= 0 && msg.cursor < len(m.session.queue) {
			m.session.queue[msg.cursor].decision = msg.decision
			if msg.snapshotPath != "" {
				m.session.queue[msg.cursor].snapshotPath = msg.snapshotPath
			}
		}
		m.session.working = false
		m.err = nil
		m.retry = nil
		return m, m.advanceSession()

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		return m.handleErrorKey(msg)
	}
	if m.filtering {
		return m.handleFilterKey(msg)
	}
	if m.confirmMsg != "" {
		switch {
		case key.Matches(msg, keys.Confirm):
			paths := m.pendingPaths
			op := m.pendingOp
			if op == opUndo {
				entry := m.pendingUndo
				m.confirmMsg = ""
				m.pendingPaths = nil
				m.loading = true
				return m, m.undoRestore(entry)
			}
			if op == opAdd {
				m.confirmMsg = ""
				m.pendingPaths = nil
				m.loading = true
				return m, addCmd(m.cli, paths)
			}
			undo := m.buildUndo(op, paths)
			m.confirmMsg = ""
			m.pendingPaths = nil
			m.loading = true
			if op == opApply {
				return m, applyCmdUndo(m.cli, paths, undo)
			}
			return m, reAddCmdUndo(m.cli, paths, undo)
		case key.Matches(msg, keys.Cancel):
			m.confirmMsg = ""
			m.pendingPaths = nil
			m.pendingOp = opReAdd
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
		return m, m.switchTab(tabID((int(m.activeTab) + 1) % tabCount))
	case key.Matches(msg, keys.PrevTab):
		return m, m.switchTab(tabID((int(m.activeTab) + tabCount - 1) % tabCount))
	case key.Matches(msg, keys.OnlyMod):
		if m.activeTab == tabModified {
			return m, m.switchTab(tabAll)
		}
		return m, m.switchTab(tabModified)
	case key.Matches(msg, keys.Help):
		if m.activeTab == tabHelp {
			return m, m.switchTab(m.previousTab)
		}
		m.previousTab = m.activeTab
		return m, m.switchTab(tabHelp)
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

	if isInspectTab(m.activeTab) {
		return m.updateInspect(msg)
	}

	switch m.state {
	case viewSideBySide:
		return m.updateSide(msg)
	case viewList:
		return m.updateList(msg)
	}
	return m, nil
}

func isInspectTab(t tabID) bool {
	return t == tabUnmanaged || t == tabIgnored || t == tabDoctor
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
		if m.retry != nil {
			cmd := m.retry
			m.retry = nil
			return m, cmd
		}
		return m, loadEntriesCmd(m.cli)
	case key.Matches(msg, keys.View):
		if r, ok := m.cursorRow(); ok {
			if r.isDir {
				m.status = fmt.Sprintf("directory — open a file inside %q to view its diff", r.target)
				return m, nil
			}
			m.status = "loading…"
			return m, loadSideCmd(m.cli, m.cli, r.absolute, r.target, m.readFile)
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
		m.pendingOp = opReAdd
		m.confirmMsg = fmt.Sprintf("Re-add %d file(s)?", len(paths))
	case key.Matches(msg, keys.Apply):
		paths := m.actionPaths()
		if len(paths) == 0 {
			m.status = "nothing to apply"
			return m, nil
		}
		m.pendingPaths = paths
		m.pendingOp = opApply
		m.confirmMsg = fmt.Sprintf("Apply %d file(s) (overwrite live)?", len(paths))
	case key.Matches(msg, keys.Search):
		return m.beginFilter()
	case key.Matches(msg, keys.Undo):
		return m.beginUndo()
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
		m.pendingOp = opReAdd
		m.confirmMsg = fmt.Sprintf("Re-add %s?", m.sideDisplay)
		return m, nil
	case key.Matches(msg, keys.Apply):
		m.pendingPaths = []string{m.sideAbs}
		m.pendingOp = opApply
		m.confirmMsg = fmt.Sprintf("Apply %s (overwrite live)?", m.sideDisplay)
		return m, nil
	case key.Matches(msg, keys.Undo):
		return m.beginUndo()
	case key.Matches(msg, keys.SideMode):
		m.sideUnified = !m.sideUnified
		m.vp.SetContent(m.renderDiffBody())
		m.vp.GotoTop()
		return m, nil
	case key.Matches(msg, keys.Wrap):
		m.sideWrap = !m.sideWrap
		m.sideHScroll = 0
		m.vp.SetContent(m.renderDiffBody())
		m.vp.GotoTop()
		return m, nil
	case key.Matches(msg, keys.ScrollLeft):
		m.scrollHorizontal(-m.hScrollStep())
		return m, nil
	case key.Matches(msg, keys.ScrollRight):
		m.scrollHorizontal(m.hScrollStep())
		return m, nil
	case key.Matches(msg, keys.NextHunk):
		m.jumpHunk(1, m.hunks)
		return m, nil
	case key.Matches(msg, keys.PrevHunk):
		m.jumpHunk(-1, m.hunks)
		return m, nil
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// hScrollStep is the number of columns moved per horizontal scroll keypress.
func (m Model) hScrollStep() int { return 4 }

// scrollHorizontal shifts the diff horizontally and re-renders. Wrapping and horizontal scroll are mutually exclusive.
func (m *Model) scrollHorizontal(delta int) {
	if m.sideUnified || m.sideWrap {
		return
	}
	m.sideHScroll += delta
	if m.sideHScroll < 0 {
		m.sideHScroll = 0
	}
	m.vp.SetContent(m.renderDiffBody())
}

// jumpHunk moves the viewport to the next or previous hunk header, wrapping around. A no-op when there are no hunks.
func (m *Model) jumpHunk(dir int, offsets []int) {
	if len(offsets) == 0 {
		return
	}
	switch {
	case m.hunkCursor < 0 && dir < 0:
		m.hunkCursor = len(offsets) - 1
	case m.hunkCursor < 0:
		m.hunkCursor = 0
	default:
		m.hunkCursor = (m.hunkCursor + dir + len(offsets)) % len(offsets)
	}
	m.vp.SetYOffset(offsets[m.hunkCursor])
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
		for _, t := range tabs {
			if zone.Get(tabZoneID(t)).InBounds(msg) {
				if t == tabHelp && m.activeTab != tabHelp {
					m.previousTab = m.activeTab
				}
				return m, m.switchTab(t)
			}
		}
		if m.activeTab != tabHelp && m.state == viewList && !isInspectTab(m.activeTab) {
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
	if isInspectTab(m.activeTab) {
		return m.scrollInspect(delta)
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
		m.session.snapshotDir = defaultSnapshotDir(m.cacheDir)
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
	return renderPanels(e.target_contents, e.live_contents, bw, viewOpts{wrap: m.sideWrap, hOffset: m.sideHScroll})
}

func (m Model) handleSessionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.session.state {
	case sessionWelcome:
		switch {
		case key.Matches(msg, keys.Confirm):
			if len(m.session.queue) == 0 {
				return m, startSessionCmd(m.cli, m.cli, m.rows, m.session.snapshotDir)
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
			return m, revertCmd(m.cli, m.session.cursor, e.absolute, m.session.snapshotDir, m.readFile, m.now)
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

// switchTab changes the active tab and returns a command to fetch the tab's data on first visit.
func (m *Model) switchTab(t tabID) tea.Cmd {
	m.activeTab = t
	m.recomputeVisible()
	if m.cursor >= len(m.visibleIdxs) {
		m.cursor = max(0, len(m.visibleIdxs)-1)
	}
	return m.ensureTabLoaded(t)
}

func (m *Model) recomputeVisible() {
	m.visibleIdxs = m.visibleIdxs[:0]
	needle := strings.ToLower(strings.TrimSpace(m.filter))
	for i, r := range m.rows {
		if m.onlyMod() && !r.modified() {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(r.target), needle) {
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

func (m Model) alignTabsAndChip(tabRow, chip string) string {
	bw, _ := m.bodyDims()
	lw := lipgloss.Width(tabRow)
	space := bw - lw - 1
	if space <= 0 {
		return tabRow
	}
	// Truncate the chip to whatever room is left so it stays visible even when many tabs are shown.
	if lipgloss.Width(chip) > space {
		chip = ansi.Truncate(chip, space, "…")
		return tabRow + " " + chip
	}
	gap := bw - lw - lipgloss.Width(chip)
	return tabRow + strings.Repeat(" ", gap) + chip
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
	for _, t := range tabs {
		label := t.name()
		switch t {
		case tabModified:
			if n := m.categoryCounts()[catModified]; n > 0 {
				label = fmt.Sprintf("%s (%d)", label, n)
			}
		case tabUnmanaged:
			if n := len(m.unmanaged); n > 0 {
				label = fmt.Sprintf("%s (%d)", label, n)
			}
		case tabIgnored:
			if n := len(m.ignored); n > 0 {
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
	case m.err != nil:
		return m.viewError()
	case m.session.state == sessionWelcome:
		return m.viewSessionWelcome()
	case m.session.state == sessionReview:
		return m.viewSessionReview()
	case m.session.state == sessionSummary:
		return m.viewSessionSummary()
	case m.activeTab == tabHelp:
		return m.viewHelpPage()
	case isInspectTab(m.activeTab):
		return m.viewInspect()
	case m.loading:
		return mutedStyle.Render("loading…")
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
	if m.filter != "" {
		title += "  " + filterChipStyle.Render("filter: "+m.filter)
	}

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")

	_, bh := m.bodyDims()
	listHeight := bh - 4
	if listHeight < 5 {
		listHeight = 5
	}
	if m.filtering {
		listHeight--
	}

	if len(m.visibleIdxs) == 0 {
		b.WriteString(mutedStyle.Render("  (no matches)"))
	} else {
		b.WriteString(m.renderListWindow(listHeight))
	}

	b.WriteString("\n")
	if m.filtering {
		b.WriteString(m.filterInput.View())
		b.WriteString("\n")
		return b.String()
	}
	if m.status != "" {
		b.WriteString(mutedStyle.Render(m.status))
	} else if r, ok := m.cursorRow(); ok {
		b.WriteString(mutedStyle.Render(r.absolute))
		if d := attrDetail(r.attrs); d != "" {
			b.WriteString("  " + attrStyle.Render(d))
		}
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

	attrsPart := ""
	if len(r.attrs) > 0 {
		attrsPart = " " + withCursor(attrStyle).Render(attrChips(r.attrs))
	}

	line := plainBg.Render("  ") + markPart + plainBg.Render(" ") +
		glyphPart + plainBg.Render("  ") + targetPart + attrsPart

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

// attrChips renders a compact set of attribute glyphs, e.g. "tx".
func attrChips(attrs []chezmoi.Attribute) string {
	var b strings.Builder
	for _, a := range attrs {
		b.WriteString(a.Short())
	}
	return b.String()
}

// attrDetail renders a human-readable attribute list for the cursor detail line.
func attrDetail(attrs []chezmoi.Attribute) string {
	if len(attrs) == 0 {
		return ""
	}
	names := make([]string, len(attrs))
	for i, a := range attrs {
		names[i] = string(a)
	}
	return strings.Join(names, ", ")
}

func (m Model) contextHelp() string {
	if m.filtering {
		return mutedStyle.Render("type to filter · enter keep · esc clear")
	}
	switch m.activeTab {
	case tabUnmanaged:
		return mutedStyle.Render("space select · A add to source · R refresh · tab switch · q quit")
	case tabIgnored:
		return mutedStyle.Render("↑/↓ scroll · R refresh · tab switch · q quit")
	case tabDoctor:
		return mutedStyle.Render("↑/↓ scroll · R re-run · tab switch · q quit")
	}
	switch m.state {
	case viewSideBySide:
		if m.sideUnified {
			return mutedStyle.Render("↑/↓ scroll · [/] hunk · U side-by-side · r re-add · a apply · esc back")
		}
		if m.sideWrap {
			return mutedStyle.Render("↑/↓ scroll · w no-wrap · U unified · r re-add · a apply · esc back")
		}
		return mutedStyle.Render("↑/↓ scroll · ⇧←/⇧→ pan · w wrap · U unified · r re-add · esc back")
	default:
		if m.filter != "" {
			return mutedStyle.Render("enter view · / edit filter · esc clear filter · r re-add · a apply · ? help · q quit")
		}
		return mutedStyle.Render("enter view · / filter · r re-add · a apply · u undo · ? help · q quit")
	}
}

func (m Model) renderSidePanels() string {
	bw, _ := m.bodyDims()
	return renderPanels(m.sideTarget, m.sideLive, bw, viewOpts{wrap: m.sideWrap, hOffset: m.sideHScroll})
}

// renderDiffBody renders whichever diff mode is active.
func (m Model) renderDiffBody() string {
	if m.sideUnified {
		bw, _ := m.bodyDims()
		return renderUnified(parseUnified(m.unifiedDiff), bw)
	}
	return m.renderSidePanels()
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
	if m.sideUnified {
		headerLines[len(headerLines)-1] = mutedStyle.Render(fmt.Sprintf("unified diff · %d hunk(s)", len(m.hunks)))
	}
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
	var b strings.Builder
	b.WriteString(titleStyle.Render("Keybindings") + "\n\n")
	for _, g := range helpGroups() {
		b.WriteString(sectionHeaderStyle.Render(g.title) + "\n")
		for _, r := range g.rows {
			b.WriteString(fmt.Sprintf("%-22s %s\n", r[0], mutedStyle.Render(r[1])))
		}
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
