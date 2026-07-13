package main

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestFormatDevVersion(t *testing.T) {
	tests := []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{"no vcs info", nil, "dev"},
		{"clean revision", []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
			{Key: "vcs.modified", Value: "false"},
		}, "dev (abcdef1)"},
		{"dirty revision", []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
			{Key: "vcs.modified", Value: "true"},
		}, "dev (abcdef1-dirty)"},
		{"short revision not truncated", []debug.BuildSetting{
			{Key: "vcs.revision", Value: "ab12"},
		}, "dev (ab12)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDevVersion("dev", tt.settings); got != tt.want {
				t.Errorf("formatDevVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersionString_Released(t *testing.T) {
	origVersion, origCommit, origDate, origBuiltBy := version, commit, date, builtBy
	defer func() { version, commit, date, builtBy = origVersion, origCommit, origDate, origBuiltBy }()

	version, commit, date, builtBy = "0.1.0", "a1b2c3d", "2026-07-13T10:00:00Z", "goreleaser"

	got := versionString()
	for _, want := range []string{"auris 0.1.0", "commit:  a1b2c3d", "built:   2026-07-13T10:00:00Z", "by:      goreleaser"} {
		if !strings.Contains(got, want) {
			t.Errorf("versionString() = %q, missing %q", got, want)
		}
	}
}

func TestVersionString_DevFallback(t *testing.T) {
	// version/commit/date/builtBy are at their zero-value defaults in a
	// `go test` run (not injected via -ldflags), so this exercises the
	// real debug.ReadBuildInfo() path end-to-end as a smoke test.
	got := versionString()
	if !strings.HasPrefix(got, "auris dev") {
		t.Errorf("versionString() = %q, want prefix %q", got, "auris dev")
	}
}
