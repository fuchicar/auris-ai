// Command auris is the entry point for the Auris AI financial advisor.
//
// Usage:
//
//	auris [-setup] [-debug <path>] [-version]
//
// Without flags, Auris detects whether a configuration file exists. If not,
// the first-run setup wizard is shown automatically. If a configuration
// already exists, the user is prompted for their passphrase to unlock it
// and is taken directly to the main menu.
//
// Flags:
//
//	-setup          Re-run the setup wizard even when a configuration already exists.
//	                Prompts for confirmation on the terminal first, since finishing the
//	                wizard overwrites the existing config file (new passphrase, new KDF
//	                salt) — including the storage salt behind any already-encrypted
//	                portfolios/sessions, which then become unrecoverable under the old
//	                passphrase.
//	-debug <path>   Append per-iteration agent diagnostics to <path>. Disabled by default for privacy.
//	-version        Print version information and exit.
//
// Environment variables:
//
//	AURIS_OLLAMA_NUM_CTX   Override Ollama's per-request num_ctx (default 32768).
//	                       Increase to use larger context windows on capable models
//	                       (e.g. 131072 for gemma4:e2b's full 128k window). Higher
//	                       values consume more RAM for the KV cache.
//
//	OPENAI_BASE_URL        Fallback base URL for the OpenAI driver, used only when
//	OPENAI_API_KEY         no base URL / API key is configured in the setup wizard.
//	                       Unlike AURIS_OLLAMA_NUM_CTX, these follow the OpenAI SDKs'
//	                       own naming convention (no AURIS_ prefix) so any
//	                       OpenAI-compatible third party (MiniMax, DeepSeek, Groq,
//	                       OpenRouter, a self-hosted proxy, ...) already configured
//	                       via these variables works without extra setup.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/tui"
)

func main() {
	setupFlag := flag.Bool("setup", false, "Re-run the setup wizard")
	debugPath := flag.String("debug", "", "Append agent diagnostics to the given file path (disabled by default)")
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: auris [-setup] [-debug <path>] [-version]\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Environment variables:
  AURIS_OLLAMA_NUM_CTX
        Override Ollama's per-request num_ctx (default 32768). Increase to
        unlock larger context windows on capable models (e.g. 131072 for
        gemma4:e2b's full 128k window). Higher values consume more RAM for
        the KV cache.

  OPENAI_BASE_URL, OPENAI_API_KEY
        Fallback base URL / API key for the OpenAI driver, used only when
        the setup wizard hasn't configured one. Unlike AURIS_OLLAMA_NUM_CTX,
        these follow the naming convention OpenAI's own SDKs use (no AURIS_
        prefix), so any OpenAI-compatible third party (MiniMax, DeepSeek,
        Groq, OpenRouter, a self-hosted proxy, ...) already configured via
        these variables works without extra setup.
`)
	}
	flag.Parse()

	if *versionFlag {
		fmt.Println(versionString())
		return
	}

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

	if *setupFlag && tui.ConfigExists() {
		if !confirmSetupOverwrite() {
			fmt.Println("auris: aborted, existing configuration left untouched.")
			return
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

// confirmSetupOverwrite warns that -setup will replace the existing config
// file — a new passphrase and KDF salt, which also orphans any
// already-encrypted portfolios/sessions still keyed to the old passphrase —
// and requires the user to type "yes" on stdin before proceeding.
func confirmSetupOverwrite() bool {
	fmt.Println("auris: a configuration file already exists.")
	fmt.Println("Running -setup will replace it with a new passphrase and encryption key.")
	fmt.Println("Any portfolios or chat sessions already encrypted under the current passphrase")
	fmt.Println("will no longer be readable afterwards.")
	fmt.Print("Type \"yes\" to continue: ")

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	return strings.TrimSpace(answer) == "yes"
}
