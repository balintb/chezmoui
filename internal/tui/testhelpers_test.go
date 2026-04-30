package tui

import (
	"regexp"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/muesli/termenv"
)

func init() {
	lipgloss.SetColorProfile(termenv.TrueColor)
	zone.NewGlobal()
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

func hasBackgroundFill(s string) bool {
	matches := ansiRE.FindAllString(s, -1)
	for _, m := range matches {
		body := m[2 : len(m)-1]
		for _, code := range splitSemi(body) {
			switch code {
			case "48", "40", "41", "42", "43", "44", "45", "46", "47",
				"100", "101", "102", "103", "104", "105", "106", "107":
				return true
			}
		}
	}
	return false
}

func splitSemi(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ';' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
