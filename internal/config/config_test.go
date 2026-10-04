package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestStore_RoundTrip(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "config.json")}
	want := Config{RepoPath: "/tmp/dotfiles", RepoConfirmed: true, AllowGitWrite: true}
	if err := s.Save(want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != want {
		t.Errorf("round-trip mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

func TestLoad_MissingReturnsNotExist(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "nope.json")}
	_, err := s.Load()
	if err == nil {
		t.Fatal("expected error loading nonexistent config")
	}
	if !IsNotExist(err) {
		t.Errorf("missing config error should match IsNotExist, got %v", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing config error should wrap fs.ErrNotExist, got %v", err)
	}
}

func TestSave_AtomicReplace(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, "config.json")}
	if err := s.Save(Config{RepoPath: "/a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Config{RepoPath: "/b", RepoConfirmed: true}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoPath != "/b" || !got.RepoConfirmed {
		t.Errorf("second save did not replace: %+v", got)
	}
	tmpPath := s.Path + ".tmp"
	if _, err := s.Load(); err != nil {
		t.Fatal(err) // sanity
	}
	if _, err := os.Stat(tmpPath); err == nil {
		t.Errorf("expected no .tmp file after successful save, found %s", tmpPath)
	}
}

func TestSave_CreatesParentDir(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "nested", "deeper", "config.json")}
	if err := s.Save(Config{RepoPath: "/x"}); err != nil {
		t.Fatalf("save into nonexistent parent: %v", err)
	}
}

func TestDefaultStore_UsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/xdg")
	s := DefaultStore()
	if want := filepath.Join("/custom/xdg", "chezmoui", "config.json"); s.Path != want {
		t.Errorf("DefaultStore path = %q, want %q", s.Path, want)
	}
}

func TestDefaultDir_FallsBackToHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	got := defaultDir()
	if want := filepath.Join(home, ".config", "chezmoui"); got != want {
		t.Errorf("defaultDir = %q, want %q", got, want)
	}
}

func TestStore_Load_MalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{Path: path}
	if _, err := s.Load(); err == nil {
		t.Fatal("expected error for malformed config")
	}
}

func TestStore_Save_WriteError(t *testing.T) {
	// A file where a parent directory is expected makes MkdirAll fail.
	parent := t.TempDir()
	blocker := filepath.Join(parent, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{Path: filepath.Join(blocker, "config.json")}
	if err := s.Save(Config{}); err == nil {
		t.Fatal("expected save failure when parent is a file")
	}
}
