package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// configPathOverride is used in tests to redirect file I/O to a temp directory.
var configPathOverride string

// Path returns the platform-specific path for the auris-ai config file.
// On Linux/FreeBSD it respects $XDG_CONFIG_HOME, falling back to ~/.config.
// On macOS it uses ~/Library/Application Support.
// On Windows it uses %APPDATA%.
func Path() (string, error) {
	if configPathOverride != "" {
		return configPathOverride, nil
	}

	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("config: Path: %w", err)
			}
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, "auris-ai", "config.json"), nil

	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("config: Path: %w", err)
		}
		return filepath.Join(home, "Library", "Application Support", "auris-ai", "config.json"), nil

	default: // linux, freebsd, etc.
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("config: Path: %w", err)
			}
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "auris-ai", "config.json"), nil
	}
}
