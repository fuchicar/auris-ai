package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fuchicar/auris-ai/pkg/llm"
	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
	"github.com/fuchicar/auris-ai/pkg/registry"
)

// makeLLMEntries builds a deterministic two-provider list used by the
// cursor-positioning tests below.
func makeLLMEntries() []registry.LLMEntry {
	return []registry.LLMEntry{
		{Key: "ollama", DisplayName: "Ollama"},
		{Key: "gemini", DisplayName: "Google Gemini"},
	}
}

// makeModelsByProv builds a per-provider model list used by the
// cursor-positioning tests below.
func makeModelsByProv() map[string][]llm.Model {
	return map[string][]llm.Model{
		"ollama": {
			{ID: "llama3.2", Name: "Llama 3.2"},
			{ID: "qwen2.5", Name: "Qwen 2.5"},
		},
		"gemini": {
			{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash"},
			{ID: "gemini-1.5-pro", Name: "Gemini 1.5 Pro"},
		},
	}
}

// driveEnterToModelStep presses Enter through name, description, cash and
// currency so the screen lands on pcStepModel. Returns the updated model.
func driveEnterToModelStep(t *testing.T, m *portfolioCreateModel) *portfolioCreateModel {
	t.Helper()
	steps := []tea.KeyMsg{
		{Type: tea.KeyEnter}, // name
		{Type: tea.KeyEnter}, // description
		{Type: tea.KeyEnter}, // cash (empty, accepted)
		{Type: tea.KeyEnter}, // currency
	}
	updated := m
	for i, k := range steps {
		next, _ := updated.Update(k)
		got, ok := next.(*portfolioCreateModel)
		if !ok {
			t.Fatalf("step %d: expected *portfolioCreateModel, got %T", i, next)
		}
		updated = got
	}
	if updated.step != pcStepModel {
		t.Fatalf("expected pcStepModel after 4 Enters, got %d", updated.step)
	}
	return updated
}

// TestNewPortfolioCreateModel_Edit_PreselectsMatchingProviderAndModel is the
// core regression test for issue #40: editing a portfolio and pressing Enter
// through the steps must preserve its AI provider and model.
func TestNewPortfolioCreateModel_Edit_PreselectsMatchingProviderAndModel(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	existing := &portfolio.Portfolio{
		Name:       "MyPF",
		AIProvider: "gemini",
		AIModel:    "gemini-1.5-pro",
	}
	m := newPortfolioCreateModel(NewStyles(ThemeDark, 0), existing, nil, makeLLMEntries(), makeModelsByProv())

	if m.provCursor != 1 {
		t.Errorf("provCursor = %d, want 1 (gemini)", m.provCursor)
	}
	if m.modelCursor != 1 {
		t.Errorf("modelCursor = %d, want 1 (gemini-1.5-pro)", m.modelCursor)
	}

	// Drive through the wizard without touching the pickers and confirm
	// the resulting PortfolioCreateResult preserves the existing values.
	updated := driveEnterToModelStep(t, m)
	// First Enter in pmStepProvider advances to pmStepModel using m.provCursor.
	next, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(*portfolioCreateModel)
	if !ok {
		t.Fatalf("expected *portfolioCreateModel, got %T", next)
	}
	if got.modelStep != pmStepModel {
		t.Fatalf("expected pmStepModel after Enter on provider, got %d", got.modelStep)
	}
	// Second Enter submits the form.
	_, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command emitting ScreenDoneMsg, got nil")
	}
	msg, ok := cmd().(ScreenDoneMsg)
	if !ok {
		t.Fatalf("expected ScreenDoneMsg, got %T", msg)
	}
	res, ok := msg.Result.(PortfolioCreateResult)
	if !ok {
		t.Fatalf("expected PortfolioCreateResult, got %T", msg.Result)
	}
	if res.AIProvider != "gemini" {
		t.Errorf("AIProvider = %q, want %q (preserved)", res.AIProvider, "gemini")
	}
	if res.AIModel != "gemini-1.5-pro" {
		t.Errorf("AIModel = %q, want %q (preserved)", res.AIModel, "gemini-1.5-pro")
	}
	if res.EditID != existing.ID {
		t.Errorf("EditID = %q, want %q", res.EditID, existing.ID)
	}
}

// TestNewPortfolioCreateModel_Edit_ProviderFoundButModelMissing falls back to
// the first model when the existing provider is in the list but its specific
// model is not (e.g. provider removed that model).
func TestNewPortfolioCreateModel_Edit_ProviderFoundButModelMissing(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	existing := &portfolio.Portfolio{
		Name:       "MyPF",
		AIProvider: "ollama",
		AIModel:    "deleted-model",
	}
	m := newPortfolioCreateModel(NewStyles(ThemeDark, 0), existing, nil, makeLLMEntries(), makeModelsByProv())

	if m.provCursor != 0 {
		t.Errorf("provCursor = %d, want 0 (ollama)", m.provCursor)
	}
	if m.modelCursor != 0 {
		t.Errorf("modelCursor = %d, want 0 (fallback when model not found)", m.modelCursor)
	}
}

// TestNewPortfolioCreateModel_Edit_ProviderRemoved falls back to the first
// provider and first model when the portfolio's AI provider is no longer
// registered. Sanity default, no panic.
func TestNewPortfolioCreateModel_Edit_ProviderRemoved(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	existing := &portfolio.Portfolio{
		Name:       "MyPF",
		AIProvider: "anthropic",
		AIModel:    "claude-opus",
	}
	m := newPortfolioCreateModel(NewStyles(ThemeDark, 0), existing, nil, makeLLMEntries(), makeModelsByProv())

	if m.provCursor != 0 {
		t.Errorf("provCursor = %d, want 0 (fallback when provider not found)", m.provCursor)
	}
	if m.modelCursor != 0 {
		t.Errorf("modelCursor = %d, want 0 (no existing provider means no existing model)", m.modelCursor)
	}
}

// TestNewPortfolioCreateModel_Edit_SingleProvider_Matching: with exactly one
// available provider that matches the existing one, the form jumps straight
// to the model step with the existing model cursor preserved.
func TestNewPortfolioCreateModel_Edit_SingleProvider_Matching(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	entries := []registry.LLMEntry{{Key: "ollama", DisplayName: "Ollama"}}
	byProv := map[string][]llm.Model{
		"ollama": {{ID: "llama3.2"}, {ID: "qwen2.5"}},
	}
	existing := &portfolio.Portfolio{
		Name:       "MyPF",
		AIProvider: "ollama",
		AIModel:    "qwen2.5",
	}
	m := newPortfolioCreateModel(NewStyles(ThemeDark, 0), existing, nil, entries, byProv)

	if m.selProvider != "ollama" {
		t.Errorf("selProvider = %q, want %q", m.selProvider, "ollama")
	}
	if m.modelStep != pmStepModel {
		t.Errorf("modelStep = %d, want pmStepModel (%d)", m.modelStep, pmStepModel)
	}
	if m.modelCursor != 1 {
		t.Errorf("modelCursor = %d, want 1 (qwen2.5)", m.modelCursor)
	}
}

// TestNewPortfolioCreateModel_Edit_SingleProvider_NotMatching: with exactly
// one available provider that does NOT match the existing one, the form falls
// back to the only available provider and resets the model cursor to 0.
func TestNewPortfolioCreateModel_Edit_SingleProvider_NotMatching(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	entries := []registry.LLMEntry{{Key: "ollama", DisplayName: "Ollama"}}
	byProv := map[string][]llm.Model{"ollama": {{ID: "llama3.2"}}}
	existing := &portfolio.Portfolio{
		Name:       "MyPF",
		AIProvider: "anthropic",
		AIModel:    "claude-opus",
	}
	m := newPortfolioCreateModel(NewStyles(ThemeDark, 0), existing, nil, entries, byProv)

	if m.selProvider != "ollama" {
		t.Errorf("selProvider = %q, want %q (only available provider)", m.selProvider, "ollama")
	}
	if m.modelStep != pmStepModel {
		t.Errorf("modelStep = %d, want pmStepModel (%d)", m.modelStep, pmStepModel)
	}
	if m.modelCursor != 0 {
		t.Errorf("modelCursor = %d, want 0 (reset to first model of new provider)", m.modelCursor)
	}
}

// TestNewPortfolioCreateModel_Create_NoExisting: cursors stay at 0 in the
// create flow, no regression from the edit fix.
func TestNewPortfolioCreateModel_Create_NoExisting(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	m := newPortfolioCreateModel(NewStyles(ThemeDark, 0), nil, nil, makeLLMEntries(), makeModelsByProv())

	if m.provCursor != 0 {
		t.Errorf("provCursor = %d, want 0", m.provCursor)
	}
	if m.modelCursor != 0 {
		t.Errorf("modelCursor = %d, want 0", m.modelCursor)
	}
	if m.selProvider != "" {
		t.Errorf("selProvider = %q, want empty", m.selProvider)
	}
	if m.modelStep != pmStepProvider {
		t.Errorf("modelStep = %d, want pmStepProvider (%d)", m.modelStep, pmStepProvider)
	}
}

// TestPortfolioCreateModel_Update_ModelsLoadedMsg_PositionsCursors covers the
// defensively-correct path: if the screen ever receives a modelsLoadedMsg
// directly (currently intercepted by AppModel.Update, but kept correct here as
// a robustness invariant), cursors must be positioned on the existing values.
func TestPortfolioCreateModel_Update_ModelsLoadedMsg_PositionsCursors(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	existing := &portfolio.Portfolio{
		Name:       "MyPF",
		AIProvider: "gemini",
		AIModel:    "gemini-1.5-pro",
	}
	// Build with empty lists to simulate the screen being constructed before
	// models have loaded, then send modelsLoadedMsg directly.
	m := newPortfolioCreateModel(NewStyles(ThemeDark, 0), existing, nil, nil, nil)

	next, _ := m.Update(modelsLoadedMsg{
		entries: makeLLMEntries(),
		byProv:  makeModelsByProv(),
	})
	got, ok := next.(*portfolioCreateModel)
	if !ok {
		t.Fatalf("expected *portfolioCreateModel, got %T", next)
	}
	if got.provCursor != 1 {
		t.Errorf("provCursor = %d, want 1 (gemini)", got.provCursor)
	}
	if got.modelCursor != 1 {
		t.Errorf("modelCursor = %d, want 1 (gemini-1.5-pro)", got.modelCursor)
	}
	if !got.modelsReady {
		t.Error("modelsReady should be true after successful modelsLoadedMsg")
	}
}

// TestNewPortfolioCreateModel_Edit_ScrollOffKeepsCursorVisible ensures that
// when the existing provider/model sits beyond the first screenful, the
// scroll offset is advanced along with the cursor — otherwise the picker
// would render with the selection highlighted off-screen, silently defeating
// the point of issue #40 (the user has no visual confirmation their existing
// selection was preserved).
func TestNewPortfolioCreateModel_Edit_ScrollOffKeepsCursorVisible(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	var entries []registry.LLMEntry
	byProv := map[string][]llm.Model{}
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("prov%02d", i)
		entries = append(entries, registry.LLMEntry{Key: key, DisplayName: key})
		byProv[key] = []llm.Model{{ID: "m0"}, {ID: "m1"}}
	}
	existing := &portfolio.Portfolio{
		Name:       "MyPF",
		AIProvider: "prov15",
		AIModel:    "m1",
	}
	m := newPortfolioCreateModel(NewStyles(ThemeDark, 0), existing, nil, entries, byProv)

	maxVis := m.maxVisible()
	if m.provCursor < m.provScrollOff || m.provCursor >= m.provScrollOff+maxVis {
		t.Errorf("provCursor %d not within visible window [%d,%d) — selection would render off-screen",
			m.provCursor, m.provScrollOff, m.provScrollOff+maxVis)
	}
	if m.modelCursor < m.modelScrollOff || m.modelCursor >= m.modelScrollOff+maxVis {
		t.Errorf("modelCursor %d not within visible window [%d,%d) — selection would render off-screen",
			m.modelCursor, m.modelScrollOff, m.modelScrollOff+maxVis)
	}
}
