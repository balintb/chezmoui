package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/balintb/chezmoui/internal/chezmoi"
	tea "github.com/charmbracelet/bubbletea"
)

func repoBackend() *stubBackend {
	b := sampleBackend()
	b.repoStatus = chezmoi.RepoStatus{
		Branch:   "main",
		Tracking: "origin/main",
		Ahead:    2,
		Changes: []chezmoi.GitChange{
			{XY: " M", Path: "dot_bashrc"},
			{XY: "??", Path: "dot_zshrc"},
		},
	}
	return b
}

func sourceModel(t *testing.T, b *stubBackend, gitWrite bool) Model {
	t.Helper()
	m := goTab(t, loadedModel(t, b), tabSource)
	m.gitWrite = gitWrite
	return m
}

func TestSource_LoadsStatusOnVisit(t *testing.T) {
	m := sourceModel(t, repoBackend(), false)
	if m.activeTab != tabSource {
		t.Fatalf("expected Source tab, got %v", m.activeTab)
	}
	if m.repoStatus.Branch != "main" || len(m.repoStatus.Changes) != 2 {
		t.Fatalf("status not loaded: %#v", m.repoStatus)
	}
}

func TestSource_RendersBranchAndChanges(t *testing.T) {
	m := sourceModel(t, repoBackend(), false)
	view := stripANSI(m.View())
	for _, want := range []string{"Source", "main", "origin/main", "↑2", "dot_bashrc", "dot_zshrc", "untracked"} {
		if !strings.Contains(view, want) {
			t.Errorf("source view missing %q:\n%s", want, view)
		}
	}
}

func TestSource_GitWriteDisabledBlocksActions(t *testing.T) {
	b := repoBackend()
	m := sourceModel(t, b, false)
	m, cmd := press(t, m, 'c')
	if cmd != nil || m.commitPrompt || m.confirmMsg != "" {
		t.Error("commit must be blocked when git write is disabled")
	}
	if !strings.Contains(m.status, "disabled") {
		t.Errorf("status = %q", m.status)
	}
	m, _ = press(t, m, 'P')
	if m.confirmMsg != "" {
		t.Error("push must be blocked when git write is disabled")
	}
	if len(b.commitCalls) != 0 || b.pushCalls != 0 {
		t.Error("no git writes should occur")
	}
}

func TestSource_CommitPromptAndConfirm(t *testing.T) {
	b := repoBackend()
	m := sourceModel(t, b, true)
	m, _ = press(t, m, 'c')
	if !m.commitPrompt {
		t.Fatal("c should open the commit prompt")
	}
	for _, r := range "update bashrc" {
		m, _ = m.handleCommitPrompt(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.handleCommitPrompt(tea.KeyMsg{Type: tea.KeyEnter})
	if m.commitPrompt {
		t.Fatal("enter should close the prompt")
	}
	if !strings.Contains(m.confirmMsg, "Commit 2 change") {
		t.Fatalf("confirm = %q", m.confirmMsg)
	}
	if m.pendingOp != opCommit {
		t.Fatalf("pendingOp = %v, want opCommit", m.pendingOp)
	}
	m, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("confirm should fire the commit command")
	}
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	if len(b.commitCalls) != 1 || b.commitCalls[0] != "update bashrc" {
		t.Errorf("commit calls = %v", b.commitCalls)
	}
}

func TestSource_CommitPromptEmptyMessageIgnored(t *testing.T) {
	b := repoBackend()
	m := sourceModel(t, b, true)
	m, _ = press(t, m, 'c')
	next, _ := m.handleCommitPrompt(tea.KeyMsg{Type: tea.KeyEnter})
	m = next
	if m.confirmMsg != "" {
		t.Error("empty message must not raise a confirm")
	}
	if !m.commitPrompt {
		t.Error("prompt should stay open on empty input")
	}
}

func TestSource_CommitPromptEsc(t *testing.T) {
	b := repoBackend()
	m := sourceModel(t, b, true)
	m, _ = press(t, m, 'c')
	next, _ := m.handleCommitPrompt(tea.KeyMsg{Type: tea.KeyEsc})
	m = next
	if m.commitPrompt {
		t.Error("esc should close the prompt")
	}
	if len(b.commitCalls) != 0 {
		t.Error("esc must not commit")
	}
}

func TestSource_CommitCleanTree(t *testing.T) {
	b := repoBackend()
	b.repoStatus.Changes = nil
	m := sourceModel(t, b, true)
	m, _ = press(t, m, 'c')
	if m.commitPrompt {
		t.Error("no prompt when the tree is clean")
	}
	if !strings.Contains(m.status, "nothing to commit") {
		t.Errorf("status = %q", m.status)
	}
}

func TestSource_PushPromptAndConfirm(t *testing.T) {
	b := repoBackend()
	m := sourceModel(t, b, true)
	m, _ = press(t, m, 'P')
	if !strings.Contains(m.confirmMsg, "Push main") {
		t.Fatalf("confirm = %q", m.confirmMsg)
	}
	m, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	runCmd(t, cmd)
	if b.pushCalls != 1 {
		t.Errorf("push calls = %d, want 1", b.pushCalls)
	}
}

func TestSource_PushNoBranch(t *testing.T) {
	b := repoBackend()
	b.repoStatus.Branch = ""
	m := sourceModel(t, b, true)
	m, _ = press(t, m, 'P')
	if m.confirmMsg != "" {
		t.Error("no push confirm without a branch")
	}
	if !strings.Contains(m.status, "no branch") {
		t.Errorf("status = %q", m.status)
	}
}

func TestSource_CursorNavigation(t *testing.T) {
	m := sourceModel(t, repoBackend(), false)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.repoCursor != 1 {
		t.Errorf("cursor = %d, want 1", m.repoCursor)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.repoCursor != 1 {
		t.Errorf("cursor should clamp, got %d", m.repoCursor)
	}
}

func TestSource_RefreshReloads(t *testing.T) {
	b := repoBackend()
	m := sourceModel(t, b, false)
	b.repoStatus.Branch = "feature"
	m, cmd := press(t, m, 'R')
	if cmd == nil {
		t.Fatal("R should reload")
	}
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	if m.repoStatus.Branch != "feature" {
		t.Errorf("branch = %q, want feature", m.repoStatus.Branch)
	}
}

func TestSource_GitActionDoneReloads(t *testing.T) {
	b := repoBackend()
	m := sourceModel(t, b, true)
	before := m.repoLoaded
	if !before {
		t.Fatal("setup: repo should be loaded")
	}
	m, cmd := applyMsg(t, m, gitActionDoneMsg{action: "committed"})
	if cmd == nil {
		t.Fatal("git action done should reload status")
	}
	if m.repoLoaded {
		t.Error("loaded flag should reset")
	}
	if !strings.Contains(m.status, "committed") {
		t.Errorf("status = %q", m.status)
	}
}

func TestSource_TabCountShown(t *testing.T) {
	m := sourceModel(t, repoBackend(), false)
	if !strings.Contains(stripANSI(m.View()), "Source (2)") {
		t.Errorf("tab bar should show source change count:\n%s", stripANSI(m.View()))
	}
}

func TestSource_CommitCmdErrorCarriesRetry(t *testing.T) {
	b := repoBackend()
	b.commitErr = errors.New("hook failed")
	msg := gitCommitCmd(b, "x")()
	e, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("want errMsg, got %T", msg)
	}
	if e.retry == nil {
		t.Error("failed commit must carry a retry")
	}
}

func TestGitBranchChip(t *testing.T) {
	chip := stripANSI(gitBranchChip(chezmoi.RepoStatus{Branch: "main", Tracking: "origin/main", Ahead: 1, Behind: 2}))
	for _, want := range []string{"main", "origin/main", "↑1", "↓2"} {
		if !strings.Contains(chip, want) {
			t.Errorf("chip missing %q: %q", want, chip)
		}
	}
	if got := stripANSI(gitBranchChip(chezmoi.RepoStatus{})); !strings.Contains(got, "no branch") {
		t.Errorf("empty branch chip = %q", got)
	}
}
