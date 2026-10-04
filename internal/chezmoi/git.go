package chezmoi

import (
	"context"
	"strconv"
	"strings"
)

// GitChange is a single entry from `git status --porcelain`. XY is the raw two-column status code (index status then work-tree status); Path is the current path and OrigPath is set for renames/copies.
type GitChange struct {
	XY       string
	Path     string
	OrigPath string
}

// Staged reports whether the change is staged in the index.
func (g GitChange) Staged() bool { return g.XY != "" && g.XY[0] != ' ' && g.XY[0] != '?' }

// Unstaged reports whether a tracked file's work tree differs from the index.
func (g GitChange) Unstaged() bool {
	return !g.Untracked() && len(g.XY) > 1 && g.XY[1] != ' '
}

// Untracked reports whether the file is untracked.
func (g GitChange) Untracked() bool { return g.XY == "??" }

// Label renders a short, human-readable status for the change.
func (g GitChange) Label() string {
	switch {
	case g.XY == "??":
		return "untracked"
	case g.XY == "!!":
		return "ignored"
	case g.XY == "AA" || g.XY == "UU":
		return "conflict"
	case strings.HasPrefix(g.XY, "R"):
		return "renamed"
	case strings.HasPrefix(g.XY, "C"):
		return "copied"
	case strings.HasPrefix(g.XY, "A"):
		return "added"
	case strings.HasPrefix(g.XY, "D") || strings.HasSuffix(g.XY, "D"):
		return "deleted"
	case strings.HasPrefix(g.XY, "M") || strings.HasSuffix(g.XY, "M"):
		return "modified"
	}
	return "changed"
}

// RepoStatus is the parsed result of `git status --porcelain=v1 --branch`.
type RepoStatus struct {
	Branch   string
	Tracking string
	Ahead    int
	Behind   int
	Changes  []GitChange
}

// Staged returns the number of staged changes.
func (r RepoStatus) Staged() int {
	return countChanges(r.Changes, func(c GitChange) bool { return c.Staged() })
}

// Unstaged returns the number of unstaged (tracked) changes.
func (r RepoStatus) Unstaged() int {
	return countChanges(r.Changes, func(c GitChange) bool { return c.Unstaged() })
}

// Untracked returns the number of untracked changes.
func (r RepoStatus) Untracked() int {
	return countChanges(r.Changes, func(c GitChange) bool { return c.Untracked() })
}

// Clean reports whether the working tree has no changes.
func (r RepoStatus) Clean() bool { return len(r.Changes) == 0 }

func countChanges(changes []GitChange, pred func(GitChange) bool) int {
	n := 0
	for _, c := range changes {
		if pred(c) {
			n++
		}
	}
	return n
}

// RepoStatus returns the parsed status of the source repository.
func (c *Client) RepoStatus(ctx context.Context) (RepoStatus, error) {
	out, err := c.run(ctx, "git", "--", "status", "--porcelain=v1", "--branch")
	if err != nil {
		return RepoStatus{}, err
	}
	return ParseRepoStatus(string(out)), nil
}

// ParseRepoStatus parses `git status --porcelain=v1 --branch` output. Renames and copies ("R  old -> new") record both paths.
func ParseRepoStatus(out string) RepoStatus {
	var s RepoStatus
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			s.Branch, s.Tracking, s.Ahead, s.Behind = parseBranchLine(strings.TrimPrefix(line, "## "))
			continue
		}
		if len(line) < 3 {
			continue
		}
		ch := GitChange{XY: line[:2], Path: line[3:]}
		if i := strings.Index(ch.Path, " -> "); i >= 0 {
			ch.OrigPath = ch.Path[:i]
			ch.Path = ch.Path[i+4:]
		}
		s.Changes = append(s.Changes, ch)
	}
	return s
}

func parseBranchLine(rest string) (branch, tracking string, ahead, behind int) {
	if i := strings.Index(rest, "..."); i >= 0 {
		branch = rest[:i]
		up := rest[i+3:]
		if sp := strings.Index(up, " "); sp >= 0 {
			tracking = up[:sp]
			up = up[sp+1:]
		} else {
			tracking = up
			return
		}
		ahead, behind = parseAheadBehind(up)
		return
	}
	branch = strings.TrimSpace(rest)
	if i := strings.Index(branch, " "); i >= 0 {
		// "No commits yet on main" -> keep the branch name.
		branch = branch[:i]
	}
	return
}

func parseAheadBehind(s string) (ahead, behind int) {
	s = strings.Trim(s, "[] ")
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "ahead "):
			ahead, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(part, "ahead ")))
		case strings.HasPrefix(part, "behind "):
			behind, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(part, "behind ")))
		}
	}
	return
}

// Git runs a git subcommand in the source directory via `chezmoi git`.
func (c *Client) Git(ctx context.Context, args ...string) (string, error) {
	full := append([]string{"git", "--"}, args...)
	out, err := c.run(ctx, full...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// GitCommit stages all source changes and commits them with message.
func (c *Client) GitCommit(ctx context.Context, message string) error {
	if strings.TrimSpace(message) == "" {
		return errNoMessage
	}
	if _, err := c.Git(ctx, "add", "-A"); err != nil {
		return err
	}
	_, err := c.Git(ctx, "commit", "-m", message)
	return err
}

// GitPush pushes the current branch, setting its upstream on first push.
func (c *Client) GitPush(ctx context.Context) error {
	_, err := c.Git(ctx, "push", "-u", "origin", "HEAD")
	return err
}
