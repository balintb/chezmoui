//go:build integration

package chezmoi

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (homeDir, targetAbs string) {
	t.Helper()
	homeDir = t.TempDir()
	srcDir := filepath.Join(homeDir, ".local", "share", "chezmoi")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "dot_testfile"), []byte("source content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	targetAbs = filepath.Join(homeDir, ".testfile")
	if err := os.WriteFile(targetAbs, []byte("live content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return homeDir, targetAbs
}

func newTestClient(t *testing.T, homeDir string) *Client {
	t.Helper()
	cli, err := New(WithExtraEnv("HOME=" + homeDir))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return cli
}

func TestIntegration_Managed_FindsSeededFile(t *testing.T) {
	home, _ := fixture(t)
	cli := newTestClient(t, home)
	entries, err := cli.Managed(context.Background())
	if err != nil {
		t.Fatalf("Managed: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Target == ".testfile" {
			found = true
			if !strings.HasPrefix(e.Absolute, home) {
				t.Errorf("Absolute %q not under HOME %q", e.Absolute, home)
			}
			if e.SourceRelative != "dot_testfile" {
				t.Errorf("SourceRelative = %q, want dot_testfile", e.SourceRelative)
			}
		}
	}
	if !found {
		t.Errorf("did not find .testfile in Managed output: %#v", entries)
	}
}

func TestIntegration_Status_ReportsModified(t *testing.T) {
	home, _ := fixture(t)
	cli := newTestClient(t, home)
	statuses, err := cli.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	for _, s := range statuses {
		if s.Path == ".testfile" {
			if !s.Modified() {
				t.Errorf(".testfile should be modified, got source=%c target=%c", s.Source, s.Target)
			}
			return
		}
	}
	t.Errorf(".testfile not found in Status output: %#v", statuses)
}

func TestIntegration_Diff_AbsolutePathRequired_Regression(t *testing.T) {
	home, targetAbs := fixture(t)
	cli := newTestClient(t, home)

	otherDir := t.TempDir()
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(otherDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWd) })

	if _, err := cli.Diff(context.Background(), ".testfile", true); err == nil {
		t.Error("expected error when passing relative path with cwd != HOME")
	}

	out, err := cli.Diff(context.Background(), targetAbs, true)
	if err != nil {
		t.Fatalf("Diff with absolute path failed: %v", err)
	}
	if !strings.Contains(out, "live content") {
		t.Errorf("reverse diff missing live content: %q", out)
	}
}

func TestIntegration_ReAdd_PromotesLiveIntoSource(t *testing.T) {
	home, targetAbs := fixture(t)
	cli := newTestClient(t, home)

	if err := cli.ReAdd(context.Background(), targetAbs); err != nil {
		t.Fatalf("ReAdd: %v", err)
	}
	srcContents, err := os.ReadFile(filepath.Join(home, ".local", "share", "chezmoi", "dot_testfile"))
	if err != nil {
		t.Fatal(err)
	}
	if string(srcContents) != "live content\n" {
		t.Errorf("source after re-add = %q, want %q", srcContents, "live content\n")
	}
	statuses, err := cli.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range statuses {
		if s.Path == ".testfile" && s.Modified() {
			t.Errorf(".testfile still reports modified after re-add: %+v", s)
		}
	}
}

func TestIntegration_Cat_ReturnsTargetContents(t *testing.T) {
	home, targetAbs := fixture(t)
	cli := newTestClient(t, home)
	got, err := cli.Cat(context.Background(), targetAbs)
	if err != nil {
		t.Fatalf("Cat: %v", err)
	}
	if got != "source content\n" {
		t.Errorf("Cat = %q, want %q", got, "source content\n")
	}
}

func TestIntegration_Cat_StillWorksWithoutLiveFile(t *testing.T) {
	home, targetAbs := fixture(t)
	if err := os.Remove(targetAbs); err != nil {
		t.Fatal(err)
	}
	cli := newTestClient(t, home)
	got, err := cli.Cat(context.Background(), targetAbs)
	if err != nil {
		t.Fatalf("Cat with missing live file: %v", err)
	}
	if got != "source content\n" {
		t.Errorf("Cat = %q, want %q", got, "source content\n")
	}
}

func TestIntegration_Managed_IncludesDirectories(t *testing.T) {
	home, _ := fixture(t)
	subDir := filepath.Join(home, ".local", "share", "chezmoi", "dot_subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "inner"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cli := newTestClient(t, home)
	entries, err := cli.Managed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var sawDir, sawFile bool
	for _, e := range entries {
		if e.Target == ".subdir" {
			sawDir = true
		}
		if e.Target == ".subdir/inner" {
			sawFile = true
		}
	}
	if !sawDir {
		t.Errorf("Managed output missing the .subdir directory entry: %v", entries)
	}
	if !sawFile {
		t.Errorf("Managed output missing the .subdir/inner file entry: %v", entries)
	}
}

func TestIntegration_Cat_OnDirectoryErrors(t *testing.T) {
	home, _ := fixture(t)
	subDir := filepath.Join(home, ".local", "share", "chezmoi", "dot_subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cli := newTestClient(t, home)
	_, err := cli.Cat(context.Background(), filepath.Join(home, ".subdir"))
	if err == nil {
		t.Fatal("expected error from chezmoi cat on a directory")
	}
}

func TestIntegration_Apply_RevertsLiveToSource(t *testing.T) {
	home, targetAbs := fixture(t)
	cli := newTestClient(t, home)
	if err := cli.Apply(context.Background(), targetAbs); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	live, err := os.ReadFile(targetAbs)
	if err != nil {
		t.Fatal(err)
	}
	if string(live) != "source content\n" {
		t.Errorf("after apply, live = %q, want %q", live, "source content\n")
	}
}

func TestIntegration_Apply_RejectsEmptyPaths(t *testing.T) {
	home, _ := fixture(t)
	cli := newTestClient(t, home)
	if err := cli.Apply(context.Background()); err == nil {
		t.Fatal("Apply with no paths must error")
	}
}

func TestIntegration_SourcePath_ReturnsConfiguredDir(t *testing.T) {
	home, _ := fixture(t)
	cli := newTestClient(t, home)
	got, err := cli.SourcePath(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "share", "chezmoi")
	if got != want {
		t.Errorf("SourcePath = %q, want %q", got, want)
	}
}

func TestIntegration_GitStatus_ParsesBranchAndCounts(t *testing.T) {
	home, _ := fixture(t)
	srcDir := filepath.Join(home, ".local", "share", "chezmoi")
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "--allow-empty", "-m", "init", "-q"},
		{"add", "dot_testfile"}, // stage one
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = srcDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(srcDir, "dot_other"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cli := newTestClient(t, home)
	gs, err := cli.GitStatus(context.Background())
	if err != nil {
		t.Fatalf("GitStatus: %v", err)
	}
	if gs.Branch != "main" {
		t.Errorf("Branch = %q, want main", gs.Branch)
	}
	if gs.Staged < 1 {
		t.Errorf("Staged should be ≥1, got %d", gs.Staged)
	}
	if gs.Untracked < 1 {
		t.Errorf("Untracked should be ≥1, got %d", gs.Untracked)
	}
}

func TestIntegration_ReAdd_RejectsEmptyPaths(t *testing.T) {
	home, _ := fixture(t)
	cli := newTestClient(t, home)
	if err := cli.ReAdd(context.Background()); err == nil {
		t.Fatal("ReAdd with no paths must error to prevent re-adding everything")
	}
}
