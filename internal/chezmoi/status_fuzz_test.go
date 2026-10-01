package chezmoi

import "testing"

// FuzzParseStatus asserts the parser never panics and never manufactures more entries than there are lines.
func FuzzParseStatus(f *testing.F) {
	f.Add(" M .bashrc\n")
	f.Add("MM .config/foo\n")
	f.Add("")
	f.Add("\n\n\n")
	f.Add(" A path with spaces\n")
	f.Add("?? untracked\n")
	f.Add("\x00\x01\x02 weird")
	f.Fuzz(func(t *testing.T, out string) {
		statuses := ParseStatus(out)
		lines := 0
		for i := 0; i < len(out); i++ {
			if out[i] == '\n' {
				lines++
			}
		}
		if len(out) > 0 && out[len(out)-1] != '\n' {
			lines++
		}
		if len(statuses) > lines {
			t.Fatalf("parsed %d statuses from %d lines", len(statuses), lines)
		}
		for _, s := range statuses {
			_ = s.Modified()
			if s.Path == "" {
				t.Fatalf("parser emitted empty path from %q", out)
			}
		}
	})
}
