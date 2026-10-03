package tui

import "strings"

// segment is a token of a line tagged with whether it changed between the two sides of a diff. Segments always concatenate back to the original line.
type segment struct {
	text    string
	changed bool
}

// tokenize splits a line into alternating runs of whitespace and non-whitespace while preserving every byte. Joining the tokens reproduces the input exactly.
func tokenize(s string) []string {
	if s == "" {
		return nil
	}
	var tokens []string
	var b strings.Builder
	inSpace := s[0] == ' ' || s[0] == '\t'
	for i := 0; i < len(s); i++ {
		space := s[i] == ' ' || s[i] == '\t'
		if space != inSpace && b.Len() > 0 {
			tokens = append(tokens, b.String())
			b.Reset()
		}
		inSpace = space
		b.WriteByte(s[i])
	}
	if b.Len() > 0 {
		tokens = append(tokens, b.String())
	}
	return tokens
}

// wordDiff computes the segments of left and right with changed tokens marked. It runs an LCS over tokens; tokens not in the common subsequence are marked changed on their respective side.
func wordDiff(left, right string) ([]segment, []segment) {
	a := tokenize(left)
	b := tokenize(right)
	if len(a) == 0 && len(b) == 0 {
		return nil, nil
	}

	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	leftChanged := make([]bool, len(a))
	rightChanged := make([]bool, len(b))
	i, j := len(a), len(b)
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			i--
			j--
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			rightChanged[j-1] = true
			j--
		default:
			leftChanged[i-1] = true
			i--
		}
	}

	return tagSegments(a, leftChanged), tagSegments(b, rightChanged)
}

func tagSegments(tokens []string, changed []bool) []segment {
	if len(tokens) == 0 {
		return nil
	}
	segs := make([]segment, len(tokens))
	for i, t := range tokens {
		segs[i] = segment{text: t, changed: changed[i]}
	}
	return segs
}

// joinSegments reproduces the original text from a segment slice. Used by tests to guarantee formatting is lossless.
func joinSegments(segs []segment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.text)
	}
	return b.String()
}
