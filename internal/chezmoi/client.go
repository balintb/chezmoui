package chezmoi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type Client struct {
	bin      string
	extraEnv []string
}

type Option func(*Client)

func WithExtraEnv(env ...string) Option {
	return func(c *Client) { c.extraEnv = append(c.extraEnv, env...) }
}

func New(opts ...Option) (*Client, error) {
	bin, err := exec.LookPath("chezmoi")
	if err != nil {
		return nil, fmt.Errorf("chezmoi not found in PATH: %w", err)
	}
	c := &Client{bin: bin}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	full := append([]string{
		"--no-pager",
		"--color=false",
		"--no-tty",
	}, args...)
	cmd := exec.CommandContext(ctx, c.bin, full...)
	if len(c.extraEnv) > 0 {
		cmd.Env = append(os.Environ(), c.extraEnv...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.Bytes(), fmt.Errorf("chezmoi %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}

type Entry struct {
	Target         string
	Absolute       string
	SourceAbsolute string
	SourceRelative string
}

func (c *Client) Managed(ctx context.Context) ([]Entry, error) {
	out, err := c.run(ctx,
		"managed",
		"--format=json",
		"--path-style=all",
		"--exclude=encrypted",
	)
	if err != nil {
		return nil, err
	}
	type rawEntry struct {
		Absolute       string `json:"absolute"`
		SourceAbsolute string `json:"sourceAbsolute"`
		SourceRelative string `json:"sourceRelative"`
	}
	raw := map[string]rawEntry{}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse managed json: %w", err)
	}
	entries := make([]Entry, 0, len(raw))
	for target, v := range raw {
		entries = append(entries, Entry{
			Target:         target,
			Absolute:       v.Absolute,
			SourceAbsolute: v.SourceAbsolute,
			SourceRelative: v.SourceRelative,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Target < entries[j].Target })
	return entries, nil
}

type StatusCode byte

const (
	StatusClean    StatusCode = ' '
	StatusAdded    StatusCode = 'A'
	StatusDeleted  StatusCode = 'D'
	StatusModified StatusCode = 'M'
	StatusRun      StatusCode = 'R'
)

type Status struct {
	Source StatusCode
	Target StatusCode
	Path   string
}

func (s Status) Modified() bool {
	return s.Source == StatusModified || s.Target == StatusModified
}

func (c *Client) Status(ctx context.Context) ([]Status, error) {
	out, err := c.run(ctx, "status")
	if err != nil {
		return nil, err
	}
	var statuses []Status
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 4 {
			continue
		}
		statuses = append(statuses, Status{
			Source: StatusCode(line[0]),
			Target: StatusCode(line[1]),
			Path:   line[3:],
		})
	}
	return statuses, nil
}

func (c *Client) Cat(ctx context.Context, path string) (string, error) {
	out, err := c.run(ctx, "cat", path)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (c *Client) Diff(ctx context.Context, path string, reverse bool) (string, error) {
	args := []string{"diff", "--use-builtin-diff"}
	if reverse {
		args = append(args, "--reverse")
	}
	args = append(args, path)
	out, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (c *Client) ReAdd(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return errors.New("re-add requires at least one path")
	}
	args := append([]string{"re-add", "--keep-going"}, paths...)
	_, err := c.run(ctx, args...)
	return err
}

func (c *Client) Apply(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return errors.New("apply requires at least one path")
	}
	args := append([]string{"apply", "--force", "--keep-going"}, paths...)
	_, err := c.run(ctx, args...)
	return err
}

func (c *Client) SourcePath(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "source-path")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

type GitStatus struct {
	Branch    string
	Tracking  string
	Staged    int
	Unstaged  int
	Untracked int
}

func (c *Client) GitStatus(ctx context.Context) (GitStatus, error) {
	out, err := c.run(ctx, "git", "--", "status", "--porcelain=v1", "--branch")
	if err != nil {
		return GitStatus{}, err
	}
	var s GitStatus
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			rest := strings.TrimPrefix(line, "## ")
			if i := strings.Index(rest, "..."); i >= 0 {
				s.Branch = rest[:i]
				up := rest[i+3:]
				if sp := strings.Index(up, " "); sp >= 0 {
					up = up[:sp]
				}
				s.Tracking = up
			} else {
				s.Branch = strings.TrimSpace(rest)
			}
			continue
		}
		if len(line) < 3 {
			continue
		}
		x, y := line[0], line[1]
		switch {
		case x == '?' && y == '?':
			s.Untracked++
		default:
			if x != ' ' && x != '?' {
				s.Staged++
			}
			if y != ' ' {
				s.Unstaged++
			}
		}
	}
	return s, nil
}
