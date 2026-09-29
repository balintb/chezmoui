package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func sideFixture(t *testing.T) (Model, *stubBackend) {
	t.Helper()
	b := sampleBackend()
	b.cat = "alpha\nbravo\ncharlie\n"
	b.diff = sampleUnified
	m := loadedModel(t, b).WithReadFile(func(string) ([]byte, error) {
		return []byte("alpha\nBRAVO\ncharlie\ndelta\n"), nil
	})
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	if m.state != viewSideBySide {
		t.Fatalf("setup: expected viewSideBySide, got %v", m.state)
	}
	return m, b
}

func TestSideView_LoadsUnifiedDiff(t *testing.T) {
	m, b := sideFixture(t)
	if m.unifiedDiff != sampleUnified {
		t.Errorf("unified diff not stored: %q", m.unifiedDiff)
	}
	if len(m.hunks) != 2 {
		t.Errorf("expected 2 hunks, got %d", len(m.hunks))
	}
	if len(b.diffCalls) != 1 || b.diffCalls[0] != "/home/u/.config/btop/btop.conf" {
		t.Errorf("Diff must be called with the absolute path, got %v", b.diffCalls)
	}
}

func TestSideView_ToggleUnified(t *testing.T) {
	m, _ := sideFixture(t)
	if m.sideUnified {
		t.Fatal("should default to side-by-side")
	}
	m, _ = press(t, m, 'u')
	if !m.sideUnified {
		t.Fatal("u should switch to unified")
	}
	view := stripANSI(m.vp.View())
	if !strings.Contains(view, "@@ -1,3 +1,4 @@") {
		t.Errorf("unified view should show hunk header:\n%s", view)
	}
	if strings.Contains(view, "target (chezmoi)") {
		t.Errorf("unified view should not render the side-by-side panels:\n%s", view)
	}
	m, _ = press(t, m, 'u')
	if m.sideUnified {
		t.Fatal("u should toggle back to side-by-side")
	}
}

func TestSideView_HunkNavigation(t *testing.T) {
	m, _ := sideFixture(t)
	m, _ = press(t, m, 'u') // unified view

	// The fixture diff is short; pad it so the viewport can actually scroll.
	m.unifiedDiff = padDiff(sampleUnified, 40)
	m.hunks = hunkOffsets(parseUnified(m.unifiedDiff))
	m.vp.SetContent(m.renderDiffBody())

	if len(m.hunks) != 2 {
		t.Fatalf("setup: want 2 hunks, got %d", len(m.hunks))
	}
	m, _ = press(t, m, ']')
	if m.vp.YOffset != m.hunks[0] {
		t.Errorf("] should jump to first hunk offset %d, got %d", m.hunks[0], m.vp.YOffset)
	}
	m, _ = press(t, m, ']')
	if m.vp.YOffset != m.hunks[1] {
		t.Errorf("] again should jump to second hunk %d, got %d", m.hunks[1], m.vp.YOffset)
	}
	m, _ = press(t, m, '[')
	if m.vp.YOffset != m.hunks[0] {
		t.Errorf("[ should go back to first hunk %d, got %d", m.hunks[0], m.vp.YOffset)
	}
}

// padDiff appends context lines after the diff so hunk offsets exceed a typical viewport height.
func padDiff(diff string, extra int) string {
	var b strings.Builder
	b.WriteString(diff)
	for i := 0; i < extra; i++ {
		b.WriteString(" context line\n")
	}
	return b.String()
}

func TestSideView_HunkNavigationWraps(t *testing.T) {
	m, _ := sideFixture(t)
	m, _ = press(t, m, 'u')
	m.unifiedDiff = padDiff(sampleUnified, 40)
	m.hunks = hunkOffsets(parseUnified(m.unifiedDiff))
	m.vp.SetContent(m.renderDiffBody())
	m, _ = press(t, m, ']')
	m, _ = press(t, m, ']')
	m, _ = press(t, m, ']') // wraps past the end back to first
	if m.vp.YOffset != m.hunks[0] {
		t.Errorf("hunk navigation should wrap to first, got offset %d", m.vp.YOffset)
	}
}

func TestSideView_HunkNavigationNoHunks(t *testing.T) {
	b := sampleBackend()
	b.cat = "same\n"
	b.diff = ""
	m := loadedModel(t, b).WithReadFile(func(string) ([]byte, error) {
		return []byte("same\n"), nil
	})
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	before := m.vp.YOffset
	m, _ = press(t, m, ']')
	if m.vp.YOffset != before {
		t.Errorf("hunk navigation with no hunks must be a no-op")
	}
}

func TestSideView_ApplyPromptsWithAbsPath(t *testing.T) {
	m, b := sideFixture(t)
	m, _ = press(t, m, 'a')
	if m.pendingOp != opApply {
		t.Fatalf("pendingOp should be opApply, got %v", m.pendingOp)
	}
	if len(m.pendingPaths) != 1 || m.pendingPaths[0] != "/home/u/.config/btop/btop.conf" {
		t.Fatalf("pending path should be absolute, got %v", m.pendingPaths)
	}
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	runCmd(t, cmd)
	if len(b.applyCalls) != 1 {
		t.Errorf("apply should run from side view, got %v", b.applyCalls)
	}
}

func TestSideView_ReAddPrompts(t *testing.T) {
	m, b := sideFixture(t)
	m, _ = press(t, m, 'r')
	if m.pendingOp != opReAdd {
		t.Fatalf("pendingOp should be opReAdd, got %v", m.pendingOp)
	}
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	runCmd(t, cmd)
	if len(b.reAddCalls) != 1 {
		t.Errorf("re-add should run from side view, got %v", b.reAddCalls)
	}
}

func TestSideView_UnifiedHeaderShowsHunkCount(t *testing.T) {
	m, _ := sideFixture(t)
	m, _ = press(t, m, 'u')
	view := stripANSI(m.View())
	if !strings.Contains(view, "unified diff · 2 hunk") {
		t.Errorf("unified header should report hunk count:\n%s", view)
	}
}

func TestSideView_DiffErrorFallsBackToSideBySide(t *testing.T) {
	b := sampleBackend()
	b.cat = "target\n"
	b.diffErr = errText("diff unavailable")
	m := loadedModel(t, b).WithReadFile(func(string) ([]byte, error) {
		return []byte("live\n"), nil
	})
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := runCmd(t, cmd)
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("diff failure must not abort the view: %v", e.err)
	}
	m, _ = applyMsg(t, m, msg)
	if m.state != viewSideBySide {
		t.Fatalf("should still enter side-by-side, got %v", m.state)
	}
	if m.unifiedDiff != "" {
		t.Errorf("unified diff should be empty on Diff error, got %q", m.unifiedDiff)
	}
}

type errText string

func (e errText) Error() string { return string(e) }
