package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// diffLineKind classifies a line of unified diff output.
type diffLineKind int

const (
	diffContext diffLineKind = iota
	diffAdd
	diffRemove
	diffHunk
	diffHeader
)

// diffLine is one parsed line of unified diff output.
type diffLine struct {
	kind diffLineKind
	text string
}

// parseUnified parses unified diff output into classified lines. Hunk headers ("@@ ... @@") and file headers are recognised; every other line is classified by its leading marker, defaulting to context.
func parseUnified(diff string) []diffLine {
	if diff == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(diff, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	out := make([]diffLine, 0, len(lines))
	for _, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "@@"):
			out = append(out, diffLine{kind: diffHunk, text: ln})
		case strings.HasPrefix(ln, "diff ") ||
			strings.HasPrefix(ln, "index ") ||
			strings.HasPrefix(ln, "--- ") ||
			strings.HasPrefix(ln, "+++ ") ||
			strings.HasPrefix(ln, "new file") ||
			strings.HasPrefix(ln, "deleted file"):
			out = append(out, diffLine{kind: diffHeader, text: ln})
		case strings.HasPrefix(ln, "+"):
			out = append(out, diffLine{kind: diffAdd, text: ln})
		case strings.HasPrefix(ln, "-"):
			out = append(out, diffLine{kind: diffRemove, text: ln})
		default:
			out = append(out, diffLine{kind: diffContext, text: ln})
		}
	}
	return out
}

// hunkOffsets returns the indices of hunk-header lines in a parsed diff.
func hunkOffsets(lines []diffLine) []int {
	var idx []int
	for i, ln := range lines {
		if ln.kind == diffHunk {
			idx = append(idx, i)
		}
	}
	return idx
}

// renderUnified renders parsed diff lines to a single styled block.
func renderUnified(lines []diffLine, width int) string {
	if len(lines) == 0 {
		return mutedStyle.Render("(no differences)")
	}
	var b strings.Builder
	for _, ln := range lines {
		switch ln.kind {
		case diffAdd:
			b.WriteString(addLineStyle.Render(padTo(ln.text, width)))
		case diffRemove:
			b.WriteString(delLineStyle.Render(padTo(ln.text, width)))
		case diffHunk:
			b.WriteString(hunkLineStyle.Render(padTo(ln.text, width)))
		case diffHeader:
			b.WriteString(mutedStyle.Render(padTo(ln.text, width)))
		default:
			b.WriteString(padTo(ln.text, width))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func padTo(s string, width int) string {
	if width <= 0 {
		return s
	}
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}
