package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/llm"
	"auris/pkg/locale"
	"auris/pkg/portfolio"
	"auris/pkg/registry"
)

// PortfolioCreateResult is emitted when a portfolio has been created or edited.
type PortfolioCreateResult struct {
	Name        string
	Description string
	Cash        float64
	AIProvider  string
	AIModel     string
	EditID      string // non-empty when editing an existing portfolio
}

// portfolioCreateStep tracks the current sub-step of the form.
type portfolioCreateStep int

const (
	pcStepName  portfolioCreateStep = iota // text input for portfolio name
	pcStepDesc                             // text input for description (optional)
	pcStepCash                             // text input for available cash (optional)
	pcStepModel                            // provider/model picker (async load)
)

// portfolioModelStep tracks the two-phase model picker.
type portfolioModelStep int

const (
	pmStepProvider portfolioModelStep = iota
	pmStepModel
)

// portfolioCreateModel is the multi-step form for creating or editing a portfolio.
type portfolioCreateModel struct {
	step      portfolioCreateStep
	modelStep portfolioModelStep
	editID    string // non-empty when editing
	allNames  []string
	nameInput textinput.Model
	descInput textinput.Model
	cashInput textinput.Model
	nameErr   string
	cashErr   string
	// model picker state (same as AIDefaultModelModel)
	entries        []registry.LLMEntry
	modelsByProv   map[string][]llm.Model
	provCursor     int
	provScrollOff  int
	modelCursor    int
	modelScrollOff int
	selProvider    string
	selModel       string
	// spinner for when models are still loading
	spin        spinner.Model
	modelsReady bool
	modelErr    string
	height      int
	styles      *Styles
}

// newPortfolioCreateModel creates the form.
// existing is non-nil when editing; it pre-populates the fields.
// allPortfolios is used for uniqueness validation.
// entries and byProv come from the models-loaded message.
func newPortfolioCreateModel(
	s *Styles,
	existing *portfolio.Portfolio,
	allPortfolios []*portfolio.Portfolio,
	entries []registry.LLMEntry,
	byProv map[string][]llm.Model,
) *portfolioCreateModel {
	nameInput := textinput.New()
	nameInput.Placeholder = locale.T("portfolio.create.name.label")
	nameInput.CharLimit = 80

	descInput := textinput.New()
	descInput.Placeholder = locale.T("portfolio.create.desc.label")
	descInput.CharLimit = 200

	cashInput := textinput.New()
	cashInput.Placeholder = locale.T("portfolio.create.cash.label")
	cashInput.CharLimit = 20

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = s.Spinner

	m := &portfolioCreateModel{
		nameInput:    nameInput,
		descInput:    descInput,
		cashInput:    cashInput,
		spin:         sp,
		styles:       s,
		entries:      entries,
		modelsByProv: byProv,
		modelsReady:  len(entries) > 0,
	}

	// Build list of all portfolio names for uniqueness checking.
	for _, p := range allPortfolios {
		m.allNames = append(m.allNames, p.Name)
	}

	if existing != nil {
		m.editID = existing.ID
		nameInput.SetValue(existing.Name)
		descInput.SetValue(existing.Description)
		if existing.Cash != 0 {
			cashInput.SetValue(strconv.FormatFloat(existing.Cash, 'f', -1, 64))
		}
		// Remove the current name from uniqueness check.
		filtered := m.allNames[:0]
		for _, n := range m.allNames {
			if n != existing.Name {
				filtered = append(filtered, n)
			}
		}
		m.allNames = filtered
		// Pre-select existing provider/model.
		m.selProvider = existing.AIProvider
		m.selModel = existing.AIModel
	}

	nameInput.Focus()
	m.nameInput = nameInput
	m.descInput = descInput
	m.cashInput = cashInput

	// If only one provider, pre-select it and go straight to model step.
	if len(entries) == 1 {
		m.selProvider = entries[0].Key
		m.modelStep = pmStepModel
	}

	return m
}

func (m *portfolioCreateModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, textinput.Blink)
}

func (m *portfolioCreateModel) maxVisible() int {
	if m.height == 0 {
		return 12
	}
	n := m.height - 6
	if n < 3 {
		return 3
	}
	return n
}

func (m *portfolioCreateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		if !m.modelsReady {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case modelsLoadedMsg:
		if msg.err != nil {
			m.modelErr = locale.Tp("portfolio.create.model.error", map[string]any{"Error": msg.err.Error()})
		} else {
			m.entries = msg.entries
			m.modelsByProv = msg.byProv
			m.modelsReady = true
			if len(m.entries) == 1 {
				m.selProvider = m.entries[0].Key
				m.modelStep = pmStepModel
			}
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *portfolioCreateModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case pcStepName:
		return m.handleNameStep(key)
	case pcStepDesc:
		return m.handleDescStep(key)
	case pcStepCash:
		return m.handleCashStep(key)
	case pcStepModel:
		return m.handleModelStep(key)
	}
	return m, nil
}

func (m *portfolioCreateModel) handleNameStep(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioCreate, Result: nil}
		}
	case tea.KeyEnter:
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			m.nameErr = locale.T("portfolio.create.name.error_empty")
			return m, nil
		}
		for _, n := range m.allNames {
			if strings.EqualFold(n, name) {
				m.nameErr = locale.T("portfolio.create.name.error_duplicate")
				return m, nil
			}
		}
		m.nameErr = ""
		m.step = pcStepDesc
		m.nameInput.Blur()
		m.descInput.Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(key)
	return m, cmd
}

func (m *portfolioCreateModel) handleDescStep(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		m.step = pcStepName
		m.descInput.Blur()
		m.nameInput.Focus()
		return m, textinput.Blink
	case tea.KeyEnter:
		m.step = pcStepCash
		m.descInput.Blur()
		m.cashInput.Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.descInput, cmd = m.descInput.Update(key)
	return m, cmd
}

func (m *portfolioCreateModel) handleCashStep(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		m.step = pcStepDesc
		m.cashInput.Blur()
		m.descInput.Focus()
		return m, textinput.Blink
	case tea.KeyEnter:
		raw := strings.TrimSpace(m.cashInput.Value())
		if raw != "" {
			cash, err := strconv.ParseFloat(raw, 64)
			if err != nil || cash < 0 {
				m.cashErr = locale.T("portfolio.create.cash.error_invalid")
				return m, nil
			}
		}
		m.cashErr = ""
		m.step = pcStepModel
		m.cashInput.Blur()
		if !m.modelsReady {
			return m, m.spin.Tick
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.cashInput, cmd = m.cashInput.Update(key)
	return m, cmd
}

func (m *portfolioCreateModel) handleModelStep(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.modelsReady {
		return m, nil // still loading
	}
	maxVis := m.maxVisible()

	switch m.modelStep {
	case pmStepProvider:
		switch key.Type {
		case tea.KeyUp:
			if m.provCursor > 0 {
				m.provCursor--
				m.provScrollOff = clampScrollOff(m.provCursor, m.provScrollOff, maxVis)
			}
		case tea.KeyDown:
			if m.provCursor < len(m.entries)-1 {
				m.provCursor++
				m.provScrollOff = clampScrollOff(m.provCursor, m.provScrollOff, maxVis)
			}
		case tea.KeyEsc:
			m.step = pcStepCash
			m.cashInput.Focus()
			return m, textinput.Blink
		case tea.KeyEnter:
			m.selProvider = m.entries[m.provCursor].Key
			m.modelCursor = 0
			m.modelScrollOff = 0
			m.modelStep = pmStepModel
		}

	case pmStepModel:
		models := m.modelsByProv[m.selProvider]
		switch key.Type {
		case tea.KeyUp:
			if m.modelCursor > 0 {
				m.modelCursor--
				m.modelScrollOff = clampScrollOff(m.modelCursor, m.modelScrollOff, maxVis)
			}
		case tea.KeyDown:
			if m.modelCursor < len(models)-1 {
				m.modelCursor++
				m.modelScrollOff = clampScrollOff(m.modelCursor, m.modelScrollOff, maxVis)
			}
		case tea.KeyEsc:
			if len(m.entries) > 1 {
				m.modelStep = pmStepProvider
			} else {
				m.step = pcStepCash
				m.cashInput.Focus()
				return m, textinput.Blink
			}
		case tea.KeyEnter:
			if len(models) == 0 {
				return m, nil
			}
			prov := m.selProvider
			modelID := models[m.modelCursor].ID
			name := strings.TrimSpace(m.nameInput.Value())
			desc := strings.TrimSpace(m.descInput.Value())
			var cash float64
			if raw := strings.TrimSpace(m.cashInput.Value()); raw != "" {
				cash, _ = strconv.ParseFloat(raw, 64)
			}
			editID := m.editID
			return m, func() tea.Msg {
				return ScreenDoneMsg{
					From: ScreenPortfolioCreate,
					Result: PortfolioCreateResult{
						Name:        name,
						Description: desc,
						Cash:        cash,
						AIProvider:  prov,
						AIModel:     modelID,
						EditID:      editID,
					},
				}
			}
		}
	}
	return m, nil
}

func (m *portfolioCreateModel) View() string {
	titleKey := "portfolio.create.title"
	if m.editID != "" {
		titleKey = "portfolio.edit.title"
	}
	title := m.styles.Title.Render(locale.T(titleKey))

	stepNum := int(m.step) + 1
	progress := m.styles.Hint.Render(locale.Tp("portfolio.create.progress", map[string]any{
		"Current": stepNum,
		"Total":   4,
	}))

	var body []string
	switch m.step {
	case pcStepName:
		label := m.styles.Subtitle.Render(locale.T("portfolio.create.name.label"))
		input := m.styles.Input.Render(m.nameInput.View())
		hint := m.styles.Hint.Render(locale.T("portfolio.create.name.hint"))
		body = append(body, label, input, hint)
		if m.nameErr != "" {
			body = append(body, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.nameErr)))
		}

	case pcStepDesc:
		label := m.styles.Subtitle.Render(locale.T("portfolio.create.desc.label"))
		input := m.styles.Input.Render(m.descInput.View())
		hint := m.styles.Hint.Render(locale.T("portfolio.create.desc.hint"))
		body = append(body, label, input, hint)

	case pcStepCash:
		label := m.styles.Subtitle.Render(locale.T("portfolio.create.cash.label"))
		input := m.styles.Input.Render(m.cashInput.View())
		hint := m.styles.Hint.Render(locale.T("portfolio.create.cash.hint"))
		body = append(body, label, input, hint)
		if m.cashErr != "" {
			body = append(body, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.cashErr)))
		}

	case pcStepModel:
		if m.modelErr != "" {
			body = append(body, m.styles.Error.Render(m.modelErr))
			break
		}
		if !m.modelsReady {
			body = append(body,
				m.styles.Subtitle.Render(locale.T("portfolio.create.model.connecting")),
				m.spin.View(),
			)
			break
		}
		body = append(body, m.viewModelPicker()...)
	}

	parts := []string{title, progress, ""}
	parts = append(parts, body...)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *portfolioCreateModel) viewModelPicker() []string {
	maxVis := m.maxVisible()

	switch m.modelStep {
	case pmStepProvider:
		title := m.styles.Subtitle.Render(locale.T("setup.ai.model.provider.label"))
		labels := make([]string, len(m.entries))
		for i, e := range m.entries {
			labels[i] = e.DisplayName
		}
		rows := m.renderScrollList(labels, m.provCursor, m.provScrollOff, maxVis)
		hint := m.styles.Hint.Render(locale.T("portfolio.create.model.hint"))
		result := []string{title}
		result = append(result, rows...)
		result = append(result, hint)
		return result

	case pmStepModel:
		var provName string
		for _, e := range m.entries {
			if e.Key == m.selProvider {
				provName = e.DisplayName
				break
			}
		}
		titleText := locale.T("setup.ai.model.model.label")
		if provName != "" {
			titleText = fmt.Sprintf("%s — %s", titleText, provName)
		}
		title := m.styles.Subtitle.Render(titleText)
		models := m.modelsByProv[m.selProvider]
		var rows []string
		if len(models) == 0 {
			rows = append(rows, m.styles.Hint.Render(locale.T("setup.ai.model.no_models")))
		} else {
			labels := make([]string, len(models))
			for i, mod := range models {
				if mod.Name != "" {
					labels[i] = mod.Name
				} else {
					labels[i] = mod.ID
				}
			}
			rows = m.renderScrollList(labels, m.modelCursor, m.modelScrollOff, maxVis)
		}
		var hintText string
		if len(m.entries) > 1 {
			hintText = locale.T("setup.ai.model.esc_back")
		} else {
			hintText = locale.T("portfolio.create.model.hint")
		}
		hint := m.styles.Hint.Render(hintText)
		result := []string{title}
		result = append(result, rows...)
		result = append(result, hint)
		return result
	}
	return nil
}

func (m *portfolioCreateModel) renderScrollList(items []string, cursor, scrollOff, maxVis int) []string {
	end := scrollOff + maxVis
	if end > len(items) {
		end = len(items)
	}
	var rows []string
	if scrollOff > 0 {
		rows = append(rows, m.styles.Hint.Render(locale.T("setup.ai.model.scroll_up")))
	}
	for i := scrollOff; i < end; i++ {
		if i == cursor {
			rows = append(rows, fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(items[i])))
		} else {
			rows = append(rows, fmt.Sprintf("  %s", m.styles.Unselected.Render(items[i])))
		}
	}
	if end < len(items) {
		rows = append(rows, m.styles.Hint.Render(locale.T("setup.ai.model.scroll_down")))
	}
	return rows
}
