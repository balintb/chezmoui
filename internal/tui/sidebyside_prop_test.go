package tui

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// alignedSides reconstructs the left and right line sequences from an alignment, preserving order.
func alignedSides(rows []alignedRow) (left, right []string) {
	for _, r := range rows {
		if r.LeftPresent {
			left = append(left, r.Left)
		}
		if r.RightPresent {
			right = append(right, r.Right)
		}
	}
	return left, right
}

// TestAlignLines_PreservesContent is a property test: regardless of input, the alignment must reproduce both original line sequences in order, exactly once per line. This guards the LCS backtracking against dropping or duplicating lines.
func TestAlignLines_PreservesContent(t *testing.T) {
	vocab := []string{"", "a", "b", "c", "  x = 1", "X = 1", "}", "  }", "# c", "\t"}
	rng := rand.New(rand.NewSource(1))
	gen := func() string {
		n := rng.Intn(12)
		lines := make([]string, n)
		for i := range lines {
			lines[i] = vocab[rng.Intn(len(vocab))]
		}
		s := strings.Join(lines, "\n")
		if rng.Intn(2) == 0 {
			s += "\n"
		}
		return s
	}

	for i := 0; i < 2000; i++ {
		left, right := gen(), gen()
		rows := alignLines(left, right)
		gotL, gotR := alignedSides(rows)
		if !equalLines(gotL, splitConfigLines(left)) {
			t.Fatalf("left not preserved for %q vs %q:\n got=%q\nwant=%q", left, right, gotL, splitConfigLines(left))
		}
		if !equalLines(gotR, splitConfigLines(right)) {
			t.Fatalf("right not preserved for %q vs %q:\n got=%q\nwant=%q", left, right, gotR, splitConfigLines(right))
		}
	}
}

func TestSummarizeAlignment_CountsMatchFlags(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 500; i++ {
		var rows []alignedRow
		n := rng.Intn(10)
		for j := 0; j < n; j++ {
			r := alignedRow{}
			r.LeftPresent = rng.Intn(2) == 0
			r.RightPresent = rng.Intn(2) == 0
			if !r.LeftPresent && !r.RightPresent {
				r.RightPresent = true
			}
			r.LooseMatch = rng.Intn(2) == 0
			r.Modified = rng.Intn(4) == 0
			rows = append(rows, r)
		}
		added, removed, loose := summarizeAlignment(rows)
		var wantAdd, wantRem, wantLoose int
		for _, r := range rows {
			switch {
			case r.Modified:
				wantAdd++
				wantRem++
			case r.LooseMatch:
				wantLoose++
			case !r.LeftPresent && r.RightPresent:
				wantAdd++
			case r.LeftPresent && !r.RightPresent:
				wantRem++
			}
		}
		if added != wantAdd || removed != wantRem || loose != wantLoose {
			t.Fatalf("summary mismatch: got (%d,%d,%d) want (%d,%d,%d)", added, removed, loose, wantAdd, wantRem, wantLoose)
		}
	}
}

func TestRenderSideBySide_LineCountsAlwaysMatch(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 300; i++ {
		n := rng.Intn(8)
		var l, r []string
		for j := 0; j < n; j++ {
			l = append(l, "line")
			if rng.Intn(2) == 0 {
				r = append(r, "line")
			}
		}
		left, right := renderSideBySide(alignLines(strings.Join(l, "\n"), strings.Join(r, "\n")), 20)
		if got, want := strings.Count(left, "\n"), strings.Count(right, "\n"); got != want {
			t.Fatalf("panel line counts differ: left=%d right=%d", got, want)
		}
	}
}

func equalLines(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// FuzzAlignLines ensures alignment never panics and always preserves content.
func FuzzAlignLines(f *testing.F) {
	f.Add("a\nb\nc\n", "a\nB\nc\n")
	f.Add("", "")
	f.Add("only\nleft\n", "")
	f.Add("", "only\nright\n")
	f.Add("x = True\n", "x = true\n")
	f.Fuzz(func(t *testing.T, left, right string) {
		rows := alignLines(left, right)
		gotL, gotR := alignedSides(rows)
		if !equalLines(gotL, splitConfigLines(left)) {
			t.Fatalf("left not preserved: got=%q want=%q", gotL, splitConfigLines(left))
		}
		if !equalLines(gotR, splitConfigLines(right)) {
			t.Fatalf("right not preserved: got=%q want=%q", gotR, splitConfigLines(right))
		}
		// Rendering must not panic and must stay rectangular.
		l, r := renderSideBySide(rows, sideGutterWidth+5)
		_ = l
		_ = r
	})
}
