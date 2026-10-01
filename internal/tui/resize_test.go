package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestResize_PreservesSessionDiff_Regression guards against WindowSizeMsg wiping the session viewport content on resize.
func TestResize_PreservesSessionDiff_Regression(t *testing.T) {
	m, _ := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = applyMsg(t, m, runCmd(t, cmd))
	m, _ = applyMsg(t, m, runCmd(t, cmd))

	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: 90, Height: 28})
	view := stripANSI(m.vp.View())
	if !strings.Contains(view, "target line") || !strings.Contains(view, "live line") {
		t.Errorf("resize must preserve session diff content, got:\n%s", view)
	}
}

// TestResize_PreservesSideBySide confirms the same for the side-by-side view.
func TestResize_PreservesSideBySide(t *testing.T) {
	b := sampleBackend()
	b.cat = "target line\n"
	m := loadedModel(t, b).WithReadFile(func(string) ([]byte, error) {
		return []byte("live line\n"), nil
	})
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))

	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: 90, Height: 28})
	view := stripANSI(m.vp.View())
	if !strings.Contains(view, "target line") || !strings.Contains(view, "live line") {
		t.Errorf("resize must preserve side-by-side content, got:\n%s", view)
	}
}
