package tui

import (
	"strings"
	"testing"
)

func TestTokenize_ReconstructsInput(t *testing.T) {
	cases := []string{
		"",
		"a",
		"a b c",
		"  leading",
		"trailing   ",
		"x = True",
		"\ttabbed\tinput",
		"no=spaces",
		"a  b   c",
		strings.Repeat("word ", 20),
	}
	for _, in := range cases {
		if got := strings.Join(tokenize(in), ""); got != in {
			t.Errorf("tokenize(%q) did not reconstruct: %q", in, got)
		}
	}
}

func TestTokenize_SplitsWhitespaceAndWords(t *testing.T) {
	got := tokenize("x = True")
	want := []string{"x", " ", "=", " ", "True"}
	if len(got) != len(want) {
		t.Fatalf("tokenize = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWordDiff_MarksOnlyChangedTokens(t *testing.T) {
	left, right := wordDiff("EDITOR=vi", "EDITOR=nvim")
	// Left: "EDITOR=vi" splits into ["EDITOR=vi"] (one token). Right likewise.
	// With no common token the whole line is changed on both sides.
	if !allChanged(left) || !allChanged(right) {
		t.Fatalf("expected whole-token change, got left=%v right=%v", left, right)
	}
}

func TestWordDiff_WordGranularity(t *testing.T) {
	l, r := wordDiff("export PATH=/usr/bin", "export PATH=/usr/local/bin")
	// "export", " ", "PATH=/usr/bin" vs "export", " ", "PATH=/usr/local/bin": the leading "export " is shared; the rest changes.
	if len(l) == 0 || l[0].changed {
		t.Errorf("left first token should be unchanged: %v", l)
	}
	if !lastChanged(l) || !lastChanged(r) {
		t.Errorf("trailing changed token expected: left=%v right=%v", l, r)
	}
}

func TestWordDiff_LosslessPerSide(t *testing.T) {
	cases := [][2]string{
		{"a b c", "a x c"},
		{"", ""},
		{"only left", ""},
		{"", "only right"},
		{"same", "same"},
		{"x = True", "x = false"},
		{"foo(bar, baz)", "foo(bar, qux)"},
	}
	for _, c := range cases {
		l, r := wordDiff(c[0], c[1])
		if got := joinSegments(l); got != c[0] {
			t.Errorf("left not lossless for %q/%q: got %q", c[0], c[1], got)
		}
		if got := joinSegments(r); got != c[1] {
			t.Errorf("right not lossless for %q/%q: got %q", c[0], c[1], got)
		}
	}
}

func TestWordDiff_IdenticalNoChanges(t *testing.T) {
	l, r := wordDiff("no change here", "no change here")
	if anyChanged(l) || anyChanged(r) {
		t.Errorf("identical lines must have no changed tokens: l=%v r=%v", l, r)
	}
}

func TestWordDiff_Insertion(t *testing.T) {
	l, r := wordDiff("a c", "a b c")
	for _, s := range l {
		if s.changed {
			t.Errorf("left should be fully unchanged on insertion, got %v", l)
		}
	}
	if !anyChanged(r) {
		t.Errorf("inserted token should be marked changed: %v", r)
	}
}

func TestWordDiff_Deletion(t *testing.T) {
	l, r := wordDiff("a b c", "a c")
	for _, s := range r {
		if s.changed {
			t.Errorf("right should be fully unchanged on deletion, got %v", r)
		}
	}
	if !anyChanged(l) {
		t.Errorf("deleted token should be marked changed: %v", l)
	}
}

func anyChanged(segs []segment) bool {
	for _, s := range segs {
		if s.changed {
			return true
		}
	}
	return false
}

func allChanged(segs []segment) bool {
	if len(segs) == 0 {
		return false
	}
	for _, s := range segs {
		if !s.changed {
			return false
		}
	}
	return true
}

func lastChanged(segs []segment) bool {
	return len(segs) > 0 && segs[len(segs)-1].changed
}

// FuzzWordDiff asserts the diff is lossless on both sides and never panics.
func FuzzWordDiff(f *testing.F) {
	f.Add("a b c", "a x c")
	f.Add("", "")
	f.Add("EDITOR=vi", "EDITOR=nvim")
	f.Add("  spaces  ", "spaces")
	f.Fuzz(func(t *testing.T, left, right string) {
		l, r := wordDiff(left, right)
		if got := joinSegments(l); got != left {
			t.Fatalf("left not lossless: %q -> %q", left, got)
		}
		if got := joinSegments(r); got != right {
			t.Fatalf("right not lossless: %q -> %q", right, got)
		}
	})
}
