package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// memFS is a tiny in-memory file layer for deterministic undo tests.
type memFS struct {
	files map[string][]byte
}

func newMemFS(files map[string][]byte) *memFS {
	m := &memFS{files: map[string][]byte{}}
	for k, v := range files {
		m.files[k] = append([]byte(nil), v...)
	}
	return m
}

func (m *memFS) read(path string) ([]byte, error) {
	data, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (m *memFS) write(path string, data []byte, _ os.FileMode) error {
	m.files[path] = append([]byte(nil), data...)
	return nil
}

func undoModel(t *testing.T, fs *memFS) Model {
	t.Helper()
	b := sampleBackend()
	m := loadedModel(t, b)
	m = m.WithReadFile(fs.read).WithWriteFile(fs.write)
	m.session.snapshotDir = t.TempDir()
	m.now = fixedClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return m
}

func TestBuildUndo_ApplySnapshotsLive(t *testing.T) {
	fs := newMemFS(map[string][]byte{"/home/u/.config/btop/btop.conf": []byte("live A")})
	m := undoModel(t, fs)
	entries := m.buildUndo(opApply, []string{"/home/u/.config/btop/btop.conf"})
	if len(entries) != 1 {
		t.Fatalf("want 1 undo entry, got %d", len(entries))
	}
	e := entries[0]
	if e.action != undoRestoreLive {
		t.Errorf("apply undo must restore live")
	}
	if e.restorePath != "/home/u/.config/btop/btop.conf" {
		t.Errorf("restorePath = %q", e.restorePath)
	}
	data, err := os.ReadFile(e.snapshotPath)
	if err != nil {
		t.Fatalf("snapshot unreadable: %v", err)
	}
	if string(data) != "live A" {
		t.Errorf("snapshot = %q, want live A", data)
	}
}

func TestBuildUndo_ReAddSnapshotsSource(t *testing.T) {
	fs := newMemFS(map[string][]byte{"/src/private_dot_bashrc": []byte("source A")})
	m := undoModel(t, fs)
	m = cursorTo(t, m, ".bashrc")
	abs := "/home/u/.bashrc"
	entries := m.buildUndo(opReAdd, []string{abs})
	if len(entries) != 1 {
		t.Fatalf("want 1 undo entry, got %d", len(entries))
	}
	e := entries[0]
	if e.action != undoRestoreSource {
		t.Errorf("re-add undo must restore source")
	}
	if e.restorePath != "/src/private_dot_bashrc" {
		t.Errorf("restorePath = %q, want the source abs path", e.restorePath)
	}
	data, _ := os.ReadFile(e.snapshotPath)
	if string(data) != "source A" {
		t.Errorf("snapshot = %q", data)
	}
}

func TestUndo_RestoresAndIsItselfReversible(t *testing.T) {
	fs := newMemFS(map[string][]byte{"/home/u/.config/btop/btop.conf": []byte("live A")})
	m := undoModel(t, fs)
	abs := "/home/u/.config/btop/btop.conf"

	// Simulate an apply that overwrote the live file.
	m.pushUndo(m.buildUndo(opApply, []string{abs}))
	fs.files[abs] = []byte("source B")

	m, _ = press(t, m, 'u')
	if m.confirmMsg == "" {
		t.Fatal("u should raise a confirm prompt")
	}
	if m.pendingOp != opUndo {
		t.Fatalf("pendingOp should be opUndo, got %v", m.pendingOp)
	}
	m, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	msg := runCmd(t, cmd)
	done, ok := msg.(undoDoneMsg)
	if !ok {
		t.Fatalf("want undoDoneMsg, got %T (%v)", msg, msg)
	}
	m, _ = applyMsg(t, m, msg)

	if string(fs.files[abs]) != "live A" {
		t.Errorf("live after undo = %q, want live A", fs.files[abs])
	}
	if !done.hasPush {
		t.Fatal("undo should push a redo entry")
	}
	if m.undoDepth() != 1 {
		t.Fatalf("redo entry should remain on the stack, depth=%d", m.undoDepth())
	}

	// Redo: applying the pushed entry should restore "source B".
	m, _ = press(t, m, 'u')
	m, cmd = applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m, _ = applyMsg(t, m, runCmd(t, cmd))
	if string(fs.files[abs]) != "source B" {
		t.Errorf("after redo live = %q, want source B", fs.files[abs])
	}
}

func TestUndo_EmptyStackIsNoOp(t *testing.T) {
	fs := newMemFS(nil)
	m := undoModel(t, fs)
	m, cmd := press(t, m, 'u')
	if cmd != nil {
		t.Errorf("undo on empty stack should be a no-op, got cmd")
	}
	if m.confirmMsg != "" {
		t.Errorf("no confirm should be raised, got %q", m.confirmMsg)
	}
	if !strings.Contains(m.status, "nothing to undo") {
		t.Errorf("status = %q", m.status)
	}
}

func TestUndo_ConfirmCancelKeepsStack(t *testing.T) {
	fs := newMemFS(map[string][]byte{"/a": []byte("x")})
	m := undoModel(t, fs)
	entries := m.buildUndo(opApply, []string{"/a"})
	m.pushUndo(entries)
	before := m.undoDepth()
	m, _ = press(t, m, 'u')
	m, _ = press(t, m, 'n')
	if m.undoDepth() != before {
		t.Errorf("cancel must not pop the stack: %d -> %d", before, m.undoDepth())
	}
	if len(m.undo) != 1 || m.undo[0].snapshotPath != entries[0].snapshotPath {
		t.Errorf("stack contents changed after cancel")
	}
}

func TestUndo_RestoreErrorSurfaces(t *testing.T) {
	fs := newMemFS(map[string][]byte{"/a": []byte("x")})
	m := undoModel(t, fs)
	m.pushUndo(m.buildUndo(opApply, []string{"/a"}))
	m, _ = press(t, m, 'u')

	// Delete the snapshot so the restore fails to read it.
	entry := m.pendingUndo
	if err := os.Remove(entry.snapshotPath); err != nil {
		t.Fatal(err)
	}
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	msg := runCmd(t, cmd)
	if _, ok := msg.(errMsg); !ok {
		t.Fatalf("want errMsg from failed restore, got %T", msg)
	}
}

func TestUndo_WriteErrorSurfaces(t *testing.T) {
	fs := newMemFS(map[string][]byte{"/a": []byte("x")})
	m := undoModel(t, fs)
	m = m.WithWriteFile(func(string, []byte, os.FileMode) error { return errors.New("disk full") })
	m.pushUndo(m.buildUndo(opApply, []string{"/a"}))
	m, _ = press(t, m, 'u')
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if _, ok := runCmd(t, cmd).(errMsg); !ok {
		t.Fatal("write failure should surface as an error")
	}
}

func TestUndo_StackCappedAtMax(t *testing.T) {
	fs := newMemFS(map[string][]byte{"/a": []byte("x")})
	m := undoModel(t, fs)
	for i := 0; i < 150; i++ {
		m.pushUndo(m.buildUndo(opApply, []string{"/a"}))
	}
	if m.undoDepth() > 100 {
		t.Errorf("undo stack should be capped at 100, got %d", m.undoDepth())
	}
}

func TestWriteFileAtomic_NoLeftoverTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := writeFileAtomic(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "hello" {
		t.Fatalf("content = %q, err = %v", got, err)
	}
	if _, err := os.Stat(path + ".chezmoui.tmp"); err == nil {
		t.Error("temp file should not remain after a successful write")
	}
}

func TestUndo_RestorePreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "secret")
	if err := os.WriteFile(live, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := newMemFS(map[string][]byte{live: []byte("pre")})
	m := undoModel(t, fs)
	m = m.WithWriteFile(writeFileAtomic)
	m.pushUndo(m.buildUndo(opApply, []string{live}))
	m, _ = press(t, m, 'u')
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if msg := runCmd(t, cmd); msg != nil {
		if e, ok := msg.(errMsg); ok {
			t.Fatalf("restore errored: %v", e.err)
		}
	}
	info, err := os.Stat(live)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", info.Mode().Perm())
	}
	data, _ := os.ReadFile(live)
	if string(data) != "pre" {
		t.Errorf("restored content = %q", data)
	}
}

func TestUndoEntry_Label(t *testing.T) {
	if got := (undoEntry{op: "apply"}).label(); got != "apply" {
		t.Errorf("label without target = %q", got)
	}
	if got := (undoEntry{op: "apply", target: "/x"}).label(); got != "apply /x" {
		t.Errorf("label with target = %q", got)
	}
}

func TestUndo_StagedEntryMismatchAborts(t *testing.T) {
	fs := newMemFS(map[string][]byte{"/a": []byte("x")})
	m := undoModel(t, fs)
	m.pushUndo(m.buildUndo(opApply, []string{"/a"}))
	m, _ = press(t, m, 'u')

	// Push a new entry on top, then confirm: the staged entry no longer matches.
	m.pushUndo(m.buildUndo(opApply, []string{"/a"}))
	before := m.undoDepth()
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd != nil {
		t.Error("mismatched staged entry should not run a restore command")
	}
	// The popped entry is put back, so the stack is unchanged.
	if m.undoDepth() != before {
		t.Errorf("mismatch must leave the stack intact: %d -> %d", before, m.undoDepth())
	}
}

func TestSnapshotPaths_SkipsUnreadable(t *testing.T) {
	fs := newMemFS(map[string][]byte{"/ok": []byte("data")})
	m := undoModel(t, fs)
	entries := m.buildUndo(opApply, []string{"/ok", "/missing", "/also-missing"})
	if len(entries) != 1 {
		t.Fatalf("unreadable paths should be skipped, got %d entries", len(entries))
	}
	if entries[0].restorePath != "/ok" {
		t.Errorf("wrong entry kept: %#v", entries[0])
	}
}

func TestWriteFileAtomic_ErrorWhenDirMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "file")
	if err := writeFileAtomic(path, []byte("x"), 0o644); err == nil {
		t.Fatal("expected error writing into a missing directory")
	}
}
