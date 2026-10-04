package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWindowRuns_ExtractsWindow(t *testing.T) {
	runs := []styledRun{{text: "abcdefgh"}}
	got, left, right := windowRuns(runs, 2, 3)
	if plain := plainRuns(got); plain != "cde" {
		t.Errorf("window = %q, want cde", plain)
	}
	if !left || !right {
		t.Errorf("clipping flags: left=%v right=%v, want both true", left, right)
	}
}

func TestWindowRuns_NoClipping(t *testing.T) {
	runs := []styledRun{{text: "abc"}}
	got, left, right := windowRuns(runs, 0, 5)
	if plain := plainRuns(got); plain != "abc" {
		t.Errorf("window = %q", plain)
	}
	if left || right {
		t.Errorf("no clipping expected: left=%v right=%v", left, right)
	}
}

func TestWindowRuns_OffsetPastEnd(t *testing.T) {
	runs := []styledRun{{text: "abc"}}
	got, left, _ := windowRuns(runs, 10, 4)
	if plain := plainRuns(got); plain != "" {
		t.Errorf("window past end should be empty, got %q", plain)
	}
	if !left {
		t.Error("offset past end should still report left clipping")
	}
}

func TestWindowRuns_PreservesRunStyles(t *testing.T) {
	runs := []styledRun{
		{text: "aaaa", style: addLineStyle},
		{text: "bbbb", style: addEmphStyle},
	}
	// Window covers the tail of the first run and head of the second.
	got, _, _ := windowRuns(runs, 2, 4)
	if len(got) != 2 {
		t.Fatalf("want 2 runs, got %d: %#v", len(got), got)
	}
	if got[0].text != "aa" || got[1].text != "bb" {
		t.Errorf("run split wrong: %#v", got)
	}
	if got[1].style.GetBold() != addEmphStyle.GetBold() {
		t.Error("emphasis style not preserved across the window")
	}
}

func TestWindowRuns_NegativeOffsetClamped(t *testing.T) {
	runs := []styledRun{{text: "abcd"}}
	got, left, _ := windowRuns(runs, -5, 2)
	if plain := plainRuns(got); plain != "ab" {
		t.Errorf("negative offset should clamp to 0, got %q", plain)
	}
	if left {
		t.Error("clamped offset should not report left clipping")
	}
}

func TestWrapRuns_ChunksByWidth(t *testing.T) {
	runs := []styledRun{{text: "abcdefgh", style: addLineStyle}}
	chunks := wrapRuns(runs, 3)
	if len(chunks) != 3 {
		t.Fatalf("want 3 chunks, got %d: %#v", len(chunks), chunks)
	}
	if got := plainRuns(chunks[0]); got != "abc" {
		t.Errorf("chunk 0 = %q", got)
	}
	if got := plainRuns(chunks[2]); got != "gh" {
		t.Errorf("chunk 2 = %q", got)
	}
}

func TestWrapRuns_LosslessAcrossChunks(t *testing.T) {
	runs := []styledRun{{text: "hello "}, {text: "world", style: addEmphStyle}}
	chunks := wrapRuns(runs, 4)
	var all strings.Builder
	for _, c := range chunks {
		all.WriteString(plainRuns(c))
	}
	if all.String() != "hello world" {
		t.Errorf("wrap lost content: %q", all.String())
	}
}

func TestWrapRuns_EmptyProducesOneChunk(t *testing.T) {
	chunks := wrapRuns(nil, 10)
	if len(chunks) != 1 {
		t.Errorf("empty input should yield one chunk, got %d", len(chunks))
	}
}

func TestOverlayClipped_Markers(t *testing.T) {
	runs := []styledRun{{text: "abcdef"}}
	got := overlayClipped(runs, true, true, 6)
	pr := []rune(plainRuns(got))
	if pr[0] != '‹' || pr[len(pr)-1] != '›' {
		t.Errorf("expected scroll markers, got %q", string(pr))
	}
	if string(pr[1:len(pr)-1]) != "bcde" {
		t.Errorf("interior should be preserved, got %q", string(pr))
	}
}

// unwrappedPanel renders one side of a single row without wrapping.
func unwrappedPanel(content string, width, hOffset int) string {
	r := alignedRow{Left: content, LeftPresent: true, LeftNum: 1}
	lines := panelVisualLines(r, panelSideLeft, viewOpts{contentW: width, hOffset: hOffset})
	if len(lines) != 1 {
		return strings.Join(lines, "\n")
	}
	return stripANSI(lines[0])
}

func TestPanelVisualLines_UnwrappedWindowShowsMarkerLeft(t *testing.T) {
	got := unwrappedPanel(strings.Repeat("x", 50), 20, 10)
	if !strings.HasPrefix(got, "   1 ‹") {
		t.Errorf("scrolled-right line should start with a left marker, got %q", got)
	}
	if !strings.HasSuffix(got, "›") {
		t.Errorf("line should end with a right marker, got %q", got)
	}
}

func TestPanelVisualLines_UnwrappedShowsTail(t *testing.T) {
	content := "abcdefghij"
	got := unwrappedPanel(content, 5, 5)
	// Window [5,10) = "fghij"; the first visible cell becomes a left marker, leaving "‹ghij" at the end of the line.
	plain := strings.TrimPrefix(got, "   1 ")
	if !strings.HasSuffix(plain, "ghij") {
		t.Errorf("window should contain the tail, got %q", got)
	}
	if []rune(plain)[0] != '‹' {
		t.Errorf("expected a left marker, got %q", plain)
	}
}

func TestPanelVisualLines_WrappedPadsContinuationGutter(t *testing.T) {
	r := alignedRow{Left: strings.Repeat("y", 25), LeftPresent: true, LeftNum: 7}
	lines := panelVisualLines(r, panelSideLeft, viewOpts{contentW: 10, wrap: true})
	if len(lines) != 3 {
		t.Fatalf("want 3 wrapped lines, got %d", len(lines))
	}
	if !strings.HasPrefix(stripANSI(lines[0]), "   7 ") {
		t.Errorf("first line should carry the gutter: %q", stripANSI(lines[0]))
	}
	for i := 1; i < len(lines); i++ {
		if strings.Contains(stripANSI(lines[i]), "7") {
			t.Errorf("continuation line %d should use a blank gutter: %q", i, stripANSI(lines[i]))
		}
	}
}

func TestFormatRowCells_KeepsPanelsAlignedWhenWrapping(t *testing.T) {
	// Left wraps to 3 lines, right to 1: both panels must yield 3 visual lines.
	r := alignedRow{
		Left:        strings.Repeat("a", 25),
		Right:       "short",
		LeftPresent: true, RightPresent: true,
		LeftNum: 1, RightNum: 1,
	}
	cells := formatRowCells(r, viewOpts{contentW: 10, wrap: true})
	if len(cells) != 3 {
		t.Fatalf("want 3 row cells, got %d", len(cells))
	}
	for i, c := range cells {
		if l, rr := strings.Count(c.left, "\n"), strings.Count(c.right, "\n"); l != 0 || rr != 0 {
			t.Errorf("cell %d contains embedded newline", i)
		}
	}
}

// sideWrapFixture returns a side-by-side model with a long line.
func sideWrapFixture(t *testing.T) Model {
	t.Helper()
	b := sampleBackend()
	long := strings.Repeat("abcdefghij", 8) // 80 chars
	b.cat = long + "\n"
	m := loadedModel(t, b).WithReadFile(func(string) ([]byte, error) {
		return []byte(long + "X\n"), nil
	})
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	return m
}

func TestSideView_WrapToggle(t *testing.T) {
	m := sideWrapFixture(t)
	if m.sideWrap {
		t.Fatal("wrap should default off")
	}
	m, _ = press(t, m, 'w')
	if !m.sideWrap {
		t.Fatal("w should enable wrap")
	}
	m, _ = press(t, m, 'w')
	if m.sideWrap {
		t.Fatal("w should toggle wrap off")
	}
}

func TestSideView_ScrollHorizontally(t *testing.T) {
	m := sideWrapFixture(t)
	if m.sideHScroll != 0 {
		t.Fatal("hscroll should default 0")
	}
	// shift+right scrolls right.
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyShiftRight})
	m = next.(Model)
	if m.sideHScroll <= 0 {
		t.Fatalf("shift+right should increase hscroll, got %d", m.sideHScroll)
	}
	// shift+left scrolls back and clamps at 0.
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyShiftLeft})
	m = next.(Model)
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyShiftLeft})
	m = next.(Model)
	if m.sideHScroll != 0 {
		t.Errorf("hscroll should clamp at 0, got %d", m.sideHScroll)
	}
}

func TestSideView_ScrollIgnoredWhenWrapped(t *testing.T) {
	m := sideWrapFixture(t)
	m, _ = press(t, m, 'w')
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyShiftRight})
	m = next.(Model)
	if m.sideHScroll != 0 {
		t.Errorf("horizontal scroll should be ignored while wrapping, got %d", m.sideHScroll)
	}
}

func TestSideView_WrapResetsHScroll(t *testing.T) {
	m := sideWrapFixture(t)
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyShiftRight})
	m = next.(Model)
	if m.sideHScroll == 0 {
		t.Fatal("setup: expected nonzero hscroll")
	}
	m, _ = press(t, m, 'w')
	if m.sideHScroll != 0 {
		t.Errorf("enabling wrap should reset hscroll, got %d", m.sideHScroll)
	}
}

// FuzzWrapRuns asserts wrapping is lossless for valid UTF-8 content and that no chunk exceeds the width. Invalid UTF-8 is normalised to U+FFFD by []rune conversion, so it is excluded.
func FuzzWrapRuns(f *testing.F) {
	f.Add("hello world", 5)
	f.Add("", 3)
	f.Add("a", 1)
	f.Fuzz(func(t *testing.T, s string, width int) {
		if width < 1 || width > 10000 || len(s) > 10000 || !utf8.ValidString(s) {
			t.Skip()
		}
		chunks := wrapRuns([]styledRun{{text: s}}, width)
		var all strings.Builder
		for _, c := range chunks {
			chunk := plainRuns(c)
			all.WriteString(chunk)
			if got := len([]rune(chunk)); got > width {
				t.Fatalf("chunk wider than %d: %d", width, got)
			}
		}
		if all.String() != s {
			t.Fatalf("wrap not lossless: %q -> %q", s, all.String())
		}
	})
}
