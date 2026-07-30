package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
)

// DisclaimerModel is shown once during first-run setup.
// The user must type "yes", "si", or "sí" (case-insensitive) to proceed.
//
// The legal-style body text is ~746 chars and overflows a 24-row terminal
// inside the bordered WarnBox. To respect the wording in full while still
// fitting an 80×24 terminal we embed the body in a scrollable bubbles
// viewport (issues #37). Title, label, input and confirmation hint stay
// outside the viewport so they're always visible.
type DisclaimerModel struct {
	styles   *Styles
	width    int
	height   int
	input    textinput.Model
	viewport viewport.Model
	err      string
	ready    bool
}

// disclaimerChromeReserve is the rows consumed by everything outside
// the scrollable body on an 80×24 (or smaller) terminal:
//
//   3  Title (with Padding(1, 0))
//   2  WarnBox top + bottom border
//   1  blank separator (from JoinVertical's "")
//   1–2  Label (the 65-char input label wraps to 2 rows on terminals
//        narrower than ~72 cols)
//   3  Input box (border + content + border)
//   1  Bottom hint (or error)
//
// On 80×24 the label stays on 1 row → chrome = 11 → viewport budget
// = 13. On 60×24 the label wraps to 2 rows → chrome = 12 → viewport
// budget = 12. Either way the visible block fits in 24 rows. We
// compute the budget dynamically in resizeViewport() rather than
// baking a fixed reserve here (issues #37).
//
// The hard lower bound for the viewport height is 3 (so a user on a
// truly minimal terminal can still scroll to type their answer).
const disclaimerChromeReserveHint = 11 // for 80×24 and wider; narrow terminals add +1

// disclaimerMinViewportHeight is the absolute minimum viewport height
// the disclaimer keeps, even on the tightest terminal — without it
// the user wouldn't be able to scroll to the legal text they came to
// read (issues #37).
const disclaimerMinViewportHeight = 3

func newDisclaimerModel(s *Styles) *DisclaimerModel {
	ti := textinput.New()
	ti.Placeholder = locale.T("disclaimer.input.placeholder")
	ti.CharLimit = 10
	ti.Focus()
	return &DisclaimerModel{
		input:    ti,
		styles:   s,
		viewport: viewport.New(0, 0), // sized on first WindowSizeMsg
	}
}

// Init implements [tea.Model].
func (m *DisclaimerModel) Init() tea.Cmd { return textinput.Blink }

// Update implements [tea.Model].
// Keys are first offered to the viewport (PgDn/PgUp/arrows) so the user
// can scroll the body; on Enter we validate the typed answer.
func (m *DisclaimerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewport()
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		return m.validate()
	}
	// Hand the rest to the viewport so PgDn/PgUp/arrows/End/Home work.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	// And to the text input so typing characters still works.
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *DisclaimerModel) validate() (tea.Model, tea.Cmd) {
	v := strings.ToLower(strings.TrimSpace(m.input.Value()))
	if v == "yes" || v == "si" || v == "sí" {
		return m, func() tea.Msg { return ScreenDoneMsg{From: ScreenDisclaimer} }
	}
	m.err = locale.T("disclaimer.input.error")
	m.input.SetValue("")
	return m, textinput.Blink
}

// resizeViewport (re)computes the viewport dimensions to fit the
// available terminal area and (re-)renders the body. Idempotent and
// cheap, called on every WindowSizeMsg.
func (m *DisclaimerModel) resizeViewport() {
	if m.width <= 0 {
		return
	}
	// The WarnBox outer width equals m.styles.PanelWidth; its inner
	// content area (between border + padding) is PanelWidth -
	// boxInnerInset (border 1 + padding 1 per side). The viewport body
	// must match that so it doesn't re-wrap inside the box, which
	// previously inflated the rendered height by ~2× and broke the
	// 80×24 fit (issues #37).
	bodyW := m.styles.PanelWidth - boxInnerInset
	// The fixed chrome outside the viewport is title (3 rows) +
	// WarnBox borders (2) + blank separator (1) + label (1–2 rows,
	// measured below) + input box (3) + bottom (1). Measure the label
	// rather than hardcoding it because its wrap count depends on the
	// current terminal width (issues #37).
	labelH := lipgloss.Height(m.styles.Warning.Render(locale.T("disclaimer.input.label")))
	chromeH := 3 /* title */ + 2 /* wb borders */ + 1 /* blank */ + labelH +
		3 /* input box */ + 1 /* bottom hint or error */
	bodyH := m.height - chromeH
	if bodyH < disclaimerMinViewportHeight {
		bodyH = disclaimerMinViewportHeight
	}
	if m.viewport.Width != bodyW || m.viewport.Height != bodyH {
		m.viewport = viewport.New(bodyW, bodyH)
	}
	wrapped := lipgloss.NewStyle().Width(bodyW).Render(locale.T("disclaimer.body"))
	m.viewport.SetContent(wrapped)
	m.ready = true
}

// View implements [tea.Model].
func (m *DisclaimerModel) View() string {
	if !m.ready {
		m.resizeViewport()
	}
	title := m.styles.Title.Render(locale.T("disclaimer.title"))
	label := m.styles.Warning.Render(locale.T("disclaimer.input.label"))
	inp := m.styles.Input.Render(m.input.View())
	body := m.styles.WarnBox.Render(m.viewport.View())

	var bottom string
	if m.err != "" {
		bottom = m.styles.Error.Render("✗ " + m.err)
	} else {
		scrollHint := m.styles.Hint.Render(locale.T("disclaimer.scroll_hint"))
		bottom = lipgloss.JoinHorizontal(lipgloss.Left,
			m.styles.Hint.Render(locale.T("disclaimer.input.hint")),
			"  ",
			scrollHint,
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left, title, body, "", label, inp, bottom)
}
