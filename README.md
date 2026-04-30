# chezmoui

A simple TUI for [chezmoi](https://www.chezmoi.io/).

## What it does

- side-by-side diff with line numbers and synced scrolling
- notices logical vs. semantic differences
- sync session: walk through every drifted file, decide keep/revert/skip per file
- finds your dotfiles repo

## Install

```sh
brew install balintb/tap/chezmoui
```

or via Go (needs ≥ 1.22 and `chezmoi` on PATH):

```sh
go install github.com/balintb/chezmoui/cmd/cmui@latest
```

or from source:

```sh
git clone https://github.com/balintb/chezmoui
cd chezmoui
go run ./cmd/cmui
```

## Testing

```sh
go test ./...                          # unit
go test -tags=integration ./...        # + real chezmoi
go test -tags='integration e2e' ./...  # + e2e
```

Backups for reverts go under `~/.cache/chezmoui/recoverable/`.
Config at `~/.config/chezmoui/config.json`.
