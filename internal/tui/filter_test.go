package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func typeRunes(m Model, s string) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, r := range s {
		m, cmd = m.handleFilterKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m, cmd
}

// key drives one key through handleKey, asserting the concrete Model type.
func press(t *testing.T, m Model, r rune) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	mm, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T", next)
	}
	return mm, cmd
}

func keyType(t *testing.T, m Model, typ tea.KeyType) Model {
	t.Helper()
	next, _ := m.handleKey(tea.KeyMsg{Type: typ})
	mm, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T", next)
	}
	return mm
}

func TestFilter_NarrowsVisibleRows(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m, _ = press(t, m, '/')
	if !m.filtering {
		t.Fatal("/ should enter filtering mode")
	}
	m, _ = typeRunes(m, "btop")
	if len(m.visibleIdxs) != 1 {
		t.Fatalf("filter 'btop' should narrow to 1 row, got %d", len(m.visibleIdxs))
	}
	if m.rows[m.visibleIdxs[0]].target != ".config/btop/btop.conf" {
		t.Errorf("wrong row visible: %q", m.rows[m.visibleIdxs[0]].target)
	}
}

func TestFilter_CaseInsensitive(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m, _ = press(t, m, '/')
	m, _ = typeRunes(m, "BASHRC")
	if len(m.visibleIdxs) != 1 || m.rows[m.visibleIdxs[0]].target != ".bashrc" {
		t.Errorf("case-insensitive filter failed: %v", m.visibleIdxs)
	}
}

func TestFilter_MetacharsAreLiteral(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m, _ = press(t, m, '/')
	m, _ = typeRunes(m, ".*(")
	if len(m.visibleIdxs) != 0 {
		t.Errorf("regex metachars must be literal; got %d matches", len(m.visibleIdxs))
	}
}

func TestFilter_EnterKeepsFilterAndClearsInputMode(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m, _ = press(t, m, '/')
	m, _ = typeRunes(m, "vim")
	m, _ = m.handleFilterKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.filtering {
		t.Error("enter should leave filtering mode")
	}
	if m.filter != "vim" {
		t.Errorf("filter should be kept after enter, got %q", m.filter)
	}
	if len(m.visibleIdxs) != 1 {
		t.Errorf("filter should remain applied, got %d rows", len(m.visibleIdxs))
	}
}

func TestFilter_EscClearsFilter(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m, _ = press(t, m, '/')
	m, _ = typeRunes(m, "vim")
	m, _ = m.handleFilterKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.filtering || m.filter != "" {
		t.Errorf("esc should clear filter and mode: filtering=%v filter=%q", m.filtering, m.filter)
	}
	if len(m.visibleIdxs) != len(m.rows) {
		t.Errorf("all rows should be visible after clearing, got %d/%d", len(m.visibleIdxs), len(m.rows))
	}
}

func TestFilter_ComposesWithModifiedTab(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m, _ = press(t, m, 'm')
	if m.activeTab != tabModified {
		t.Fatal("setup: should be on Modified tab")
	}
	m, _ = press(t, m, '/')
	m, _ = typeRunes(m, "btop")
	if len(m.visibleIdxs) != 1 {
		t.Fatalf("modified+filter should show 1 row, got %d", len(m.visibleIdxs))
	}
	// A clean row matching the filter must stay hidden on the Modified tab.
	m, _ = m.handleFilterKey(tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = m.beginFilter()
	m, _ = typeRunes(m, "vimrc")
	if len(m.visibleIdxs) != 0 {
		t.Errorf("clean .vimrc must not appear on Modified tab, got %d", len(m.visibleIdxs))
	}
}

func TestFilter_CursorClampsWhenResultShrinks(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m = cursorTo(t, m, ".vimrc")
	m, _ = press(t, m, '/')
	m, _ = typeRunes(m, "bashrc")
	if m.cursor >= len(m.visibleIdxs) {
		t.Errorf("cursor %d out of range for %d visible rows", m.cursor, len(m.visibleIdxs))
	}
}

func TestFilter_NoMatchesRendersPlaceholder(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m, _ = press(t, m, '/')
	m, _ = typeRunes(m, "zzzznope")
	view := stripANSI(m.View())
	if !strings.Contains(view, "no matches") {
		t.Errorf("empty result should show 'no matches':\n%s", view)
	}
}

func TestFilter_BlocksActionKeys(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m, _ = press(t, m, '/')
	// "r" and "q" must be typed, not act.
	m, _ = typeRunes(m, "rq")
	if m.confirmMsg != "" {
		t.Error("r must not trigger re-add while filtering")
	}
	if m.filter != "rq" {
		t.Errorf("filter should contain typed keys, got %q", m.filter)
	}
}

func TestFilter_ChipShown(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m, _ = press(t, m, '/')
	m, _ = typeRunes(m, "vim")
	m, _ = m.handleFilterKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(stripANSI(m.View()), "filter: vim") {
		t.Errorf("applied filter should be shown as a chip:\n%s", stripANSI(m.View()))
	}
}

func TestFilter_SearchKeyIgnoredOnHelpTab(t *testing.T) {
	m := loadedModel(t, sampleBackend())
	m, _ = press(t, m, '?')
	m, _ = press(t, m, '/')
	if m.filtering {
		t.Error("/ must not start filtering while the Help tab is open")
	}
}
