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

// viewOpts controls how a panel is laid out for one render pass.
type viewOpts struct {
	contentW int
	wrap     bool
	hOffset  int
}

func renderSideBySide(rows []alignedRow, colWidth int) (left, right string) {
	return renderSideBySideOpts(rows, colWidth, viewOpts{})
}

func renderSideBySideOpts(rows []alignedRow, colWidth int, opts viewOpts) (left, right string) {
	if colWidth < sideGutterWidth+5 {
		colWidth = sideGutterWidth + 5
	}
	opts.contentW = colWidth - sideGutterWidth

	var lb, rb strings.Builder
	for _, r := range rows {
		for _, cell := range formatRowCells(r, opts) {
			lb.WriteString(cell.left)
			lb.WriteString("\n")
			rb.WriteString(cell.right)
			rb.WriteString("\n")
		}
	}
	return lb.String(), rb.String()
}

// rowCell is one visual line for both panels of a single aligned row. A logical row that wraps produces several rowCells, one per visual line.
type rowCell struct {
	left  string
	right string
}

// formatRowCells builds the visual line(s) for an aligned row, keeping both panels at the same number of lines so windowed scrolling stays in sync.
func formatRowCells(r alignedRow, opts viewOpts) []rowCell {
	leftLines := panelVisualLines(r, panelSideLeft, opts)
	rightLines := panelVisualLines(r, panelSideRight, opts)
	n := len(leftLines)
	if len(rightLines) > n {
		n = len(rightLines)
	}
	blank := gutterStyle.Render("   ~ ") + phantomLineStyle.Render(strings.Repeat(" ", opts.contentW))
	cells := make([]rowCell, n)
	for i := 0; i < n; i++ {
		l, rr := blank, blank
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			rr = rightLines[i]
		}
		cells[i] = rowCell{left: l, right: rr}
	}
	return cells
}

// styledRun is a run of text with a single style. A row's logical content is a sequence of runs whose plain text concatenates back to the source line.
type styledRun struct {
	text  string
	style lipgloss.Style
}

// panelRuns returns the styled runs for one side of an aligned row.
func panelRuns(r alignedRow, side panelSide) []styledRun {
	content := r.Left
	otherPresent := r.RightPresent
	base := lipgloss.NewStyle()
	if side == panelSideRight {
		content = r.Right
		otherPresent = r.LeftPresent
	}
	switch {
	case r.LooseMatch:
		return []styledRun{{text: content, style: looseLineStyle}}
	case !otherPresent && side == panelSideLeft:
		return []styledRun{{text: content, style: delLineStyle}}
	case !otherPresent && side == panelSideRight:
		return []styledRun{{text: content, style: addLineStyle}}
	case r.Modified:
		return changedRuns(r.Left, r.Right, side)
	default:
		return []styledRun{{text: content, style: base}}
	}
}

// changedRuns returns the runs for a modified line, emphasizing changed tokens.
func changedRuns(left, right string, side panelSide) []styledRun {
	leftSegs, rightSegs := wordDiff(left, right)
	segs := leftSegs
	base, emph := delLineStyle, delEmphStyle
	if side == panelSideRight {
		segs = rightSegs
		base, emph = addLineStyle, addEmphStyle
	}
	runs := make([]styledRun, 0, len(segs))
	for _, s := range segs {
		style := base
		if s.changed {
			style = emph
		}
		runs = append(runs, styledRun{text: s.text, style: style})
	}
	return runs
}

// panelVisualLines renders one side of an aligned row into one or more visual lines according to the wrap and horizontal-scroll settings. The first line carries the line-number gutter; wrapped continuations use a blank gutter.
func panelVisualLines(r alignedRow, side panelSide, opts viewOpts) []string {
	present := r.LeftPresent
	lineNum := r.LeftNum
	if side == panelSideRight {
		present = r.RightPresent
		lineNum = r.RightNum
	}
	if !present {
		return []string{gutterStyle.Render("   ~ ") + phantomLineStyle.Render(strings.Repeat(" ", opts.contentW))}
	}

	gutter := fmt.Sprintf("%4d ", lineNum)
	blankGutter := "     "
	runs := panelRuns(r, side)

	if opts.wrap {
		chunks := wrapRuns(runs, opts.contentW)
		if len(chunks) == 0 {
			chunks = [][]styledRun{nil}
		}
		lines := make([]string, len(chunks))
		for i, chunk := range chunks {
			g := gutter
			if i > 0 {
				g = blankGutter
			}
			lines[i] = gutterStyle.Render(g) + renderRuns(chunk, opts.contentW)
		}
		return lines
	}

	windowed, clippedLeft, clippedRight := windowRuns(runs, opts.hOffset, opts.contentW)
	windowed = overlayClipped(windowed, clippedLeft, clippedRight, opts.contentW)
	return []string{gutterStyle.Render(gutter) + renderRuns(windowed, opts.contentW)}
}

// windowRuns returns the run window [offset, offset+width) with styles intact, plus whether the line is clipped on the left/right.
func windowRuns(runs []styledRun, offset, width int) ([]styledRun, bool, bool) {
	total := 0
	for _, run := range runs {
		total += len([]rune(run.text))
	}
	clippedLeft := offset > 0
	clippedRight := offset+width < total
	if offset < 0 {
		offset = 0
	}

	var out []styledRun
	pos := 0
	remaining := width
	for _, run := range runs {
		if remaining <= 0 {
			break
		}
		runes := []rune(run.text)
		runEnd := pos + len(runes)
		if runEnd <= offset {
			pos = runEnd
			continue
		}
		start := max(0, offset-pos)
		end := len(runes)
		if end-start > remaining {
			end = start + remaining
		}
		if start < end {
			out = append(out, styledRun{text: string(runes[start:end]), style: run.style})
			remaining -= end - start
		}
		pos = runEnd
	}
	return out, clippedLeft, clippedRight
}

// wrapRuns splits runs into chunks of at most width runes, preserving styles.
func wrapRuns(runs []styledRun, width int) [][]styledRun {
	if width <= 0 {
		return [][]styledRun{runs}
	}
	var chunks [][]styledRun
	var cur []styledRun
	used := 0
	for _, run := range runs {
		runes := []rune(run.text)
		for len(runes) > 0 {
			space := width - used
			if space == 0 {
				chunks = append(chunks, cur)
				cur, used = nil, 0
				space = width
			}
			take := min(space, len(runes))
			cur = append(cur, styledRun{text: string(runes[:take]), style: run.style})
			used += take
			runes = runes[take:]
		}
	}
	if len(cur) > 0 || len(chunks) == 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

// renderRuns styles runs and pads the result to width using the last run's style (all runs adjacent in a row share a background).
func renderRuns(runs []styledRun, width int) string {
	var b strings.Builder
	visible := 0
	var padStyle lipgloss.Style
	for _, run := range runs {
		b.WriteString(run.style.Render(run.text))
		visible += len([]rune(run.text))
		padStyle = run.style
	}
	if pad := width - visible; pad > 0 {
		b.WriteString(padStyle.Render(strings.Repeat(" ", pad)))
	}
	return b.String()
}

// overlayClipped replaces the first/last visible rune with directional markers when the window is clipped, leaving the rest of the runs and styles intact.
func overlayClipped(runs []styledRun, clippedLeft, clippedRight bool, width int) []styledRun {
	if (!clippedLeft && !clippedRight) || width <= 0 {
		return runs
	}
	out := make([]styledRun, len(runs))
	copy(out, runs)
	if clippedLeft && len(out) > 0 {
		out[0].text = replaceFirstRune(out[0].text, '‹')
	}
	if clippedRight && len(out) > 0 {
		last := len(out) - 1
		out[last].text = replaceLastRune(out[last].text, '›')
	}
	return out
}

// plainRuns concatenates the text of runs, discarding styles.
func plainRuns(runs []styledRun) string {
	var b strings.Builder
	for _, run := range runs {
		b.WriteString(run.text)
	}
	return b.String()
}

func replaceFirstRune(s string, r rune) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return string(r)
	}
	runes[0] = r
	return string(runes)
}

func replaceLastRune(s string, r rune) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return string(r)
	}
	runes[len(runes)-1] = r
	return string(runes)
}

type panelSide int

const (
	panelSideLeft panelSide = iota
	panelSideRight
)

// renderPanels lays out two bordered panels that together fit within width. panelStyle contributes a one-cell border on each side, so the content width passed to Width must reserve those two columns.
func renderPanels(left, right string, width int, opts viewOpts) string {
	outer := (width - 1) / 2
	inner := outer - 4
	if inner < sideGutterWidth+5 {
		inner = sideGutterWidth + 5
	}
	rows := alignLines(left, right)
	l, r := renderSideBySideOpts(rows, inner, opts)
	lp := panelStyle.Width(outer - 2).Render(strings.TrimRight(l, "\n"))
	rp := panelStyle.Width(outer - 2).Render(strings.TrimRight(r, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, lp, " ", rp)
}
