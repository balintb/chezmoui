package tui

import (
	"errors"
	"strings"

	"github.com/balintb/chezmoui/internal/chezmoi"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// opError attaches the TUI operation and target to an underlying error so the error view can present the user with actionable context.
type opError struct {
	Op     string
	Target string
	Err    error
}

func (e opError) Error() string {
	if e.Target != "" {
		return e.Op + " " + e.Target + ": " + e.Err.Error()
	}
	return e.Op + ": " + e.Err.Error()
}

func (e opError) Unwrap() error { return e.Err }

// viewError renders a structured error screen with the failing operation, the captured stderr (when available), and a retry hint.
func (m Model) viewError() string {
	var oe opError
	hasOp := errors.As(m.err, &oe)
	op, target, cause := "operation", "", m.err
	if hasOp {
		op, target, cause = oe.Op, oe.Target, oe.Err
	}

	var ce *chezmoi.CommandError
	hasCmd := errors.As(cause, &ce)

	var b strings.Builder
	b.WriteString(errorStyle.Render("error: "+op) + "\n\n")
	if target != "" {
		b.WriteString("  " + mutedStyle.Render("target   ") + target + "\n")
	}
	if hasCmd {
		b.WriteString("  " + mutedStyle.Render("command  ") + "chezmoi " + strings.Join(ce.Args, " ") + "\n")
	}
	detail := cause.Error()
	if hasCmd && ce.Stderr != "" {
		detail = ce.Stderr
	}
	b.WriteString("  " + mutedStyle.Render("detail   ") + detail + "\n")

	hint := "[R] retry"
	if m.retry == nil {
		hint = "[R] reload"
	}
	b.WriteString("\n" + statusStyle.Render(hint) + "   " + statusStyle.Render("[q]") + " quit")
	return b.String()
}

// handleErrorKey routes keys while an error is displayed, so retry works even from inside an active sync session.
func (m Model) handleErrorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Refresh):
		m.err = nil
		m.loading = true
		if m.retry != nil {
			cmd := m.retry
			m.retry = nil
			return m, cmd
		}
		return m, loadEntriesCmd(m.cli)
	case key.Matches(msg, keys.Cancel):
		m.err = nil
		m.retry = nil
		m.status = "dismissed"
		return m, nil
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	}
	return m, nil
}
