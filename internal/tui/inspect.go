package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/balintb/chezmoui/internal/chezmoi"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type unmanagedLoadedMsg struct{ paths []string }
type ignoredLoadedMsg struct{ paths []string }
type doctorLoadedMsg struct{ checks []chezmoi.DoctorCheck }
type addDoneMsg struct {
	count int
	paths []string
}

func loadUnmanagedCmd(e Enumer) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		paths, err := e.Unmanaged(ctx)
		if err != nil {
			return errMsg{err: opError{Op: "list unmanaged", Err: err}, retry: loadUnmanagedCmd(e)}
		}
		return unmanagedLoadedMsg{paths: paths}
	}
}

func loadIgnoredCmd(e Enumer) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		paths, err := e.Ignored(ctx)
		if err != nil {
			return errMsg{err: opError{Op: "list ignored", Err: err}, retry: loadIgnoredCmd(e)}
		}
		return ignoredLoadedMsg{paths: paths}
	}
}

func loadDoctorCmd(d Diagnoser) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		checks, err := d.Doctor(ctx)
		if err != nil {
			return errMsg{err: opError{Op: "run doctor", Err: err}, retry: loadDoctorCmd(d)}
		}
		return doctorLoadedMsg{checks: checks}
	}
}

func addCmd(cli Mutator, paths []string) tea.Cmd {
	return func() tea.Msg {
		retry := addCmd(cli, paths)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := cli.Add(ctx, paths...); err != nil {
			return errMsg{err: opError{Op: "add", Target: strings.Join(paths, ", "), Err: err}, retry: retry}
		}
		return addDoneMsg{count: len(paths), paths: paths}
	}
}

// loadTabCmd returns the load command for an inspect tab, or nil when the tab is not data-backed.
func (m *Model) loadTabCmd(t tabID) tea.Cmd {
	switch t {
	case tabUnmanaged:
		return loadUnmanagedCmd(m.cli)
	case tabIgnored:
		return loadIgnoredCmd(m.cli)
	case tabDoctor:
		return loadDoctorCmd(m.cli)
	}
	return nil
}

// ensureTabLoaded fetches the tab's data the first time it is shown.
func (m *Model) ensureTabLoaded(t tabID) tea.Cmd {
	if m.inspectLoaded[t] {
		return nil
	}
	m.inspectLoaded[t] = true
	return m.loadTabCmd(t)
}

func (m Model) currentInspectList() []string {
	switch m.activeTab {
	case tabUnmanaged:
		return m.unmanaged
	case tabIgnored:
		return m.ignored
	}
	return nil
}

func (m Model) inspectCursor() int {
	switch m.activeTab {
	case tabUnmanaged:
		return m.unmanagedCursor
	case tabIgnored:
		return m.ignoredCursor
	}
	return 0
}

func (m *Model) setInspectCursor(v int) {
	switch m.activeTab {
	case tabUnmanaged:
		m.unmanagedCursor = v
	case tabIgnored:
		m.ignoredCursor = v
	}
}

// updateInspect handles keys on the unmanaged/ignored/doctor tabs.
func (m Model) updateInspect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.activeTab {
	case tabDoctor:
		return m.updateDoctor(msg)
	case tabUnmanaged, tabIgnored:
		return m.updateInspectList(msg)
	}
	return m, nil
}

func (m Model) updateInspectList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.currentInspectList()
	n := len(items)
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Up):
		if c := m.inspectCursor(); c > 0 {
			m.setInspectCursor(c - 1)
		}
	case key.Matches(msg, keys.Down):
		if c := m.inspectCursor(); c < n-1 {
			m.setInspectCursor(c + 1)
		}
	case key.Matches(msg, keys.PgUp):
		m.setInspectCursor(max(0, m.inspectCursor()-10))
	case key.Matches(msg, keys.PgDown):
		m.setInspectCursor(min(max(0, n-1), m.inspectCursor()+10))
	case key.Matches(msg, keys.Home):
		m.setInspectCursor(0)
	case key.Matches(msg, keys.End):
		m.setInspectCursor(max(0, n-1))
	case key.Matches(msg, keys.Refresh):
		m.inspectLoaded[m.activeTab] = false
		return m, m.ensureTabLoaded(m.activeTab)
	}
	if m.activeTab == tabUnmanaged {
		switch {
		case key.Matches(msg, keys.Toggle):
			if c := m.inspectCursor(); c >= 0 && c < n {
				p := items[c]
				if m.unmanagedSel[p] {
					delete(m.unmanagedSel, p)
				} else {
					m.unmanagedSel[p] = true
				}
			}
		case key.Matches(msg, keys.Add):
			paths := m.addPaths()
			if len(paths) == 0 {
				m.status = "nothing to add"
				return m, nil
			}
			m.pendingPaths = paths
			m.pendingOp = opAdd
			m.confirmMsg = fmt.Sprintf("Add %d file(s) to source?", len(paths))
		}
	}
	return m, nil
}

// addPaths returns the selected unmanaged paths, or the cursor row when nothing is selected.
func (m Model) addPaths() []string {
	var paths []string
	if len(m.unmanagedSel) > 0 {
		for _, p := range m.unmanaged {
			if m.unmanagedSel[p] {
				paths = append(paths, p)
			}
		}
		return paths
	}
	if c := m.unmanagedCursor; c >= 0 && c < len(m.unmanaged) {
		paths = append(paths, m.unmanaged[c])
	}
	return paths
}

func (m Model) updateDoctor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.doctor)
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Up):
		if m.doctorCursor > 0 {
			m.doctorCursor--
		}
	case key.Matches(msg, keys.Down):
		if m.doctorCursor < n-1 {
			m.doctorCursor++
		}
	case key.Matches(msg, keys.Refresh):
		m.inspectLoaded[tabDoctor] = false
		return m, m.ensureTabLoaded(tabDoctor)
	}
	return m, nil
}

// scrollInspect moves the cursor on an inspect tab in response to the wheel.
func (m Model) scrollInspect(delta int) (tea.Model, tea.Cmd) {
	if m.activeTab == tabDoctor {
		m.doctorCursor = clampCursor(m.doctorCursor+delta, len(m.doctor))
		return m, nil
	}
	items := m.currentInspectList()
	m.setInspectCursor(clampCursor(m.inspectCursor()+delta, len(items)))
	return m, nil
}

func clampCursor(v, n int) int {
	if v < 0 {
		return 0
	}
	if v > n-1 {
		return max(0, n-1)
	}
	return v
}

func (m Model) viewInspect() string {
	switch m.activeTab {
	case tabUnmanaged:
		return m.viewPathList("Unmanaged", m.unmanaged, m.unmanagedCursor, m.unmanagedSel, true,
			"unmanaged files — press space to select, A to add to source")
	case tabIgnored:
		return m.viewPathList("Ignored", m.ignored, m.ignoredCursor, nil, false,
			"ignored by chezmoi (see .chezmoiignore)")
	case tabDoctor:
		return m.viewDoctor()
	}
	return ""
}

func (m Model) viewPathList(title string, items []string, cursor int, selected map[string]bool, selectable bool, hint string) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(title) + "  " + mutedStyle.Render(fmt.Sprintf("%d", len(items))))
	b.WriteString("\n\n")

	_, bh := m.bodyDims()
	height := bh - 4
	if height < 5 {
		height = 5
	}

	if len(items) == 0 {
		b.WriteString(mutedStyle.Render("  (none)"))
	} else {
		b.WriteString(renderWindow(len(items), cursor, height, func(i int) string {
			mark := " "
			if selectable && selected[items[i]] {
				mark = selectedMark.Render("✓")
			}
			line := "  " + mark + " " + shortenHome(items[i])
			if i == cursor {
				return cursorStyle.Render(padTo(line, m.bodyWidth()))
			}
			return line
		}))
	}

	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(mutedStyle.Render(m.status))
	} else {
		b.WriteString(mutedStyle.Render(hint))
	}
	b.WriteString("\n")
	b.WriteString(m.contextHelp())
	return b.String()
}

// renderWindow renders a scrolling slice of n items keeping cursor visible.
func renderWindow(n, cursor, height int, render func(int) string) string {
	start := 0
	if cursor >= height {
		start = cursor - height + 1
	}
	end := start + height
	if end > n {
		end = n
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		b.WriteString(render(i))
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) viewDoctor() string {
	var b strings.Builder
	ok, warn, bad := 0, 0, 0
	for _, c := range m.doctor {
		switch {
		case c.Result == "ok":
			ok++
		case c.Result == "info" || c.Result == "warning":
			warn++
		default:
			bad++
		}
	}
	summary := fmt.Sprintf("%s %d ok   %s %d warn   %s %d failed",
		addStyle.Render("●"), ok,
		looseLineStyle.Render("●"), warn,
		delStyle.Render("●"), bad)
	b.WriteString(titleStyle.Render("Doctor") + "  " + summary)
	b.WriteString("\n\n")

	_, bh := m.bodyDims()
	height := bh - 4
	if height < 5 {
		height = 5
	}

	if len(m.doctor) == 0 {
		b.WriteString(mutedStyle.Render("  (no checks)"))
	} else {
		b.WriteString(renderWindow(len(m.doctor), m.doctorCursor, height, func(i int) string {
			c := m.doctor[i]
			line := fmt.Sprintf("  %-8s %-24s %s", c.Result, c.Check, c.Message)
			if i == m.doctorCursor {
				return cursorStyle.Render(padTo(line, m.bodyWidth()))
			}
			return doctorResultStyle(c.Result).Render(fmt.Sprintf("  %-8s", c.Result)) +
				" " + fmt.Sprintf("%-24s %s", c.Check, c.Message)
		}))
	}

	b.WriteString("\n")
	b.WriteString(m.contextHelp())
	return b.String()
}

func doctorResultStyle(result string) lipgloss.Style {
	switch result {
	case "ok":
		return addStyle
	case "info", "warning":
		return looseLineStyle
	default:
		return delStyle
	}
}

func (m Model) bodyWidth() int {
	w, _ := m.bodyDims()
	return w
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
