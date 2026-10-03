package chezmoi

import (
	"context"
	"sort"
	"strings"
)

// Unmanaged lists destination paths that chezmoi does not manage. Absolute paths are returned so they can be fed straight to `add`.
func (c *Client) Unmanaged(ctx context.Context) ([]string, error) {
	out, err := c.run(ctx, "unmanaged", "--path-style=absolute")
	if err != nil {
		return nil, err
	}
	return parsePathList(string(out)), nil
}

// Ignored lists targets ignored by chezmoi (for example via .chezmoiignore).
func (c *Client) Ignored(ctx context.Context) ([]string, error) {
	out, err := c.run(ctx, "ignored")
	if err != nil {
		return nil, err
	}
	return parsePathList(string(out)), nil
}

// parsePathList turns newline-separated command output into a sorted, deduped list of non-empty paths.
func parsePathList(out string) []string {
	seen := map[string]bool{}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		paths = append(paths, line)
	}
	sort.Strings(paths)
	return paths
}

// Add promotes destination paths into the source state.
func (c *Client) Add(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return errNoPaths
	}
	args := append([]string{"add"}, paths...)
	_, err := c.run(ctx, args...)
	return err
}

// DoctorCheck is one row of `chezmoi doctor` output.
type DoctorCheck struct {
	Result  string
	Check   string
	Message string
}

// Failed reports whether the check did not pass.
func (d DoctorCheck) Failed() bool {
	switch d.Result {
	case "ok", "info", "warning":
		return false
	default:
		return true
	}
}

// Doctor runs chezmoi's self-diagnostics. Network checks are skipped so the call stays fast and offline-safe.
func (c *Client) Doctor(ctx context.Context) ([]DoctorCheck, error) {
	out, err := c.run(ctx, "doctor", "--no-network")
	if err != nil {
		return nil, err
	}
	return ParseDoctor(string(out)), nil
}

// ParseDoctor parses the fixed-column `chezmoi doctor` table using the column offsets from its header row, so multi-word messages survive intact. Lines shorter than the check column are ignored.
func ParseDoctor(out string) []DoctorCheck {
	lines := strings.Split(out, "\n")
	checkStart, msgStart := doctorColumnOffsets(lines)
	var checks []DoctorCheck
	for i, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line == "" || i == 0 {
			continue
		}
		if len(line) < checkStart {
			continue
		}
		c := DoctorCheck{
			Result:  strings.TrimSpace(slice(line, 0, checkStart)),
			Check:   strings.TrimSpace(slice(line, checkStart, msgStart)),
			Message: strings.TrimSpace(slice(line, msgStart, len(line))),
		}
		if c.Result == "" && c.Check == "" && c.Message == "" {
			continue
		}
		checks = append(checks, c)
	}
	return checks
}

// doctorColumnOffsets derives the CHECK and MESSAGE column offsets from the header line, defaulting to chezmoi's known layout.
func doctorColumnOffsets(lines []string) (checkStart, msgStart int) {
	checkStart, msgStart = 10, 38
	if len(lines) == 0 {
		return
	}
	h := lines[0]
	if c := strings.Index(h, "CHECK"); c >= 0 {
		checkStart = c
	}
	if m := strings.Index(h, "MESSAGE"); m >= 0 {
		msgStart = m
	}
	if msgStart <= checkStart {
		checkStart, msgStart = 10, 38
	}
	return
}

func slice(s string, start, end int) string {
	if start > len(s) {
		start = len(s)
	}
	if end > len(s) {
		end = len(s)
	}
	if start > end {
		start = end
	}
	return s[start:end]
}
