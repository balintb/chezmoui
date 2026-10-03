package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const sideGutterWidth = 5

type alignedRow struct {
	Left, Right               string
	LeftPresent, RightPresent bool
	LeftNum, RightNum         int
	LooseMatch                bool
	// Modified marks a replacement pair: differing lines aligned so the renderer can emphasize the tokens that changed.
	Modified bool
}

var boolLiteralRe = regexp.MustCompile(`(?i)\b(true|false)\b`)

func normalizeForLooseMatch(s string) string {
	s = strings.TrimRight(s, " \t")
	return boolLiteralRe.ReplaceAllStringFunc(s, strings.ToLower)
}

func alignLines(left, right string) []alignedRow {
	L := splitConfigLines(left)
	R := splitConfigLines(right)
	m, n := len(L), len(R)

	Ln := make([]string, m)
	for i, s := range L {
		Ln[i] = normalizeForLooseMatch(s)
	}
	Rn := make([]string, n)
	for j, s := range R {
		Rn[j] = normalizeForLooseMatch(s)
	}

	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if Ln[i-1] == Rn[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	var rev []alignedRow
	i, j := m, n
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && Ln[i-1] == Rn[j-1]:
			rev = append(rev, alignedRow{
				Left: L[i-1], Right: R[j-1],
				LeftPresent: true, RightPresent: true,
				LeftNum: i, RightNum: j,
				LooseMatch: L[i-1] != R[j-1],
			})
			i--
			j--
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			rev = append(rev, alignedRow{Right: R[j-1], RightPresent: true, RightNum: j})
			j--
		default:
			rev = append(rev, alignedRow{Left: L[i-1], LeftPresent: true, LeftNum: i})
			i--
		}
	}
	for a, b := 0, len(rev)-1; a < b; a, b = a+1, b-1 {
		rev[a], rev[b] = rev[b], rev[a]
	}
	return pairReplacements(rev)
}

// pairReplacements pairs adjacent runs of deleted lines with a following run of added lines, 1:1, so single-line replacements render as a word-level diff instead of separate add/delete rows. Unpaired leftovers keep their original add/delete form.
func pairReplacements(rows []alignedRow) []alignedRow {
	out := make([]alignedRow, 0, len(rows))
	for i := 0; i < len(rows); {
		if rows[i].LeftPresent && !rows[i].RightPresent {
			// Collect the run of deletions.
			delStart := i
			for i < len(rows) && rows[i].LeftPresent && !rows[i].RightPresent {
				i++
			}
			dels := rows[delStart:i]
			// Collect the run of insertions that immediately follows.
			insStart := i
			for i < len(rows) && !rows[i].LeftPresent && rows[i].RightPresent {
				i++
			}
			adds := rows[insStart:i]

			paired := min(len(dels), len(adds))
			for k := 0; k < paired; k++ {
				out = append(out, alignedRow{
					Left: dels[k].Left, Right: adds[k].Right,
					LeftPresent: true, RightPresent: true,
					LeftNum: dels[k].LeftNum, RightNum: adds[k].RightNum,
					Modified: true,
				})
			}
			out = append(out, dels[paired:]...)
			out = append(out, adds[paired:]...)
			continue
		}
		out = append(out, rows[i])
		i++
	}
	return out
}

func summarizeAlignment(rows []alignedRow) (added, removed, loose int) {
	for _, r := range rows {
		switch {
		case r.Modified:
			// A paired replacement counts as one add and one remove.
			added++
			removed++
		case r.LooseMatch:
			loose++
		case !r.LeftPresent && r.RightPresent:
			added++
		case r.LeftPresent && !r.RightPresent:
			removed++
		}
	}
	return added, removed, loose
}

func splitConfigLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

func renderSideBySide(rows []alignedRow, colWidth int) (left, right string) {
	if colWidth < sideGutterWidth+5 {
		colWidth = sideGutterWidth + 5
	}
	contentW := colWidth - sideGutterWidth
	var lb, rb strings.Builder
	for _, r := range rows {
		lb.WriteString(formatPanelCell(r, panelSideLeft, contentW))
		lb.WriteString("\n")
		rb.WriteString(formatPanelCell(r, panelSideRight, contentW))
		rb.WriteString("\n")
	}
	return lb.String(), rb.String()
}

type panelSide int

const (
	panelSideLeft panelSide = iota
	panelSideRight
)

func formatPanelCell(r alignedRow, side panelSide, contentW int) string {
	present := r.LeftPresent
	content := r.Left
	lineNum := r.LeftNum
	otherSidePresent := r.RightPresent
	if side == panelSideRight {
		present = r.RightPresent
		content = r.Right
		lineNum = r.RightNum
		otherSidePresent = r.LeftPresent
	}

	var gutter string
	if present {
		gutter = fmt.Sprintf("%4d ", lineNum)
	} else {
		gutter = "   ~ "
	}

	if !present {
		return gutterStyle.Render(gutter) + phantomLineStyle.Render(strings.Repeat(" ", contentW))
	}

	switch {
	case r.LooseMatch:
		return gutterStyle.Render(gutter) + looseLineStyle.Render(truncOrPad(content, contentW))
	case !otherSidePresent && side == panelSideLeft:
		return gutterStyle.Render(gutter) + delLineStyle.Render(truncOrPad(content, contentW))
	case !otherSidePresent && side == panelSideRight:
		return gutterStyle.Render(gutter) + addLineStyle.Render(truncOrPad(content, contentW))
	case r.Modified:
		// A replacement pair: color each side and emphasize the changed tokens.
		return gutterStyle.Render(gutter) + renderChanged(r.Left, r.Right, side, contentW)
	default:
		return gutterStyle.Render(gutter) + truncOrPad(content, contentW)
	}
}

// renderChanged renders one side of a modified line, emphasizing the tokens that differ from the other side.
func renderChanged(left, right string, side panelSide, contentW int) string {
	leftSegs, rightSegs := wordDiff(left, right)
	segs := leftSegs
	base, emph := delLineStyle, delEmphStyle
	if side == panelSideRight {
		segs = rightSegs
		base, emph = addLineStyle, addEmphStyle
	}
	var b strings.Builder
	visible := 0
	for _, s := range segs {
		if visible >= contentW {
			break
		}
		runes := []rune(s.text)
		if visible+len(runes) > contentW {
			runes = runes[:contentW-visible]
		}
		style := base
		if s.changed {
			style = emph
		}
		b.WriteString(style.Render(string(runes)))
		visible += len(runes)
	}
	if pad := contentW - visible; pad > 0 {
		b.WriteString(base.Render(strings.Repeat(" ", pad)))
	}
	return b.String()
}

func truncOrPad(s string, width int) string {
	r := []rune(s)
	if len(r) > width {
		return string(r[:width])
	}
	if len(r) < width {
		return s + strings.Repeat(" ", width-len(r))
	}
	return s
}

// renderPanels lays out two bordered panels that together fit within width. panelStyle contributes a one-cell border on each side, so the content width passed to Width must reserve those two columns.
func renderPanels(left, right string, width int) string {
	outer := (width - 1) / 2
	inner := outer - 4
	if inner < sideGutterWidth+5 {
		inner = sideGutterWidth + 5
	}
	rows := alignLines(left, right)
	l, r := renderSideBySide(rows, inner)
	lp := panelStyle.Width(outer - 2).Render(strings.TrimRight(l, "\n"))
	rp := panelStyle.Width(outer - 2).Render(strings.TrimRight(r, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, lp, " ", rp)
}
