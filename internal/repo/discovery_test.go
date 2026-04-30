package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeRepo(t *testing.T, opts struct {
	Git, ChezmoiRoot bool
	Prefixed         int
	BaseName         string
}) string {
	t.Helper()
	parent := t.TempDir()
	name := opts.BaseName
	if name == "" {
		name = "repo"
	}
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if opts.Git {
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if opts.ChezmoiRoot {
		if err := os.WriteFile(filepath.Join(dir, ".chezmoiroot"), []byte("dot_root"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < opts.Prefixed; i++ {
		f := filepath.Join(dir, "dot_file"+string(rune('a'+i)))
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestScore_HighConfidenceCombo(t *testing.T) {
	dir := makeRepo(t, struct {
		Git, ChezmoiRoot bool
		Prefixed         int
		BaseName         string
	}{Git: true, ChezmoiRoot: true, Prefixed: 12, BaseName: "dotfiles"})

	c := scoreCandidate(dir)
	if c.Confidence < 90 {
		t.Errorf("max-signal candidate should be ≥ 90, got %d (reasons: %v)", c.Confidence, c.Reasons)
	}
	if c.Label() != "high" {
		t.Errorf("Label = %q, want high", c.Label())
	}
	for _, want := range []string{"is a git repository", "contains .chezmoiroot", "chezmoi-prefixed entries"} {
		found := false
		for _, r := range c.Reasons {
			if strings.Contains(r, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected reason containing %q, got %v", want, c.Reasons)
		}
	}
}

func TestScore_LowConfidenceEmptyDir(t *testing.T) {
	dir := t.TempDir()
	c := scoreCandidate(dir)
	if c.Confidence > 20 {
		t.Errorf("empty dir should score ≤ 20, got %d", c.Confidence)
	}
	if c.Label() != "low" {
		t.Errorf("Label = %q, want low", c.Label())
	}
}

func TestScore_GitRepoWithoutChezmoiContent(t *testing.T) {
	dir := makeRepo(t, struct {
		Git, ChezmoiRoot bool
		Prefixed         int
		BaseName         string
	}{Git: true})
	c := scoreCandidate(dir)
	if c.Label() == "high" {
		t.Errorf("plain git repo should not be high-confidence, got %d (reasons: %v)", c.Confidence, c.Reasons)
	}
}

func TestDiscover_RanksChezmoiSourceFirst(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	chezmoiSrc := makeRepo(t, struct {
		Git, ChezmoiRoot bool
		Prefixed         int
		BaseName         string
	}{Git: true, ChezmoiRoot: true, Prefixed: 5, BaseName: "chezmoi-src"})

	cands := Discover(chezmoiSrc)
	if len(cands) == 0 {
		t.Fatal("expected at least one candidate")
	}
	wantResolved, _ := filepath.EvalSymlinks(chezmoiSrc)
	gotResolved, _ := filepath.EvalSymlinks(cands[0].Path)
	if wantResolved != gotResolved {
		t.Errorf("chezmoi source should rank first, got %s (full %v)", cands[0].Path, cands)
	}
}

func TestDiscover_FollowsSymlinkToRealRepo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	real := makeRepo(t, struct {
		Git, ChezmoiRoot bool
		Prefixed         int
		BaseName         string
	}{Git: true, ChezmoiRoot: true, BaseName: "real-dotfiles"})
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	cands := Discover(link)
	if len(cands) == 0 {
		t.Fatal("expected candidates")
	}
	wantResolved, _ := filepath.EvalSymlinks(real)
	gotResolved, _ := filepath.EvalSymlinks(cands[0].Path)
	if wantResolved != gotResolved {
		t.Errorf("symlink should resolve to real path: want %q got %q", wantResolved, gotResolved)
	}
}

func TestDiscover_WalksUpToGitRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	gitRoot := filepath.Join(root, "monorepo")
	if err := os.MkdirAll(filepath.Join(gitRoot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(gitRoot, "configs", "dotfiles")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "dot_x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cands := Discover(subdir)
	wantResolved, _ := filepath.EvalSymlinks(gitRoot)
	var sawRoot bool
	for _, c := range cands {
		got, _ := filepath.EvalSymlinks(c.Path)
		if got == wantResolved {
			sawRoot = true
		}
	}
	if !sawRoot {
		paths := make([]string, len(cands))
		for i, c := range cands {
			paths[i] = c.Path
		}
		t.Errorf("git root %q not in candidates: %v", gitRoot, paths)
	}
}

func TestValidate_RejectsFiles(t *testing.T) {
	f := filepath.Join(t.TempDir(), "regular.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(f); err == nil {
		t.Error("Validate must reject a regular file")
	}
}

func TestValidate_RejectsMissing(t *testing.T) {
	if _, err := Validate("/no/such/dir/here"); err == nil {
		t.Error("Validate must reject nonexistent path")
	}
}

func TestValidate_AcceptsDir(t *testing.T) {
	dir := makeRepo(t, struct {
		Git, ChezmoiRoot bool
		Prefixed         int
		BaseName         string
	}{Git: true})
	c, err := Validate(dir)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if c.Path == "" || len(c.Reasons) == 0 {
		t.Errorf("Validate should populate the candidate, got %+v", c)
	}
}

func TestScore_ExactDotfilesNameOutscoresFuzzy(t *testing.T) {
	exact := makeRepo(t, struct {
		Git, ChezmoiRoot bool
		Prefixed         int
		BaseName         string
	}{Git: true, BaseName: "dotfiles"})
	fuzzy := makeRepo(t, struct {
		Git, ChezmoiRoot bool
		Prefixed         int
		BaseName         string
	}{Git: true, BaseName: "my-dotfiles-tracker"})

	ec := scoreCandidate(exact)
	fc := scoreCandidate(fuzzy)
	if ec.Confidence <= fc.Confidence {
		t.Errorf("exact 'dotfiles' (%d) should outscore fuzzy '%s' (%d)",
			ec.Confidence, "my-dotfiles-tracker", fc.Confidence)
	}
}

func TestDiscover_HomeDepth1Glob(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	dotfiles := filepath.Join(tmpHome, "personal", "dotfiles")
	if err := os.MkdirAll(filepath.Join(dotfiles, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cands := Discover("")
	wantResolved, _ := filepath.EvalSymlinks(dotfiles)
	for _, c := range cands {
		got, _ := filepath.EvalSymlinks(c.Path)
		if got == wantResolved {
			return
		}
	}
	paths := make([]string, len(cands))
	for i, c := range cands {
		paths[i] = c.Path
	}
	t.Errorf("depth-1 'dotfiles' not picked up: want %s, got %v", dotfiles, paths)
}
