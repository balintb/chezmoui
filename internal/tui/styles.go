package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent = lipgloss.AdaptiveColor{Light: "#5C5CFF", Dark: "#7D7DFF"}
	colorMuted  = lipgloss.AdaptiveColor{Light: "#666", Dark: "#888"}
	colorAdd    = lipgloss.AdaptiveColor{Light: "#0a7d28", Dark: "#5fd75f"}
	colorDel    = lipgloss.AdaptiveColor{Light: "#a30000", Dark: "#ff6b6b"}
	colorWarn   = lipgloss.AdaptiveColor{Light: "#a35200", Dark: "#ffaf00"}
	colorSelBg  = lipgloss.AdaptiveColor{Light: "#dcdcff", Dark: "#2a2a4a"}

	colorAddBg     = lipgloss.AdaptiveColor{Light: "#d4f4d4", Dark: "#1c4521"}
	colorAddBgFg   = lipgloss.AdaptiveColor{Light: "#0a4d10", Dark: "#a6e9a6"}
	colorDelBg     = lipgloss.AdaptiveColor{Light: "#f4d4d4", Dark: "#4d1c1c"}
	colorDelBgFg   = lipgloss.AdaptiveColor{Light: "#5a0a0a", Dark: "#ffb0b0"}
	colorPhantomBg = lipgloss.AdaptiveColor{Light: "#ececec", Dark: "#202020"}
	colorHunkBg    = lipgloss.AdaptiveColor{Light: "#dde4ff", Dark: "#1f2540"}

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	mutedStyle    = lipgloss.NewStyle().Foreground(colorMuted)
	statusStyle   = lipgloss.NewStyle().Foreground(colorAccent)
	errorStyle    = lipgloss.NewStyle().Foreground(colorDel).Bold(true)
	cursorStyle   = lipgloss.NewStyle().Background(colorSelBg).Bold(true)
	selectedMark  = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	modifiedStyle = lipgloss.NewStyle().Foreground(colorWarn)

	addStyle  = lipgloss.NewStyle().Foreground(colorAdd)
	delStyle  = lipgloss.NewStyle().Foreground(colorDel)
	hunkStyle = lipgloss.NewStyle().Foreground(colorAccent)

	addLineStyle     = lipgloss.NewStyle().Background(colorAddBg).Foreground(colorAddBgFg)
	delLineStyle     = lipgloss.NewStyle().Background(colorDelBg).Foreground(colorDelBgFg)
	phantomLineStyle = lipgloss.NewStyle().Background(colorPhantomBg).Foreground(colorMuted)
	hunkLineStyle    = lipgloss.NewStyle().Background(colorHunkBg).Foreground(colorAccent).Bold(true)

	colorLooseBg   = lipgloss.AdaptiveColor{Light: "#fff5d0", Dark: "#3a3220"}
	colorLooseBgFg = lipgloss.AdaptiveColor{Light: "#5a4500", Dark: "#e0c97a"}
	looseLineStyle = lipgloss.NewStyle().Background(colorLooseBg).Foreground(colorLooseBgFg)

	badgeModified = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	badgeAdded    = lipgloss.NewStyle().Foreground(colorAdd).Bold(true)
	badgeDeleted  = lipgloss.NewStyle().Foreground(colorDel).Bold(true)
	badgeRun      = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	badgeClean    = lipgloss.NewStyle().Foreground(colorMuted)

	sectionHeaderStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true).
				Underline(true)

	gutterStyle = lipgloss.NewStyle().Foreground(colorMuted)

	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)

	windowStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1)

	tabActiveStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Background(colorAccent).
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#0a0a14"}).
			Bold(true)
	tabInactiveStyle = lipgloss.NewStyle().
				Padding(0, 2).
				Foreground(colorMuted)
	tabSeparatorStyle = lipgloss.NewStyle().
				Foreground(colorMuted)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder(), true).
			BorderForeground(colorMuted).
			Padding(0, 1)
)
