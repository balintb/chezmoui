package tui

import (
	"strings"
	"testing"
)

const sampleUnified = `diff --git a/dot_bashrc b/dot_bashrc
index 111..222 100644
--- a/dot_bashrc
+++ b/dot_bashrc
@@ -1,3 +1,4 @@
 export PATH=/usr/bin
-export EDITOR=vi
+export EDITOR=nvim
+export PAGER=less
 alias ll='ls -la'
@@ -10,2 +11,2 @@
 keep me
-drop this
+add this
`

func TestParseUnified_ClassifiesLines(t *testing.T) {
	lines := parseUnified(sampleUnified)
	got := map[diffLineKind]int{}
	for _, l := range lines {
		got[l.kind]++
	}
	if got[diffHunk] != 2 {
		t.Errorf("hunks: want 2, got %d", got[diffHunk])
	}
	if got[diffAdd] != 3 {
		t.Errorf("adds: want 3, got %d", got[diffAdd])
	}
	if got[diffRemove] != 2 {
		t.Errorf("removes: want 2, got %d", got[diffRemove])
	}
	if got[diffHeader] < 3 {
		t.Errorf("headers: want >=3, got %d", got[diffHeader])
	}
}

func TestParseUnified_Empty(t *testing.T) {
	if got := parseUnified(""); got != nil {
		t.Errorf("empty diff should yield nil, got %#v", got)
	}
}

func TestParseUnified_OnlyNewline(t *testing.T) {
	if got := parseUnified("\n"); len(got) != 0 {
		t.Errorf("bare newline should yield no lines, got %#v", got)
	}
}

func TestParseUnified_PlusPlusPlusIsHeaderNotAdd(t *testing.T) {
	lines := parseUnified("+++ b/file\n++-not-a-header\n")
	if lines[0].kind != diffHeader {
		t.Errorf("'+++' must be a header, got %v", lines[0].kind)
	}
	if lines[1].kind != diffAdd {
		t.Errorf("'++-' must be an addition, got %v", lines[1].kind)
	}
}

func TestParseUnified_MinusMinusMinusIsHeaderNotRemove(t *testing.T) {
	lines := parseUnified("--- a/file\n--not-header\n")
	if lines[0].kind != diffHeader {
		t.Errorf("'---' must be a header, got %v", lines[0].kind)
	}
	if lines[1].kind != diffRemove {
		t.Errorf("'--n' must be a removal, got %v", lines[1].kind)
	}
}

func TestHunkOffsets(t *testing.T) {
	offsets := hunkOffsets(parseUnified(sampleUnified))
	if len(offsets) != 2 {
		t.Fatalf("want 2 hunk offsets, got %d (%v)", len(offsets), offsets)
	}
	for _, o := range offsets {
		if !strings.HasPrefix(parseUnified(sampleUnified)[o].text, "@@") {
			t.Errorf("offset %d is not a hunk header", o)
		}
	}
	if offsets[0] >= offsets[1] {
		t.Errorf("hunk offsets must be increasing: %v", offsets)
	}
}

func TestHunkOffsets_None(t *testing.T) {
	if got := hunkOffsets(parseUnified("no hunks here\n")); len(got) != 0 {
		t.Errorf("want no offsets, got %v", got)
	}
}

func TestRenderUnified_AllLinesPresent(t *testing.T) {
	lines := parseUnified(sampleUnified)
	out := stripANSI(renderUnified(lines, 80))
	for _, want := range []string{
		"@@ -1,3 +1,4 @@",
		"-export EDITOR=vi",
		"+export EDITOR=nvim",
		"+export PAGER=less",
		" alias ll='ls -la'",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered unified diff missing %q:\n%s", want, out)
		}
	}
}

func TestRenderUnified_Empty(t *testing.T) {
	out := stripANSI(renderUnified(nil, 80))
	if !strings.Contains(out, "no differences") {
		t.Errorf("empty diff should render a placeholder, got %q", out)
	}
}

func TestRenderUnified_AddUsesBackgroundFill(t *testing.T) {
	lines := parseUnified("+added\n")
	out := renderUnified(lines, 20)
	if !hasBackgroundFill(out) {
		t.Errorf("added line should carry a background fill: %q", out)
	}
}

func TestRenderUnified_RemoveUsesBackgroundFill(t *testing.T) {
	lines := parseUnified("-removed\n")
	out := renderUnified(lines, 20)
	if !hasBackgroundFill(out) {
		t.Errorf("removed line should carry a background fill: %q", out)
	}
}

func TestRenderUnified_PadsToWidth(t *testing.T) {
	lines := parseUnified("+x\n")
	out := stripANSI(renderUnified(lines, 10))
	if w := len(strings.TrimRight(out, "\n")); w < 10 {
		t.Errorf("line should be padded to width 10, got %d: %q", w, out)
	}
}

// FuzzParseUnified asserts the parser never panics and preserves every line in order, regardless of input.
func FuzzParseUnified(f *testing.F) {
	f.Add(sampleUnified)
	f.Add("")
	f.Add("\n")
	f.Add("@@ -1 +1 @@\n")
	f.Add("+++\n---\n++x\n--y\n")
	f.Add("\x00\x01\n")
	f.Fuzz(func(t *testing.T, diff string) {
		lines := parseUnified(diff)
		// The parser trims trailing newlines, then emits one diffLine per remaining line, in order, preserving the text verbatim.
		trimmed := strings.TrimRight(diff, "\n")
		var want []string
		if trimmed != "" {
			want = strings.Split(trimmed, "\n")
		}
		if len(lines) != len(want) {
			t.Fatalf("line count: got %d want %d for %q", len(lines), len(want), diff)
		}
		for i := range want {
			if lines[i].text != want[i] {
				t.Fatalf("line %d: got %q want %q", i, lines[i].text, want[i])
			}
		}
		_ = hunkOffsets(lines)
		_ = renderUnified(lines, 0)
	})
}
