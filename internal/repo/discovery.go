package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Candidate struct {
	Path       string
	Confidence int
	Reasons    []string
}

func Discover(chezmoiSourcePath string) []Candidate {
	seen := map[string]bool{}
	var cands []Candidate

	add := func(path string, hints ...string) {
		if path == "" {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		if seen[abs] {
			return
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			return
		}
		seen[abs] = true
		cands = append(cands, scoreCandidate(abs, hints...))
	}

	if chezmoiSourcePath != "" {
		if real, err := filepath.EvalSymlinks(chezmoiSourcePath); err == nil {
			add(real, "chezmoi source-path")
			if root := walkUpToGitRoot(real); root != "" && root != real {
				add(root, "git root above chezmoi source")
			}
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		for _, sub := range []string{
			"dotfiles",
			".dotfiles",
			"code/dotfiles",
			"src/dotfiles",
			"projects/dotfiles",
			"git/dotfiles",
			"workspace/dotfiles",
			"Developer/dotfiles",
		} {
			add(filepath.Join(home, sub), "conventional location")
		}
		for _, pat := range []string{"*/dotfiles", "*/.dotfiles"} {
			matches, _ := filepath.Glob(filepath.Join(home, pat))
			for _, p := range matches {
				add(p, "found in $HOME")
			}
		}
	}

	sort.SliceStable(cands, func(i, j int) bool {
		return cands[i].Confidence > cands[j].Confidence
	})
	return cands
}

func Validate(path string) (Candidate, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Candidate{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Candidate{}, err
	}
	if !info.IsDir() {
		return Candidate{}, fmt.Errorf("%s is not a directory", abs)
	}
	return scoreCandidate(abs, "manually entered"), nil
}

func scoreCandidate(path string, hints ...string) Candidate {
	c := Candidate{Path: path}

	for _, h := range hints {
		c.Confidence += 15
		c.Reasons = append(c.Reasons, h)
	}

	if isGitRepo(path) {
		c.Confidence += 25
		c.Reasons = append(c.Reasons, "is a git repository")
	}

	for _, name := range []string{
		".chezmoiroot", ".chezmoi.toml", ".chezmoi.toml.tmpl",
		".chezmoiignore", ".chezmoidata.toml", ".chezmoiscripts",
	} {
		if _, err := os.Stat(filepath.Join(path, name)); err == nil {
			c.Confidence += 30
			c.Reasons = append(c.Reasons, "contains "+name)
			break
		}
	}

	if n := countChezmoiPrefixedEntries(path); n > 0 {
		add := n * 3
		if add > 30 {
			add = 30
		}
		c.Confidence += add
		c.Reasons = append(c.Reasons, fmt.Sprintf("%d chezmoi-prefixed entries", n))
	}

	base := strings.ToLower(filepath.Base(path))
	switch {
	case base == "dotfiles" || base == ".dotfiles":
		c.Confidence += 25
		c.Reasons = append(c.Reasons, "named exactly 'dotfiles'")
	case strings.Contains(base, "dotfile") || strings.Contains(base, "chezmoi"):
		c.Confidence += 10
		c.Reasons = append(c.Reasons, "directory name suggests dotfiles")
	}

	if c.Confidence > 100 {
		c.Confidence = 100
	}
	return c
}

func isGitRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func countChezmoiPrefixedEntries(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	prefixes := []string{
		"dot_", "private_", "executable_", "encrypted_",
		"run_", "symlink_", "modify_", "create_", "exact_",
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		for _, p := range prefixes {
			if strings.HasPrefix(name, p) {
				n++
				break
			}
		}
	}
	return n
}

func walkUpToGitRoot(path string) string {
	home, _ := os.UserHomeDir()
	cur := path
	for {
		if isGitRepo(cur) {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		if home != "" && cur == home {
			return ""
		}
		cur = parent
	}
}

func (c Candidate) Label() string {
	switch {
	case c.Confidence >= 60:
		return "high"
	case c.Confidence >= 30:
		return "medium"
	default:
		return "low"
	}
}
