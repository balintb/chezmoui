package chezmoi

import (
	"errors"
	"testing"
)

func TestCommandError_Formatting(t *testing.T) {
	err := &CommandError{
		Args:   []string{"cat", "/home/u/.bashrc"},
		Stderr: "no such target",
		Err:    errors.New("exit status 1"),
	}
	if got, want := err.Error(), "chezmoi cat /home/u/.bashrc: no such target"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, err.Err) {
		t.Error("CommandError must unwrap to its cause")
	}
}

func TestCommandError_FormattingWithoutStderr(t *testing.T) {
	err := &CommandError{Args: []string{"status"}, Err: errors.New("exit status 2")}
	if got, want := err.Error(), "chezmoi status: exit status 2"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
