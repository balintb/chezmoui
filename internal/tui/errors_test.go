package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/balintb/chezmoui/internal/chezmoi"
	tea "github.com/charmbracelet/bubbletea"
)

func TestOpError_ErrorAndUnwrap(t *testing.T) {
	base := errors.New("boom")
	withTarget := opError{Op: "re-add", Target: "/home/u/.bashrc", Err: base}
	if got := withTarget.Error(); got != "re-add /home/u/.bashrc: boom" {
		t.Errorf("Error() = %q", got)
	}
	if !errors.Is(withTarget, base) {
		t.Error("opError should unwrap to its cause")
	}
	noTarget := opError{Op: "refresh", Err: base}
	if got := noTarget.Error(); got != "refresh: boom" {
		t.Errorf("Error() without target = %q", got)
	}
}

func TestViewError_StructuredFields(t *testing.T) {
	b := sampleBackend()
	b.catErr = &chezmoi.CommandError{Args: []string{"cat", "/home/u/.bashrc"}, Stderr: "no such target", Err: errors.New("exit status 1")}
	m := loadedModel(t, b)
	m = cursorTo(t, m, ".config/btop/btop.conf")
	_, cmd := applyMsg(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(t, m, runCmd(t, cmd))

	view := stripANSI(m.View())
	for _, want := range []string{"error: load target contents", "/home/u/.config/btop/btop.conf", "chezmoi cat", "no such target"} {
		if !strings.Contains(view, want) {
			t.Errorf("error view missing %q:\n%s", want, view)
		}
	}
}

func TestErrMsg_CarriesCmdUnderlyingError(t *testing.T) {
	sentinel := errors.New("kaboom")
	msg := errMsg{err: opError{Op: "x", Err: sentinel}}
	if !errors.Is(msg.err, sentinel) {
		t.Error("errMsg must wrap the underlying error for errors.Is")
	}
	if msg.Error() != "x: kaboom" {
		t.Errorf("errMsg.Error() = %q", msg.Error())
	}
}
