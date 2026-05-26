package tui

import (
	"context"
	"log"
	"os"
	"sync"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"

	"auris/pkg/agent"
	"auris/pkg/config"
	"auris/pkg/llm"
	"auris/pkg/locale"
	"auris/pkg/market"
	"auris/pkg/portfolio"
	"auris/pkg/registry"
)

// Screen identifies which screen is currently rendered by the [AppModel].
type Screen int

const (
	ScreenWelcome          Screen = iota
	ScreenLocale                  // language selection (shown when auto-detection is uncertain)
	ScreenUnlock                  // passphrase prompt for an existing config
	ScreenTheme                   // theme selection
	ScreenPassphrase              // passphrase creation (setup only)
	ScreenProfile                 // financial profile questionnaire (setup only)
	ScreenProvider                // market data provider selection (setup only)
	ScreenAPIKey                  // API key input and validation (setup only)
	ScreenAIProviderSelect        // multi-select which AI providers to configure (setup only)
	ScreenAIProviderConfig        // configure one AI provider at a time (setup only)
	ScreenAIDefaultModel          // select default AI provider and model (setup only)
	ScreenMenu                    // main menu
	ScreenAgent                   // agent chat UI
	ScreenSessionSelect           // session picker
	ScreenDisclaimer              // AI/financial disclaimer — first-run setup only
	ScreenPortfolioMenu           // portfolio list
	ScreenPortfolioCreate         // portfolio create/edit form
	ScreenPortfolioView           // portfolio dashboard
	ScreenPortfolioInstruments    // instrument list for a portfolio
	ScreenInstrumentSearch        // search + add instrument flow
	ScreenPortfolioInstrumentView // single instrument detail
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
	// FlowAgent means settings changes save to disk and return to agent mode.
	FlowAgent
	// FlowPortfolio means settings changes save to disk and return to the active portfolio view.
	FlowPortfolio
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

// modelsLoadedMsg carries the result of an async connect+list-models call
// initiated by the /model command.
type modelsLoadedMsg struct {
	entries []registry.LLMEntry
	byProv  map[string][]llm.Model
	err     error
}

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

// AIProviderSelectResult is the payload emitted by the AI provider selection
// screen. Keys is the ordered list of provider keys to configure; empty means
// the user chose to skip AI setup.
type AIProviderSelectResult struct{ Keys []string }

// AIProviderConfigResult is the payload emitted after one AI provider has been
// configured and its models listed.
type AIProviderConfigResult struct {
	Key     string
	BaseURL string
	APIKey  string
	Models  []llm.Model
}

// AIDefaultModelResult is the payload emitted after the user selects the
// default AI provider and model.
type AIDefaultModelResult struct {
	Provider string
	Model    string
}

// CommandResult is emitted by MenuModel when the user types a slash command.
// If Args is empty the caller should navigate to the relevant settings screen;
// if Args contains a value the caller may apply the change directly.
type CommandResult struct {
	Cmd  string   // e.g. "theme", "language", "agent", "exit"
	Args []string // optional: ["dark"], ["es"]
}

// SessionSelectResult is the payload emitted by the session picker screen.
type SessionSelectResult struct{ ID string }

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
	// DebugLogger, when non-nil, receives diagnostic output from the agent loop
	// and tool dispatcher. Activated by the `-debug <path>` CLI flag.
	DebugLogger *log.Logger
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

	// AI setup state — used during the setup wizard and /aiproviders management.
	pendingLLMProviders []string               // provider keys still to be configured
	pendingLLMIdx       int                    // index of the provider currently being configured
	pendingLLMModels    map[string][]llm.Model // models discovered per provider key
	managingProviders   bool                   // true when in /aiproviders flow (not initial setup)

	// debugLogger, when non-nil, is forwarded to the agent for diagnostic output.
	debugLogger *log.Logger

	// Portfolio management state.
	activePortfolio        *portfolio.Portfolio  // portfolio currently being viewed
	activeInstrument       *portfolio.Instrument // instrument currently being viewed
	instrumentSearchOrigin Screen                // where to return after instrument search
	pendingPortfolioCreate bool                  // waiting for modelsLoadedMsg to open create form
	pendingPortfolioEdit   bool                  // waiting for modelsLoadedMsg to open edit form
}

// NewApp constructs the root model. The Welcome screen is always shown first.
func NewApp(opts AppOptions) *AppModel {
	styles := NewStyles(ThemeDark) // default; overridden after theme selection
	a := &AppModel{
		setupMode:        opts.SetupMode,
		showLocaleSelect: opts.ShowLocaleSelect,
		detectedLocale:   opts.DetectedLocale,
		styles:           styles,
		cfg: &config.AurisConfig{
			Providers:   make(map[string]*config.ProviderConfig),
			AIProviders: make(map[string]*config.AIProviderConfig),
		},
		pendingLLMModels: make(map[string][]llm.Model),
		debugLogger:      opts.DebugLogger,
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
		// Always forward resize events so screens that need dimensions (e.g.
		// AgentModel's viewport) can update themselves.
		updated, cmd := a.current.Update(msg)
		a.current = updated
		return a, cmd

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return a, tea.Quit
		}
		// Global Shift+Tab toggles between menu and agent mode.
		if msg.Type == tea.KeyShiftTab && (a.screen == ScreenMenu || a.screen == ScreenAgent) {
			return a.toggleMode()
		}

	case activeSessionChangedMsg:
		a.cfg.ActiveSessionID = msg.id
		a.saveConfig()
		return a, nil

	case modelsLoadedMsg:
		if msg.err != nil {
			// Loading failed — stay on current screen silently.
			return a, nil
		}
		if a.pendingPortfolioCreate {
			a.pendingPortfolioCreate = false
			allPortfolios, _ := portfolio.ListPortfolios()
			a.screen = ScreenPortfolioCreate
			a.current = newPortfolioCreateModel(a.styles, nil, allPortfolios, msg.entries, msg.byProv)
			return a, a.current.Init()
		}
		if a.pendingPortfolioEdit {
			a.pendingPortfolioEdit = false
			allPortfolios, _ := portfolio.ListPortfolios()
			a.screen = ScreenPortfolioCreate
			a.current = newPortfolioCreateModel(a.styles, a.activePortfolio, allPortfolios, msg.entries, msg.byProv)
			return a, a.current.Init()
		}
		a.screen = ScreenAIDefaultModel
		a.current = newAIDefaultModelModel(msg.entries, msg.byProv, a.styles, true)
		return a, a.current.Init()

	case ScreenDoneMsg:
		return a.transition(msg)
	}

	updated, cmd := a.current.Update(msg)
	a.current = updated
	return a, cmd
}

// View implements [tea.Model]. Setup screens are centred; the agent chat uses
// the full terminal so it is rendered without lipgloss.Place.
func (a *AppModel) View() string {
	inner := a.current.View()
	if a.width == 0 {
		return inner // before first resize event
	}
	if a.screen == ScreenAgent || a.screen == ScreenSessionSelect {
		return inner // full-terminal layout
	}
	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center, inner)
}

// transition advances to the next screen based on the routing state machine.
func (a *AppModel) transition(msg ScreenDoneMsg) (tea.Model, tea.Cmd) {
	switch msg.From {

	case ScreenWelcome:
		if a.setupMode {
			a.screen = ScreenDisclaimer
			a.current = newDisclaimerModel(a.styles)
		} else {
			a.screen = ScreenUnlock
			a.current = newUnlockModel(a.styles)
		}

	case ScreenDisclaimer:
		if a.showLocaleSelect {
			a.flowContext = FlowSetup
			a.screen = ScreenLocale
			a.current = newLocaleModel(a.styles, false)
		} else {
			a.screen = ScreenTheme
			a.current = newThemeModel(a.styles, false)
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
			a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")
		} else {
			a.screen = ScreenTheme
			a.current = newThemeModel(a.styles, false)
		}

	case ScreenUnlock:
		if r, ok := msg.Result.(UnlockResult); ok {
			a.passphrase = r.Passphrase
			a.cfg = r.Config
			a.applyStoredTheme()
		}
		a.screen = ScreenMenu
		a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")

	case ScreenTheme:
		if r, ok := msg.Result.(ThemeResult); ok {
			a.cfg.Theme = r.Theme
			a.styles = NewStyles(Theme(r.Theme))
		}
		if a.flowContext == FlowMenu {
			a.saveConfig()
			a.screen = ScreenMenu
			a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")
		} else {
			a.screen = ScreenPassphrase
			a.current = newPassphraseModel(a.styles)
		}

	case ScreenPassphrase:
		if r, ok := msg.Result.(PassphraseResult); ok {
			a.passphrase = r.Passphrase
		}
		a.screen = ScreenProfile
		a.current = newProfileModel(a.styles, nil, false)

	case ScreenProfile:
		if r, ok := msg.Result.(ProfileResult); ok {
			a.cfg.FinancialProfile = &r.Profile
		}
		if a.flowContext == FlowMenu {
			a.saveConfig()
			a.screen = ScreenMenu
			a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")
		} else {
			a.screen = ScreenProvider
			a.current = newProviderModel(a.styles)
		}

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
			// Config saved after AI setup completes (or skipped).
		}
		a.screen = ScreenAIProviderSelect
		a.current = newAIProviderSelectModel(a.styles, nil, false)

	case ScreenAIProviderSelect:
		if r, ok := msg.Result.(AIProviderSelectResult); ok {
			if a.managingProviders {
				return a.applyProviderSelection(r.Keys)
			}
			if len(r.Keys) == 0 {
				// User skipped AI setup.
				a.saveConfig()
				a.screen = ScreenMenu
				a.current = newMenuModel(a.styles, false)
			} else {
				a.pendingLLMProviders = r.Keys
				a.pendingLLMIdx = 0
				a.pendingLLMModels = make(map[string][]llm.Model)
				a.screen = ScreenAIProviderConfig
				entry, _ := findLLMEntry(r.Keys[0])
				a.current = newAIProviderConfigModel(entry, a.styles)
			}
			return a, a.current.Init()
		}
		// nil result = cancelled via escape
		a.managingProviders = false
		if a.flowContext == FlowAgent && a.cfg.ActiveAIProvider != "" {
			return a.enterAgentMode()
		}
		a.screen = ScreenMenu
		a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")

	case ScreenAIProviderConfig:
		if r, ok := msg.Result.(AIProviderConfigResult); ok {
			if a.cfg.AIProviders == nil {
				a.cfg.AIProviders = make(map[string]*config.AIProviderConfig)
			}
			a.cfg.AIProviders[r.Key] = &config.AIProviderConfig{
				BaseURL: r.BaseURL,
				APIKey:  r.APIKey,
			}
			a.pendingLLMModels[r.Key] = r.Models
			a.pendingLLMIdx++

			if a.pendingLLMIdx < len(a.pendingLLMProviders) {
				nextKey := a.pendingLLMProviders[a.pendingLLMIdx]
				entry, _ := findLLMEntry(nextKey)
				a.screen = ScreenAIProviderConfig
				a.current = newAIProviderConfigModel(entry, a.styles)
			} else if a.managingProviders {
				// /aiproviders flow: only go to model selection when no active provider is set.
				a.managingProviders = false
				if a.cfg.ActiveAIProvider == "" {
					var entries []registry.LLMEntry
					for _, key := range a.pendingLLMProviders {
						if e, ok := findLLMEntry(key); ok {
							entries = append(entries, e)
						}
					}
					a.screen = ScreenAIDefaultModel
					a.current = newAIDefaultModelModel(entries, a.pendingLLMModels, a.styles, true)
				} else {
					a.saveConfig()
					return a.returnFromProviderManagement()
				}
			} else {
				// Setup flow: pick a default model from the newly configured providers.
				var entries []registry.LLMEntry
				for _, key := range a.pendingLLMProviders {
					if e, ok := findLLMEntry(key); ok {
						entries = append(entries, e)
					}
				}
				a.screen = ScreenAIDefaultModel
				a.current = newAIDefaultModelModel(entries, a.pendingLLMModels, a.styles, false)
			}
		}

	case ScreenAIDefaultModel:
		if r, ok := msg.Result.(AIDefaultModelResult); ok {
			a.cfg.ActiveAIProvider = r.Provider
			a.cfg.DefaultAIModel = r.Model
		}
		a.saveConfig()
		if a.flowContext == FlowAgent {
			return a.enterAgentMode()
		}
		a.screen = ScreenMenu
		a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")

	case ScreenMenu:
		switch r := msg.Result.(type) {
		case nil:
			// Exit selected.
			return a, tea.Quit
		case CommandResult:
			return a.handleCommand(r)
		}

	case ScreenAgent:
		switch r := msg.Result.(type) {
		case nil:
			a.saveConfig()
			a.screen = ScreenMenu
			a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")
		case CommandResult:
			return a.handleAgentCommand(r)
		}

	case ScreenSessionSelect:
		switch r := msg.Result.(type) {
		case SessionSelectResult:
			if a.flowContext == FlowPortfolio && a.activePortfolio != nil {
				if s, err := config.LoadSession(r.ID); err == nil && s != nil {
					a.activePortfolio.ActiveSessionID = r.ID
					_ = portfolio.SavePortfolio(a.activePortfolio)
					return a.enterPortfolioAgentModeWithSession(a.activePortfolio, s)
				}
				return a.enterPortfolioAgentMode(a.activePortfolio)
			}
			// Switch to the selected global session.
			if s, err := config.LoadSession(r.ID); err == nil && s != nil {
				a.cfg.ActiveSessionID = r.ID
				a.saveConfig()
				return a.enterAgentModeWithSession(s)
			}
			return a.enterAgentMode()
		default:
			_ = r
			// Esc: return to current session unchanged.
			if a.flowContext == FlowPortfolio && a.activePortfolio != nil {
				return a.enterPortfolioAgentMode(a.activePortfolio)
			}
			return a.enterAgentMode()
		}

	case ScreenPortfolioMenu:
		switch r := msg.Result.(type) {
		case nil:
			a.screen = ScreenMenu
			a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")
		case PortfolioMenuResult:
			switch r.Action {
			case "create":
				a.pendingPortfolioCreate = true
				return a, a.loadModelsCmd()
			case "view":
				a.activePortfolio = r.Portfolio
				mp := a.buildMarketProvider()
				a.screen = ScreenPortfolioView
				a.current = newPortfolioViewModel(a.activePortfolio, mp, a.styles)
			}
		}

	case ScreenPortfolioCreate:
		switch r := msg.Result.(type) {
		case nil:
			// Cancelled — go back to portfolio menu.
			portfolios, _ := portfolio.ListPortfolios()
			a.screen = ScreenPortfolioMenu
			a.current = newPortfolioMenuModel(a.styles, portfolios)
		case PortfolioCreateResult:
			if r.EditID != "" {
				// Edit existing portfolio.
				if p, err := portfolio.LoadPortfolio(r.EditID); err == nil && p != nil {
					p.Name = r.Name
					p.Description = r.Description
					p.AIProvider = r.AIProvider
					p.AIModel = r.AIModel
					_ = portfolio.SavePortfolio(p)
					a.activePortfolio = p
				}
			} else {
				// New portfolio.
				p := portfolio.NewPortfolio(r.Name)
				p.Description = r.Description
				p.AIProvider = r.AIProvider
				p.AIModel = r.AIModel
				_ = portfolio.SavePortfolio(p)
				a.activePortfolio = p
			}
			mp := a.buildMarketProvider()
			a.screen = ScreenPortfolioView
			a.current = newPortfolioViewModel(a.activePortfolio, mp, a.styles)
		}

	case ScreenPortfolioView:
		switch r := msg.Result.(type) {
		case nil:
			portfolios, _ := portfolio.ListPortfolios()
			a.screen = ScreenPortfolioMenu
			a.current = newPortfolioMenuModel(a.styles, portfolios)
		case PortfolioViewResult:
			switch r.Action {
			case "agent":
				a.activePortfolio = r.Portfolio
				return a.enterPortfolioAgentMode(r.Portfolio)
			case "instruments":
				a.activePortfolio = r.Portfolio
				a.screen = ScreenPortfolioInstruments
				a.current = newPortfolioInstrumentsModel(r.Portfolio, a.styles)
			case "add":
				a.activePortfolio = r.Portfolio
				a.instrumentSearchOrigin = ScreenPortfolioView
				mp := a.buildMarketProvider()
				a.screen = ScreenInstrumentSearch
				a.current = newInstrumentSearchModel(mp, a.styles)
			case "edit":
				a.activePortfolio = r.Portfolio
				a.pendingPortfolioEdit = true
				return a, a.loadModelsCmd()
			case "deleted":
				portfolios, _ := portfolio.ListPortfolios()
				a.screen = ScreenPortfolioMenu
				a.current = newPortfolioMenuModel(a.styles, portfolios)
			}
		}

	case ScreenPortfolioInstruments:
		switch r := msg.Result.(type) {
		case PortfolioInstrumentsResult:
			switch r.Action {
			case "back":
				a.activePortfolio = r.Portfolio
				mp := a.buildMarketProvider()
				a.screen = ScreenPortfolioView
				a.current = newPortfolioViewModel(a.activePortfolio, mp, a.styles)
			case "view":
				a.activePortfolio = r.Portfolio
				a.activeInstrument = r.Instrument
				mp := a.buildMarketProvider()
				a.screen = ScreenPortfolioInstrumentView
				a.current = newPortfolioInstrumentViewModel(r.Portfolio, r.Instrument, mp, a.styles)
			case "add":
				a.activePortfolio = r.Portfolio
				a.instrumentSearchOrigin = ScreenPortfolioInstruments
				mp := a.buildMarketProvider()
				a.screen = ScreenInstrumentSearch
				a.current = newInstrumentSearchModel(mp, a.styles)
			}
		}

	case ScreenInstrumentSearch:
		switch r := msg.Result.(type) {
		case nil:
			// Cancelled — go back to origin screen.
			return a.returnFromInstrumentSearch()
		case InstrumentSearchResult:
			// Add the instrument to the active portfolio.
			if a.activePortfolio != nil {
				ins := portfolio.Instrument{
					ID:     portfolio.NewInstrumentID(),
					Symbol: r.Symbol,
					Name:   r.Name,
					Type:   r.Type,
				}
				if r.Lot != nil {
					ins.Lots = []portfolio.Lot{*r.Lot}
				}
				a.activePortfolio.Instruments = append(a.activePortfolio.Instruments, ins)
				_ = portfolio.SavePortfolio(a.activePortfolio)
			}
			return a.returnFromInstrumentSearch()
		}

	case ScreenPortfolioInstrumentView:
		switch r := msg.Result.(type) {
		case PortfolioInstrumentViewResult:
			if r.Portfolio != nil {
				a.activePortfolio = r.Portfolio
			}
			a.screen = ScreenPortfolioInstruments
			a.current = newPortfolioInstrumentsModel(a.activePortfolio, a.styles)
		}
	}

	return a, a.current.Init()
}

// handleCommand processes a slash command received from the main menu.
func (a *AppModel) handleCommand(cmd CommandResult) (tea.Model, tea.Cmd) {
	switch cmd.Cmd {
	case "exit":
		return a, tea.Quit

	case "agent":
		if a.cfg.ActiveAIProvider == "" {
			// AI not configured — stay on menu (the item already shows an error
			// when clicked from nav mode; command mode just resets silently).
			a.current = newMenuModel(a.styles, false)
			return a, a.current.Init()
		}
		return a.enterAgentMode()

	case "menu":
		// /menu from anywhere (including within command mode) returns to menu.
		a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")
		return a, a.current.Init()

	case "theme":
		if len(cmd.Args) == 1 {
			// Immediate apply: /theme <name>
			t := cmd.Args[0]
			if IsValidTheme(t) {
				a.cfg.Theme = t
				a.styles = NewStyles(Theme(t))
				a.saveConfig()
			}
			a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")
			return a, a.current.Init()
		}
		// Navigate to theme picker.
		a.flowContext = FlowMenu
		a.screen = ScreenTheme
		a.current = newThemeModel(a.styles, true)

	case "language":
		if len(cmd.Args) == 1 {
			// Immediate apply: /language es
			tag := cmd.Args[0]
			if err := locale.Init(tag); err == nil {
				a.detectedLocale = tag
				a.cfg.Locale = tag
				a.saveConfig()
			}
			a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")
			return a, a.current.Init()
		}
		// Navigate to language picker.
		a.flowContext = FlowMenu
		a.screen = ScreenLocale
		a.current = newLocaleModel(a.styles, true)

	case "profile":
		a.flowContext = FlowMenu
		a.screen = ScreenProfile
		a.current = newProfileModel(a.styles, a.cfg.FinancialProfile, true)

	case "model":
		if a.cfg.ActiveAIProvider == "" {
			a.current = newMenuModel(a.styles, false)
			return a, a.current.Init()
		}
		a.flowContext = FlowMenu
		return a, a.loadModelsCmd()

	case "aiproviders":
		preSelected := make(map[string]bool, len(a.cfg.AIProviders))
		for k := range a.cfg.AIProviders {
			preSelected[k] = true
		}
		a.flowContext = FlowMenu
		a.managingProviders = true
		a.screen = ScreenAIProviderSelect
		a.current = newAIProviderSelectModel(a.styles, preSelected, true)

	case "portfolios":
		portfolios, _ := portfolio.ListPortfolios()
		a.screen = ScreenPortfolioMenu
		a.current = newPortfolioMenuModel(a.styles, portfolios)
	}

	return a, a.current.Init()
}

// handleAgentCommand processes a slash command received from the agent screen.
// Theme and language changes are applied in-place so the user stays in agent mode.
func (a *AppModel) handleAgentCommand(cmd CommandResult) (tea.Model, tea.Cmd) {
	switch cmd.Cmd {
	case "menu":
		a.saveConfig()
		if a.flowContext == FlowPortfolio && a.activePortfolio != nil {
			// Return to portfolio view instead of main menu.
			mp := a.buildMarketProvider()
			a.screen = ScreenPortfolioView
			a.current = newPortfolioViewModel(a.activePortfolio, mp, a.styles)
			return a, a.current.Init()
		}
		a.screen = ScreenMenu
		a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")
		return a, a.current.Init()

	case "exit":
		a.saveConfig()
		return a, tea.Quit

	case "new":
		if a.flowContext == FlowPortfolio && a.activePortfolio != nil {
			s := config.NewSession()
			s.PortfolioID = a.activePortfolio.ID
			_ = config.SaveSession(s)
			a.activePortfolio.ActiveSessionID = s.ID
			_ = portfolio.SavePortfolio(a.activePortfolio)
			return a.enterPortfolioAgentModeWithSession(a.activePortfolio, s)
		}
		s := config.NewSession()
		_ = config.SaveSession(s)
		a.cfg.ActiveSessionID = s.ID
		a.saveConfig()
		return a.enterAgentModeWithSession(s)

	case "session":
		if a.flowContext == FlowPortfolio && a.activePortfolio != nil {
			sessions, _ := config.ListPortfolioSessions(a.activePortfolio.ID)
			a.screen = ScreenSessionSelect
			a.current = newSessionSelectModel(sessions, a.activePortfolio.ActiveSessionID, a.styles, a.width, a.height)
			return a, a.current.Init()
		}
		sessions, _ := config.ListSessions()
		a.screen = ScreenSessionSelect
		a.current = newSessionSelectModel(sessions, a.cfg.ActiveSessionID, a.styles, a.width, a.height)
		return a, a.current.Init()

	case "theme":
		if len(cmd.Args) == 1 {
			t := cmd.Args[0]
			if IsValidTheme(t) {
				a.cfg.Theme = t
				a.styles = NewStyles(Theme(t))
				if agent, ok := a.current.(*AgentModel); ok {
					agent.styles = a.styles
				}
				a.saveConfig()
			}
		}
		return a, nil

	case "language":
		if len(cmd.Args) == 1 {
			tag := cmd.Args[0]
			if err := locale.Init(tag); err == nil {
				a.detectedLocale = tag
				a.cfg.Locale = tag
				a.saveConfig()
			}
		}
		return a, nil

	case "model":
		a.flowContext = FlowAgent
		return a, a.loadModelsCmd()

	case "aiproviders":
		preSelected := make(map[string]bool, len(a.cfg.AIProviders))
		for k := range a.cfg.AIProviders {
			preSelected[k] = true
		}
		a.flowContext = FlowAgent
		a.managingProviders = true
		a.screen = ScreenAIProviderSelect
		a.current = newAIProviderSelectModel(a.styles, preSelected, true)
		return a, a.current.Init()
	}

	return a, nil
}

// enterAgentMode resolves the active session (migrating legacy ChatHistory if
// needed) and switches to the agent chat screen.
func (a *AppModel) enterAgentMode() (tea.Model, tea.Cmd) {
	if a.cfg.ActiveAIProvider == "" || a.cfg.DefaultAIModel == "" {
		return a, nil
	}

	session := a.resolveActiveSession()
	return a.enterAgentModeWithSession(session)
}

// enterAgentModeWithSession switches to the agent chat screen using the
// provided session.
func (a *AppModel) enterAgentModeWithSession(session *config.Session) (tea.Model, tea.Cmd) {
	if a.cfg.ActiveAIProvider == "" || a.cfg.DefaultAIModel == "" {
		return a, nil
	}
	entry, ok := findLLMEntry(a.cfg.ActiveAIProvider)
	if !ok {
		return a, nil
	}
	var baseURL, apiKey string
	if aiCfg, exists := a.cfg.AIProviders[a.cfg.ActiveAIProvider]; exists {
		baseURL = aiCfg.BaseURL
		apiKey = aiCfg.APIKey
	}
	provider := entry.New(baseURL, apiKey)

	var mp market.ProviderAPI
	if a.cfg.ActiveProvider != "" {
		if pc, ok := a.cfg.Providers[a.cfg.ActiveProvider]; ok {
			for _, e := range registry.AllMarket() {
				if e.Key == a.cfg.ActiveProvider {
					mp = e.New(pc.APIKey)
					break
				}
			}
		}
	}

	a.screen = ScreenAgent
	a.current = newAgentModel(provider, mp, session, a.cfg.DefaultAIModel, a.styles, a.width, a.height, a.cfg.FinancialProfile, a.cfg.NewsFeeds, a.debugLogger, nil)
	return a, a.current.Init()
}

// resolveActiveSession loads the currently active session, creating a new one
// when none is set. Legacy ChatHistory entries are migrated to a session file.
func (a *AppModel) resolveActiveSession() *config.Session {
	// Migrate legacy flat ChatHistory to a session file.
	if a.cfg.ActiveSessionID == "" && len(a.cfg.ChatHistory) > 0 {
		s := config.NewSession()
		s.History = a.cfg.ChatHistory
		_ = config.SaveSession(s)
		a.cfg.ChatHistory = nil
		a.cfg.ActiveSessionID = s.ID
		a.saveConfig()
		return s
	}

	if a.cfg.ActiveSessionID != "" {
		if s, err := config.LoadSession(a.cfg.ActiveSessionID); err == nil && s != nil {
			return s
		}
	}

	// No active session — create a fresh one.
	s := config.NewSession()
	_ = config.SaveSession(s)
	a.cfg.ActiveSessionID = s.ID
	a.saveConfig()
	return s
}

// toggleMode switches between menu mode and agent mode.
func (a *AppModel) toggleMode() (tea.Model, tea.Cmd) {
	if a.screen == ScreenMenu {
		if a.cfg.ActiveAIProvider == "" {
			return a, nil // AI not configured — stay in menu
		}
		return a.enterAgentMode()
	}
	if a.screen == ScreenAgent {
		a.saveConfig()
		a.screen = ScreenMenu
		a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")
		return a, a.current.Init()
	}
	return a, nil
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

// applyProviderSelection processes the diff from /aiproviders: removes providers
// the user unchecked, then queues newly added providers for configuration.
func (a *AppModel) applyProviderSelection(newKeys []string) (tea.Model, tea.Cmd) {
	newSet := make(map[string]bool, len(newKeys))
	for _, k := range newKeys {
		newSet[k] = true
	}

	// Remove providers the user deselected.
	for k := range a.cfg.AIProviders {
		if !newSet[k] {
			delete(a.cfg.AIProviders, k)
			if a.cfg.ActiveAIProvider == k {
				a.cfg.ActiveAIProvider = ""
				a.cfg.DefaultAIModel = ""
			}
		}
	}

	// Collect providers that need to be added (not yet in config).
	var toAdd []string
	for _, k := range newKeys {
		if _, exists := a.cfg.AIProviders[k]; !exists {
			toAdd = append(toAdd, k)
		}
	}

	if len(toAdd) > 0 {
		a.pendingLLMProviders = toAdd
		a.pendingLLMIdx = 0
		a.pendingLLMModels = make(map[string][]llm.Model)
		a.screen = ScreenAIProviderConfig
		entry, _ := findLLMEntry(toAdd[0])
		a.current = newAIProviderConfigModel(entry, a.styles)
		return a, a.current.Init()
	}

	// Only removals — finalise without configuring new providers.
	a.managingProviders = false
	if a.cfg.ActiveAIProvider == "" && len(a.cfg.AIProviders) > 0 {
		// Active provider was removed; pick any remaining one and load its models
		// so the user can choose a new default via AIDefaultModelModel.
		for k := range a.cfg.AIProviders {
			a.cfg.ActiveAIProvider = k
			break
		}
		return a, a.loadModelsCmd()
	}
	a.saveConfig()
	return a.returnFromProviderManagement()
}

// returnFromProviderManagement navigates back to the menu or agent depending on
// the active flow context after /aiproviders completes.
func (a *AppModel) returnFromProviderManagement() (tea.Model, tea.Cmd) {
	if a.flowContext == FlowAgent && a.cfg.ActiveAIProvider != "" {
		return a.enterAgentMode()
	}
	a.screen = ScreenMenu
	a.current = newMenuModel(a.styles, a.cfg.ActiveAIProvider != "")
	return a, a.current.Init()
}

// findLLMEntry looks up a registered LLM provider by its stable key.
func findLLMEntry(key string) (registry.LLMEntry, bool) {
	for _, e := range registry.AllLLM() {
		if e.Key == key {
			return e, true
		}
	}
	return registry.LLMEntry{}, false
}

// buildMarketProvider instantiates the configured market data provider, or nil if none.
func (a *AppModel) buildMarketProvider() market.ProviderAPI {
	if a.cfg.ActiveProvider == "" {
		return nil
	}
	pc, ok := a.cfg.Providers[a.cfg.ActiveProvider]
	if !ok {
		return nil
	}
	for _, e := range registry.AllMarket() {
		if e.Key == a.cfg.ActiveProvider {
			return e.New(pc.APIKey)
		}
	}
	return nil
}

// resolvePortfolioSession returns the active session for a portfolio, creating
// and persisting a new portfolio-scoped session when none exists.
func (a *AppModel) resolvePortfolioSession(p *portfolio.Portfolio) *config.Session {
	if p.ActiveSessionID != "" {
		if s, err := config.LoadSession(p.ActiveSessionID); err == nil && s != nil {
			return s
		}
	}
	s := config.NewSession()
	s.PortfolioID = p.ID
	_ = config.SaveSession(s)
	p.ActiveSessionID = s.ID
	_ = portfolio.SavePortfolio(p)
	return s
}

// enterPortfolioAgentMode launches agent mode scoped to a portfolio.
func (a *AppModel) enterPortfolioAgentMode(p *portfolio.Portfolio) (tea.Model, tea.Cmd) {
	if p == nil {
		return a, nil
	}
	session := a.resolvePortfolioSession(p)
	return a.enterPortfolioAgentModeWithSession(p, session)
}

// enterPortfolioAgentModeWithSession launches portfolio agent mode using the
// provided session. It uses the portfolio's assigned AI provider and model,
// and injects a portfolio-specific system prompt.
func (a *AppModel) enterPortfolioAgentModeWithSession(p *portfolio.Portfolio, session *config.Session) (tea.Model, tea.Cmd) {
	if p == nil {
		return a, nil
	}
	providerKey := p.AIProvider
	modelID := p.AIModel
	if providerKey == "" || modelID == "" {
		// Fall back to global defaults.
		providerKey = a.cfg.ActiveAIProvider
		modelID = a.cfg.DefaultAIModel
	}
	entry, ok := findLLMEntry(providerKey)
	if !ok {
		return a, nil
	}
	var baseURL, apiKey string
	if aiCfg, exists := a.cfg.AIProviders[providerKey]; exists {
		baseURL = aiCfg.BaseURL
		apiKey = aiCfg.APIKey
	}
	provider := entry.New(baseURL, apiKey)

	mp := a.buildMarketProvider()
	sysMsg := agent.BuildPortfolioSystemMessage(p, a.cfg.FinancialProfile)

	a.screen = ScreenAgent
	a.flowContext = FlowPortfolio
	a.current = newAgentModel(provider, mp, session, modelID, a.styles, a.width, a.height, a.cfg.FinancialProfile, a.cfg.NewsFeeds, a.debugLogger, sysMsg)
	return a, a.current.Init()
}

// returnFromInstrumentSearch returns to whichever screen launched the instrument search.
func (a *AppModel) returnFromInstrumentSearch() (tea.Model, tea.Cmd) {
	switch a.instrumentSearchOrigin {
	case ScreenPortfolioInstruments:
		a.screen = ScreenPortfolioInstruments
		a.current = newPortfolioInstrumentsModel(a.activePortfolio, a.styles)
	default:
		mp := a.buildMarketProvider()
		a.screen = ScreenPortfolioView
		a.current = newPortfolioViewModel(a.activePortfolio, mp, a.styles)
	}
	return a, a.current.Init()
}

// loadModelsCmd connects to all configured AI providers in parallel, lists
// their models, and returns the aggregated result as a [modelsLoadedMsg].
// Providers that fail to connect are silently skipped. Used by /model command.
func (a *AppModel) loadModelsCmd() tea.Cmd {
	type provCfg struct {
		entry   registry.LLMEntry
		baseURL string
		apiKey  string
	}
	// Snapshot provider configs in registry order before entering the goroutine.
	var cfgs []provCfg
	for _, e := range registry.AllLLM() {
		if aiCfg, ok := a.cfg.AIProviders[e.Key]; ok {
			cfgs = append(cfgs, provCfg{entry: e, baseURL: aiCfg.BaseURL, apiKey: aiCfg.APIKey})
		}
	}
	if len(cfgs) == 0 {
		return func() tea.Msg { return modelsLoadedMsg{err: context.DeadlineExceeded} }
	}

	return func() tea.Msg {
		type result struct {
			entry  registry.LLMEntry
			models []llm.Model
		}
		results := make([]result, len(cfgs))
		var wg sync.WaitGroup
		for i, cfg := range cfgs {
			wg.Add(1)
			go func(i int, cfg provCfg) {
				defer wg.Done()
				drv := cfg.entry.New(cfg.baseURL, cfg.apiKey)
				ctx, cancel := context.WithTimeout(context.Background(), aiConnectTimeout)
				defer cancel()
				if err := drv.Connect(ctx); err != nil {
					return
				}
				models, err := drv.ListModels(ctx)
				if err != nil {
					return
				}
				results[i] = result{entry: cfg.entry, models: models}
			}(i, cfg)
		}
		wg.Wait()

		byProv := make(map[string][]llm.Model, len(cfgs))
		var entries []registry.LLMEntry
		for _, r := range results {
			if r.entry.Key == "" {
				continue // provider failed to load
			}
			entries = append(entries, r.entry)
			byProv[r.entry.Key] = r.models
		}
		if len(entries) == 0 {
			return modelsLoadedMsg{err: context.DeadlineExceeded}
		}
		return modelsLoadedMsg{entries: entries, byProv: byProv}
	}
}
