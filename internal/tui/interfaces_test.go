package tui

import (
	"context"
	"testing"

	"github.com/balintb/chezmoui/internal/chezmoi"
)

// The full stub satisfies every narrow interface and the composite Backend.
var (
	_ Lister   = (*stubBackend)(nil)
	_ Reader   = (*stubBackend)(nil)
	_ Mutator  = (*stubBackend)(nil)
	_ RepoInfo = (*stubBackend)(nil)
	_ Backend  = (*stubBackend)(nil)
)

// listerOnly implements Lister and nothing else, proving that read paths do not depend on mutation or repo operations.
type listerOnly struct{ entries []chezmoi.Entry }

func (l listerOnly) Managed(context.Context) ([]chezmoi.Entry, error) { return l.entries, nil }
func (l listerOnly) Status(context.Context) ([]chezmoi.Status, error) { return nil, nil }

func TestLoadEntriesCmd_AcceptsListerOnly(t *testing.T) {
	cmd := loadEntriesCmd(listerOnly{entries: []chezmoi.Entry{{Target: ".a", Absolute: "/a"}}})
	msg := cmd()
	loaded, ok := msg.(entriesLoadedMsg)
	if !ok {
		t.Fatalf("want entriesLoadedMsg, got %T", msg)
	}
	if len(loaded.rows) != 1 || loaded.rows[0].target != ".a" {
		t.Errorf("unexpected rows: %#v", loaded.rows)
	}
}

// readerOnly implements Reader and nothing else.
type readerOnly struct{ contents string }

func (r readerOnly) Cat(context.Context, string) (string, error) { return r.contents, nil }

func TestLoadSideCmd_AcceptsReaderOnly(t *testing.T) {
	cmd := loadSideCmd(readerOnly{contents: "target\n"}, "/x", ".x", func(string) ([]byte, error) {
		return []byte("live\n"), nil
	})
	msg := cmd()
	side, ok := msg.(sideLoadedMsg)
	if !ok {
		t.Fatalf("want sideLoadedMsg, got %T", msg)
	}
	if side.target != "target\n" || side.live != "live\n" {
		t.Errorf("unexpected side contents: %#v", side)
	}
}

// mutatorOnly implements Mutator and nothing else.
type mutatorOnly struct{ reAdded [][]string }

func (m *mutatorOnly) ReAdd(_ context.Context, paths ...string) error {
	m.reAdded = append(m.reAdded, paths)
	return nil
}
func (m *mutatorOnly) Apply(context.Context, ...string) error { return nil }

func TestReAddCmd_AcceptsMutatorOnly(t *testing.T) {
	m := &mutatorOnly{}
	msg := reAddCmd(m, []string{"/a", "/b"})()
	if _, ok := msg.(reAddDoneMsg); !ok {
		t.Fatalf("want reAddDoneMsg, got %T", msg)
	}
	if len(m.reAdded) != 1 || len(m.reAdded[0]) != 2 {
		t.Errorf("re-add not forwarded: %#v", m.reAdded)
	}
}
