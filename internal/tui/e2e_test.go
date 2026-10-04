//go:build e2e

package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/balintb/chezmoui/internal/chezmoi"
	"github.com/balintb/chezmoui/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func e2eFixture(t *testing.T) (Model, *chezmoi.Client, string) {
	t.Helper()
	home := t.TempDir()
	srcDir := filepath.Join(home, ".local", "share", "chezmoi")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "dot_testfile"), []byte("source content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".testfile"), []byte("live content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cli, err := chezmoi.New(chezmoi.WithExtraEnv("HOME=" + home))
	if err != nil {
		t.Fatalf("New client: %v", err)
	}
	store := &config.Store{Path: filepath.Join(t.TempDir(), "config.json")}
	_ = store.Save(config.Config{RepoConfirmed: true})

	m := NewModel(cli).WithConfigStore(store)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	return m, cli, home
}

func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	mm, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned non-Model: %T", next)
	}
	return mm, cmd
}

func drainCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		return msg
	case <-time.After(10 * time.Second):
		t.Fatal("tea.Cmd timed out (chezmoi exec stuck?)")
		return nil
	}
}

func TestE2E_DriftReconciler(t *testing.T) {
	m, cli, home := e2eFixture(t)

	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	var msg tea.Msg = nil
	_ = msg
	if len(m.rows) == 0 {
		t.Fatal("no rows loaded from real chezmoi")
	}

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if len(m.visibleIdxs) != 1 {
		t.Fatalf("after filter: want 1 visible row, got %d", len(m.visibleIdxs))
	}
	if m.rows[m.visibleIdxs[0]].target != ".testfile" {
		t.Fatalf("filtered row: %q", m.rows[m.visibleIdxs[0]].target)
	}

	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg = drainCmd(t, cmd)
	m, _ = step(t, m, msg)
	if m.state != viewSideBySide {
		t.Fatalf("expected viewSideBySide after enter, got %v", m.state)
	}
	if !strings.Contains(m.sideLive, "live content") {
		t.Errorf("live pane missing live content: %q", m.sideLive)
	}
	if !strings.Contains(m.sideTarget, "source content") {
		t.Errorf("target pane missing source content: %q", m.sideTarget)
	}

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.confirmMsg == "" {
		t.Fatal("expected confirm prompt after r")
	}

	m, cmd = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	msg = drainCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("re-add failed: %v", e.err)
	}
	m, _ = step(t, m, msg)

	srcContents, err := os.ReadFile(filepath.Join(home, ".local", "share", "chezmoi", "dot_testfile"))
	if err != nil {
		t.Fatal(err)
	}
	if string(srcContents) != "live content\n" {
		t.Errorf("source after e2e re-add = %q, want %q", srcContents, "live content\n")
	}

	statuses, err := cli.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range statuses {
		if s.Path == ".testfile" && s.Modified() {
			t.Errorf(".testfile still modified after e2e re-add: %+v", s)
		}
	}
}

func TestE2E_SideBySide_RealChezmoi(t *testing.T) {
	m, _, _ := e2eFixture(t)

	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	var msg tea.Msg = nil
	_ = msg
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if len(m.visibleIdxs) != 1 {
		t.Fatalf("expected single modified row, got %d", len(m.visibleIdxs))
	}

	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	msg = drainCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("side-by-side load failed: %v", e.err)
	}
	m, _ = step(t, m, msg)

	if m.state != viewSideBySide {
		t.Fatalf("expected viewSideBySide, got %v", m.state)
	}
	if !strings.Contains(m.sideTarget, "source content") {
		t.Errorf("target pane missing source content: %q", m.sideTarget)
	}
	if !strings.Contains(m.sideLive, "live content") {
		t.Errorf("live pane missing live content: %q", m.sideLive)
	}
	view := m.vp.View()
	if !strings.Contains(view, "source") || !strings.Contains(view, "live") {
		t.Errorf("rendered side-by-side missing one of the contents:\n%s", view)
	}
}

func TestE2E_SideBySide_LiveMissing(t *testing.T) {
	m, _, home := e2eFixture(t)

	if err := os.Remove(filepath.Join(home, ".testfile")); err != nil {
		t.Fatal(err)
	}

	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	var msg tea.Msg = nil
	_ = msg

	for i := range m.rows {
		if m.rows[i].target == ".testfile" {
			m.cursor = i
			m.recomputeVisible()
			m.cursor = i
			break
		}
	}

	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	msg = drainCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("missing live should not error, got %v", e.err)
	}
	m, _ = step(t, m, msg)

	if m.state != viewSideBySide {
		t.Fatalf("expected viewSideBySide, got %v", m.state)
	}
	if !m.sideLiveMissing {
		t.Error("sideLiveMissing should be true after live file removed")
	}
	if m.sideLive != "" {
		t.Errorf("sideLive should be empty, got %q", m.sideLive)
	}
	if !strings.Contains(m.View(), "missing") {
		t.Errorf("header should annotate 'missing':\n%s", m.View())
	}
}

func TestE2E_SyncSession_KeepThenRevert(t *testing.T) {
	m, _, home := e2eFixture(t)

	srcDir := filepath.Join(home, ".local", "share", "chezmoi")
	if err := os.WriteFile(filepath.Join(srcDir, "dot_other"), []byte("source other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	otherLive := filepath.Join(home, ".other")
	if err := os.WriteFile(otherLive, []byte("live other\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	snapshotDir := t.TempDir()
	m.session.snapshotDir = snapshotDir

	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	var msg tea.Msg = nil
	_ = msg

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	if m.session.state != sessionWelcome {
		t.Fatalf("expected sessionWelcome, got %v", m.session.state)
	}

	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg = drainCmd(t, cmd)
	m, _ = step(t, m, msg)
	if m.session.state != sessionReview {
		t.Fatalf("expected sessionReview, got %v", m.session.state)
	}
	if len(m.session.queue) != 2 {
		t.Fatalf("queue should have 2 entries, got %d", len(m.session.queue))
	}

	firstTarget := m.session.queue[0].absolute

	m, cmd = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if cmd == nil {
		t.Fatal("k should fire keepLiveCmd")
	}
	msg = drainCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("keep cmd errored: %v", e.err)
	}
	m, cmd2 := step(t, m, msg)
	if cmd2 != nil {
		drainCmd(t, cmd2)
	}

	srcFirst, err := os.ReadFile(filepath.Join(srcDir, sourceNameFor(firstTarget, home)))
	if err != nil {
		t.Fatalf("read source for first file: %v", err)
	}
	if !strings.Contains(string(srcFirst), "live") {
		t.Errorf("first file's source should now contain live content, got %q", srcFirst)
	}

	secondLive := m.session.queue[m.session.cursor].absolute
	m, cmd = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if cmd == nil {
		t.Fatal("v should fire revertCmd")
	}
	msg = drainCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("revert cmd errored: %v", e.err)
	}
	m, cmd2 = step(t, m, msg)
	if cmd2 != nil {
		drainCmd(t, cmd2)
	}

	liveSecond, err := os.ReadFile(secondLive)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(liveSecond), "live") {
		t.Errorf("second live should be reverted to source, got %q", liveSecond)
	}

	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Error("expected at least one snapshot file in snapshotDir")
	}

	if m.session.state != sessionSummary {
		t.Errorf("state should be sessionSummary after deciding all entries, got %v", m.session.state)
	}
}

func sourceNameFor(absLive, home string) string {
	rel := strings.TrimPrefix(absLive, home+"/")
	return "dot_" + strings.TrimPrefix(rel, ".")
}

func TestE2E_SideBySide_LogicallyIdenticalBanner(t *testing.T) {
	home := t.TempDir()
	srcDir := filepath.Join(home, ".local", "share", "chezmoi")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "dot_btop"),
		[]byte("theme_background = True\nvim_keys = False\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".btop"),
		[]byte("theme_background = true\nvim_keys = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cli, err := chezmoi.New(chezmoi.WithExtraEnv("HOME=" + home))
	if err != nil {
		t.Fatal(err)
	}

	store := &config.Store{Path: filepath.Join(t.TempDir(), "config.json")}
	_ = store.Save(config.Config{RepoConfirmed: true})
	m := NewModel(cli).WithConfigStore(store)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}

	for i, r := range m.rows {
		if r.target == ".btop" {
			m.cursor = 0
			for vi, vidx := range m.visibleIdxs {
				if vidx == i {
					m.cursor = vi
				}
			}
			break
		}
	}
	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := drainCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("side-by-side load errored: %v", e.err)
	}
	m, _ = step(t, m, msg)

	if m.sideAdded != 0 || m.sideRemoved != 0 {
		t.Errorf("case-only diff should be +0 −0, got +%d −%d", m.sideAdded, m.sideRemoved)
	}
	if m.sideLoose < 1 {
		t.Errorf("expected loose-match rows, got %d", m.sideLoose)
	}
	if !strings.Contains(stripANSI(m.View()), "logically identical") {
		t.Errorf("header should announce logically identical:\n%s", stripANSI(m.View()))
	}
}

func TestE2E_RepoSetup_FirstLaunchDiscoversAndPersists(t *testing.T) {
	home := t.TempDir()
	srcDir := filepath.Join(home, ".local", "share", "chezmoi")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "dot_testfile"), []byte("source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".testfile"), []byte("live\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cli, err := chezmoi.New(chezmoi.WithExtraEnv("HOME=" + home))
	if err != nil {
		t.Fatal(err)
	}

	storePath := filepath.Join(t.TempDir(), "config.json")
	store := &config.Store{Path: storePath}

	m := NewModel(cli).WithConfigStore(store)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	queue := []tea.Cmd{m.Init()}
	for len(queue) > 0 {
		cmd := queue[0]
		queue = queue[1:]
		if cmd == nil {
			continue
		}
		out := drainCmd(t, cmd)
		if batch, ok := out.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		var next tea.Cmd
		m, next = step(t, m, out)
		if next != nil {
			queue = append(queue, next)
		}
	}

	if m.state != viewRepoSetup {
		t.Fatalf("expected viewRepoSetup, got %v", m.state)
	}
	if len(m.repoSetup.candidates) == 0 {
		t.Fatal("discovery should produce candidates from the chezmoi source dir")
	}

	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should fire saveConfigCmd")
	}
	if msg := drainCmd(t, cmd); msg != nil {
		if e, ok := msg.(configSavedMsg); ok && e.err != nil {
			t.Fatalf("config save errored: %v", e.err)
		}
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("config not persisted: %v", err)
	}
	if !loaded.RepoConfirmed {
		t.Error("RepoConfirmed must be true after first-launch confirm")
	}
	if loaded.RepoPath == "" {
		t.Error("RepoPath should be populated after first-launch confirm")
	}
}

func TestE2E_DirectoryEnter_NoErrorScreen(t *testing.T) {
	m, _, home := e2eFixture(t)
	subSrc := filepath.Join(home, ".local", "share", "chezmoi", "dot_subdir")
	if err := os.MkdirAll(subSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subSrc, "inner"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	var msg tea.Msg = nil
	_ = msg

	dirIdx := -1
	for i, r := range m.rows {
		if r.target == ".subdir" {
			dirIdx = i
			break
		}
	}
	if dirIdx < 0 {
		t.Fatal(".subdir row not present in model")
	}
	if !m.rows[dirIdx].isDir {
		t.Fatal(".subdir row should be marked isDir=true after load")
	}
	for i, vi := range m.visibleIdxs {
		if vi == dirIdx {
			m.cursor = i
			break
		}
	}

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.err != nil {
		t.Fatalf("enter on directory must not surface an error, got %v", m.err)
	}
	if m.state != viewList {
		t.Errorf("must remain on list view after enter on a dir, got state=%v", m.state)
	}
	if !strings.Contains(m.status, "directory") {
		t.Errorf("expected status hint about directory, got %q", m.status)
	}
}

func TestE2E_RegressionAbsolutePathPlumbing(t *testing.T) {
	m, _, _ := e2eFixture(t)

	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	var msg tea.Msg = nil
	_ = msg
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if len(m.visibleIdxs) != 1 {
		t.Fatalf("expected single modified row, got %d", len(m.visibleIdxs))
	}

	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg = drainCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("side-by-side via Model failed (regression): %v", e.err)
	}
	m, _ = step(t, m, msg)
	if m.state != viewSideBySide {
		t.Fatalf("expected viewSideBySide, got %v", m.state)
	}
}

func TestE2E_Filter_NarrowsRealManagedList(t *testing.T) {
	m, _, home := e2eFixture(t)
	srcDir := filepath.Join(home, ".local", "share", "chezmoi")
	if err := os.WriteFile(filepath.Join(srcDir, "dot_other"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".other"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	if len(m.rows) < 2 {
		t.Fatalf("expected >=2 managed rows, got %d", len(m.rows))
	}

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "testfile" {
		m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(m.visibleIdxs) != 1 {
		t.Fatalf("filter should narrow to 1 row, got %d", len(m.visibleIdxs))
	}
	if m.rows[m.visibleIdxs[0]].target != ".testfile" {
		t.Errorf("wrong row visible: %q", m.rows[m.visibleIdxs[0]].target)
	}
}

func TestE2E_Apply_OverwritesLiveFromSource(t *testing.T) {
	m, _, home := e2eFixture(t)

	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if len(m.visibleIdxs) != 1 {
		t.Fatalf("expected single modified row, got %d", len(m.visibleIdxs))
	}

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if m.confirmMsg == "" {
		t.Fatal("expected confirm prompt after a")
	}
	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	msg := drainCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("apply failed: %v", e.err)
	}
	m, _ = step(t, m, msg)

	live, err := os.ReadFile(filepath.Join(home, ".testfile"))
	if err != nil {
		t.Fatal(err)
	}
	if string(live) != "source content\n" {
		t.Errorf("live after apply = %q, want source content", live)
	}
}

func TestE2E_UnifiedDiff_Toggle(t *testing.T) {
	m, _, _ := e2eFixture(t)

	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := drainCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("side-by-side load failed: %v", e.err)
	}
	m, _ = step(t, m, msg)

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	if !m.sideUnified {
		t.Fatal("u should enable unified mode")
	}
	view := stripANSI(m.vp.View())
	if !strings.Contains(view, "@@") {
		t.Errorf("unified view should contain hunk headers:\n%s", view)
	}
}

func TestE2E_Undo_ReAddThenRestoreSource(t *testing.T) {
	m, _, home := e2eFixture(t)

	srcPath := filepath.Join(home, ".local", "share", "chezmoi", "dot_testfile")
	before, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.confirmMsg == "" {
		t.Fatal("expected re-add confirm prompt")
	}
	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	msg := drainCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("re-add failed: %v", e.err)
	}
	m, _ = step(t, m, msg)

	promoted, _ := os.ReadFile(srcPath)
	if string(promoted) == string(before) {
		t.Fatal("re-add should have changed the source file")
	}

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if m.confirmMsg == "" {
		t.Fatal("u should prompt to undo the re-add")
	}
	m, cmd = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	msg = drainCmd(t, cmd)
	m, _ = step(t, m, msg)

	restored, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(before) {
		t.Errorf("source after undo = %q, want %q", restored, before)
	}
}

// gotoE2ETab switches tabs and drives the resulting load command.
func gotoE2ETab(t *testing.T, m Model, n int) Model {
	t.Helper()
	for i := 0; i < n; i++ {
		var cmd tea.Cmd
		next, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyTab})
		m = next
		if cmd != nil {
			m, _ = step(t, m, drainCmd(t, cmd))
		}
	}
	return m
}

func TestE2E_Unmanaged_AddToSource(t *testing.T) {
	m, _, home := e2eFixture(t)
	if err := os.WriteFile(filepath.Join(home, ".fresh"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	m = gotoE2ETab(t, m, 2) // Unmanaged
	if m.activeTab != tabUnmanaged {
		t.Fatalf("expected Unmanaged tab, got %v", m.activeTab)
	}

	// Move the cursor onto .fresh, add it, confirm.
	freshAbs := filepath.Join(home, ".fresh")
	for i, p := range m.unmanaged {
		if p == freshAbs {
			m.unmanagedCursor = i
		}
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if m.confirmMsg == "" {
		t.Fatal("expected add confirm prompt")
	}
	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	// The add completion batches an entries refresh and an unmanaged refresh;
	// drive the whole queue so both land.
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		out := drainCmd(t, c)
		if batch, ok := out.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		if e, ok := out.(errMsg); ok {
			t.Fatalf("add failed: %v", e.err)
		}
		var next tea.Cmd
		m, next = step(t, m, out)
		if next != nil {
			queue = append(queue, next)
		}
	}

	// .fresh must now be managed.
	m = gotoE2ETab(t, m, len(tabs)-2) // back around to All
	managed := false
	for _, r := range m.rows {
		if r.target == ".fresh" {
			managed = true
		}
	}
	if !managed {
		t.Errorf(".fresh should be managed after adding; rows=%d", len(m.rows))
	}
}

func TestE2E_Doctor_ShowsChecks(t *testing.T) {
	m, _, _ := e2eFixture(t)
	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	m = gotoE2ETab(t, m, 4) // Doctor
	if m.activeTab != tabDoctor {
		t.Fatalf("expected Doctor tab, got %v", m.activeTab)
	}
	if len(m.doctor) == 0 {
		t.Fatal("doctor should populate checks")
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "version") {
		t.Errorf("doctor view should list checks:\n%s", view)
	}
}

func TestE2E_WordLevelDiff_PairsModifiedLines(t *testing.T) {
	m, _, home := e2eFixture(t)
	srcDir := filepath.Join(home, ".local", "share", "chezmoi")
	if err := os.WriteFile(filepath.Join(srcDir, "dot_testfile"), []byte("export EDITOR=vi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".testfile"), []byte("export EDITOR=nvim\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := drainCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("load failed: %v", e.err)
	}
	m, _ = step(t, m, msg)

	rows := alignLines(m.sideTarget, m.sideLive)
	modified := 0
	for _, r := range rows {
		if r.Modified {
			modified++
		}
	}
	if modified != 1 {
		t.Fatalf("want 1 paired modification, got %d (%#v)", modified, rows)
	}
	// The rendered view must contain the emphasis escape sequences.
	if !strings.Contains(m.vp.View(), "\x1b[1;") {
		t.Errorf("word-level emphasis should appear in the rendered diff")
	}
}

func TestE2E_WrapAndScroll_RealDiff(t *testing.T) {
	m, _, home := e2eFixture(t)
	srcDir := filepath.Join(home, ".local", "share", "chezmoi")
	long := strings.Repeat("ab", 80) // 160 chars, wider than a panel
	if err := os.WriteFile(filepath.Join(srcDir, "dot_testfile"), []byte(long+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".testfile"), []byte(long+"ZZ\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, msg := range drainBatch(m.Init()) {
		m, _ = step(t, m, msg)
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := drainCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("load failed: %v", e.err)
	}
	m, _ = step(t, m, msg)

	// Unwrapped: the clipped long line carries a scroll marker.
	if !strings.Contains(stripANSI(m.vp.View()), "›") {
		t.Errorf("clipped long line should show a scroll marker:\n%s", stripANSI(m.vp.View()))
	}

	// Wrap: the line splits into multiple visual lines with a blank gutter.
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if !m.sideWrap {
		t.Fatal("w should enable wrap")
	}
	wrapped := stripANSI(m.vp.View())
	if !strings.Contains(wrapped, "ZZ") {
		t.Errorf("wrapped view should reveal the differing tail:\n%s", wrapped)
	}
}
