package main

import (
	"fmt"
	"runtime/debug"
)

// version, commit, date, and builtBy are populated at link time by
// GoReleaser via its own default ldflags template:
//
//	-X main.version={{.Version}} -X main.commit={{.Commit}}
//	-X main.date={{.Date}} -X main.builtBy=goreleaser
//
// These names are load-bearing: renaming any of them requires adding an
// explicit ldflags: block to .goreleaser.yaml to keep them in sync (see
// DD-8 in doc/adr.md). A plain `go build`/`go install` (no GoReleaser)
// leaves them at these defaults.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

// versionString builds the text printed by `auris -version`. When the
// binary was built by GoReleaser, it reports the injected version/commit/
// date/builtBy. Otherwise it falls back to Go's automatic VCS stamping
// (runtime/debug.ReadBuildInfo, available with zero extra tooling since
// Go 1.18) to still show a real commit instead of a bare "dev".
func versionString() string {
	if version != "dev" {
		return fmt.Sprintf("auris %s\ncommit:  %s\nbuilt:   %s\nby:      %s", version, commit, date, builtBy)
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "auris " + version
	}
	return "auris " + formatDevVersion(version, info.Settings)
}

// formatDevVersion extracts vcs.revision (truncated to 7 hex chars) and
// vcs.modified from build settings and formats them onto base. Falls back
// to base unchanged if no vcs.revision is present (e.g. built outside a
// git checkout, or with -buildvcs=false).
func formatDevVersion(base string, settings []debug.BuildSetting) string {
	var rev string
	var modified bool
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if rev == "" {
		return base
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if modified {
		rev += "-dirty"
	}
	return fmt.Sprintf("%s (%s)", base, rev)
}
