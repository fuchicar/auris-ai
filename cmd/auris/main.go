// Command auris is the entry point for the Auris AI financial advisor.
//
// Usage:
//
//	auris [-setup]
//
// Without flags, Auris detects whether a configuration file exists. If not,
// the first-run setup wizard is shown automatically. If a configuration
// already exists, the user is prompted for their passphrase to unlock it
// and is taken directly to the main menu.
//
// Flags:
//
//	-setup   Re-run the setup wizard even when a configuration already exists.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"auris/pkg/locale"
	"auris/pkg/tui"
)

func main() {
	setupFlag := flag.Bool("setup", false, "Re-run the setup wizard")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: auris [-setup]\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	// Detect the OS locale and initialise the translation bundle before any
	// TUI output so that even early error messages are localised.
	detectedLocale, localeKnown := locale.Detect()
	if err := locale.Init(detectedLocale); err != nil {
		fmt.Fprintf(os.Stderr, "auris: locale init: %v\n", err)
		os.Exit(1)
	}

	setupMode := *setupFlag || !tui.ConfigExists()

	app := tui.NewApp(tui.AppOptions{
		SetupMode:        setupMode,
		DetectedLocale:   detectedLocale,
		ShowLocaleSelect: setupMode && !localeKnown,
	})

	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "auris: %v\n", err)
		os.Exit(1)
	}
}
