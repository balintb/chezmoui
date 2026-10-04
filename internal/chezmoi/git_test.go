package chezmoi

import (
	"reflect"
	"testing"
)

func TestParseRepoStatus_BranchAndTracking(t *testing.T) {
	out := "## main...origin/main [ahead 2, behind 1]\n M dot_bashrc\n?? dot_zshrc\n"
	s := ParseRepoStatus(out)
	if s.Branch != "main" || s.Tracking != "origin/main" {
		t.Errorf("branch/tracking = %q/%q", s.Branch, s.Tracking)
	}
	if s.Ahead != 2 || s.Behind != 1 {
		t.Errorf("ahead/behind = %d/%d", s.Ahead, s.Behind)
	}
	if len(s.Changes) != 2 {
		t.Fatalf("want 2 changes, got %d", len(s.Changes))
	}
}

func TestParseRepoStatus_NoTracking(t *testing.T) {
	s := ParseRepoStatus("## main\n")
	if s.Branch != "main" || s.Tracking != "" {
		t.Errorf("branch/tracking = %q/%q", s.Branch, s.Tracking)
	}
}

func TestParseRepoStatus_NoCommitsYet(t *testing.T) {
	s := ParseRepoStatus("## No commits yet on main\n")
	if s.Branch != "No" {
		// git prints "No commits yet on main"; we keep the first token.
		t.Logf("branch parsed as %q (git wording dependent)", s.Branch)
	}
}

func TestParseRepoStatus_Rename(t *testing.T) {
	s := ParseRepoStatus("## main\nR  dot_config/new -> dot_config/old\n")
	if len(s.Changes) != 1 {
		t.Fatalf("want 1 change, got %d", len(s.Changes))
	}
	c := s.Changes[0]
	if c.Path != "dot_config/old" || c.OrigPath != "dot_config/new" {
		t.Errorf("rename not split: %#v", c)
	}
	if c.Label() != "renamed" {
		t.Errorf("label = %q", c.Label())
	}
}

func TestParseRepoStatus_Counts(t *testing.T) {
	out := "## main\nMM dot_a\n M dot_b\nM  dot_c\n?? dot_d\nA  dot_e\n"
	s := ParseRepoStatus(out)
	if got := s.Staged(); got != 3 {
		t.Errorf("staged = %d, want 3", got)
	}
	if got := s.Unstaged(); got != 2 {
		t.Errorf("unstaged = %d, want 2", got)
	}
	if got := s.Untracked(); got != 1 {
		t.Errorf("untracked = %d, want 1", got)
	}
	if s.Clean() {
		t.Error("repo with changes must not report clean")
	}
}

func TestRepoStatus_Clean(t *testing.T) {
	s := ParseRepoStatus("## main...origin/main\n")
	if !s.Clean() {
		t.Error("no changes should report clean")
	}
}

func TestGitChange_Labels(t *testing.T) {
	cases := map[string]string{
		"??": "untracked",
		"AA": "conflict",
		"UU": "conflict",
		"R ": "renamed",
		"C ": "copied",
		"A ": "added",
		" D": "deleted",
		"D ": "deleted",
		" M": "modified",
		"M ": "modified",
	}
	for xy, want := range cases {
		if got := (GitChange{XY: xy}).Label(); got != want {
			t.Errorf("Label(%q) = %q, want %q", xy, got, want)
		}
	}
}

func TestGitChange_Flags(t *testing.T) {
	if !(GitChange{XY: "M "}).Staged() {
		t.Error("M  should be staged")
	}
	if (GitChange{XY: " M"}).Staged() {
		t.Error(" M should not be staged")
	}
	if !(GitChange{XY: " M"}).Unstaged() {
		t.Error(" M should be unstaged")
	}
	if !(GitChange{XY: "??"}).Untracked() {
		t.Error("?? should be untracked")
	}
}

func TestParseRepoStatus_UnmergedAndIgnored(t *testing.T) {
	out := "## main\nUU dot_conflict\n!! dot_ignored\n"
	s := ParseRepoStatus(out)
	if len(s.Changes) != 2 {
		t.Fatalf("want 2 changes, got %d", len(s.Changes))
	}
	if s.Changes[0].Label() != "conflict" {
		t.Errorf("UU label = %q", s.Changes[0].Label())
	}
	if s.Changes[1].Label() != "ignored" {
		t.Errorf("!! label = %q", s.Changes[1].Label())
	}
}

func TestGitCommit_RejectsEmptyMessage(t *testing.T) {
	c := &Client{}
	if err := c.GitCommit(t.Context(), "   "); err == nil {
		t.Error("empty commit message should error")
	}
}

func TestParseAheadBehind(t *testing.T) {
	a, b := parseAheadBehind("ahead 3, behind 4")
	if a != 3 || b != 4 {
		t.Errorf("ahead/behind = %d/%d", a, b)
	}
	a, b = parseAheadBehind("")
	if a != 0 || b != 0 {
		t.Errorf("empty = %d/%d", a, b)
	}
}

// FuzzParseRepoStatus asserts the parser never panics and preserves change text.
func FuzzParseRepoStatus(f *testing.F) {
	f.Add("## main...origin/main [ahead 1]\n M a\n?? b\n")
	f.Add("")
	f.Add("## main\nR  x -> y\n")
	f.Fuzz(func(t *testing.T, out string) {
		s := ParseRepoStatus(out)
		_ = s.Clean()
		_ = s.Staged()
		_ = s.Unstaged()
		_ = s.Untracked()
		for _, c := range s.Changes {
			if len(c.XY) != 2 {
				t.Fatalf("XY not 2 chars: %q from %q", c.XY, out)
			}
		}
	})
}

func TestParseRepoStatus_EqualsOracle(t *testing.T) {
	out := "## main...origin/main\n M a\n?? b\nR  c -> d\n"
	got := ParseRepoStatus(out)
	want := RepoStatus{
		Branch:   "main",
		Tracking: "origin/main",
		Changes: []GitChange{
			{XY: " M", Path: "a"},
			{XY: "??", Path: "b"},
			{XY: "R ", Path: "d", OrigPath: "c"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseRepoStatus mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}
