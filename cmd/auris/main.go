// Command auris is the entry point for the Auris AI financial advisor.
//
// Usage:
//
//	auris [-setup] [-debug <path>]
//
// Without flags, Auris detects whether a configuration file exists. If not,
// the first-run setup wizard is shown automatically. If a configuration
// already exists, the user is prompted for their passphrase to unlock it
// and is taken directly to the main menu.
//
// Flags:
//
//	-setup          Re-run the setup wizard even when a configuration already exists.
//	-debug <path>   Append per-iteration agent diagnostics to <path>. Disabled by default for privacy.
//
// Environment variables:
//
//	AURIS_OLLAMA_NUM_CTX   Override Ollama's per-request num_ctx (default 32768).
//	                       Increase to use larger context windows on capable models
//	                       (e.g. 131072 for gemma4:e2b's full 128k window). Higher
//	                       values consume more RAM for the KV cache.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"auris/pkg/locale"
	"auris/pkg/tui"
)

func main() {
	setupFlag := flag.Bool("setup", false, "Re-run the setup wizard")
	debugPath := flag.String("debug", "", "Append agent diagnostics to the given file path (disabled by default)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: auris [-setup] [-debug <path>]\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Environment variables:
  AURIS_OLLAMA_NUM_CTX
        Override Ollama's per-request num_ctx (default 32768). Increase to
        unlock larger context windows on capable models (e.g. 131072 for
        gemma4:e2b's full 128k window). Higher values consume more RAM for
        the KV cache.
`)
	}
	flag.Parse()

	// Detect the OS locale and initialise the translation bundle before any
	// TUI output so that even early error messages are localised.
	detectedLocale, localeKnown := locale.Detect()
	if err := locale.Init(detectedLocale); err != nil {
		fmt.Fprintf(os.Stderr, "auris: locale init: %v\n", err)
		os.Exit(1)
	}

	var debugLogger *log.Logger
	if *debugPath != "" {
		f, err := os.OpenFile(*debugPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "auris: -debug: cannot open %q: %v (continuing without debug logging)\n", *debugPath, err)
		} else {
			defer f.Close()
			debugLogger = log.New(f, "", log.LstdFlags|log.Lmicroseconds)
			debugLogger.Printf("[startup] auris debug log opened")
		}
	}

	setupMode := *setupFlag || !tui.ConfigExists()

	app := tui.NewApp(tui.AppOptions{
		SetupMode:        setupMode,
		DetectedLocale:   detectedLocale,
		ShowLocaleSelect: setupMode && !localeKnown,
		DebugLogger:      debugLogger,
	})

	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "auris: %v\n", err)
		os.Exit(1)
	}
}
