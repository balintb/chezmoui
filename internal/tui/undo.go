package tui

import (
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// undoAction identifies how a snapshot should be restored.
type undoAction int

const (
	undoRestoreLive undoAction = iota
	undoRestoreSource
)

// undoEntry is a reversible record of a single file mutation.
type undoEntry struct {
	op           string
	target       string
	restorePath  string
	snapshotPath string
	action       undoAction
}

func (e undoEntry) label() string {
	if e.target == "" {
		return e.op
	}
	return e.op + " " + e.target
}

// snapshotPaths captures the current bytes of each path before a mutation. Paths that cannot be read (missing, directories) are skipped rather than failing the whole action. action selects how each snapshot is later restored.
func snapshotPaths(dir string, paths []string, action undoAction, op string, reader func(string) ([]byte, error), now func() time.Time) []undoEntry {
	var entries []undoEntry
	for _, p := range paths {
		data, err := reader(p)
		if err != nil {
			continue
		}
		sp, err := writeSnapshot(dir, p, data, now())
		if err != nil {
			continue
		}
		entries = append(entries, undoEntry{
			op:           op,
			target:       p,
			restorePath:  p,
			snapshotPath: sp,
			action:       action,
		})
	}
	return entries
}

// buildUndo snapshots the state that a mutation is about to overwrite. For an apply the live file is captured (restore to live); for a re-add the source file is captured (restore to source). Rows are looked up by absolute path so source paths are available for re-add.
func (m Model) buildUndo(op pendingOp, paths []string) []undoEntry {
	dir := m.session.snapshotDir
	if dir == "" {
		dir = defaultSnapshotDir(m.cacheDir)
	}
	srcByPath := map[string]string{}
	for _, r := range m.rows {
		if r.sourceAbs != "" {
			srcByPath[r.absolute] = r.sourceAbs
		}
	}
	var entries []undoEntry
	switch op {
	case opApply:
		entries = snapshotPaths(dir, paths, undoRestoreLive, "apply", m.readFile, m.now)
	case opReAdd:
		for _, p := range paths {
			src := srcByPath[p]
			if src == "" {
				continue
			}
			entries = append(entries, snapshotPaths(dir, []string{src}, undoRestoreSource, "re-add", m.readFile, m.now)...)
		}
	}
	return entries
}

// undoRestore removes the top of the undo stack and returns a command that restores its snapshot. The removed entry is restored to the stack by undoDoneMsg when the restore succeeds, making undo itself reversible.
func (m *Model) undoRestore(entry undoEntry) tea.Cmd {
	e, ok := m.popUndo()
	if !ok || e.snapshotPath != entry.snapshotPath {
		// The staged entry no longer matches the top; restore it and abort.
		if ok {
			m.undo = append(m.undo, e)
		}
		m.status = "undo stack changed"
		return nil
	}
	dir := m.session.snapshotDir
	if dir == "" {
		dir = defaultSnapshotDir(m.cacheDir)
	}
	return undoRestoreCmd(e, dir, m.readFile, m.writeFile, m.now)
}

// beginUndo stages the most recent undo entry and raises the confirm prompt. It is a no-op (with an explanatory status) when the stack is empty.
func (m Model) beginUndo() (Model, tea.Cmd) {
	e, ok := m.peekUndo()
	if !ok {
		m.status = "nothing to undo"
		return m, nil
	}
	m.pendingOp = opUndo
	m.pendingUndo = e
	m.confirmMsg = "Undo " + e.label() + "?"
	return m, nil
}

// peekUndo returns the most recent entry without removing it.
func (m Model) peekUndo() (undoEntry, bool) {
	if len(m.undo) == 0 {
		return undoEntry{}, false
	}
	return m.undo[len(m.undo)-1], true
}

// pushUndo appends entries, keeping the newest last.
func (m *Model) pushUndo(entries []undoEntry) {
	m.undo = append(m.undo, entries...)
	const maxUndo = 100
	if len(m.undo) > maxUndo {
		m.undo = m.undo[len(m.undo)-maxUndo:]
	}
}

// popUndo removes and returns the most recent undo entry.
func (m *Model) popUndo() (undoEntry, bool) {
	if len(m.undo) == 0 {
		return undoEntry{}, false
	}
	e := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	return e, true
}

func (m *Model) undoDepth() int { return len(m.undo) }

// undoDoneMsg reports a completed restore plus an optional new undo entry that makes the restore itself reversible.
type undoDoneMsg struct {
	restored string
	pushed   undoEntry
	hasPush  bool
}

// undoRestoreCmd restores one undo entry by copying its snapshot back to the restore path. Before overwriting, the current contents are snapshotted so the undo is itself reversible.
func undoRestoreCmd(entry undoEntry, dir string, reader func(string) ([]byte, error), writer func(string, []byte, os.FileMode) error, now func() time.Time) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(entry.snapshotPath)
		if err != nil {
			return errMsg{err: opError{Op: "read snapshot", Target: entry.snapshotPath, Err: err}}
		}
		mode := os.FileMode(0o644)
		if info, statErr := os.Stat(entry.restorePath); statErr == nil {
			mode = info.Mode().Perm()
		}
		done := undoDoneMsg{restored: entry.restorePath}
		if pre, rerr := reader(entry.restorePath); rerr == nil {
			if p, serr := writeSnapshot(dir, entry.restorePath, pre, now()); serr == nil {
				done.pushed = undoEntry{
					op:           "undo " + entry.op,
					target:       entry.target,
					restorePath:  entry.restorePath,
					snapshotPath: p,
					action:       entry.action,
				}
				done.hasPush = true
			}
		}
		if err := writer(entry.restorePath, data, mode); err != nil {
			return errMsg{err: opError{Op: "restore", Target: entry.restorePath, Err: err}}
		}
		return done
	}
}

// writeFileAtomic writes data to path via a temp file and rename so a crash mid-write cannot leave a truncated dotfile.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".chezmoui.tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
