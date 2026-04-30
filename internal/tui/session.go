package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type sessionState int

const (
	sessionInactive sessionState = iota
	sessionWelcome
	sessionReview
	sessionSummary
)

type sessionDecision int

const (
	decisionPending sessionDecision = iota
	decisionKept
	decisionReverted
	decisionSkipped
)

type sessionEntry struct {
	target          string
	absolute        string
	target_contents string
	live_contents   string
	liveMissing     bool
	loaded          bool
	added, removed  int
	loose           int
	decision        sessionDecision
	snapshotPath    string
}

type sessionData struct {
	state   sessionState
	queue   []sessionEntry
	cursor  int
	working bool

	sourceRepoPath string
	gitBranch      string
	gitChanged     int

	snapshotDir string
}

func (s sessionData) entry() (sessionEntry, bool) {
	if s.cursor < 0 || s.cursor >= len(s.queue) {
		return sessionEntry{}, false
	}
	return s.queue[s.cursor], true
}

func (s sessionData) summary() (kept, reverted, skipped, pending int) {
	for _, e := range s.queue {
		switch e.decision {
		case decisionKept:
			kept++
		case decisionReverted:
			reverted++
		case decisionSkipped:
			skipped++
		default:
			pending++
		}
	}
	return
}

type sessionStartedMsg struct {
	queue          []sessionEntry
	sourceRepoPath string
	gitBranch      string
	gitChanged     int
}
type sessionEntryLoadedMsg struct {
	cursor      int
	target      string
	live        string
	liveMissing bool
	added       int
	removed     int
	loose       int
}
type sessionDecisionDoneMsg struct {
	cursor       int
	decision     sessionDecision
	snapshotPath string
}

func startSessionCmd(cli Backend, rows []row, snapshotDir string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var queue []sessionEntry
		for _, r := range rows {
			if !r.modified() || r.isDir {
				continue
			}
			queue = append(queue, sessionEntry{target: r.target, absolute: r.absolute})
		}
		srcPath, _ := cli.SourcePath(ctx)
		gs, _ := cli.GitStatus(ctx)
		return sessionStartedMsg{
			queue:          queue,
			sourceRepoPath: srcPath,
			gitBranch:      gs.Branch,
			gitChanged:     gs.Staged + gs.Unstaged + gs.Untracked,
		}
	}
}

func loadSessionEntryCmd(cli Backend, cursor int, absPath string, reader func(string) ([]byte, error)) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		target, err := cli.Cat(ctx, absPath)
		if err != nil {
			return errMsg{err}
		}
		var live []byte
		var liveMissing bool
		live, err = reader(absPath)
		switch {
		case err == nil:
		case os.IsNotExist(err), isDirReadError(err):
			liveMissing = true
		default:
			return errMsg{err}
		}
		rows := alignLines(target, string(live))
		added, removed, loose := summarizeAlignment(rows)
		return sessionEntryLoadedMsg{
			cursor:      cursor,
			target:      target,
			live:        string(live),
			liveMissing: liveMissing,
			added:       added,
			removed:     removed,
			loose:       loose,
		}
	}
}

func keepLiveCmd(cli Backend, cursor int, absPath string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := cli.ReAdd(ctx, absPath); err != nil {
			return errMsg{err}
		}
		return sessionDecisionDoneMsg{cursor: cursor, decision: decisionKept}
	}
}

func revertCmd(cli Backend, cursor int, absPath, snapshotDir string, reader func(string) ([]byte, error)) tea.Cmd {
	return func() tea.Msg {
		var snapshot string
		if data, err := reader(absPath); err == nil {
			if p, err := writeSnapshot(snapshotDir, absPath, data); err == nil {
				snapshot = p
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := cli.Apply(ctx, absPath); err != nil {
			return errMsg{err}
		}
		return sessionDecisionDoneMsg{cursor: cursor, decision: decisionReverted, snapshotPath: snapshot}
	}
}

func writeSnapshot(dir, absPath string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format("20060102T150405")
	name := stamp + "_" + sanitizeForFilename(absPath)
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, data, 0o600); err != nil {
		return "", err
	}
	return full, nil
}

func sanitizeForFilename(p string) string {
	r := strings.NewReplacer(
		string(filepath.Separator), "_",
		"/", "_",
		"\\", "_",
		":", "_",
		" ", "_",
	)
	out := r.Replace(p)
	out = strings.TrimLeft(out, "_")
	if out == "" {
		out = "file"
	}
	return out
}

func defaultSnapshotDir() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	return filepath.Join(cache, "chezmoui", "recoverable")
}

func (m Model) viewSessionWelcome() string {
	counts := m.categoryCounts()
	mod := counts[catModified]
	var b strings.Builder
	b.WriteString(titleStyle.Render("Sync Session") + "\n\n")
	if mod == 0 {
		b.WriteString("No drift to review — your dotfiles are in sync.\n\n")
		b.WriteString(mutedStyle.Render("[ esc ] back to file list"))
		return b.String()
	}
	b.WriteString(fmt.Sprintf("You have %s with drift.\n\n", titleStyle.Render(fmt.Sprintf("%d files", mod))))
	b.WriteString("For each file, choose what to do with your live edits:\n\n")
	b.WriteString("  " + addStyle.Render("●") + "  keep live    — re-add into source (your edits win)\n")
	b.WriteString("  " + delStyle.Render("●") + "  revert       — chezmoi apply (source wins; backup kept)\n")
	b.WriteString("  " + mutedStyle.Render("●") + "  skip         — defer until next time\n\n")
	b.WriteString(statusStyle.Render("[ enter ]") + " start    " + statusStyle.Render("[ esc ]") + " back")
	return b.String()
}

func (m Model) viewSessionReview() string {
	e, ok := m.session.entry()
	if !ok {
		return mutedStyle.Render("session: queue empty")
	}
	kept, reverted, skipped, pending := m.session.summary()
	done := kept + reverted + skipped
	header := fmt.Sprintf("%s %d of %d — %s",
		titleStyle.Render("Reviewing"),
		m.session.cursor+1,
		len(m.session.queue),
		titleStyle.Render(e.target),
	)
	progress := mutedStyle.Render(fmt.Sprintf("%d done · %d left", done, pending))
	summaryChip := ""
	if e.loaded {
		summaryChip = "  " + addStyle.Render(fmt.Sprintf("+%d", e.added)) +
			"  " + delStyle.Render(fmt.Sprintf("−%d", e.removed))
		if e.loose > 0 {
			summaryChip += "  " + looseLineStyle.Render(fmt.Sprintf(" ≈%d ", e.loose))
		}
	}
	looseBanner := ""
	if e.loaded && e.added == 0 && e.removed == 0 && e.loose > 0 {
		looseBanner = "\n" + looseLineStyle.Render(
			" ≈ logically identical — only case/whitespace differs ")
	}

	var body string
	switch {
	case m.session.working:
		body = mutedStyle.Render("\n  applying…\n")
	case !e.loaded:
		body = mutedStyle.Render("\n  loading diff…\n")
	default:
		body = m.vp.View()
	}

	actionBar := strings.Join([]string{
		statusStyle.Render("[k]") + " keep live",
		statusStyle.Render("[v]") + " revert",
		statusStyle.Render("[s]") + " skip",
		mutedStyle.Render("[←]") + " " + mutedStyle.Render("back"),
		mutedStyle.Render("[q/esc]") + " " + mutedStyle.Render("end session"),
	}, "   ")

	return lipgloss.JoinVertical(lipgloss.Left,
		header+summaryChip+"   "+progress+looseBanner,
		body,
		actionBar,
	)
}

func (m Model) viewSessionSummary() string {
	kept, reverted, skipped, _ := m.session.summary()
	var b strings.Builder
	b.WriteString(titleStyle.Render("Session complete") + "\n\n")
	b.WriteString(fmt.Sprintf("  %s %d promoted to source\n", addStyle.Render("✓"), kept))
	b.WriteString(fmt.Sprintf("  %s %d reverted to source\n", delStyle.Render("↺"), reverted))
	b.WriteString(fmt.Sprintf("  %s %d skipped — still drifted\n", mutedStyle.Render("⏭"), skipped))
	if reverted > 0 {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("  backups in " + m.session.snapshotDir))
		b.WriteString("\n")
	}
	if m.session.sourceRepoPath != "" {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("Source repo: ") + m.session.sourceRepoPath + "\n")
		if m.session.gitBranch != "" {
			b.WriteString(mutedStyle.Render("  branch: "))
			b.WriteString(m.session.gitBranch)
			if m.session.gitChanged > 0 {
				b.WriteString(mutedStyle.Render(fmt.Sprintf(" · %d files changed", m.session.gitChanged)))
			} else {
				b.WriteString("  " + addStyle.Render("(clean)"))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(statusStyle.Render("[enter]") + " back to file list   " + statusStyle.Render("[q]") + " quit")
	return b.String()
}

func (m Model) sessionFooter() string {
	if m.session.sourceRepoPath == "" || m.session.gitBranch == "" {
		return ""
	}
	chip := mutedStyle.Render("repo · ") + m.session.gitBranch
	if m.session.gitChanged > 0 {
		chip += "  " + modifiedStyle.Render(fmt.Sprintf("%d uncommitted", m.session.gitChanged))
	} else {
		chip += "  " + addStyle.Render("clean")
	}
	return chip
}
