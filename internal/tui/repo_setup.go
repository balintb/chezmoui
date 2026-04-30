package tui

import (
	"context"
	"os"
	"strings"

	"github.com/balintb/chezmoui/internal/config"
	"github.com/balintb/chezmoui/internal/repo"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type repoSetupMode int

const (
	setupPick repoSetupMode = iota
	setupManual
)

type repoSetupData struct {
	mode       repoSetupMode
	candidates []repo.Candidate
	cursor     int
	manualIn   textinput.Model
	manualErr  string
}

type configLoadedMsg struct {
	cfg     config.Config
	loadErr error
}
type repoCandidatesMsg struct {
	candidates []repo.Candidate
}
type configSavedMsg struct{ err error }

func loadConfigCmd(store *config.Store) tea.Cmd {
	return func() tea.Msg {
		c, err := store.Load()
		if err != nil && !config.IsNotExist(err) {
			return configLoadedMsg{cfg: c, loadErr: err}
		}
		return configLoadedMsg{cfg: c}
	}
}

func discoverRepoCmd(cli Backend) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5e9)
		defer cancel()
		src, _ := cli.SourcePath(ctx)
		return repoCandidatesMsg{candidates: repo.Discover(src)}
	}
}

func saveConfigCmd(store *config.Store, c config.Config) tea.Cmd {
	return func() tea.Msg {
		return configSavedMsg{err: store.Save(c)}
	}
}

func (m Model) viewRepoSetup() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Locate your dotfiles repo") + "\n\n")
	switch m.repoSetup.mode {
	case setupManual:
		b.WriteString("Enter a path to your dotfiles repo:\n\n")
		b.WriteString("  " + m.repoSetup.manualIn.View() + "\n")
		if m.repoSetup.manualErr != "" {
			b.WriteString("\n  " + errorStyle.Render(m.repoSetup.manualErr) + "\n")
		}
		b.WriteString("\n")
		b.WriteString(statusStyle.Render("[enter]") + " confirm   " +
			statusStyle.Render("[esc]") + " back to candidates")
		return b.String()
	}

	if len(m.repoSetup.candidates) == 0 {
		b.WriteString(mutedStyle.Render("No likely repo locations found.\n\n"))
	} else {
		b.WriteString("Pick the directory that holds your chezmoi-managed dotfiles.\n\n")
		for i, c := range m.repoSetup.candidates {
			marker := "   "
			rowStyle := lipgloss.NewStyle()
			if i == m.repoSetup.cursor {
				marker = " ▶ "
				rowStyle = rowStyle.Bold(true)
			}
			label := strings.ToUpper(c.Label())
			var labelStyle lipgloss.Style
			switch c.Label() {
			case "high":
				labelStyle = badgeAdded
			case "medium":
				labelStyle = badgeRun
			default:
				labelStyle = mutedStyle
			}
			b.WriteString(marker + rowStyle.Render(c.Path) + "\n")
			b.WriteString("       " + labelStyle.Render("["+label+"]") + " " +
				mutedStyle.Render(strings.Join(c.Reasons, " · ")) + "\n\n")
		}
	}
	b.WriteString(mutedStyle.Render("──── or ────") + "\n\n")
	b.WriteString("  " + statusStyle.Render("[m]") + " enter a path manually\n")
	b.WriteString("  " + statusStyle.Render("[s]") + " skip — I'll set it up later\n\n")
	b.WriteString(statusStyle.Render("[enter]") + " confirm   " +
		statusStyle.Render("[↑/↓]") + " move   " +
		statusStyle.Render("[q]") + " quit")
	return b.String()
}

func (m Model) repoChip() string {
	if m.repoPath == "" {
		return ""
	}
	short := m.repoPath
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(short, home) {
		short = "~" + strings.TrimPrefix(short, home)
	}
	return mutedStyle.Render("repo · ") + short
}

func (m *Model) enterRepoSetup() tea.Cmd {
	m.repoSetup = repoSetupData{mode: setupPick}
	m.state = viewRepoSetup
	return discoverRepoCmd(m.cli)
}

func (m Model) handleRepoSetupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.repoSetup.mode == setupManual {
		return m.handleRepoSetupManualKey(msg)
	}
	switch {
	case keyMatchesAny(msg, "up", "k"):
		if m.repoSetup.cursor > 0 {
			m.repoSetup.cursor--
		}
	case keyMatchesAny(msg, "down", "j"):
		if m.repoSetup.cursor < len(m.repoSetup.candidates)-1 {
			m.repoSetup.cursor++
		}
	case keyMatchesAny(msg, "enter"):
		if len(m.repoSetup.candidates) == 0 {
			m.repoSetup.mode = setupManual
			m.repoSetup.manualIn = newRepoTextInput("")
			return m, textinput.Blink
		}
		c := m.repoSetup.candidates[m.repoSetup.cursor]
		return m.confirmRepoPath(c.Path)
	case keyMatchesAny(msg, "m"):
		m.repoSetup.mode = setupManual
		m.repoSetup.manualIn = newRepoTextInput(m.repoPath)
		return m, textinput.Blink
	case keyMatchesAny(msg, "s"):
		return m.confirmRepoPath("")
	case keyMatchesAny(msg, "q", "ctrl+c"):
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleRepoSetupManualKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatchesAny(msg, "esc"):
		m.repoSetup.mode = setupPick
		m.repoSetup.manualErr = ""
		return m, nil
	case keyMatchesAny(msg, "enter"):
		path := strings.TrimSpace(m.repoSetup.manualIn.Value())
		if path == "" {
			m.repoSetup.manualErr = "path cannot be empty"
			return m, nil
		}
		if strings.HasPrefix(path, "~") {
			if home, err := os.UserHomeDir(); err == nil && home != "" {
				path = home + strings.TrimPrefix(path, "~")
			}
		}
		if _, err := repo.Validate(path); err != nil {
			m.repoSetup.manualErr = err.Error()
			return m, nil
		}
		return m.confirmRepoPath(path)
	}
	var cmd tea.Cmd
	m.repoSetup.manualIn, cmd = m.repoSetup.manualIn.Update(msg)
	return m, cmd
}

func (m Model) confirmRepoPath(path string) (Model, tea.Cmd) {
	m.repoPath = path
	m.repoSetup = repoSetupData{}
	m.state = viewList
	cfg := config.Config{RepoPath: path, RepoConfirmed: true}
	if path == "" {
		m.status = "no repo configured — set one later from the Help tab"
	} else {
		m.status = "repo: " + path
	}
	return m, saveConfigCmd(m.configStore, cfg)
}

func newRepoTextInput(initial string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "/Users/you/dotfiles"
	ti.SetValue(initial)
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 60
	return ti
}

func keyMatchesAny(msg tea.KeyMsg, names ...string) bool {
	s := msg.String()
	for _, n := range names {
		if s == n {
			return true
		}
	}
	return false
}
