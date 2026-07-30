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
//	3  Title (with Padding(1, 0))
//	2  WarnBox top + bottom border
//	1  blank separator (from JoinVertical's "")
//	1–2  Label (the 65-char input label wraps to 2 rows on terminals
//	     narrower than ~72 cols)
//	3  Input box (border + content + border)
//	1  Bottom hint (or error)
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
//
// Key dispatch rules (issues #37 follow-up review):
//
//   - WindowSizeMsg and Enter (validation) are handled explicitly.
//   - Everything else is forwarded to both the viewport (for scroll)
//     AND the text input (for typing the answer), batched via
//     tea.Batch so neither cmd is discarded — the previous
//     implementation overwrote cmd and silently dropped the viewport's
//     side (which is normally nil for viewport, but the convention
//     matters for any future change and the lints complain).
//   - Keys that bind to the viewport's default KeyMap (h, j, k, l,
//     u, d, b, f, space) scroll the body. They must NOT also be
//     inserted into the answer field as their literal character — a
//     stray 'j' or ' ' while scrolling would pollute "yes" / "si".
//     viewportConsumesKey gates the input in those cases.
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

	vpCmd := viewportUpdate(&m.viewport, msg)
	if key, ok := msg.(tea.KeyMsg); ok && viewportConsumesKey(key) {
		// The viewport is going to (or already has) consume this key
		// for scrolling; don't leak the literal character into the
		// answer field.
		return m, vpCmd
	}
	inCmd := inputUpdate(&m.input, msg)
	return m, tea.Batch(vpCmd, inCmd)
}

// viewportUpdate wraps viewport.Model.Update so the rest of this file
// doesn't repeat the address-of dance. Returned cmd is normally nil
// (the viewport doesn't issue commands).
func viewportUpdate(vp *viewport.Model, msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	*vp, cmd = vp.Update(msg)
	return cmd
}

// inputUpdate wraps textinput.Model.Update for the same reason.
func inputUpdate(in *textinput.Model, msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	return cmd
}

// viewportConsumesKey returns true when the viewport's default KeyMap
// (h/j/k/l/u/d/b/f/space) would scroll the viewport for this key, so
// the literal character should not be typed into the answer field.
// Arrows, PgUp/PgDn, Home and End are NOT filtered — the input
// component naturally ignores them, so forwarding is harmless.
func viewportConsumesKey(k tea.KeyMsg) bool {
	if k.Type == tea.KeySpace {
		return true
	}
	if k.Type != tea.KeyRunes || len(k.Runes) != 1 {
		return false
	}
	switch k.Runes[0] {
	case 'h', 'j', 'k', 'l', 'u', 'd', 'b', 'f':
		return true
	}
	return false
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
