package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeForFilename_Invariants(t *testing.T) {
	inputs := []string{
		"/home/u/.config/btop/btop.conf",
		`C:\Users\me\.bashrc`,
		"with spaces and\ttabs",
		"a:b:c",
		"",
		"////",
		"..",
		"naïve/ünïcode",
	}
	for _, in := range inputs {
		got := sanitizeForFilename(in)
		if got == "" {
			t.Errorf("sanitize(%q) returned empty", in)
			continue
		}
		if strings.ContainsAny(got, `/\: `) {
			t.Errorf("sanitize(%q) = %q still contains a path or space separator", in, got)
		}
		if strings.ContainsRune(got, filepath.Separator) {
			t.Errorf("sanitize(%q) = %q contains the OS separator", in, got)
		}
	}
}

// FuzzSanitizeForFilename asserts the sanitizer always yields a single, non-empty path element that cannot escape the snapshot directory.
func FuzzSanitizeForFilename(f *testing.F) {
	f.Add("/home/u/.bashrc")
	f.Add("")
	f.Add("../../etc/passwd")
	f.Add("a\x00b")
	f.Fuzz(func(t *testing.T, in string) {
		got := sanitizeForFilename(in)
		if got == "" {
			t.Fatal("sanitize returned empty")
		}
		if strings.ContainsAny(got, `/\`) {
			t.Fatalf("sanitize(%q) = %q contains a separator", in, got)
		}
		if filepath.Base(got) != got {
			t.Fatalf("sanitize(%q) = %q is not a single path element", in, got)
		}
	})
}

func TestWriteSnapshot_CannotEscapeDirectory(t *testing.T) {
	dir := t.TempDir()
	// A hostile path must be flattened into dir, not traverse out of it.
	path, err := writeSnapshot(dir, "../../../../etc/passwd", []byte("x"), testTime())
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		t.Fatalf("snapshot escaped dir: path=%q rel=%q", path, rel)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("snapshot not written inside dir: %v", err)
	}
}
