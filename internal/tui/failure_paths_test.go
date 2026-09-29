package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/balintb/chezmoui/internal/chezmoi"
)

func TestWriteSnapshot_MkdirFails(t *testing.T) {
	// A regular file where a directory is expected makes MkdirAll fail.
	parent := t.TempDir()
	file := filepath.Join(parent, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := writeSnapshot(file, "/a/b", []byte("data"), testTime()); err == nil {
		t.Fatal("expected MkdirAll failure when the snapshot root is a file")
	}
}

func TestKeepLiveCmd_ErrorCarriesRetry(t *testing.T) {
	b := sampleBackend()
	b.reAddErr = errors.New("read-only source")
	msg := keepLiveCmd(b, 3, "/home/u/.bashrc")()
	e, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("want errMsg, got %T", msg)
	}
	var oe opError
	if !errors.As(e.err, &oe) || oe.Op != "keep live (re-add)" || oe.Target != "/home/u/.bashrc" {
		t.Errorf("unexpected opError: %#v", e)
	}
	if e.retry == nil {
		t.Error("failed keep must carry a retry")
	}
}

func TestRevertCmd_ApplyErrorCarriesRetry(t *testing.T) {
	b := sampleBackend()
	b.applyErr = errors.New("conflict")
	dir := t.TempDir()
	msg := revertCmd(b, 1, "/home/u/.bashrc", dir, func(string) ([]byte, error) {
		return []byte("live"), nil
	}, fixedClock(testTime()))()
	e, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("want errMsg, got %T", msg)
	}
	var oe opError
	if !errors.As(e.err, &oe) || oe.Op != "revert to source" {
		t.Errorf("unexpected opError: %#v", e)
	}
	if e.retry == nil {
		t.Error("failed revert must carry a retry")
	}
	// A snapshot is still taken before the failed apply, so the live file is recoverable.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Error("expected a pre-apply snapshot even when apply fails")
	}
}

func TestLoadSessionEntryCmd_LiveReadError(t *testing.T) {
	b := sampleBackend()
	b.cat = "target\n"
	reader := func(string) ([]byte, error) {
		return nil, &os.PathError{Op: "open", Path: "x", Err: os.ErrPermission}
	}
	msg := loadSessionEntryCmd(b, 0, "/home/u/.bashrc", reader)()
	e, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("want errMsg, got %T", msg)
	}
	if !strings.Contains(e.err.Error(), "read live file") {
		t.Errorf("unexpected error: %v", e.err)
	}
	if e.retry == nil {
		t.Error("failed session entry load must carry a retry")
	}
}

func TestLoadEntriesCmd_ManagedError(t *testing.T) {
	msg := loadEntriesCmd(failingLister{err: errors.New("nope")})()
	e, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("want errMsg, got %T", msg)
	}
	var oe opError
	if !errors.As(e.err, &oe) || oe.Op != "list managed targets" {
		t.Errorf("unexpected error: %v", e.err)
	}
}

type failingLister struct{ err error }

func (f failingLister) Managed(context.Context) ([]chezmoi.Entry, error) { return nil, f.err }
func (f failingLister) Status(context.Context) ([]chezmoi.Status, error) { return nil, f.err }
