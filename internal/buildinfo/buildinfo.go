// Package buildinfo carries the values injected at build time via -ldflags.
package buildinfo

// Version, Commit and Date are overridden by goreleaser (-X .../buildinfo.Version=...).
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// Label is the short version string shown in the UI.
func Label() string { return "chezmoui " + Version }
