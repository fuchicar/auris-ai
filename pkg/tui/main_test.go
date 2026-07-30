package tui

import (
	"os"
	"testing"
)

// TestMain sandboxes every test in this package behind an isolated
// XDG_CONFIG_HOME. AppModel.transition can reach a.saveConfig() (config.Save)
// on several branches, and config.Path() has no package-external override —
// only XDG_CONFIG_HOME redirects it from outside pkg/config. This blanket
// guard means a test that builds a real *AppModel and drives it into one of
// those branches can never write to the user's actual ~/.config/auris-ai,
// even if it forgets to sandbox itself individually.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "auris-tui-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_CONFIG_HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
