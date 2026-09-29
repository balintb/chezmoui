package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/balintb/chezmoui/internal/chezmoi"
	tea "github.com/charmbracelet/bubbletea"
)

func TestRetry_FailedReAdd_RetriesSamePaths(t *testing.T) {
	b := sampleBackend()
	b.reAddErr = errors.New("transient")
	m := loadedModel(t, b)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m, _ = applyMsg(t, m, runCmd(t, cmd))

	if m.err == nil {
		t.Fatal("expected error after failed re-add")
	}
	if m.retry == nil {
		t.Fatal("failed command should record a retry")
	}
	if len(b.reAddCalls) != 1 {
		t.Fatalf("expected 1 re-add call, got %d", len(b.reAddCalls))
	}

	b.reAddErr = nil
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if m.retry != nil {
		t.Error("retry should be consumed on press")
	}
	if !m.loading {
		t.Error("retry should show loading")
	}

	// Drive the queued retry command to completion.
	msg := cmd
	for msg != nil {
		out := msg()
		var next tea.Cmd
		m, next = applyMsg(t, m, out)
		msg = next
	}
	if len(b.reAddCalls) != 2 {
		t.Fatalf("retry should re-issue re-add, got %d calls", len(b.reAddCalls))
	}
	if got := b.reAddCalls[1]; len(got) != 1 || got[0] != "/home/u/.config/btop/btop.conf" {
		t.Errorf("retry path wrong: %v", got)
	}
	if m.err != nil {
		t.Errorf("error should clear after successful retry, got %v", m.err)
	}
}

func TestRetry_DefaultReloadWhenNoCommand(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m.err = errors.New("stale")
	m.retry = nil
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if cmd == nil {
		t.Fatal("R without a recorded retry should still reload entries")
	}
	msg := cmd()
	if _, ok := msg.(entriesLoadedMsg); !ok {
		t.Fatalf("default R should reload entries, got %T", msg)
	}
}

func TestViewError_SurfacesCommandArgs(t *testing.T) {
	b := sampleBackend()
	b.managed = nil
	b.status = nil
	b.sourcePath = "/tmp/repo"
	m := loadedModel(t, b)

	m.err = opError{
		Op:  "read status",
		Err: &chezmoi.CommandError{Args: []string{"status"}, Stderr: "boom"},
	}
	m.retry = nil
	view := stripANSI(m.View())
	if !containsAll(view, "read status", "chezmoi status", "boom") {
		t.Errorf("error view incomplete:\n%s", view)
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			return false
		}
	}
	return true
}

func TestErrorKey_DismissWithEsc(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m.err = opError{Op: "read status", Err: errors.New("boom")}
	m.retry = nil
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.err != nil {
		t.Errorf("esc should dismiss the error, got %v", m.err)
	}
}

func TestErrorKey_Quit(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m.err = opError{Op: "read status", Err: errors.New("boom")}
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("q should emit Quit, got %T", cmd())
	}
}

func TestErrorKey_RetryDuringSession(t *testing.T) {
	m, b := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))

	// Fail a keep, then retry it with R while still in the session.
	b.reAddErr = errors.New("transient")
	_, keepCmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m, _ = applyMsg(t, m, runCmd(t, keepCmd))
	if m.err == nil {
		t.Fatal("expected error from failed keep")
	}
	b.reAddErr = nil
	m, cmd = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if cmd == nil {
		t.Fatal("R should retry the failed keep during a session")
	}
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	if m.err != nil {
		t.Errorf("retry should clear the error, got %v", m.err)
	}
	if len(b.reAddCalls) != 2 {
		t.Errorf("expected keep to be attempted twice, got %d", len(b.reAddCalls))
	}
}

func TestErrorKey_UnrelatedKeyIsNoOp(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m.err = opError{Op: "read status", Err: errors.New("boom")}
	m.retry = nil
	after, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if cmd != nil {
		t.Errorf("unrelated key while errored should be a no-op, got cmd %v", cmd)
	}
	if after.err == nil || after.loading {
		t.Errorf("state should be unchanged: err=%v loading=%v", after.err, after.loading)
	}
}
