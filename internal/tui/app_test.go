package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/balintb/chezmoui/internal/chezmoi"
	"github.com/balintb/chezmoui/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type stubBackend struct {
	managed []chezmoi.Entry
	status  []chezmoi.Status
	cat     string
	catErr  error

	sourcePath string
	gitStatus  chezmoi.GitStatus

	catCalls   []string
	reAddCalls [][]string
	applyCalls [][]string
	reAddErr   error
	applyErr   error
}

func (s *stubBackend) Managed(context.Context) ([]chezmoi.Entry, error) {
	return s.managed, nil
}
func (s *stubBackend) Status(context.Context) ([]chezmoi.Status, error) {
	return s.status, nil
}
func (s *stubBackend) Cat(_ context.Context, path string) (string, error) {
	s.catCalls = append(s.catCalls, path)
	return s.cat, s.catErr
}
func (s *stubBackend) ReAdd(_ context.Context, paths ...string) error {
	cp := make([]string, len(paths))
	copy(cp, paths)
	s.reAddCalls = append(s.reAddCalls, cp)
	return s.reAddErr
}
func (s *stubBackend) Apply(_ context.Context, paths ...string) error {
	cp := make([]string, len(paths))
	copy(cp, paths)
	s.applyCalls = append(s.applyCalls, cp)
	return s.applyErr
}
func (s *stubBackend) SourcePath(context.Context) (string, error) {
	return s.sourcePath, nil
}
func (s *stubBackend) GitStatus(context.Context) (chezmoi.GitStatus, error) {
	return s.gitStatus, nil
}

func sampleBackend() *stubBackend {
	return &stubBackend{
		managed: []chezmoi.Entry{
			{Target: ".bashrc", Absolute: "/home/u/.bashrc", SourceRelative: "dot_bashrc"},
			{Target: ".config/btop/btop.conf", Absolute: "/home/u/.config/btop/btop.conf", SourceRelative: "dot_config/btop/btop.conf"},
			{Target: ".vimrc", Absolute: "/home/u/.vimrc", SourceRelative: "dot_vimrc"},
		},
		status: []chezmoi.Status{
			{Source: ' ', Target: 'M', Path: ".config/btop/btop.conf"},
			{Source: ' ', Target: 'A', Path: ".bashrc"},
		},
	}
}

func applyMsg(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	mm, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned non-Model: %T", next)
	}
	return mm, cmd
}

func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected tea.Cmd, got nil")
	}
	return cmd()
}

func loadedModel(t *testing.T, b Backend) Model {
	t.Helper()
	m := NewModel(b).WithConfigStore(testConfigStore(t, true))
	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, msg := range drainBatch(m.Init()) {
		m, _ = applyMsg(t, m, msg)
	}
	return m
}

func testConfigStore(t *testing.T, confirmed bool) *config.Store {
	t.Helper()
	s := &config.Store{Path: filepath.Join(t.TempDir(), "config.json")}
	if confirmed {
		if err := s.Save(config.Config{RepoConfirmed: true}); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func drainBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	out := cmd()
	if batch, ok := out.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			msgs = append(msgs, drainBatch(c)...)
		}
		return msgs
	}
	return []tea.Msg{out}
}

func driveToQuiescence(t *testing.T, m Model) Model {
	t.Helper()
	queue := []tea.Cmd{m.Init()}
	const maxSteps = 50 // safety net against accidental cmd loops
	steps := 0
	for len(queue) > 0 {
		steps++
		if steps > maxSteps {
			t.Fatalf("driveToQuiescence: cmd queue did not drain in %d steps", maxSteps)
		}
		cmd := queue[0]
		queue = queue[1:]
		if cmd == nil {
			continue
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		var next tea.Cmd
		m, next = applyMsg(t, m, msg)
		if next != nil {
			queue = append(queue, next)
		}
	}
	return m
}

func cursorTo(t *testing.T, m Model, target string) Model {
	t.Helper()
	for i, idx := range m.visibleIdxs {
		if m.rows[idx].target == target {
			m.cursor = i
			return m
		}
	}
	t.Fatalf("target %q not visible", target)
	return m
}

func TestModel_LoadEntries_PopulatesRows(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	if len(m.rows) != 3 {
		t.Fatalf("rows: want 3, got %d", len(m.rows))
	}
	for _, r := range m.rows {
		if r.target == ".config/btop/btop.conf" && !r.modified() {
			t.Errorf("btop.conf should be modified, got status=%c srcDrift=%c", r.status, r.srcDrift)
		}
	}
}

func TestSortRows_GroupsByCategory(t *testing.T) {
	rows := []row{
		{target: "z-clean"},
		{target: "a-modified", status: 'M'},
		{target: "b-added", status: 'A'},
		{target: "a-clean"},
		{target: "a-added", status: 'A'},
	}
	sortRows(rows)
	got := make([]string, len(rows))
	for i, r := range rows {
		got[i] = r.target
	}
	want := []string{"a-modified", "a-added", "b-added", "a-clean", "z-clean"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: want %q, got %q (full order %v)", i, want[i], got[i], got)
		}
	}
}

func TestSortRows_StableWithinCategory(t *testing.T) {
	rows := []row{
		{target: "z", status: 'M'},
		{target: "a", status: 'M'},
		{target: "m", status: 'M'},
	}
	sortRows(rows)
	for i, want := range []string{"a", "m", "z"} {
		if rows[i].target != want {
			t.Errorf("Modified order broken at %d: got %v", i, rows)
			break
		}
	}
}

func TestModel_OnlyModFilter(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if m.activeTab != tabModified {
		t.Fatalf("m should jump to Modified tab, got %v", m.activeTab)
	}
	if !m.onlyMod() {
		t.Fatal("onlyMod() must be true on Modified tab")
	}
	if len(m.visibleIdxs) != 1 {
		t.Errorf("visible after filter: want 1, got %d", len(m.visibleIdxs))
	}
	if m.rows[m.visibleIdxs[0]].target != ".config/btop/btop.conf" {
		t.Errorf("filtered row: %q", m.rows[m.visibleIdxs[0]].target)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if m.activeTab != tabAll {
		t.Errorf("second m press should return to All, got %v", m.activeTab)
	}
}

func TestModel_ReAddNothingWhenNothingModified(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m = cursorTo(t, m, ".bashrc") // catAdded, not Modified
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.confirmMsg != "" {
		t.Errorf("did not expect confirm prompt, got %q", m.confirmMsg)
	}
	if !strings.Contains(m.status, "nothing") {
		t.Errorf("status: want 'nothing to re-add', got %q", m.status)
	}
}

func TestModel_ReAddUsesAbsolutePath_Regression(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}) // filter to modified
	m = cursorTo(t, m, ".config/btop/btop.conf")
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.confirmMsg == "" {
		t.Fatal("expected confirm prompt")
	}
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	msg := runCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("re-add returned error: %v", e.err)
	}
	if len(b.reAddCalls) != 1 || len(b.reAddCalls[0]) != 1 ||
		b.reAddCalls[0][0] != "/home/u/.config/btop/btop.conf" {
		t.Errorf("re-add args: want absolute path, got %v", b.reAddCalls)
	}
}

func TestModel_ReAdd_BulkSelection(t *testing.T) {
	b := sampleBackend()
	b.status = []chezmoi.Status{
		{Source: ' ', Target: 'M', Path: ".bashrc"},
		{Source: ' ', Target: 'M', Path: ".config/btop/btop.conf"},
	}
	m := loadedModel(t, b)
	m = cursorTo(t, m, ".bashrc")
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = cursorTo(t, m, ".config/btop/btop.conf")
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.confirmMsg == "" || !strings.Contains(m.confirmMsg, "2 file") {
		t.Fatalf("expected confirm for 2 files, got %q", m.confirmMsg)
	}
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	runCmd(t, cmd)
	if len(b.reAddCalls) != 1 || len(b.reAddCalls[0]) != 2 {
		t.Fatalf("re-add: want 1 call with 2 paths, got %#v", b.reAddCalls)
	}
}

func TestModel_ReAdd_CancelDoesNotCall(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.confirmMsg != "" {
		t.Errorf("confirm prompt should be cleared, got %q", m.confirmMsg)
	}
	if len(b.reAddCalls) != 0 {
		t.Errorf("cancel must not call ReAdd, got %v", b.reAddCalls)
	}
}

func TestModel_BackendErrorSurfaces(t *testing.T) {
	b := sampleBackend()
	b.cat = ""
	b.catErr = context.DeadlineExceeded
	m := loadedModel(t, b)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	msg := runCmd(t, cmd)
	m, _ = applyMsg(t, m, msg)
	if m.err == nil {
		t.Fatal("expected error to be surfaced on Model.err")
	}
}

func TestModel_SideBySide_OpensFromList(t *testing.T) {
	b := sampleBackend()
	b.cat = "target line\n"
	m := loadedModel(t, b).WithReadFile(func(string) ([]byte, error) { return []byte("live line\n"), nil })
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := runCmd(t, cmd)
	m, _ = applyMsg(t, m, msg)
	if m.state != viewSideBySide {
		t.Fatalf("expected viewSideBySide, got %v", m.state)
	}
	if m.sideTarget != "target line\n" || m.sideLive != "live line\n" {
		t.Errorf("panes wrong: target=%q live=%q", m.sideTarget, m.sideLive)
	}
}

func TestModel_SideBySide_UsesAbsolutePath_Regression(t *testing.T) {
	b := sampleBackend()
	b.cat = "T\n"
	m := loadedModel(t, b).WithReadFile(func(string) ([]byte, error) { return []byte("L\n"), nil })
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	runCmd(t, cmd)
	if len(b.catCalls) != 1 || b.catCalls[0] != "/home/u/.config/btop/btop.conf" {
		t.Errorf("Cat must be called with absolute path, got %v", b.catCalls)
	}
}

func TestModel_SideBySide_LiveMissing_ENOENT(t *testing.T) {
	b := sampleBackend()
	b.cat = "target line\n"
	m := loadedModel(t, b).WithReadFile(func(string) ([]byte, error) {
		return nil, &os.PathError{Op: "open", Path: "x", Err: os.ErrNotExist}
	})
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := runCmd(t, cmd)
	m, _ = applyMsg(t, m, msg)
	if m.state != viewSideBySide {
		t.Fatalf("ENOENT must not abort load; state = %v, err = %v", m.state, m.err)
	}
	if !m.sideLiveMissing {
		t.Error("sideLiveMissing should be true when live file is absent")
	}
	if m.sideLive != "" {
		t.Errorf("sideLive should be empty for missing file, got %q", m.sideLive)
	}
	if !strings.Contains(m.View(), "missing") {
		t.Error("header should annotate 'missing' when live side is absent")
	}
}

func TestModel_SideBySide_LiveReadOtherErrorBubbles(t *testing.T) {
	b := sampleBackend()
	b.cat = "target\n"
	m := loadedModel(t, b).WithReadFile(func(string) ([]byte, error) {
		return nil, &os.PathError{Op: "open", Path: "x", Err: os.ErrPermission}
	})
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := runCmd(t, cmd)
	m, _ = applyMsg(t, m, msg)
	if m.err == nil {
		t.Fatal("permission error must bubble up, not be swallowed as 'missing'")
	}
}

func TestModel_SideBySide_BackKeyReturnsToList(t *testing.T) {
	b := sampleBackend()
	b.cat = "x\n"
	m := loadedModel(t, b).WithReadFile(func(string) ([]byte, error) { return []byte("y\n"), nil })
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := runCmd(t, cmd)
	m, _ = applyMsg(t, m, msg)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != viewList {
		t.Errorf("expected viewList after esc, got %v", m.state)
	}
}

func TestModel_SideBySide_LogicallyIdenticalBanner(t *testing.T) {
	b := sampleBackend()
	b.cat = "theme_background = True\nvim_keys = False\n"
	m := loadedModel(t, b).WithReadFile(func(string) ([]byte, error) {
		return []byte("theme_background = true\nvim_keys = false\n"), nil
	})
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := runCmd(t, cmd)
	m, _ = applyMsg(t, m, msg)

	if m.sideAdded != 0 || m.sideRemoved != 0 {
		t.Errorf("logically-identical file should report 0 real changes, got +%d −%d",
			m.sideAdded, m.sideRemoved)
	}
	if m.sideLoose < 1 {
		t.Errorf("expected ≥1 loose-match rows, got %d", m.sideLoose)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "logically identical") {
		t.Errorf("header should show 'logically identical' banner:\n%s", view)
	}
}

func TestModel_SideBySide_HeaderShowsSummary(t *testing.T) {
	b := sampleBackend()
	b.cat = "a\nb\nc\n"
	m := loadedModel(t, b).WithReadFile(func(string) ([]byte, error) {
		return []byte("a\nB\nc\nd\n"), nil
	})
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := runCmd(t, cmd)
	m, _ = applyMsg(t, m, msg)
	view := stripANSI(m.View())
	if !strings.Contains(view, "+2") || !strings.Contains(view, "−1") {
		t.Errorf("header should contain +2/−1 summary, got:\n%s", view)
	}
}

func TestModel_HelpToggle(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	if m.activeTab != tabAll {
		t.Fatalf("starting tab should be All, got %v", m.activeTab)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if m.activeTab != tabHelp {
		t.Fatal("? should switch to Help tab")
	}
	if !strings.Contains(stripANSI(m.View()), "Keybindings") {
		t.Error("Help tab should render the Keybindings page")
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if m.activeTab != tabAll {
		t.Errorf("? a second time should return to previous tab, got %v", m.activeTab)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.activeTab != tabAll {
		t.Errorf("esc on Help tab should return to previous tab, got %v", m.activeTab)
	}
}

func TestModel_HelpTabRemembersOrigin(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if m.activeTab != tabModified {
		t.Fatal("setup: should be on Modified tab")
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if m.activeTab != tabHelp {
		t.Fatal("? should jump to Help")
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.activeTab != tabModified {
		t.Errorf("esc from Help should return to Modified, got %v", m.activeTab)
	}
}

func TestModel_HelpOverlayBlocksOtherKeys(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.confirmMsg != "" {
		t.Error("r should not trigger re-add while help is open")
	}
}

func TestModel_ConfirmModalRendersCenteredBox(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.confirmMsg == "" {
		t.Fatal("expected confirm prompt")
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Re-add") {
		t.Errorf("modal should contain 'Re-add', got:\n%s", view)
	}
	if !strings.ContainsAny(view, "╭╮╰╯─│") {
		t.Errorf("modal should be bordered, got:\n%s", view)
	}
}

func TestModel_ListShowsCategoryChips(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	view := stripANSI(m.View())
	for _, want := range []string{"modified", "add", "clean"} {
		if !strings.Contains(view, want) {
			t.Errorf("title chips missing %q in:\n%s", want, view)
		}
	}
}

func TestModel_DirectoryEnter_DoesNotFireCat_Regression(t *testing.T) {
	b := sampleBackend()
	for i := range b.managed {
		if b.managed[i].Target == ".config/btop/btop.conf" {
			b.managed[i].Target = ".config/btop"
		}
	}
	b.status = []chezmoi.Status{{Source: ' ', Target: 'M', Path: ".config/btop"}}
	m := NewModel(b).WithConfigStore(testConfigStore(t, true))
	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, msg := range drainBatch(m.Init()) {
		m, _ = applyMsg(t, m, msg)
	}

	for i := range m.rows {
		if m.rows[i].target == ".config/btop" {
			m.rows[i].isDir = true
		}
	}
	m = cursorTo(t, m, ".config/btop")

	m, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		out := cmd()
		if _, isErr := out.(errMsg); isErr {
			t.Fatal("enter on a directory must not produce an error message")
		}
		if _, isSide := out.(sideLoadedMsg); isSide {
			t.Fatal("enter on a directory must not load side-by-side")
		}
	}
	if len(b.catCalls) != 0 {
		t.Errorf("Cat must NOT be called on a directory row, got %v", b.catCalls)
	}
	if !strings.Contains(m.status, "directory") {
		t.Errorf("status should explain why nothing happened, got %q", m.status)
	}
}

func TestFormatRow_CursorHighlight_SpansFullRow_Regression(t *testing.T) {
	r := row{target: ".bashrc", status: 'M'}
	const width = 40
	cursor := formatRow(r, false, true, width)
	plain := formatRow(r, false, false, width)

	if got := lipgloss.Width(cursor); got != width {
		t.Errorf("cursor row visible width: want %d, got %d (line=%q)", width, got, stripANSI(cursor))
	}

	if !hasBackgroundFill(cursor) {
		t.Errorf("cursor row should have BG fill: %q", cursor)
	}
	if hasBackgroundFill(plain) {
		t.Errorf("non-cursor row should NOT have cursor BG fill: %q", plain)
	}

	bgCount := strings.Count(cursor, "\x1b[48;")
	if bgCount < 2 {
		t.Errorf("cursor row should re-apply BG at least twice (badge + target); got %d:\n%q", bgCount, cursor)
	}

	if !strings.Contains(stripANSI(cursor), ".bashrc") {
		t.Errorf("cursor row missing target text: %q", stripANSI(cursor))
	}
}

func TestModel_DirectoryRowShowsTrailingSlash(t *testing.T) {
	b := sampleBackend()
	m := NewModel(b).WithConfigStore(testConfigStore(t, true))
	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, msg := range drainBatch(m.Init()) {
		m, _ = applyMsg(t, m, msg)
	}
	for i := range m.rows {
		if m.rows[i].target == ".config/btop/btop.conf" {
			m.rows[i].isDir = true
		}
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, ".config/btop/btop.conf/") {
		t.Errorf("directory row should render with trailing slash; view:\n%s", view)
	}
}

func TestIsDirReadError_Detection(t *testing.T) {
	_, err := os.ReadFile(t.TempDir())
	if err == nil {
		t.Fatal("expected EISDIR-like error reading a directory")
	}
	if !isDirReadError(err) {
		t.Errorf("isDirReadError should detect %q", err.Error())
	}
	if isDirReadError(nil) {
		t.Error("nil error should not be detected as 'is a directory'")
	}
}

func TestModel_TabKey_CyclesForward(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	if m.activeTab != tabAll {
		t.Fatalf("setup: should start on All, got %v", m.activeTab)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.activeTab != tabModified {
		t.Errorf("tab from All: want Modified, got %v", m.activeTab)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.activeTab != tabHelp {
		t.Errorf("tab from Modified: want Help, got %v", m.activeTab)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.activeTab != tabAll {
		t.Errorf("tab from Help should wrap to All, got %v", m.activeTab)
	}
}

func TestModel_TabKey_CyclesBackward(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.activeTab != tabHelp {
		t.Errorf("shift+tab from All should wrap to Help, got %v", m.activeTab)
	}
}

func TestModel_TabSwitch_ResyncsCursor(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m = cursorTo(t, m, ".vimrc") // last visible row, clean
	prev := m.cursor
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if m.cursor >= len(m.visibleIdxs) {
		t.Errorf("cursor must be clamped after tab switch (was %d, visible=%d)", prev, len(m.visibleIdxs))
	}
}

func TestModel_View_RendersWindowChrome(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	view := stripANSI(m.View())
	if !strings.ContainsAny(view, "╭╮╰╯") {
		t.Errorf("View should be wrapped in a rounded border:\n%s", view)
	}
	for _, want := range []string{"All", "Modified", "Help"} {
		if !strings.Contains(view, want) {
			t.Errorf("tab label %q missing:\n%s", want, view)
		}
	}
}

func TestModel_MouseWheel_ScrollsList(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m, _ = applyMsg(t, m, tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
	})
	if m.cursor != 1 {
		t.Errorf("wheel down: cursor want 1, got %d", m.cursor)
	}
	m, _ = applyMsg(t, m, tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelUp,
	})
	if m.cursor != 0 {
		t.Errorf("wheel up: cursor want 0, got %d", m.cursor)
	}
}

func TestModel_MouseWheel_DoesNotUnderflow(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	for i := 0; i < 5; i++ {
		m, _ = applyMsg(t, m, tea.MouseMsg{
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelUp,
		})
	}
	if m.cursor != 0 {
		t.Errorf("wheel up underflow: want 0, got %d", m.cursor)
	}
}

func sessionFixture(t *testing.T) (Model, *stubBackend) {
	t.Helper()
	b := sampleBackend()
	b.status = []chezmoi.Status{
		{Source: ' ', Target: 'M', Path: ".bashrc"},
		{Source: ' ', Target: 'M', Path: ".config/btop/btop.conf"},
	}
	b.cat = "target line\n"
	b.sourcePath = "/tmp/dotfiles"
	b.gitStatus = chezmoi.GitStatus{Branch: "main", Staged: 2, Unstaged: 1}
	tmp := t.TempDir()
	m := NewModel(b).
		WithReadFile(func(string) ([]byte, error) { return []byte("live line\n"), nil }).
		WithConfigStore(testConfigStore(t, true))
	m.session.snapshotDir = tmp
	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	for _, msg := range drainBatch(m.Init()) {
		m, _ = applyMsg(t, m, msg)
	}
	return m, b
}

func TestSession_StartShowsWelcome(t *testing.T) {
	m, _ := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	if m.session.state != sessionWelcome {
		t.Fatalf("expected sessionWelcome, got %v", m.session.state)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Sync Session") {
		t.Errorf("welcome screen should contain 'Sync Session':\n%s", view)
	}
	if !strings.Contains(view, "2 files") {
		t.Errorf("welcome screen should mention 2 modified files:\n%s", view)
	}
}

func TestSession_EnterFromWelcomeBuildsQueue(t *testing.T) {
	m, b := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := runCmd(t, cmd)
	startedMsg, ok := msg.(sessionStartedMsg)
	if !ok {
		t.Fatalf("expected sessionStartedMsg, got %T", msg)
	}
	if len(startedMsg.queue) != 2 {
		t.Errorf("queue should have 2 entries, got %d", len(startedMsg.queue))
	}
	if startedMsg.sourceRepoPath != b.sourcePath {
		t.Errorf("sourceRepoPath: want %q, got %q", b.sourcePath, startedMsg.sourceRepoPath)
	}
	if startedMsg.gitBranch != "main" {
		t.Errorf("gitBranch: want main, got %q", startedMsg.gitBranch)
	}
	m, _ = applyMsg(t, m, msg)
	if m.session.state != sessionReview {
		t.Errorf("expected sessionReview after queue built, got %v", m.session.state)
	}
}

func TestSession_KeepCallsReAdd_Advances(t *testing.T) {
	m, b := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	startCursor := m.session.cursor
	_, cmd = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if cmd == nil {
		t.Fatal("expected keep cmd")
	}
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	if len(b.reAddCalls) != 1 {
		t.Fatalf("ReAdd should be called once, got %v", b.reAddCalls)
	}
	if m.session.queue[startCursor].decision != decisionKept {
		t.Errorf("decision should be Kept, got %v", m.session.queue[startCursor].decision)
	}
	if m.session.cursor == startCursor && m.session.state == sessionReview {
		t.Error("cursor did not advance after keep")
	}
}

func TestSession_RevertCallsApply_AndSnapshots(t *testing.T) {
	m, b := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	startCursor := m.session.cursor
	_, cmd = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if cmd == nil {
		t.Fatal("expected revert cmd")
	}
	doneMsg := runCmd(t, cmd)
	if e, ok := doneMsg.(errMsg); ok {
		t.Fatalf("revert returned error: %v", e.err)
	}
	m, _ = applyMsg(t, m, doneMsg)
	if len(b.applyCalls) != 1 {
		t.Fatalf("Apply should be called once, got %v", b.applyCalls)
	}
	if m.session.queue[startCursor].decision != decisionReverted {
		t.Errorf("decision should be Reverted, got %v", m.session.queue[startCursor].decision)
	}
	snap := m.session.queue[startCursor].snapshotPath
	if snap == "" {
		t.Fatal("revert should record a snapshot path")
	}
	data, err := os.ReadFile(snap)
	if err != nil {
		t.Fatalf("snapshot file unreadable: %v", err)
	}
	if string(data) != "live line\n" {
		t.Errorf("snapshot content wrong: %q", data)
	}
}

func TestSession_SkipDoesNotInvokeBackend(t *testing.T) {
	m, b := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	startCursor := m.session.cursor
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if len(b.applyCalls) != 0 || len(b.reAddCalls) != 0 {
		t.Errorf("skip must not call backend; reAdd=%v apply=%v", b.reAddCalls, b.applyCalls)
	}
	if m.session.queue[startCursor].decision != decisionSkipped {
		t.Errorf("decision should be Skipped, got %v", m.session.queue[startCursor].decision)
	}
}

func TestSession_BackEntry_NavigatesPreviousFile(t *testing.T) {
	m, _ := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.session.cursor != 1 {
		t.Fatalf("setup: cursor should be 1 after skip, got %d", m.session.cursor)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.session.cursor != 0 {
		t.Errorf("← should rewind cursor to 0, got %d", m.session.cursor)
	}
}

func TestSession_AllDecided_TransitionsToSummary(t *testing.T) {
	m, _ := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.session.state != sessionSummary {
		t.Fatalf("expected sessionSummary after every entry skipped, got %v", m.session.state)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Session complete") {
		t.Errorf("summary should say 'Session complete':\n%s", view)
	}
	if !strings.Contains(view, "2 skipped") {
		t.Errorf("summary should report 2 skipped:\n%s", view)
	}
}

func TestSession_EscEndsEarly(t *testing.T) {
	m, b := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.session.state != sessionSummary {
		t.Errorf("esc should jump to summary, got %v", m.session.state)
	}
	if len(b.applyCalls) != 0 || len(b.reAddCalls) != 0 {
		t.Error("esc must not invoke backend")
	}
}

func TestSession_SummaryEnterReturnsToList(t *testing.T) {
	m, _ := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m, cmd = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected refresh cmd from summary enter")
	}
	msg := cmd()
	if _, ok := msg.(entriesLoadedMsg); !ok {
		t.Errorf("summary enter should refresh entries, got %T", msg)
	}
	m, _ = applyMsg(t, m, msg)
	if m.session.state != sessionInactive {
		t.Errorf("session should be inactive after summary enter, got %v", m.session.state)
	}
}

func TestSession_KeyDuringWorkingIsBlocked(t *testing.T) {
	m, b := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	m, cmd1 := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if cmd1 == nil {
		t.Fatal("first k should produce a cmd")
	}
	if !m.session.working {
		t.Error("session.working should be true while cmd in flight")
	}
	_, cmd2 := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if cmd2 != nil {
		t.Error("second k while working should be a no-op")
	}
	m, _ = applyMsg(t, m, runCmd(t, cmd1))
	if len(b.reAddCalls) != 1 {
		t.Errorf("ReAdd should be called exactly once, got %d", len(b.reAddCalls))
	}
}

func TestSanitizeForFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/home/u/.config/btop/btop.conf", "home_u_.config_btop_btop.conf"},
		{"plain.txt", "plain.txt"},
		{"with spaces.txt", "with_spaces.txt"},
		{"", "file"},
	}
	for _, c := range cases {
		if got := sanitizeForFilename(c.in); got != c.want {
			t.Errorf("sanitizeForFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestModel_FirstLaunch_ShowsRepoSetup(t *testing.T) {
	b := sampleBackend()
	b.sourcePath = "/tmp/nope" // discovery will produce no candidates here
	store := &config.Store{Path: filepath.Join(t.TempDir(), "config.json")}
	m := NewModel(b).WithConfigStore(store)
	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	for _, msg := range drainBatch(m.Init()) {
		m, _ = applyMsg(t, m, msg)
	}
	if m.state != viewRepoSetup {
		t.Fatalf("first launch should land in repo setup, got %v", m.state)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Locate your dotfiles repo") {
		t.Errorf("setup screen should contain 'Locate your dotfiles repo':\n%s", view)
	}
}

func TestModel_SubsequentLaunch_SkipsSetup(t *testing.T) {
	b := sampleBackend()
	store := &config.Store{Path: filepath.Join(t.TempDir(), "config.json")}
	if err := store.Save(config.Config{RepoPath: "/tmp/dotfiles", RepoConfirmed: true}); err != nil {
		t.Fatal(err)
	}
	m := NewModel(b).WithConfigStore(store)
	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	for _, msg := range drainBatch(m.Init()) {
		m, _ = applyMsg(t, m, msg)
	}
	if m.state == viewRepoSetup {
		t.Errorf("confirmed config must skip setup; got state=%v", m.state)
	}
	if m.repoPath != "/tmp/dotfiles" {
		t.Errorf("repoPath: want /tmp/dotfiles, got %q", m.repoPath)
	}
}

func TestModel_RepoSetup_PicksAndPersists(t *testing.T) {
	tmpRepo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpRepo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := sampleBackend()
	b.sourcePath = tmpRepo
	store := &config.Store{Path: filepath.Join(t.TempDir(), "config.json")}
	m := NewModel(b).WithConfigStore(store)
	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = driveToQuiescence(t, m)
	if len(m.repoSetup.candidates) == 0 {
		t.Fatal("expected at least one candidate from discovery")
	}
	m, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should fire a save cmd")
	}
	saved, ok := cmd().(configSavedMsg)
	if !ok {
		t.Fatalf("expected configSavedMsg, got %T", cmd())
	}
	if saved.err != nil {
		t.Fatalf("save errored: %v", saved.err)
	}
	if m.state != viewList {
		t.Errorf("after confirm, state should be viewList, got %v", m.state)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("config not persisted: %v", err)
	}
	if !loaded.RepoConfirmed {
		t.Error("RepoConfirmed should be true after picker confirm")
	}
	if loaded.RepoPath != m.repoPath {
		t.Errorf("persisted RepoPath %q != model repoPath %q", loaded.RepoPath, m.repoPath)
	}
}

func TestModel_RepoSetup_SkipPersistsEmptyButConfirmed(t *testing.T) {
	b := sampleBackend()
	b.sourcePath = "/tmp/nope"
	store := &config.Store{Path: filepath.Join(t.TempDir(), "config.json")}
	m := NewModel(b).WithConfigStore(store)
	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	for _, msg := range drainBatch(m.Init()) {
		m, _ = applyMsg(t, m, msg)
	}
	m, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("skip should fire save cmd")
	}
	cmd() // drain
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.RepoConfirmed {
		t.Error("skip must persist confirmed=true to avoid re-prompting")
	}
	if loaded.RepoPath != "" {
		t.Errorf("skip should leave RepoPath empty, got %q", loaded.RepoPath)
	}
}

func TestModel_RepoSetup_ManualEntry(t *testing.T) {
	tmpRepo := t.TempDir()
	b := sampleBackend()
	b.sourcePath = "/tmp/nope"
	store := &config.Store{Path: filepath.Join(t.TempDir(), "config.json")}
	m := NewModel(b).WithConfigStore(store)
	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	for _, msg := range drainBatch(m.Init()) {
		m, _ = applyMsg(t, m, msg)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if m.repoSetup.mode != setupManual {
		t.Fatalf("expected setupManual mode, got %v", m.repoSetup.mode)
	}
	m.repoSetup.manualIn.SetValue(tmpRepo)
	m, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should confirm and save")
	}
	cmd()
	if m.repoPath != tmpRepo {
		t.Errorf("repoPath after manual entry: want %q, got %q", tmpRepo, m.repoPath)
	}
}

func TestModel_RepoSetup_ManualEntryRejectsBadPath(t *testing.T) {
	b := sampleBackend()
	b.sourcePath = "/tmp/nope"
	store := &config.Store{Path: filepath.Join(t.TempDir(), "config.json")}
	m := NewModel(b).WithConfigStore(store)
	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	for _, msg := range drainBatch(m.Init()) {
		m, _ = applyMsg(t, m, msg)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m.repoSetup.manualIn.SetValue("/no/such/dir/anywhere")
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.repoSetup.manualErr == "" {
		t.Error("invalid path should produce an error message")
	}
	if m.state != viewRepoSetup {
		t.Errorf("invalid manual path must keep us on setup screen, got %v", m.state)
	}
}

func TestModel_RelocateRepo_ReentersSetup(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	if m.state == viewRepoSetup {
		t.Fatal("setup: should NOT start in repo setup with confirmed config")
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	if m.state != viewRepoSetup {
		t.Errorf("L from file list should re-enter setup, got %v", m.state)
	}
}

func TestModel_RepoChip_ShownWhenConfigured(t *testing.T) {
	b := sampleBackend()
	store := &config.Store{Path: filepath.Join(t.TempDir(), "config.json")}
	if err := store.Save(config.Config{RepoPath: "/tmp/dotfiles", RepoConfirmed: true}); err != nil {
		t.Fatal(err)
	}
	m := NewModel(b).WithConfigStore(store)
	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	for _, msg := range drainBatch(m.Init()) {
		m, _ = applyMsg(t, m, msg)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "/tmp/dotfiles") {
		t.Errorf("repo chip should be visible in chrome:\n%s", view)
	}
}

func TestModel_ListShowsSectionHeaders(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	view := stripANSI(m.View())
	for _, want := range []string{"Modified", "Will add", "Clean"} {
		if !strings.Contains(view, want) {
			t.Errorf("section header %q missing from list view:\n%s", want, view)
		}
	}
}
