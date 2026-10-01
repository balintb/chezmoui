package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/balintb/chezmoui/internal/chezmoi"
	tea "github.com/charmbracelet/bubbletea"
)

func TestApply_PromptsAndCallsBackend(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m, _ = press(t, m, 'm')
	m = cursorTo(t, m, ".config/btop/btop.conf")
	m, _ = press(t, m, 'a')
	if !strings.Contains(m.confirmMsg, "Apply") {
		t.Fatalf("expected apply confirm prompt, got %q", m.confirmMsg)
	}
	if m.pendingOp != opApply {
		t.Fatalf("pendingOp should be opApply, got %v", m.pendingOp)
	}
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	msg := runCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("apply errored: %v", e.err)
	}
	if len(b.applyCalls) != 1 || len(b.applyCalls[0]) != 1 ||
		b.applyCalls[0][0] != "/home/u/.config/btop/btop.conf" {
		t.Errorf("apply args: want absolute path, got %v", b.applyCalls)
	}
}

func TestApply_RequiresConfirmation(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m, _ = press(t, m, 'm')
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if cmd != nil {
		t.Fatal("apply must not run before confirmation")
	}
	if len(b.applyCalls) != 0 {
		t.Errorf("Apply called without confirmation: %v", b.applyCalls)
	}
}

func TestApply_CancelDoesNotCall(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m, _ = press(t, m, 'm')
	m, _ = press(t, m, 'a')
	m, _ = press(t, m, 'n')
	if m.confirmMsg != "" {
		t.Errorf("confirm should clear, got %q", m.confirmMsg)
	}
	if len(b.applyCalls) != 0 {
		t.Errorf("cancel must not call Apply, got %v", b.applyCalls)
	}
}

func TestApply_NothingToApply(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m = cursorTo(t, m, ".vimrc") // clean
	m, _ = press(t, m, 'a')
	if m.confirmMsg != "" {
		t.Errorf("clean row should not prompt, got %q", m.confirmMsg)
	}
	if !strings.Contains(m.status, "nothing") {
		t.Errorf("status should say nothing to apply, got %q", m.status)
	}
}

func TestApply_BulkSelection(t *testing.T) {
	b := sampleBackend()
	b.status = []chezmoi.Status{
		{Source: ' ', Target: 'M', Path: ".bashrc"},
		{Source: ' ', Target: 'M', Path: ".config/btop/btop.conf"},
	}
	m := loadedModel(t, b)
	m, _ = press(t, m, 'm')
	m = cursorTo(t, m, ".bashrc")
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = cursorTo(t, m, ".config/btop/btop.conf")
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m, _ = press(t, m, 'a')
	if !strings.Contains(m.confirmMsg, "2 file") {
		t.Fatalf("expected confirm for 2 files, got %q", m.confirmMsg)
	}
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	runCmd(t, cmd)
	if len(b.applyCalls) != 1 || len(b.applyCalls[0]) != 2 {
		t.Fatalf("apply: want 1 call with 2 paths, got %#v", b.applyCalls)
	}
}

func TestApplyDone_ClearsSelectionAndReloads(t *testing.T) {
	b := sampleBackend()
	m := loadedModel(t, b)
	m.selected["x"] = true
	m, cmd := applyMsg(t, m, applyDoneMsg{count: 2})
	if m.selected["x"] {
		t.Error("selection should be cleared after apply")
	}
	if !m.loading {
		t.Error("apply done should trigger reload")
	}
	if cmd == nil {
		t.Fatal("apply done should return reload cmd")
	}
	if !strings.Contains(m.status, "applied 2") {
		t.Errorf("status: %q", m.status)
	}
}

func TestApplyCmd_ErrorCarriesRetry(t *testing.T) {
	b := sampleBackend()
	b.applyErr = errors.New("conflict")
	msg := applyCmd(b, []string{"/home/u/.bashrc"})()
	e, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("want errMsg, got %T", msg)
	}
	var oe opError
	if !errors.As(e.err, &oe) || oe.Op != "apply" {
		t.Errorf("unexpected error: %v", e.err)
	}
	if e.retry == nil {
		t.Error("failed apply must carry a retry")
	}
}
