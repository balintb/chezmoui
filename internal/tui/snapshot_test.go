package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// testTime returns a stable timestamp for tests that only need determinism.
func testTime() time.Time {
	return time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
}

func TestWriteSnapshot_DeterministicNameFromClock(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	got, err := writeSnapshot(dir, "/home/u/.bashrc", []byte("x"), ts)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "20260304T050607.000000000_home_u_.bashrc")
	if got != want {
		t.Errorf("snapshot path = %q, want %q", got, want)
	}
}

func TestWriteSnapshot_SameInstantDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	p1, err := writeSnapshot(dir, "/home/u/.bashrc", []byte("first"), ts)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := writeSnapshot(dir, "/home/u/.bashrc", []byte("second"), ts)
	if err != nil {
		t.Fatal(err)
	}
	p3, err := writeSnapshot(dir, "/home/u/.bashrc", []byte("third"), ts)
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 || p2 == p3 || p1 == p3 {
		t.Fatalf("colliding timestamps must yield distinct paths: %q %q %q", p1, p2, p3)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 snapshots, got %d", len(entries))
	}

	for path, want := range map[string]string{p1: "first", p2: "second", p3: "third"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != want {
			t.Errorf("%s = %q, want %q", filepath.Base(path), data, want)
		}
	}
}

func TestRevertCmd_SnapshotUsesInjectedClock(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	sb := sampleBackend()
	sb.cat = "target\n"

	cmd := revertCmd(sb, 0, "/home/u/.bashrc", dir, func(string) ([]byte, error) {
		return []byte("live\n"), nil
	}, fixedClock(ts))

	msg := cmd()
	done, ok := msg.(sessionDecisionDoneMsg)
	if !ok {
		t.Fatalf("want sessionDecisionDoneMsg, got %T (%v)", msg, msg)
	}
	want := filepath.Join(dir, "20200102T030405.000000000_home_u_.bashrc")
	if done.snapshotPath != want {
		t.Errorf("snapshotPath = %q, want %q", done.snapshotPath, want)
	}
}

func TestDefaultSnapshotDir_UsesInjectedCacheDir(t *testing.T) {
	got := defaultSnapshotDir("/custom/cache")
	want := filepath.Join("/custom/cache", "chezmoui", "recoverable")
	if got != want {
		t.Errorf("defaultSnapshotDir = %q, want %q", got, want)
	}
	if def := defaultSnapshotDir(""); def == "" {
		t.Error("empty cache dir should fall back to the OS cache dir, not empty")
	}
}

func TestModel_WithNowAndCacheDir(t *testing.T) {
	ts := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	m := NewModel(sampleBackend()).WithNow(fixedClock(ts)).WithCacheDir("/tmp/custom")
	if got := m.now(); !got.Equal(ts) {
		t.Errorf("WithNow did not take effect: %v", got)
	}
	if m.cacheDir != "/tmp/custom" {
		t.Errorf("WithCacheDir did not take effect: %q", m.cacheDir)
	}
}
