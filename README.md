# chezmoui

A simple TUI for [chezmoi](https://www.chezmoi.io/).

## What it does

- side-by-side or unified diff with line numbers and synced scrolling
- word-level emphasis on the exact tokens that changed within a line
- line wrap (`w`) or horizontal pan (shift+arrows) for long lines
- notices logical vs. semantic differences
- filter the managed list by path with `/`
- re-add (live → source) or apply (source → live), single or bulk
- undo the last action from a local snapshot
- source-state attribute chips (template, private, executable, encrypted, scripts, …)
- Unmanaged and Ignored tabs, with add-to-source
- Doctor tab for one-key `chezmoi doctor` diagnostics
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
go test -race ./...                    # unit with the race detector
go test -tags=integration ./...        # + real chezmoi
go test -tags='integration e2e' ./...  # + e2e
```

### Fuzzing

```sh
go test ./internal/tui/ -run '^$' -fuzz=FuzzAlignLines
go test ./internal/tui/ -run '^$' -fuzz=FuzzSanitizeForFilename
go test ./internal/tui/ -run '^$' -fuzz=FuzzParseUnified
go test ./internal/tui/ -run '^$' -fuzz=FuzzWordDiff
go test ./internal/tui/ -run '^$' -fuzz=FuzzWrapRuns
go test ./internal/chezmoi/ -run '^$' -fuzz=FuzzParseStatus
go test ./internal/chezmoi/ -run '^$' -fuzz=FuzzParseAttributes
go test ./internal/chezmoi/ -run '^$' -fuzz=FuzzParseDoctor
go test ./internal/chezmoi/ -run '^$' -fuzz=FuzzParsePathList
```

### Golden files

Rendered views are snapshot-tested under `internal/tui/testdata/`. Regenerate after intentional layout changes:

```sh
go test ./internal/tui -run TestGolden -update
```

Backups for reverts go under `~/.cache/chezmoui/recoverable/`.
Config at `~/.config/chezmoui/config.json`.
