package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
)

// dataModeOptions lists the two states in display order.
var dataModeOptions = []struct {
	simulation bool
	labelKey   string
}{
	{false, "datamode.real"},
	{true, "datamode.simulation"},
}

// DataModeModel lets the user choose, during first-run setup, whether to
// configure a real market data provider (API key required) or run in
// simulation mode (fully synthetic data, no signup needed). Choosing
// simulation mode skips ScreenProvider/ScreenAPIKey/ScreenAPIKeySecondary
// entirely (see AppModel.transition's ScreenDataMode case).
type DataModeModel struct {
	cursor int
	styles *Styles
}

// newDataModeModel constructs a [DataModeModel], defaulting the cursor to
// "real provider" (the safer, previously-only option).
func newDataModeModel(s *Styles) *DataModeModel {
	return &DataModeModel{cursor: 0, styles: s}
}

// Init implements [tea.Model].
func (m *DataModeModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model]. Arrow keys move the cursor; Enter confirms.
// This is a mandatory first-run step — Esc does not skip it, but does go back
// one step (to the Profile screen).
func (m *DataModeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(dataModeOptions)-1 {
				m.cursor++
			}
		case tea.KeyEsc:
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenDataMode, Result: nil}
			}
		case tea.KeyEnter:
			chosen := dataModeOptions[m.cursor].simulation
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenDataMode, Result: DataModeResult{Simulation: chosen}}
			}
		}
	}
	return m, nil
}

// View implements [tea.Model].
func (m *DataModeModel) View() string {
	title := m.styles.Subtitle.Render(locale.T("datamode.title"))

	var rows []string
	for i, opt := range dataModeOptions {
		name := locale.T(opt.labelKey)
		var row string
		if i == m.cursor {
			row = fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(name))
		} else {
			row = fmt.Sprintf("  %s", m.styles.Unselected.Render(name))
		}
		rows = append(rows, row)
	}

	hint := m.styles.Hint.Render(locale.T("datamode.hint") + "  " + locale.T("hint.esc_back"))

	parts := []string{title}
	parts = append(parts, rows...)
	parts = append(parts, hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// simulationModeOptions lists the two states in display order.
var simulationModeOptions = []struct {
	enabled  bool
	labelKey string
}{
	{true, "simulation.enabled"},
	{false, "simulation.disabled"},
}

// SimulationModeModel lets the user toggle simulation mode after first-run
// setup, reached only from the Configuration menu (/simulation).
type SimulationModeModel struct {
	cursor int
	styles *Styles
}

// newSimulationModeModel constructs a [SimulationModeModel], preselecting the
// currently active state.
func newSimulationModeModel(s *Styles, currentlyEnabled bool) *SimulationModeModel {
	cursor := 1
	if currentlyEnabled {
		cursor = 0
	}
	return &SimulationModeModel{cursor: cursor, styles: s}
}

// Init implements [tea.Model].
func (m *SimulationModeModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model]. Arrow keys move the cursor; Enter confirms; Esc cancels.
func (m *SimulationModeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(simulationModeOptions)-1 {
				m.cursor++
			}
		case tea.KeyEsc:
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenSimulationMode, Result: nil}
			}
		case tea.KeyEnter:
			chosen := simulationModeOptions[m.cursor].enabled
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenSimulationMode, Result: SimulationModeResult{Enabled: chosen}}
			}
		}
	}
	return m, nil
}

// View implements [tea.Model].
func (m *SimulationModeModel) View() string {
	title := m.styles.Subtitle.Render(locale.T("simulation.title"))

	var rows []string
	for i, opt := range simulationModeOptions {
		name := locale.T(opt.labelKey)
		var row string
		if i == m.cursor {
			row = fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(name))
		} else {
			row = fmt.Sprintf("  %s", m.styles.Unselected.Render(name))
		}
		rows = append(rows, row)
	}

	hintText := locale.T("simulation.hint") + "  " + locale.T("hint.esc_back")
	hint := m.styles.Hint.Render(hintText)

	parts := []string{title}
	parts = append(parts, rows...)
	parts = append(parts, hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
