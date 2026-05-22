package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/config"
	"auris/pkg/locale"
)

// ProfileModel drives the 10-question financial profile wizard, one question
// at a time. Single-select questions use ↑↓ + Enter; multi-select questions
// use Space to toggle and Enter to confirm. Options that require free text
// (e.g. "other" or "country only") switch the model into inputMode.
type ProfileModel struct {
	question   int                       // index of the current question (0–9)
	cursor     int                       // focused option row
	singleSel  map[int]string            // q index → selected option key
	multiSel   map[int]map[string]bool   // q index → option key → selected
	textValues map[int]map[string]string // q index → option key → typed text
	inputMode  bool                      // true while the free-text field is active
	activeKey  string                    // option key that triggered the text field
	freeInput  textinput.Model
	styles     *Styles
}

// newProfileModel constructs a [ProfileModel] at question 0. If existing is
// non-nil the model is pre-populated with the stored answers so the user only
// needs to change what they want to update.
func newProfileModel(s *Styles, existing *config.FinancialProfile) *ProfileModel {
	ti := textinput.New()
	ti.Placeholder = "..."
	m := &ProfileModel{
		singleSel:  make(map[int]string),
		multiSel:   make(map[int]map[string]bool),
		textValues: make(map[int]map[string]string),
		freeInput:  ti,
		styles:     s,
	}
	if existing != nil {
		m.initFromProfile(existing)
	}
	return m
}

// initFromProfile pre-populates the answer maps from a previously saved
// FinancialProfile, mirroring the inverse of buildProfile.
func (m *ProfileModel) initFromProfile(p *config.FinancialProfile) {
	setSingle := func(q int, v string) {
		if v != "" {
			m.singleSel[q] = v
		}
	}
	setMulti := func(q int, keys []string) {
		if len(keys) == 0 {
			return
		}
		m.multiSel[q] = make(map[string]bool)
		for _, k := range keys {
			m.multiSel[q][k] = true
		}
	}
	setText := func(q int, key, val string) {
		if val == "" {
			return
		}
		if m.textValues[q] == nil {
			m.textValues[q] = make(map[string]string)
		}
		m.textValues[q][key] = val
	}

	setSingle(0, p.LifeStage)
	setSingle(1, p.IncomeStability)
	setSingle(2, p.EmergencyFund)
	setMulti(3, p.InvestmentGoals)
	setText(3, "other", p.InvestmentGoalsOther)
	setSingle(4, p.TimeHorizon)
	setSingle(5, p.LossScenario)
	setSingle(6, p.MaxAcceptableLoss)
	setMulti(7, p.FinancialExperience)
	setSingle(8, p.InvestmentPriority)
	setMulti(9, p.Restrictions)
	setText(9, "country_only", p.RestrictionsCountry)
}

// Init implements [tea.Model].
func (m *ProfileModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model].
func (m *ProfileModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.inputMode {
		return m.updateInputMode(msg)
	}
	return m.updateSelectMode(msg)
}

// updateInputMode handles keystrokes while the free-text input is focused.
func (m *ProfileModel) updateInputMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		// Persist typed text and return to selection mode.
		if m.textValues[m.question] == nil {
			m.textValues[m.question] = make(map[string]string)
		}
		m.textValues[m.question][m.activeKey] = m.freeInput.Value()
		m.inputMode = false
		m.freeInput.Blur()
		return m, nil
	}
	updated, cmd := m.freeInput.Update(msg)
	m.freeInput = updated
	return m, cmd
}

// updateSelectMode handles navigation and selection keystrokes.
func (m *ProfileModel) updateSelectMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	q := Questions[m.question]
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < len(q.Options)-1 {
			m.cursor++
		}

	case tea.KeySpace:
		if q.Type == MultiSelect {
			opt := q.Options[m.cursor]
			if m.multiSel[m.question] == nil {
				m.multiSel[m.question] = make(map[string]bool)
			}
			isChecked := m.multiSel[m.question][opt.Key]

			if !isChecked {
				m.multiSel[m.question][opt.Key] = true
				// Trigger text input for options that need a qualifier.
				if m.needsTextField(q, opt) {
					m.activeKey = opt.Key
					m.freeInput.SetValue("")
					m.freeInput.Focus()
					m.inputMode = true
					return m, textinput.Blink
				}
			} else {
				m.multiSel[m.question][opt.Key] = false
				// Clear stored text when the option is deselected.
				if m.textValues[m.question] != nil {
					delete(m.textValues[m.question], opt.Key)
				}
			}
		}

	case tea.KeyEnter:
		switch q.Type {
		case SingleSelect:
			m.singleSel[m.question] = q.Options[m.cursor].Key
			return m.advanceOrFinish()
		case MultiSelect:
			return m.advanceOrFinish()
		}
	}

	return m, nil
}

// needsTextField returns true for options that require a free-text qualifier.
func (m *ProfileModel) needsTextField(q QuestionDef, opt OptionDef) bool {
	if opt.ShowTextField {
		return true
	}
	if q.HasOther && opt.Key == "other" {
		return true
	}
	return false
}

// advanceOrFinish moves to the next question, or emits [ScreenDoneMsg] when
// all 10 questions have been answered.
func (m *ProfileModel) advanceOrFinish() (tea.Model, tea.Cmd) {
	if m.question < len(Questions)-1 {
		m.question++
		m.cursor = 0
		return m, nil
	}
	// Build the FinancialProfile from collected answers.
	profile := m.buildProfile()
	return m, func() tea.Msg {
		return ScreenDoneMsg{From: ScreenProfile, Result: ProfileResult{Profile: profile}}
	}
}

// buildProfile converts the internal answer maps into a [config.FinancialProfile].
func (m *ProfileModel) buildProfile() config.FinancialProfile {
	str := func(q int) string { return m.singleSel[q] }
	multi := func(q int) []string {
		sel := m.multiSel[q]
		var out []string
		for _, opt := range Questions[q].Options {
			if sel[opt.Key] {
				out = append(out, opt.Key)
			}
		}
		return out
	}
	text := func(q int, key string) string {
		if m.textValues[q] == nil {
			return ""
		}
		return m.textValues[q][key]
	}

	return config.FinancialProfile{
		LifeStage:            str(0),
		IncomeStability:      str(1),
		EmergencyFund:        str(2),
		InvestmentGoals:      multi(3),
		InvestmentGoalsOther: text(3, "other"),
		TimeHorizon:          str(4),
		LossScenario:         str(5),
		MaxAcceptableLoss:    str(6),
		FinancialExperience:  multi(7),
		InvestmentPriority:   str(8),
		Restrictions:         multi(9),
		RestrictionsCountry:  text(9, "country_only"),
	}
}

// View implements [tea.Model].
func (m *ProfileModel) View() string {
	q := Questions[m.question]

	progress := m.styles.Hint.Render(
		locale.Tp("profile.progress", map[string]any{
			"Current": m.question + 1,
			"Total":   len(Questions),
		}),
	)
	label := m.styles.Subtitle.Render(locale.T(q.LabelKey))

	var rows []string
	for i, opt := range q.Options {
		label := locale.T(opt.LabelKey)
		var row string

		switch q.Type {
		case SingleSelect:
			if i == m.cursor {
				row = fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(label))
			} else {
				row = fmt.Sprintf("  %s", m.styles.Unselected.Render(label))
			}

		case MultiSelect:
			checked := m.multiSel[m.question] != nil && m.multiSel[m.question][opt.Key]
			box := "[ ]"
			if checked {
				box = "[x]"
			}
			checkbox := m.styles.Checkbox.Render(box)
			cursor := "  "
			if i == m.cursor {
				cursor = m.styles.Cursor.Render(">")
			}
			optLabel := m.styles.Unselected.Render(label)
			if i == m.cursor {
				optLabel = m.styles.Selected.Render(label)
			}
			row = fmt.Sprintf("%s %s %s", cursor, checkbox, optLabel)
		}
		rows = append(rows, row)

		// Show stored text value under checked options that collected text.
		if q.Type == MultiSelect && m.textValues[m.question] != nil {
			if v := m.textValues[m.question][opt.Key]; v != "" {
				rows = append(rows, m.styles.Hint.Render(fmt.Sprintf("    ↳ %s", v)))
			}
		}
	}

	var hintKey string
	if m.inputMode {
		hintKey = "profile.hint.other"
	} else if q.Type == MultiSelect {
		hintKey = "profile.hint.multi"
	} else {
		hintKey = "profile.hint.single"
	}
	hint := m.styles.Hint.Render(locale.T(hintKey))

	parts := []string{progress, label}
	parts = append(parts, rows...)
	if m.inputMode {
		parts = append(parts, m.styles.Input.Render(m.freeInput.View()))
	}
	parts = append(parts, hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

