package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/balintb/chezmoui/internal/chezmoi"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type repoStatusLoadedMsg struct{ status chezmoi.RepoStatus }
type gitActionDoneMsg struct {
	action string
}

func loadRepoStatusCmd(g GitRepo) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		status, err := g.RepoStatus(ctx)
		if err != nil {
			return errMsg{err: opError{Op: "read source status", Err: err}, retry: loadRepoStatusCmd(g)}
		}
		return repoStatusLoadedMsg{status: status}
	}
}

func gitCommitCmd(g GitRepo, message string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := g.GitCommit(ctx, message); err != nil {
			return errMsg{err: opError{Op: "commit", Err: err}, retry: gitCommitCmd(g, message)}
		}
		return gitActionDoneMsg{action: "committed"}
	}
}

func gitPushCmd(g GitRepo) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		if err := g.GitPush(ctx); err != nil {
			return errMsg{err: opError{Op: "push", Err: err}, retry: gitPushCmd(g)}
		}
		return gitActionDoneMsg{action: "pushed"}
	}
}

func newCommitInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = "commit: "
	ti.Placeholder = "describe your dotfiles change"
	ti.CharLimit = 200
	return ti
}

// ensureRepoStatus loads the source status on first visit to the Source tab.
func (m *Model) ensureRepoStatus() tea.Cmd {
	if m.repoLoaded {
		return nil
	}
	m.repoLoaded = true
	return loadRepoStatusCmd(m.cli)
}

func (m Model) updateSource(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.commitPrompt {
		return m.handleCommitPrompt(msg)
	}

	changes := m.repoStatus.Changes
	n := len(changes)
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Up):
		if m.repoCursor > 0 {
			m.repoCursor--
		}
	case key.Matches(msg, keys.Down):
		if m.repoCursor < n-1 {
			m.repoCursor++
		}
	case key.Matches(msg, keys.Refresh):
		m.repoLoaded = false
		return m, m.ensureRepoStatus()
	case key.Matches(msg, keys.Commit):
		return m.beginCommit()
	case key.Matches(msg, keys.Push):
		return m.beginPush()
	}
	return m, nil
}

func (m Model) beginCommit() (Model, tea.Cmd) {
	if !m.gitWrite {
		m.status = "git write disabled — set allow_git_write in config to enable"
		return m, nil
	}
	if m.repoStatus.Clean() {
		m.status = "nothing to commit"
		return m, nil
	}
	m.commitPrompt = true
	m.commitInput = newCommitInput()
	m.commitInput.Focus()
	return m, textinput.Blink
}

func (m Model) handleCommitPrompt(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.commitPrompt = false
		m.commitInput.Blur()
		return m, nil
	case tea.KeyEnter:
		message := strings.TrimSpace(m.commitInput.Value())
		if message == "" {
			return m, nil
		}
		m.commitPrompt = false
		m.commitInput.Blur()
		if m.repoStatus.Clean() {
			m.status = "nothing to commit"
			return m, nil
		}
		m.pendingOp = opCommit
		m.confirmMsg = fmt.Sprintf("Commit %d change(s)?", len(m.repoStatus.Changes))
		m.pendingCommitMsg = message
		return m, nil
	}
	var cmd tea.Cmd
	m.commitInput, cmd = m.commitInput.Update(msg)
	return m, cmd
}

func (m Model) beginPush() (Model, tea.Cmd) {
	if !m.gitWrite {
		m.status = "git write disabled — set allow_git_write in config to enable"
		return m, nil
	}
	if m.repoStatus.Branch == "" {
		m.status = "no branch to push"
		return m, nil
	}
	m.pendingOp = opPush
	m.confirmMsg = fmt.Sprintf("Push %s?", m.repoStatus.Branch)
	return m, nil
}

func (m Model) viewSource() string {
	rs := m.repoStatus
	var b strings.Builder

	header := titleStyle.Render("Source") + "  " + gitBranchChip(rs)
	b.WriteString(header)
	if !m.gitWrite {
		b.WriteString("  " + mutedStyle.Render("(read-only)"))
	}
	b.WriteString("\n\n")

	_, bh := m.bodyDims()
	height := bh - 5
	if height < 5 {
		height = 5
	}

	if len(rs.Changes) == 0 {
		b.WriteString(mutedStyle.Render("  working tree clean"))
	} else {
		b.WriteString(renderWindow(len(rs.Changes), m.repoCursor, height, func(i int) string {
			c := rs.Changes[i]
			line := fmt.Sprintf("  %-2s %-9s %s", c.XY, c.Label(), shortenHome(c.Path))
			if c.OrigPath != "" {
				line += mutedStyle.Render(" (from " + shortenHome(c.OrigPath) + ")")
			}
			if i == m.repoCursor {
				return cursorStyle.Render(padTo(line, m.bodyWidth()))
			}
			return line
		}))
	}

	b.WriteString("\n")
	if m.commitPrompt {
		b.WriteString(m.commitInput.View())
		b.WriteString("\n")
		return b.String()
	}
	if m.status != "" {
		b.WriteString(mutedStyle.Render(m.status))
	} else {
		b.WriteString(mutedStyle.Render("c commit · P push · R refresh"))
	}
	b.WriteString("\n")
	b.WriteString(m.contextHelp())
	return b.String()
}

// gitBranchChip renders the branch/tracking/ahead-behind summary.
func gitBranchChip(rs chezmoi.RepoStatus) string {
	if rs.Branch == "" {
		return mutedStyle.Render("no branch")
	}
	chip := statusStyle.Render(rs.Branch)
	if rs.Tracking != "" {
		chip += mutedStyle.Render(" → " + rs.Tracking)
	}
	if rs.Ahead > 0 {
		chip += "  " + addStyle.Render(fmt.Sprintf("↑%d", rs.Ahead))
	}
	if rs.Behind > 0 {
		chip += "  " + delStyle.Render(fmt.Sprintf("↓%d", rs.Behind))
	}
	if rs.Clean() {
		chip += "  " + addStyle.Render("clean")
	}
	return chip
}
