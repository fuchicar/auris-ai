package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"

	"auris/pkg/config"
	"auris/pkg/locale"
	"auris/pkg/registry"
)

// Screen identifies which screen is currently rendered by the [AppModel].
type Screen int

const (
	ScreenWelcome    Screen = iota
	ScreenLocale            // language selection (shown when auto-detection is uncertain)
	ScreenUnlock            // passphrase prompt for an existing config
	ScreenTheme             // theme selection
	ScreenPassphrase        // passphrase creation (setup only)
	ScreenProfile           // financial profile questionnaire (setup only)
	ScreenProvider          // market data provider selection (setup only)
	ScreenAPIKey            // API key input and validation (setup only)
	ScreenMenu              // main menu
)

// FlowContext distinguishes whether a settings screen was opened during first-
// run setup or later from the main menu. It controls where the screen returns
// to when it completes.
type FlowContext int

const (
	// FlowSetup means settings changes continue the setup wizard sequence.
	FlowSetup FlowContext = iota
	// FlowMenu means settings changes save to disk and return to the main menu.
	FlowMenu
)

// ─── Message types ──────────────────────────────────────────────────────────

// ScreenDoneMsg is emitted by a child screen when it has finished its task.
// AppModel uses it to advance to the next screen.
type ScreenDoneMsg struct {
	From   Screen
	Result any // typed payload; nil for screens that produce no data
}

// ErrMsg carries a displayable error back to AppModel.
type ErrMsg struct{ Err error }

// ─── Per-screen result payloads ─────────────────────────────────────────────

// UnlockResult is the payload emitted by the Unlock screen.
type UnlockResult struct {
	Passphrase string
	Config     *config.AurisConfig
}

// ThemeResult is the payload emitted by the Theme selection screen.
type ThemeResult struct{ Theme string }

// LocaleResult is the payload emitted by the Locale selection screen.
type LocaleResult struct{ Locale string }

// PassphraseResult is the payload emitted by the Passphrase creation screen.
type PassphraseResult struct{ Passphrase string }

// ProfileResult is the payload emitted by the Profile questionnaire screen.
type ProfileResult struct{ Profile config.FinancialProfile }

// ProviderResult is the payload emitted by the Provider selection screen.
type ProviderResult struct{ Entry registry.MarketEntry }

// APIKeyResult is the payload emitted by the API key screen after a successful
// connection test.
type APIKeyResult struct {
	Entry  registry.MarketEntry
	APIKey string
}

// CommandResult is emitted by MenuModel when the user types a slash command.
// If Args is empty the caller should navigate to the relevant settings screen;
// if Args contains a value the caller may apply the change directly.
type CommandResult struct {
	Cmd  string   // e.g. "theme", "language", "exit"
	Args []string // optional: ["dark"], ["es"]
}

// ─── AppOptions ─────────────────────────────────────────────────────────────

// AppOptions configures the root [AppModel] at construction time.
type AppOptions struct {
	// SetupMode is true when the setup wizard should be shown even if a config
	// file already exists (e.g. the user passed --setup).
	SetupMode bool
	// DetectedLocale is the BCP-47 tag auto-detected from the OS environment
	// (e.g. "en" or "es"). It is stored in the config when setup completes.
	DetectedLocale string
	// ShowLocaleSelect is true when auto-detection was uncertain and the user
	// should be prompted to choose a language explicitly.
	ShowLocaleSelect bool
}

// ─── AppModel ───────────────────────────────────────────────────────────────

// AppModel is the root BubbleTea model. It owns the active screen and all
// shared session state (theme, passphrase, partial config).
type AppModel struct {
	current          tea.Model
	screen           Screen
	cfg              *config.AurisConfig
	passphrase       string         // kept in memory only — never written to disk in plaintext
	setupMode        bool
	showLocaleSelect bool
	detectedLocale   string
	flowContext      FlowContext
	styles           *Styles
	selectedEntry    registry.MarketEntry // provider chosen in ScreenProvider
	width, height    int            // current terminal dimensions (from WindowSizeMsg)
}

// NewApp constructs the root model. The Welcome screen is always shown first.
func NewApp(opts AppOptions) *AppModel {
	styles := NewStyles(ThemeDark) // default; overridden after theme selection
	a := &AppModel{
		setupMode:        opts.SetupMode,
		showLocaleSelect: opts.ShowLocaleSelect,
		detectedLocale:   opts.DetectedLocale,
		styles:           styles,
		cfg:              &config.AurisConfig{Providers: make(map[string]*config.ProviderConfig)},
	}
	a.current = newWelcomeModel(styles)
	a.screen = ScreenWelcome
	return a
}

// ConfigExists reports whether the Auris config file already exists on disk.
func ConfigExists() bool {
	p, err := config.Path()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Init implements [tea.Model]. The welcome screen needs no I/O command.
func (a *AppModel) Init() tea.Cmd {
	return a.current.Init()
}

// Update implements [tea.Model]. It intercepts global keys, window resize
// events, and screen-done messages, delegating everything else to the active
// child screen.
func (a *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return a, tea.Quit
		}
	case ScreenDoneMsg:
		return a.transition(msg)
	}

	updated, cmd := a.current.Update(msg)
	a.current = updated
	return a, cmd
}

// View implements [tea.Model]. Content is centred in the terminal using
// lipgloss.Place once the first WindowSizeMsg has been received.
func (a *AppModel) View() string {
	inner := a.current.View()
	if a.width == 0 {
		return inner // before first resize event
	}
	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center, inner)
}

// transition advances to the next screen based on the routing state machine.
func (a *AppModel) transition(msg ScreenDoneMsg) (tea.Model, tea.Cmd) {
	switch msg.From {

	case ScreenWelcome:
		if a.setupMode {
			if a.showLocaleSelect {
				a.flowContext = FlowSetup
				a.screen = ScreenLocale
				a.current = newLocaleModel(a.styles)
			} else {
				a.screen = ScreenTheme
				a.current = newThemeModel(a.styles)
			}
		} else {
			a.screen = ScreenUnlock
			a.current = newUnlockModel(a.styles)
		}

	case ScreenLocale:
		if r, ok := msg.Result.(LocaleResult); ok {
			a.detectedLocale = r.Locale
			a.cfg.Locale = r.Locale
			_ = locale.Init(r.Locale) // reinitialise translations with chosen locale
		}
		if a.flowContext == FlowMenu {
			a.saveConfig()
			a.screen = ScreenMenu
			a.current = newMenuModel(a.styles)
		} else {
			a.screen = ScreenTheme
			a.current = newThemeModel(a.styles)
		}

	case ScreenUnlock:
		if r, ok := msg.Result.(UnlockResult); ok {
			a.passphrase = r.Passphrase
			a.cfg = r.Config
			a.applyStoredTheme()
		}
		a.screen = ScreenMenu
		a.current = newMenuModel(a.styles)

	case ScreenTheme:
		if r, ok := msg.Result.(ThemeResult); ok {
			a.cfg.Theme = r.Theme
			a.styles = NewStyles(Theme(r.Theme))
		}
		if a.flowContext == FlowMenu {
			a.saveConfig()
			a.screen = ScreenMenu
			a.current = newMenuModel(a.styles)
		} else {
			a.screen = ScreenPassphrase
			a.current = newPassphraseModel(a.styles)
		}

	case ScreenPassphrase:
		if r, ok := msg.Result.(PassphraseResult); ok {
			a.passphrase = r.Passphrase
		}
		a.screen = ScreenProfile
		a.current = newProfileModel(a.styles)

	case ScreenProfile:
		if r, ok := msg.Result.(ProfileResult); ok {
			a.cfg.FinancialProfile = &r.Profile
		}
		a.screen = ScreenProvider
		a.current = newProviderModel(a.styles)

	case ScreenProvider:
		if r, ok := msg.Result.(ProviderResult); ok {
			a.selectedEntry = r.Entry
		}
		a.screen = ScreenAPIKey
		a.current = newAPIKeyModel(a.selectedEntry, a.styles)

	case ScreenAPIKey:
		if r, ok := msg.Result.(APIKeyResult); ok {
			a.cfg.ActiveProvider = r.Entry.Key
			a.cfg.Locale = a.detectedLocale
			if a.cfg.Providers == nil {
				a.cfg.Providers = make(map[string]*config.ProviderConfig)
			}
			a.cfg.Providers[r.Entry.Key] = &config.ProviderConfig{APIKey: r.APIKey}
			a.saveConfig()
		}
		a.screen = ScreenMenu
		a.current = newMenuModel(a.styles)

	case ScreenMenu:
		switch r := msg.Result.(type) {
		case nil:
			// Exit selected.
			return a, tea.Quit
		case CommandResult:
			return a.handleCommand(r)
		}
	}

	return a, a.current.Init()
}

// handleCommand processes a slash command received from the main menu.
func (a *AppModel) handleCommand(cmd CommandResult) (tea.Model, tea.Cmd) {
	switch cmd.Cmd {
	case "exit":
		return a, tea.Quit

	case "theme":
		if len(cmd.Args) == 1 {
			// Immediate apply: /theme dark or /theme light
			t := cmd.Args[0]
			if t == string(ThemeLight) || t == string(ThemeDark) {
				a.cfg.Theme = t
				a.styles = NewStyles(Theme(t))
				a.saveConfig()
			}
			a.current = newMenuModel(a.styles)
			return a, a.current.Init()
		}
		// Navigate to theme picker.
		a.flowContext = FlowMenu
		a.screen = ScreenTheme
		a.current = newThemeModel(a.styles)

	case "language":
		if len(cmd.Args) == 1 {
			// Immediate apply: /language es
			tag := cmd.Args[0]
			if err := locale.Init(tag); err == nil {
				a.detectedLocale = tag
				a.cfg.Locale = tag
				a.saveConfig()
			}
			a.current = newMenuModel(a.styles)
			return a, a.current.Init()
		}
		// Navigate to language picker.
		a.flowContext = FlowMenu
		a.screen = ScreenLocale
		a.current = newLocaleModel(a.styles)
	}

	return a, a.current.Init()
}

// saveConfig persists the current config to disk. Errors are silently ignored
// here because the caller (screen transitions) has no way to recover mid-flow;
// a future iteration could surface them via an ErrMsg overlay.
func (a *AppModel) saveConfig() {
	_ = config.Save(a.cfg, a.passphrase)
}

// applyStoredTheme rebuilds the style set using the theme persisted in the
// loaded config, ensuring the UI matches the user's saved preference.
func (a *AppModel) applyStoredTheme() {
	if a.cfg.Theme != "" {
		a.styles = NewStyles(Theme(a.cfg.Theme))
	}
}
