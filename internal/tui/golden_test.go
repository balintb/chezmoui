package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/balintb/chezmoui/internal/chezmoi"
	tea "github.com/charmbracelet/bubbletea"
)

var updateGolden = flag.Bool("update", false, "update golden files")

// assertGolden compares got against testdata/<name>.golden, writing the file when -update is set. ANSI and zone markers must already be stripped.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	got = normalizeGolden(got)
	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run `go test ./internal/tui -run %s -update`): %v", path, t.Name(), err)
	}
	if got != normalizeGolden(string(want)) {
		t.Errorf("golden mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// normalizeGolden removes ANSI sequences and trailing spaces so golden files are stable and friendly to version control.
func normalizeGolden(s string) string {
	s = stripANSI(s)
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// goldenView renders the model at a fixed size with ANSI and zone markers removed so the output is stable across environments.
func goldenView(t *testing.T, m Model, w, h int) string {
	t.Helper()
	m, _ = applyMsg(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	return stripANSI(m.View())
}

func TestGolden_ListDefault(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	assertGolden(t, "list_default", goldenView(t, m, 100, 30))
}

func TestGolden_HelpPage(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	assertGolden(t, "help_page", goldenView(t, m, 100, 40))
}

func TestGolden_SideBySide(t *testing.T) {
	b := sampleBackend()
	b.cat = "alpha\nbravo\ncharlie\n"
	m := loadedModel(t, b).WithReadFile(func(string) ([]byte, error) {
		return []byte("alpha\nBRAVO\ncharlie\ndelta\n"), nil
	})
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	assertGolden(t, "side_by_side", goldenView(t, m, 100, 20))
}

func TestGolden_SessionWelcome(t *testing.T) {
	m, _ := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	assertGolden(t, "session_welcome", goldenView(t, m, 100, 30))
}

func TestGolden_SessionReview(t *testing.T) {
	m, _ := sessionFixture(t)
	m, _ = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = applyMsg(t, m, runCmd(t, cmd))
	if cmd == nil {
		t.Fatal("expected a session entry load command")
	}
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	assertGolden(t, "session_review", goldenView(t, m, 100, 24))
}

func TestGolden_ErrorView(t *testing.T) {
	b := sampleBackend()
	b.managed = nil
	b.status = nil
	m := loadedModel(t, b)
	m.err = opError{Op: "read status", Err: errString("chezmoi status: permission denied")}
	m.retry = nil
	assertGolden(t, "error_view", goldenView(t, m, 100, 20))
}

// gotoInspectTab loads the model then switches to an inspect tab, applying the tab's load command so the view has data.
func gotoInspectTab(t *testing.T, m Model, tab tabID) Model {
	t.Helper()
	if cmd := m.switchTab(tab); cmd != nil {
		m, _ = applyMsg(t, m, runCmd(t, cmd))
	}
	return m
}

func TestGolden_UnmanagedTab(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m = gotoInspectTab(t, m, tabUnmanaged)
	assertGolden(t, "unmanaged_tab", goldenView(t, m, 100, 24))
}

func TestGolden_IgnoredTab(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m = gotoInspectTab(t, m, tabIgnored)
	assertGolden(t, "ignored_tab", goldenView(t, m, 100, 24))
}

func TestGolden_DoctorTab(t *testing.T) {
	b := sampleBackend()
	b.doctor = []chezmoi.DoctorCheck{
		{Result: "ok", Check: "version", Message: "v2.73.0"},
		{Result: "info", Check: "config-file", Message: "not found"},
		{Result: "error", Check: "source-dir", Message: "no such file or directory"},
		{Result: "failed", Check: "hardlink", Message: "not supported"},
	}
	m := loadedModel(t, b)
	m = gotoInspectTab(t, m, tabDoctor)
	assertGolden(t, "doctor_tab", goldenView(t, m, 100, 24))
}

func TestGolden_FilterPrompt(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m, _ = press(t, m, '/')
	m, _ = typeRunes(m, "btop")
	assertGolden(t, "filter_prompt", goldenView(t, m, 100, 24))
}

func TestGolden_UnifiedDiff(t *testing.T) {
	m, _ := sideFixture(t)
	m, _ = press(t, m, 'u')
	assertGolden(t, "unified_diff", goldenView(t, m, 100, 24))
}

type errString string

func (e errString) Error() string { return string(e) }
