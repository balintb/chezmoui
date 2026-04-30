package chezmoi

import (
	"reflect"
	"testing"
)

func TestStatus_Modified(t *testing.T) {
	cases := []struct {
		name string
		s    Status
		want bool
	}{
		{"clean", Status{Source: ' ', Target: ' '}, false},
		{"target M", Status{Source: ' ', Target: 'M'}, true},
		{"source M", Status{Source: 'M', Target: ' '}, true},
		{"both M", Status{Source: 'M', Target: 'M'}, true},
		{"added", Status{Source: ' ', Target: 'A'}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.s.Modified(); got != c.want {
				t.Errorf("Modified() = %v, want %v", got, c.want)
			}
		})
	}
}

func parseStatusLines(out string) []Status {
	c := &Client{}
	_ = c
	var statuses []Status
	for _, line := range splitLines(out) {
		if len(line) < 4 {
			continue
		}
		statuses = append(statuses, Status{
			Source: StatusCode(line[0]),
			Target: StatusCode(line[1]),
			Path:   line[3:],
		})
	}
	return statuses
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func TestParseStatusLines_RealOutput(t *testing.T) {
	in := " A .brewfile\n A .claude/CLAUDE.md\n M .config/btop/btop.conf\n"
	got := parseStatusLines(in)
	want := []Status{
		{Source: ' ', Target: 'A', Path: ".brewfile"},
		{Source: ' ', Target: 'A', Path: ".claude/CLAUDE.md"},
		{Source: ' ', Target: 'M', Path: ".config/btop/btop.conf"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseStatusLines mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestParseStatusLines_Empty(t *testing.T) {
	if got := parseStatusLines(""); got != nil {
		t.Errorf("expected nil for empty input, got %#v", got)
	}
}

func TestParseStatusLines_SkipsShortLines(t *testing.T) {
	in := "\n  \n M .ok\n"
	got := parseStatusLines(in)
	if len(got) != 1 || got[0].Path != ".ok" {
		t.Errorf("expected single .ok row, got %#v", got)
	}
}

func TestParseStatusLines_PathWithSpaces(t *testing.T) {
	in := " M .config/foo bar/baz.conf\n"
	got := parseStatusLines(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].Path != ".config/foo bar/baz.conf" {
		t.Errorf("path mangled: %q", got[0].Path)
	}
}

func TestReAdd_RejectsEmptyPaths(t *testing.T) {
	c := &Client{bin: "/nonexistent"}
	err := c.ReAdd(t.Context(), []string{}...)
	if err == nil {
		t.Fatal("expected error for empty paths, got nil")
	}
}
