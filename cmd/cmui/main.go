package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/balintb/chezmoui/internal/buildinfo"
	"github.com/balintb/chezmoui/internal/chezmoi"
	"github.com/balintb/chezmoui/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("cmui %s (commit %s, built %s)\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date)
		return
	}

	cli, err := chezmoi.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	zone.NewGlobal()
	p := tea.NewProgram(
		tui.NewModel(cli),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
