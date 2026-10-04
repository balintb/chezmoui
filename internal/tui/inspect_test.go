package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/balintb/chezmoui/internal/chezmoi"
	tea "github.com/charmbracelet/bubbletea"
)

// goTab advances from the current tab to target, driving each tab's load command so the destination has data.
func goTab(t *testing.T, m Model, target tabID) Model {
	t.Helper()
	for i := 0; i <= len(tabs); i++ {
		if m.activeTab == target {
			return m
		}
		next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
		mm, ok := next.(Model)
		if !ok {
			t.Fatalf("handleKey returned %T", next)
		}
		m = mm
		if cmd != nil {
			m, _ = applyMsg(t, m, runCmd(t, cmd))
		}
	}
	if m.activeTab != target {
		t.Fatalf("could not reach tab %v, ended on %v", target, m.activeTab)
	}
	return m
}

func loadedUnmanaged(t *testing.T) Model {
	t.Helper()
	m := goTab(t, loadedModel(t, sampleBackend()), tabUnmanaged)
	if m.activeTab != tabUnmanaged {
		t.Fatalf("expected Unmanaged tab, got %v", m.activeTab)
	}
	return m
}

func TestUnmanaged_LoadsOnFirstVisit(t *testing.T) {
	m := loadedUnmanaged(t)
	if len(m.unmanaged) != 2 {
		t.Fatalf("want 2 unmanaged entries, got %d: %v", len(m.unmanaged), m.unmanaged)
	}
	if !m.inspectLoaded[tabUnmanaged] {
		t.Error("tab should be marked loaded")
	}
}

func TestUnmanaged_LoadedOnlyOnce(t *testing.T) {
	m := loadedUnmanaged(t)
	// Re-entering the tab must not re-issue the load command.
	m, _ = press(t, m, 'm') // back to Modified
	again := gotoInspectTab(t, m, tabUnmanaged)
	if len(again.unmanaged) != 2 {
		t.Errorf("data lost after re-entry: %v", again.unmanaged)
	}
}

func TestUnmanaged_ToggleSelection(t *testing.T) {
	m := loadedUnmanaged(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.unmanagedSel[".zshrc"] {
		t.Errorf("space should select the cursor row, got %v", m.unmanagedSel)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if m.unmanagedSel[".zshrc"] {
		t.Errorf("second space should deselect, got %v", m.unmanagedSel)
	}
}

func TestUnmanaged_AddSinglePromptsAndCallsBackend(t *testing.T) {
	b := sampleBackend()
	m := goTab(t, loadedModel(t, b), tabUnmanaged)
	m, _ = press(t, m, 'A')
	if !strings.Contains(m.confirmMsg, "Add") {
		t.Fatalf("A should prompt, got %q", m.confirmMsg)
	}
	if m.pendingOp != opAdd {
		t.Fatalf("pendingOp should be opAdd, got %v", m.pendingOp)
	}
	if len(m.pendingPaths) != 1 || m.pendingPaths[0] != ".zshrc" {
		t.Fatalf("pending paths = %v", m.pendingPaths)
	}
	m, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("confirm should fire the add command")
	}
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	if len(b.addCalls) != 1 || len(b.addCalls[0]) != 1 || b.addCalls[0][0] != ".zshrc" {
		t.Errorf("Add args = %v, want [.zshrc]", b.addCalls)
	}
}

func TestUnmanaged_AddBulkSelection(t *testing.T) {
	b := sampleBackend()
	m := goTab(t, loadedModel(t, b), tabUnmanaged)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m, _ = press(t, m, 'A')
	if !strings.Contains(m.confirmMsg, "2 file") {
		t.Fatalf("expected bulk confirm, got %q", m.confirmMsg)
	}
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	runCmd(t, cmd)
	if len(b.addCalls) != 1 || len(b.addCalls[0]) != 2 {
		t.Errorf("Add args = %v, want 2 paths", b.addCalls)
	}
}

func TestUnmanaged_AddCancelDoesNotCall(t *testing.T) {
	b := sampleBackend()
	m := goTab(t, loadedModel(t, b), tabUnmanaged)
	m, _ = press(t, m, 'A')
	m, _ = press(t, m, 'n')
	if m.confirmMsg != "" {
		t.Errorf("confirm should clear, got %q", m.confirmMsg)
	}
	if len(b.addCalls) != 0 {
		t.Errorf("cancel must not call Add, got %v", b.addCalls)
	}
}

func TestUnmanaged_AddNothingSelected(t *testing.T) {
	b := sampleBackend()
	b.unmanaged = nil
	m := goTab(t, loadedModel(t, b), tabUnmanaged)
	m, _ = press(t, m, 'A')
	if m.confirmMsg != "" {
		t.Errorf("empty list should not prompt, got %q", m.confirmMsg)
	}
	if !strings.Contains(m.status, "nothing to add") {
		t.Errorf("status = %q", m.status)
	}
}

func TestAddDone_ClearsSelectionAndReloads(t *testing.T) {
	m := loadedUnmanaged(t)
	m.unmanagedSel["x"] = true
	m, cmd := applyMsg(t, m, addDoneMsg{count: 1})
	if len(m.unmanagedSel) != 0 {
		t.Errorf("selection should clear after add, got %v", m.unmanagedSel)
	}
	if cmd == nil {
		t.Fatal("add done should refresh entries and unmanaged list")
	}
	if m.inspectLoaded[tabUnmanaged] {
		t.Error("unmanaged tab should be marked stale for reload")
	}
}

func TestAddCmd_ErrorCarriesRetry(t *testing.T) {
	b := sampleBackend()
	b.addErr = errors.New("secrets detected")
	msg := addCmd(b, []string{".zshrc"})()
	e, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("want errMsg, got %T", msg)
	}
	if e.retry == nil {
		t.Error("failed add must carry a retry")
	}
}

func TestUnmanaged_EmptyState(t *testing.T) {
	b := sampleBackend()
	b.unmanaged = nil
	m := goTab(t, loadedModel(t, b), tabUnmanaged)
	if !strings.Contains(stripANSI(m.View()), "(none)") {
		t.Errorf("empty unmanaged tab should show a placeholder:\n%s", stripANSI(m.View()))
	}
}

func TestIgnored_RendersReadOnly(t *testing.T) {
	m := goTab(t, loadedModel(t, sampleBackend()), tabIgnored)
	if m.activeTab != tabIgnored {
		t.Fatalf("expected Ignored tab, got %v", m.activeTab)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, ".cache/appstate") {
		t.Errorf("ignored list should show entries:\n%s", view)
	}
	if !strings.Contains(view, ".chezmoiignore") {
		t.Errorf("ignored tab should hint at .chezmoiignore:\n%s", view)
	}
	// No add action on the ignored tab.
	b := sampleBackend()
	m2 := goTab(t, loadedModel(t, b), tabIgnored)
	m2, _ = press(t, m2, 'A')
	if m2.confirmMsg != "" {
		t.Error("A must not prompt on the ignored tab")
	}
}

func TestDoctor_RendersAndCounts(t *testing.T) {
	b := sampleBackend()
	b.doctor = []chezmoi.DoctorCheck{
		{Result: "ok", Check: "version", Message: "v2"},
		{Result: "error", Check: "source-dir", Message: "missing"},
	}
	m := goTab(t, loadedModel(t, b), tabDoctor)
	if m.activeTab != tabDoctor {
		t.Fatalf("expected Doctor tab, got %v", m.activeTab)
	}
	view := stripANSI(m.View())
	for _, want := range []string{"1 ok", "1 failed", "version", "source-dir"} {
		if !strings.Contains(view, want) {
			t.Errorf("doctor view missing %q:\n%s", want, view)
		}
	}
}

func TestInspect_RefreshKeyReloads(t *testing.T) {
	b := sampleBackend()
	m := goTab(t, loadedModel(t, b), tabUnmanaged)
	// Change the backend data, then refresh: the new data must appear.
	b.unmanaged = []string{".only-after-refresh"}
	m, cmd := press(t, m, 'R')
	if cmd == nil {
		t.Fatal("R should issue a reload command")
	}
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	if len(m.unmanaged) != 1 || m.unmanaged[0] != ".only-after-refresh" {
		t.Errorf("refresh did not reload data: %v", m.unmanaged)
	}
}

func TestInspect_CursorScrolling(t *testing.T) {
	b := sampleBackend()
	b.unmanaged = []string{"a", "b", "c"}
	m := goTab(t, loadedModel(t, b), tabUnmanaged)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.unmanagedCursor != 1 {
		t.Errorf("cursor = %d, want 1", m.unmanagedCursor)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnd})
	if m.unmanagedCursor != 2 {
		t.Errorf("End should jump to last, got %d", m.unmanagedCursor)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.unmanagedCursor != 2 {
		t.Errorf("cursor should clamp at the end, got %d", m.unmanagedCursor)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyHome})
	if m.unmanagedCursor != 0 {
		t.Errorf("Home should jump to first, got %d", m.unmanagedCursor)
	}
}

func TestInspect_TabCountsInBar(t *testing.T) {
	m := goTab(t, loadedModel(t, sampleBackend()), tabUnmanaged)
	view := stripANSI(m.View())
	if !strings.Contains(view, "Unmanaged (2)") {
		t.Errorf("tab bar should show unmanaged count:\n%s", view)
	}
}

func TestInspect_ErrorSurfacesWithRetry(t *testing.T) {
	b := sampleBackend()
	b.unmanagedErr = errors.New("boom")
	m := goTab(t, loadedModel(t, b), tabUnmanaged)
	if m.err == nil {
		t.Fatal("unmanaged load error should surface")
	}
	if !strings.Contains(m.err.Error(), "list unmanaged") {
		t.Errorf("error should be prefixed with the operation: %v", m.err)
	}
}

func TestDoctor_CursorNavigation(t *testing.T) {
	b := sampleBackend()
	b.doctor = []chezmoi.DoctorCheck{
		{Result: "ok", Check: "a", Message: "1"},
		{Result: "ok", Check: "b", Message: "2"},
		{Result: "ok", Check: "c", Message: "3"},
	}
	m := goTab(t, loadedModel(t, b), tabDoctor)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.doctorCursor != 1 {
		t.Errorf("cursor = %d, want 1", m.doctorCursor)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.doctorCursor != 2 {
		t.Errorf("cursor should clamp at 2, got %d", m.doctorCursor)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.doctorCursor != 1 {
		t.Errorf("cursor = %d, want 1", m.doctorCursor)
	}
}

func TestInspect_PgUpPgDown(t *testing.T) {
	b := sampleBackend()
	b.unmanaged = make([]string, 30)
	for i := range b.unmanaged {
		b.unmanaged[i] = "p"
	}
	m := goTab(t, loadedModel(t, b), tabUnmanaged)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.unmanagedCursor != 10 {
		t.Errorf("pgdown = %d, want 10", m.unmanagedCursor)
	}
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.unmanagedCursor != 0 {
		t.Errorf("pgup = %d, want 0", m.unmanagedCursor)
	}
	// PgUp at the top must not underflow.
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.unmanagedCursor != 0 {
		t.Errorf("pgup underflow: %d", m.unmanagedCursor)
	}
}

func TestIgnored_CursorNavigation(t *testing.T) {
	b := sampleBackend()
	b.ignored = []string{"a", "b"}
	m := goTab(t, loadedModel(t, b), tabIgnored)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.ignoredCursor != 1 {
		t.Errorf("ignored cursor = %d, want 1", m.ignoredCursor)
	}
	// Scrolling on the ignored tab must move its cursor.
	m, _ = applyMsg(t, m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if m.ignoredCursor != 0 {
		t.Errorf("wheel up should move ignored cursor to 0, got %d", m.ignoredCursor)
	}
}

func TestInspect_ScrollWheelDoctor(t *testing.T) {
	b := sampleBackend()
	b.doctor = []chezmoi.DoctorCheck{
		{Result: "ok", Check: "a", Message: "1"},
		{Result: "ok", Check: "b", Message: "2"},
	}
	m := goTab(t, loadedModel(t, b), tabDoctor)
	m, _ = applyMsg(t, m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if m.doctorCursor != 1 {
		t.Errorf("wheel down on doctor = %d, want 1", m.doctorCursor)
	}
}

func TestInspectList_UnmanagedViewShownFromBarClick(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m = gotoInspectTab(t, m, tabUnmanaged)
	if m.activeTab != tabUnmanaged {
		t.Fatalf("expected Unmanaged, got %v", m.activeTab)
	}
}
