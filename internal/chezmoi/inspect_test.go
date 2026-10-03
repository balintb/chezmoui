package chezmoi

import (
	"reflect"
	"strings"
	"testing"
)

func TestParsePathList(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"whitespace", "\n\n  \n", nil},
		{"basic", ".zshrc\n.config/foo\n", []string{".config/foo", ".zshrc"}},
		{"dedup", ".a\n.a\n.b\n", []string{".a", ".b"}},
		{"trims", "  .a  \n\t.b\t\n", []string{".a", ".b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parsePathList(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("parsePathList(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

const doctorSample = `RESULT    CHECK                       MESSAGE
ok        version                     v2.73.0, commit Homebrew
ok        os-arch                     darwin/arm64
info      config-file                 ~/.config/chezmoi/chezmoi.toml: not found
error     source-dir                  open ~/.local/share/chezmoi: no such file or directory
failed    hardlink                    stat ~/.local/share/chezmoi: no such file or directory
`

func TestParseDoctor(t *testing.T) {
	checks := ParseDoctor(doctorSample)
	if len(checks) != 5 {
		t.Fatalf("want 5 checks, got %d: %#v", len(checks), checks)
	}
	if checks[0].Check != "version" || checks[0].Result != "ok" {
		t.Errorf("first check wrong: %#v", checks[0])
	}
	if !strings.Contains(checks[0].Message, "commit Homebrew") {
		t.Errorf("multi-word message truncated: %q", checks[0].Message)
	}
	// The message column contains a colon and spaces; it must survive intact.
	if got := checks[3].Message; got != "open ~/.local/share/chezmoi: no such file or directory" {
		t.Errorf("error message mangled: %q", got)
	}
}

func TestParseDoctor_SkipsHeaderAndBlanks(t *testing.T) {
	out := "RESULT    CHECK               MESSAGE\n\nok        a                   b\n"
	checks := ParseDoctor(out)
	if len(checks) != 1 {
		t.Fatalf("want 1 check, got %d: %#v", len(checks), checks)
	}
	if checks[0].Check != "a" || checks[0].Message != "b" {
		t.Errorf("unexpected check: %#v", checks[0])
	}
}

func TestParseDoctor_Empty(t *testing.T) {
	if got := ParseDoctor(""); len(got) != 0 {
		t.Errorf("empty input should yield no checks, got %#v", got)
	}
}

func TestParseDoctor_FallsBackToDefaultOffsets(t *testing.T) {
	// A header without the literal column names falls back to chezmoi's known layout (CHECK at 10, MESSAGE at 38).
	row := pad("ok", 10) + pad("check", 28) + "msg here"
	checks := ParseDoctor("COLS\n" + row + "\n")
	if len(checks) != 1 || checks[0].Check != "check" || checks[0].Message != "msg here" {
		t.Errorf("fallback parsing failed: %#v", checks)
	}
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func TestParseDoctor_SkipsAllEmptyRows_Regression(t *testing.T) {
	// A line that is long enough but yields only whitespace must not become a blank check row.
	if got := ParseDoctor("\n         \v"); len(got) != 0 {
		t.Errorf("blank rows must be skipped, got %#v", got)
	}
}

func TestDoctorCheck_Failed(t *testing.T) {
	cases := map[string]bool{
		"ok":      false,
		"info":    false,
		"warning": false,
		"error":   true,
		"failed":  true,
		"":        true,
	}
	for result, want := range cases {
		if got := (DoctorCheck{Result: result}).Failed(); got != want {
			t.Errorf("Failed() for %q = %v, want %v", result, got, want)
		}
	}
}

func TestAdd_RejectsEmptyPaths(t *testing.T) {
	c := &Client{}
	if err := c.Add(t.Context()); err == nil {
		t.Error("Add with no paths should error")
	}
}

// FuzzParseDoctor asserts the parser never panics and never emits empty checks.
func FuzzParseDoctor(f *testing.F) {
	f.Add(doctorSample)
	f.Add("")
	f.Add("RESULT CHECK MESSAGE\n")
	f.Add("ok  x  y z\n\n\n")
	f.Add("\x00\x01\x02")
	f.Fuzz(func(t *testing.T, out string) {
		for _, c := range ParseDoctor(out) {
			if c.Check == "" && c.Result == "" && c.Message == "" {
				t.Fatalf("parser emitted an entirely empty row from %q", out)
			}
		}
	})
}

// FuzzParsePathList asserts the parser never panics and never emits empties.
func FuzzParsePathList(f *testing.F) {
	f.Add(".a\n.b\n")
	f.Add("")
	f.Add("\n\n")
	f.Add("  x  \n")
	f.Fuzz(func(t *testing.T, out string) {
		for _, p := range parsePathList(out) {
			if strings.TrimSpace(p) == "" {
				t.Fatalf("parser emitted an empty path from %q", out)
			}
		}
	})
}
