package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestAlignLines_Identical(t *testing.T) {
	got := alignLines("a\nb\nc\n", "a\nb\nc\n")
	want := []alignedRow{
		{Left: "a", Right: "a", LeftPresent: true, RightPresent: true, LeftNum: 1, RightNum: 1},
		{Left: "b", Right: "b", LeftPresent: true, RightPresent: true, LeftNum: 2, RightNum: 2},
		{Left: "c", Right: "c", LeftPresent: true, RightPresent: true, LeftNum: 3, RightNum: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("identical input mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestAlignLines_LineNumbersTrackOriginalPosition(t *testing.T) {
	got := alignLines("a\nb\nc\n", "a\nNEW\nb\nc\n")
	if got[0].LeftNum != 1 || got[0].RightNum != 1 {
		t.Errorf("matched row 0: want (1,1), got (%d,%d)", got[0].LeftNum, got[0].RightNum)
	}
	if got[1].LeftNum != 0 || got[1].RightNum != 2 {
		t.Errorf("inserted row: want (0,2), got (%d,%d)", got[1].LeftNum, got[1].RightNum)
	}
	if got[2].LeftNum != 2 || got[2].RightNum != 3 {
		t.Errorf("post-insert row: want (2,3), got (%d,%d)", got[2].LeftNum, got[2].RightNum)
	}
}

func TestAlignLines_BothEmpty(t *testing.T) {
	got := alignLines("", "")
	if len(got) != 0 {
		t.Errorf("empty inputs should yield 0 rows, got %d", len(got))
	}
}

func TestAlignLines_OnlyLeft(t *testing.T) {
	got := alignLines("a\nb\n", "")
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	for i, r := range got {
		if !r.LeftPresent || r.RightPresent {
			t.Errorf("row %d: want left-only, got %+v", i, r)
		}
	}
}

func TestAlignLines_OnlyRight(t *testing.T) {
	got := alignLines("", "x\ny\n")
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	for i, r := range got {
		if r.LeftPresent || !r.RightPresent {
			t.Errorf("row %d: want right-only, got %+v", i, r)
		}
	}
}

func TestAlignLines_Insertion(t *testing.T) {
	got := alignLines("a\nb\n", "a\nNEW\nb\n")
	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d: %#v", len(got), got)
	}
	if !(got[0].LeftPresent && got[0].RightPresent && got[0].Left == "a") {
		t.Errorf("row 0 should be matched 'a': %+v", got[0])
	}
	if !(got[1].RightPresent && !got[1].LeftPresent && got[1].Right == "NEW") {
		t.Errorf("row 1 should be right-only 'NEW': %+v", got[1])
	}
	if !(got[2].LeftPresent && got[2].RightPresent && got[2].Left == "b") {
		t.Errorf("row 2 should be matched 'b': %+v", got[2])
	}
}

func TestAlignLines_Deletion(t *testing.T) {
	got := alignLines("a\nGONE\nb\n", "a\nb\n")
	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d: %#v", len(got), got)
	}
	if !(got[1].LeftPresent && !got[1].RightPresent && got[1].Left == "GONE") {
		t.Errorf("row 1 should be left-only 'GONE': %+v", got[1])
	}
}

func TestAlignLines_Modification(t *testing.T) {
	// A single changed line is paired into one Modified row (word-level diff), not two separate add/delete rows.
	got := alignLines("a\nb\nc\n", "a\nB\nc\n")
	if len(got) != 3 {
		t.Fatalf("want 3 rows (match a, Modified b/B, match c), got %d: %#v", len(got), got)
	}
	if !got[1].Modified {
		t.Errorf("middle row should be Modified, got %#v", got[1])
	}
	if got[1].Left != "b" || got[1].Right != "B" {
		t.Errorf("modified row content = %q/%q", got[1].Left, got[1].Right)
	}
}

func TestAlignLines_UnpairedReplacementStaysSplit(t *testing.T) {
	// 2 deletions vs 1 insertion: only one pair forms; the leftover stays a delete row.
	got := alignLines("a\nb\nd\nz\n", "a\nX\nz\n")
	delOnly, addOnly, modified := 0, 0, 0
	for _, r := range got {
		switch {
		case r.Modified:
			modified++
		case r.LeftPresent && !r.RightPresent:
			delOnly++
		case !r.LeftPresent && r.RightPresent:
			addOnly++
		}
	}
	if modified != 1 {
		t.Errorf("want 1 paired modification, got %d (%#v)", modified, got)
	}
	if delOnly != 1 {
		t.Errorf("want 1 leftover deletion, got %d (%#v)", delOnly, got)
	}
	if addOnly != 0 {
		t.Errorf("want 0 leftover insertions, got %d", addOnly)
	}
}

func TestAlignLines_TrailingNewlineNoPhantom(t *testing.T) {
	withNL := alignLines("a\n", "a\n")
	withoutNL := alignLines("a", "a")
	if len(withNL) != 1 || len(withoutNL) != 1 {
		t.Errorf("trailing newline should not create phantom row: with=%d without=%d", len(withNL), len(withoutNL))
	}
}

func TestRenderSideBySide_LineCountsMatch(t *testing.T) {
	rows := alignLines("a\nb\nc\n", "a\nx\nc\n")
	left, right := renderSideBySide(rows, 20)
	lc := strings.Count(left, "\n")
	rc := strings.Count(right, "\n")
	if lc != rc {
		t.Errorf("panel line counts must match for synced scrolling: left=%d right=%d", lc, rc)
	}
}

func TestRenderPanels_FitsWithinWidth_Regression(t *testing.T) {
	for _, width := range []int{60, 80, 96, 100, 120, 200} {
		got := stripANSI(renderPanels("left line\n", "right line\n", width))
		lines := strings.Split(got, "\n")
		if len(lines) == 0 {
			t.Fatalf("width %d: empty output", width)
		}
		for i, ln := range lines {
			if w := lipgloss.Width(ln); w > width {
				t.Errorf("width %d: line %d is %d cells wide, overflows: %q", width, i, w, ln)
			}
		}
	}
}

func TestRenderSideBySide_PadsToColumnWidth(t *testing.T) {
	const colW = 30
	rows := []alignedRow{{Left: "ab", Right: "xy", LeftPresent: true, RightPresent: true, LeftNum: 1, RightNum: 1}}
	left, right := renderSideBySide(rows, colW)
	leftVisible := stripANSI(strings.TrimRight(left, "\n"))
	rightVisible := stripANSI(strings.TrimRight(right, "\n"))
	if len([]rune(leftVisible)) != colW || len([]rune(rightVisible)) != colW {
		t.Errorf("rows must be padded to colW=%d: got left=%d right=%d",
			colW, len([]rune(leftVisible)), len([]rune(rightVisible)))
	}
}

func TestRenderSideBySide_GutterShowsLineNumbers(t *testing.T) {
	rows := []alignedRow{{Left: "a", Right: "a", LeftPresent: true, RightPresent: true, LeftNum: 1, RightNum: 1}}
	left, right := renderSideBySide(rows, 30)
	if !strings.Contains(stripANSI(left), "   1 ") {
		t.Errorf("left gutter missing '1': %q", stripANSI(left))
	}
	if !strings.Contains(stripANSI(right), "   1 ") {
		t.Errorf("right gutter missing '1': %q", stripANSI(right))
	}
}

func TestRenderSideBySide_AbsentSideShowsTilde(t *testing.T) {
	rows := []alignedRow{{Right: "added", RightPresent: true, RightNum: 1}}
	left, _ := renderSideBySide(rows, 30)
	if !strings.Contains(stripANSI(left), "~") {
		t.Errorf("left gutter for absent side should show '~': %q", stripANSI(left))
	}
}

func TestRenderSideBySide_AddedLineHasBackgroundFill(t *testing.T) {
	rows := []alignedRow{{Right: "new", RightPresent: true}}
	_, right := renderSideBySide(rows, 10)
	if !hasBackgroundFill(right) {
		t.Errorf("added row missing BG fill SGR codes: %q", right)
	}
}

func TestRenderSideBySide_RemovedLineHasBackgroundFill(t *testing.T) {
	rows := []alignedRow{{Left: "gone", LeftPresent: true}}
	left, _ := renderSideBySide(rows, 10)
	if !hasBackgroundFill(left) {
		t.Errorf("removed row missing BG fill SGR codes: %q", left)
	}
}

func TestRenderSideBySide_MatchedLineNoFill(t *testing.T) {
	rows := []alignedRow{{Left: "x", Right: "x", LeftPresent: true, RightPresent: true}}
	left, right := renderSideBySide(rows, 10)
	if hasBackgroundFill(left) || hasBackgroundFill(right) {
		t.Errorf("matched row should not be filled: left=%q right=%q", left, right)
	}
}

func TestRenderSideBySide_FillSpansFullColumn(t *testing.T) {
	rows := []alignedRow{{Right: "ab", RightPresent: true, RightNum: 1}}
	_, right := renderSideBySide(rows, 30)
	visible := stripANSI(strings.TrimRight(right, "\n"))
	if len([]rune(visible)) != 30 {
		t.Errorf("right pane visible width: want 30, got %d (%q)", len([]rune(visible)), visible)
	}
}

func TestSummarizeAlignment(t *testing.T) {
	rows := []alignedRow{
		{LeftPresent: true, RightPresent: true}, // matched
		{RightPresent: true},                    // added
		{LeftPresent: true},                     // removed
		{RightPresent: true},                    // added
		{LeftPresent: true, RightPresent: true}, // matched
	}
	added, removed, loose := summarizeAlignment(rows)
	if added != 2 || removed != 1 || loose != 0 {
		t.Errorf("want +2 -1 ≈0, got +%d -%d ≈%d", added, removed, loose)
	}
}

func TestNormalizeForLooseMatch_Cases(t *testing.T) {
	cases := []struct{ in, want string }{
		{"theme_background = True", "theme_background = true"},
		{"vim_keys = FALSE", "vim_keys = false"},
		{"mixed = TrUe", "mixed = true"},
		{"truesomething = 1", "truesomething = 1"},
		{"x = falsehood", "x = falsehood"},
		{"foo = 1   ", "foo = 1"},
		{"foo = 1\t", "foo = 1"},
		{"  indented = True", "  indented = true"},
	}
	for _, c := range cases {
		if got := normalizeForLooseMatch(c.in); got != c.want {
			t.Errorf("normalizeForLooseMatch(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAlignLines_LooseMatch_PairsCaseDifferences(t *testing.T) {
	left := "theme_background = True\n"
	right := "theme_background = true\n"
	rows := alignLines(left, right)
	if len(rows) != 1 {
		t.Fatalf("expected one matched row, got %d: %#v", len(rows), rows)
	}
	r := rows[0]
	if !r.LeftPresent || !r.RightPresent {
		t.Errorf("loose match should be on a single row with both sides present: %+v", r)
	}
	if !r.LooseMatch {
		t.Error("LooseMatch should be true when sides differ only in bool case")
	}
	if r.Left != "theme_background = True" || r.Right != "theme_background = true" {
		t.Errorf("originals must be preserved verbatim: %q / %q", r.Left, r.Right)
	}
}

func TestAlignLines_LooseMatch_TrailingWhitespace(t *testing.T) {
	rows := alignLines("foo = 1   \n", "foo = 1\n")
	if len(rows) != 1 || !rows[0].LooseMatch {
		t.Fatalf("trailing-whitespace difference should be loose-matched, got %#v", rows)
	}
}

func TestAlignLines_StrictMatchNotLoose(t *testing.T) {
	rows := alignLines("a = true\n", "a = true\n")
	if len(rows) != 1 || rows[0].LooseMatch {
		t.Errorf("byte-equal lines must NOT be loose-matched: %#v", rows)
	}
}

func TestAlignLines_RealDifferenceIsNotLoose(t *testing.T) {
	rows := alignLines("a = True\n", "a = False\n")
	for _, r := range rows {
		if r.LooseMatch {
			t.Errorf("True→False must produce real diff rows, not loose: %+v", r)
		}
	}
}

func TestSummarizeAlignment_ExcludesLoose(t *testing.T) {
	rows := []alignedRow{
		{LeftPresent: true, RightPresent: true},                   // strict match
		{LeftPresent: true, RightPresent: true, LooseMatch: true}, // loose
		{RightPresent: true},                                      // added
		{LeftPresent: true},                                       // removed
		{LeftPresent: true, RightPresent: true, LooseMatch: true}, // loose
	}
	added, removed, loose := summarizeAlignment(rows)
	if added != 1 || removed != 1 || loose != 2 {
		t.Errorf("want +1 −1 ≈2, got +%d −%d ≈%d", added, removed, loose)
	}
}

func TestRenderSideBySide_LooseRowUsesLooseStyle(t *testing.T) {
	rows := []alignedRow{
		{
			Left: "x = True", Right: "x = true",
			LeftPresent: true, RightPresent: true,
			LeftNum: 1, RightNum: 1, LooseMatch: true,
		},
	}
	left, right := renderSideBySide(rows, 30)

	if !hasBackgroundFill(left) {
		t.Errorf("loose row left side missing BG fill: %q", left)
	}
	if !hasBackgroundFill(right) {
		t.Errorf("loose row right side missing BG fill: %q", right)
	}
	if !strings.Contains(stripANSI(left), "x = True") {
		t.Errorf("left should contain 'x = True': %q", stripANSI(left))
	}
	if !strings.Contains(stripANSI(right), "x = true") {
		t.Errorf("right should contain 'x = true': %q", stripANSI(right))
	}
}

func TestTruncOrPad(t *testing.T) {
	if got := truncOrPad("ab", 5); got != "ab   " {
		t.Errorf("pad: %q", got)
	}
	if got := truncOrPad("abcdef", 3); got != "abc" {
		t.Errorf("trunc: %q", got)
	}
	if got := truncOrPad("abc", 3); got != "abc" {
		t.Errorf("exact: %q", got)
	}
}

func TestRenderChanged_EmphasizesOnlyChangedTokens(t *testing.T) {
	// left/right share "export " and differ in the rest.
	left, right := "export EDITOR=vi", "export EDITOR=nvim"
	l := renderChanged(left, right, panelSideLeft, 40)
	r := renderChanged(left, right, panelSideRight, 40)

	plainL := stripANSI(l)
	plainR := stripANSI(r)
	if !strings.Contains(plainL, "EDITOR=vi") || !strings.Contains(plainR, "EDITOR=nvim") {
		t.Fatalf("content missing: l=%q r=%q", plainL, plainR)
	}
	// The emph style must appear on the changed glyphs.
	if !strings.Contains(l, "\x1b[") || !strings.Contains(r, "\x1b[") {
		t.Errorf("expected styled output")
	}
	// Emphasis background (48;5 or 48;2) must appear at least once per side.
	if !hasBackgroundFill(l) || !hasBackgroundFill(r) {
		t.Errorf("changed tokens should carry an emphasis background")
	}
}

func TestRenderChanged_PadsToWidth(t *testing.T) {
	for _, w := range []int{20, 40, 80} {
		out := stripANSI(renderChanged("a b", "a c", panelSideLeft, w))
		if got := len([]rune(out)); got != w {
			t.Errorf("width %d: got %d visible runes", w, got)
		}
	}
}

func TestRenderChanged_TruncatesLongLines(t *testing.T) {
	long := strings.Repeat("x", 200)
	out := stripANSI(renderChanged(long, long+"y", panelSideLeft, 30))
	if got := len([]rune(out)); got != 30 {
		t.Errorf("truncated line should be exactly 30 runes, got %d", got)
	}
}

func TestRenderChanged_UnchangedLineHasNoEmphasis(t *testing.T) {
	out := renderChanged("same line", "same line", panelSideLeft, 30)
	// No emphasis style should be applied when nothing changed.
	if strings.Contains(out, "\x1b[1;") {
		t.Errorf("unchanged line should not be bold-emphasized: %q", out)
	}
}

func TestFormatPanelCell_ModifiedPairUsesBothColors(t *testing.T) {
	r := alignedRow{Left: "a = 1", Right: "a = 2", LeftPresent: true, RightPresent: true, LeftNum: 1, RightNum: 1, Modified: true}
	l := formatPanelCell(r, panelSideLeft, 30)
	rr := formatPanelCell(r, panelSideRight, 30)
	if stripANSI(l) == stripANSI(rr) {
		t.Error("left and right cells should render distinct content")
	}
	if !hasBackgroundFill(l) {
		t.Error("modified left cell should be colored")
	}
	if !hasBackgroundFill(rr) {
		t.Error("modified right cell should be colored")
	}
}
