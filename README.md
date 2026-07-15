# Auris

**Auris** is a terminal-based AI financial advisor. It combines real-time market data with conversational AI so you can analyse instruments, run financial calculations, and manage portfolios — entirely from your terminal.

Supported LLM backends: **Ollama** (local), **Google Gemini**, **Anthropic Claude**, **OpenAI**, and **MiniMax**.  
Supported market data: **Financial Modeling Prep (FMP)** and **EOD Historical Data (EODHD)**, combined in a fallback cascade.

---

## Features

- Interactive agent chat with access to live market data
- Instrument search across stocks, ETFs, forex, futures, and crypto
- Real-time quotes, historical candles, fundamentals, and corporate actions
- Built-in financial calculators: ROI, CAGR, Sharpe ratio, VaR, DCF, volatility, and more
- Financial news aggregation from configurable RSS/Atom feeds
- Portfolio management with lot tracking and P&L
- Encrypted configuration at rest (AES-256-GCM + Argon2id)
- Persistent conversation sessions
- English and Spanish UI (auto-detected from locale)
- Light and dark themes

---

## Requirements

| Dependency | Version |
|---|---|
| Go | ≥ 1.25 |
| FMP API key | [financialmodelingprep.com](https://financialmodelingprep.com) |
| LLM provider | Ollama (local) or an API key for Gemini / Claude / OpenAI / MiniMax |

---

## Installation

### Build from source

```bash
git clone <repository-url>
cd auris-ai
go build -o auris ./cmd/auris
./auris
```

### With Nix

See the [Nix & NixOS](#nix--nixos) section below.

### Install to PATH

After building, copy the binary to a directory in your `$PATH`:

```bash
go install ./cmd/auris
```

### Prebuilt binaries

Tagged releases (`vX.Y.Z`) publish prebuilt Linux, macOS, and Windows binaries (amd64/arm64) via GitHub Releases, built with [GoReleaser](https://goreleaser.com). Run `auris -version` to check what a given binary was built from.

---

## Quick Start

On first launch, Auris runs a guided setup wizard:

1. **Language** — auto-detected; prompted if ambiguous
2. **Theme** — light or dark (and variants)
3. **Passphrase** — used to encrypt your API keys at rest
4. **Financial profile** — 10 questions that personalise the AI's advice
5. **Market provider** — select FMP and enter your API key
6. **AI providers** — select one or more (Ollama, Gemini, Claude, OpenAI, MiniMax) and configure each
7. **Default model** — choose the provider/model pair used by default in chat

On subsequent launches, you are only asked for your passphrase to unlock the configuration.

To re-run the setup wizard at any time:

```bash
auris -setup
```

---

## Usage

```
auris [flags]

Flags:
  -setup          Force the setup wizard, even if a config already exists
  -debug <path>   Append agent diagnostic logs to the given file
  -version        Print version information and exit
```

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `AURIS_OLLAMA_NUM_CTX` | `32768` | Context window size sent to Ollama. Increase for large models (e.g. `131072`). |
| `LC_ALL` / `LANG` | system | Determines the UI language (`en` or `es`). |

---

## Providers

### LLM providers

| Provider | Notes |
|---|---|
| **Ollama** | Local inference. Default endpoint: `http://localhost:11434`. Supports custom base URL and API key for remote instances. |
| **Google Gemini** | Requires a Gemini API key. |
| **Anthropic Claude** | Requires an Anthropic API key. Supports custom base URL. |
| **OpenAI** | Requires an OpenAI API key. Also usable against any OpenAI-compatible endpoint via custom base URL. |
| **MiniMax** | Requires a MiniMax API key. |

### Market data providers

| Provider | Notes |
|---|---|
| **Financial Modeling Prep** | Requires a free or paid FMP API key. Free tier covers most features (mainly US coverage). |
| **EOD Historical Data (EODHD)** | Optional second provider used as fallback in the cascade; adds non-US coverage (e.g. BME/Madrid). |

---

## Nix & NixOS

The repository includes a `shell.nix` (development environment) and a `default.nix` (reproducible package build).

### Development shell

Enter a shell with Go and all development tools available:

```bash
nix-shell
```

If you use [direnv](https://direnv.net), the environment is activated automatically when you enter the project directory:

```bash
direnv allow
```

The shell provides: `go`, `gopls`, `golangci-lint`, `gotools`, `git`.

### Build with Nix

```bash
nix-build
./result/bin/auris
```

### Install to your Nix user profile

```bash
nix-env -if .
auris
```

To uninstall:

```bash
nix-env -e auris
```

### NixOS system configuration

Add Auris as a system package by importing the derivation directly. In your NixOS `configuration.nix`:

```nix
{ config, pkgs, ... }:
let
  auris = pkgs.callPackage (builtins.fetchGit {
    url  = "<repository-url>";
    ref  = "main";
  } + "/default.nix") {};
in {
  environment.systemPackages = [ auris ];
}
```

### home-manager

```nix
{ config, pkgs, ... }:
let
  auris = pkgs.callPackage (builtins.fetchGit {
    url = "<repository-url>";
    ref = "main";
  } + "/default.nix") {};
in {
  home.packages = [ auris ];
}
```

### Keeping nixpkgs pinned

Classic Nix resolves nixpkgs from your system channel. To pin a specific version for fully reproducible builds, create a `nixpkgs.json` pinfile or consider migrating to a `flake.nix` (the `default.nix` and `shell.nix` in this repo are designed to be importable from a flake without duplication).

---

## Development

```bash
# Build all packages
go build ./...

# Vet
go vet ./...

# Run all tests (integration tests require API keys)
go test ./... -timeout 120s

# Run a specific test
go test ./pkg/drivers/fmp/... -run TestGetQuote_AAPL -v

# Run only unit tests (no credentials needed)
go test ./pkg/drivers/fmp/... -run TestInterfaceCompliance -v
```

Integration test credentials are read from files in each driver's `test_data/` directory (see `CLAUDE.md` for details). Tests skip automatically when credentials are absent.

---

## Configuration file location

| Platform | Path |
|---|---|
| Linux | `~/.config/auris/auris.json` |
| macOS | `~/Library/Application Support/auris/auris.json` |

API keys are encrypted at rest. The `sessions/` subdirectory alongside the config file stores conversation history.

---

## Acknowledgements

This project was developed with AI coding assistance from multiple models, including Claude Sonnet 4.6, Claude Sonnet 5, Claude Opus 4.6, Claude Opus 4.7, Fable 5, Gemini Pro 3.1, and MiniMax M3. Specific per-commit attribution is not preserved — AI assistance was used for code review, refactoring, documentation, and test generation. All architectural decisions, implementation choices, and final code authorship remain with the human contributor.
