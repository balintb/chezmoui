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
	return rev
}

func summarizeAlignment(rows []alignedRow) (added, removed, loose int) {
	for _, r := range rows {
		switch {
		case !r.LeftPresent && r.RightPresent:
			added++
		case r.LeftPresent && !r.RightPresent:
			removed++
		case r.LooseMatch:
			loose++
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
	body := truncOrPad(content, contentW)
	switch {
	case r.LooseMatch:
		return gutterStyle.Render(gutter) + looseLineStyle.Render(body)
	case !otherSidePresent && side == panelSideLeft:
		return gutterStyle.Render(gutter) + delLineStyle.Render(body)
	case !otherSidePresent && side == panelSideRight:
		return gutterStyle.Render(gutter) + addLineStyle.Render(body)
	default:
		return gutterStyle.Render(gutter) + body
	}
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

func joinPanels(left, right string) string {
	sep := lipgloss.NewStyle().Foreground(colorMuted).Render("│")
	lLines := strings.Split(strings.TrimRight(left, "\n"), "\n")
	rLines := strings.Split(strings.TrimRight(right, "\n"), "\n")
	n := len(lLines)
	if len(rLines) < n {
		n = len(rLines)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(lLines[i])
		b.WriteString(" ")
		b.WriteString(sep)
		b.WriteString(" ")
		b.WriteString(rLines[i])
		b.WriteString("\n")
	}
	return b.String()
}
