package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fuchicar/auris-ai/pkg/llm"
	"github.com/fuchicar/auris-ai/pkg/registry"
)

// fakeAIProvider is a minimal llm.AIProvider stub used to test aiConnectCmd
// without any real network access.
type fakeAIProvider struct {
	connectErr       error
	models           []llm.Model
	listModelsCalled bool
}

func (f *fakeAIProvider) Name() string                     { return "fake" }
func (f *fakeAIProvider) Description() string              { return "fake" }
func (f *fakeAIProvider) Connect(context.Context) error    { return f.connectErr }
func (f *fakeAIProvider) Disconnect(context.Context) error { return nil }
func (f *fakeAIProvider) IsConnected() bool                { return true }
func (f *fakeAIProvider) Ping(context.Context) error       { return nil }
func (f *fakeAIProvider) ListModels(context.Context) ([]llm.Model, error) {
	f.listModelsCalled = true
	return f.models, nil
}
func (f *fakeAIProvider) Complete(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{}, nil
}
func (f *fakeAIProvider) Stream(context.Context, llm.CompletionRequest) (<-chan llm.StreamChunk, error) {
	return nil, nil
}

func emitScreenDone(t *testing.T, cmd tea.Cmd) ScreenDoneMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a non-nil command emitting ScreenDoneMsg")
	}
	msg, ok := cmd().(ScreenDoneMsg)
	if !ok {
		t.Fatalf("expected ScreenDoneMsg, got %T", msg)
	}
	return msg
}

func assertCancelled(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	msg := emitScreenDone(t, cmd)
	if msg.From != ScreenAIProviderConfig {
		t.Errorf("From = %v, want ScreenAIProviderConfig", msg.From)
	}
	if msg.Result != nil {
		t.Errorf("Result = %#v, want nil (cancelled)", msg.Result)
	}
}

// --- Esc cancels at every step ---

func TestAIProviderConfigModel_OllamaMode_EscCancels(t *testing.T) {
	entry := registry.LLMEntry{Key: "ollama", DisplayName: "Ollama"}
	m := newAIProviderConfigModel(entry, NewStyles(ThemeDark, 0))

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assertCancelled(t, cmd)
}

func TestAIProviderConfigModel_InstanceNameStep_EscCancels(t *testing.T) {
	entry := registry.LLMEntry{Key: "openai_compatible", DisplayName: "OpenAI-Compatible"}
	m := newAIProviderConfigModel(entry, NewStyles(ThemeDark, 0))
	if m.step != aiStepInstanceName {
		t.Fatalf("expected initial step aiStepInstanceName for openai_compatible, got %v", m.step)
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assertCancelled(t, cmd)
}

func TestAIProviderConfigModel_InstanceNameStep_EmptyValueDoesNotAdvance(t *testing.T) {
	entry := registry.LLMEntry{Key: "openai_compatible", DisplayName: "OpenAI-Compatible"}
	m := newAIProviderConfigModel(entry, NewStyles(ThemeDark, 0))
	m.input.SetValue("")

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.step != aiStepInstanceName {
		t.Errorf("step = %v, want aiStepInstanceName (empty name must not advance)", m.step)
	}
}

func TestAIProviderConfigModel_BaseURLStep_EscCancels(t *testing.T) {
	entry := registry.LLMEntry{Key: "openai_compatible", DisplayName: "OpenAI-Compatible"}
	m := newAIProviderConfigModel(entry, NewStyles(ThemeDark, 0))
	m.input.SetValue("DeepSeek")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.step != aiStepBaseURL {
		t.Fatalf("expected step aiStepBaseURL after naming the instance, got %v", m.step)
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assertCancelled(t, cmd)
}

func TestAIProviderConfigModel_APIKeyStep_EscCancels_NonOllama(t *testing.T) {
	entry := registry.LLMEntry{Key: "anthropic", DisplayName: "Anthropic Claude"}
	m := newAIProviderConfigModel(entry, NewStyles(ThemeDark, 0))
	if m.step != aiStepAPIKey {
		t.Fatalf("expected initial step aiStepAPIKey, got %v", m.step)
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assertCancelled(t, cmd)
}

func TestAIProviderConfigModel_APIKeyStep_EscStillSkipsForOllama(t *testing.T) {
	// Ollama's Esc-to-skip-key shortcut is deliberate, not the bug being
	// fixed here, and must keep working: it starts connecting (never emits a
	// cancel ScreenDoneMsg).
	entry := registry.LLMEntry{Key: "ollama", DisplayName: "Ollama"}
	m := newAIProviderConfigModel(entry, NewStyles(ThemeDark, 0))
	m.step = aiStepAPIKey

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.step != aiStepConnecting {
		t.Errorf("step = %v, want aiStepConnecting (Ollama should skip key and connect)", m.step)
	}
	if cmd == nil {
		t.Fatal("expected a non-nil command starting the connect")
	}
}

func TestAIProviderConfigModel_ModelNameStep_EscCancels(t *testing.T) {
	entry := registry.LLMEntry{Key: "openai_compatible", DisplayName: "OpenAI-Compatible"}
	m := newAIProviderConfigModel(entry, NewStyles(ThemeDark, 0))
	m.step = aiStepModelName

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assertCancelled(t, cmd)
}

func TestAIProviderConfigModel_ErrorStep_EscCancels(t *testing.T) {
	entry := registry.LLMEntry{Key: "anthropic", DisplayName: "Anthropic Claude"}
	m := newAIProviderConfigModel(entry, NewStyles(ThemeDark, 0))
	m.step = aiStepError
	m.errStep = aiStepAPIKey

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assertCancelled(t, cmd)
}

// --- Error retry lets the user edit instead of blindly resubmitting ---

func TestAIProviderConfigModel_ErrorStep_EnterReturnsToFailedStepForEditing(t *testing.T) {
	entry := registry.LLMEntry{Key: "anthropic", DisplayName: "Anthropic Claude"}
	m := newAIProviderConfigModel(entry, NewStyles(ThemeDark, 0))
	m.input.SetValue("bad-key")
	m.apiKey = "bad-key"
	m.err = "invalid credentials"
	m.step = aiStepError
	m.errStep = aiStepAPIKey

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.step != aiStepAPIKey {
		t.Errorf("step = %v, want aiStepAPIKey (back to editing, not a blind retry)", m.step)
	}
	if m.input.Value() != "bad-key" {
		t.Errorf("input value = %q, want %q (preserved for editing)", m.input.Value(), "bad-key")
	}
	if m.err != "" {
		t.Errorf("err = %q, want cleared", m.err)
	}
	if cmd == nil {
		t.Fatal("expected a command re-blinking the cursor")
	}
	// Must NOT jump straight back to aiStepConnecting on its own.
	if m.step == aiStepConnecting {
		t.Error("Enter after an error must not blindly reconnect with the same value")
	}
}

// --- openai_compatible: instance name -> base_url -> api_key -> model name, all in the wizard ---

func TestAIProviderConfigModel_OpenAICompatible_FullStepFlow(t *testing.T) {
	entry := registry.LLMEntry{Key: "openai_compatible", DisplayName: "OpenAI-Compatible"}
	m := newAIProviderConfigModel(entry, NewStyles(ThemeDark, 0))

	if m.step != aiStepInstanceName {
		t.Fatalf("initial step = %v, want aiStepInstanceName", m.step)
	}

	m.input.SetValue("MiniMax")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.step != aiStepBaseURL {
		t.Fatalf("step after instance name = %v, want aiStepBaseURL", m.step)
	}
	if m.instanceName != "MiniMax" {
		t.Errorf("instanceName = %q", m.instanceName)
	}

	m.input.SetValue("https://api.minimax.io/v1")
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		// textinput.Blink command expected; not fatal if nil in some bubbles
		// versions, but step must have advanced regardless.
		_ = cmd
	}
	if m.step != aiStepAPIKey {
		t.Fatalf("step after base URL = %v, want aiStepAPIKey", m.step)
	}
	if m.baseURL != "https://api.minimax.io/v1" {
		t.Errorf("baseURL = %q", m.baseURL)
	}

	m.input.SetValue("sk-test-key")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.step != aiStepModelName {
		t.Fatalf("step after API key = %v, want aiStepModelName", m.step)
	}
	if m.apiKey != "sk-test-key" {
		t.Errorf("apiKey = %q", m.apiKey)
	}

	m.input.SetValue("MiniMax-M2.7")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.step != aiStepConnecting {
		t.Fatalf("step after model name = %v, want aiStepConnecting", m.step)
	}
	if m.modelName != "MiniMax-M2.7" {
		t.Errorf("modelName = %q", m.modelName)
	}
	if m.errStep != aiStepModelName {
		t.Errorf("errStep = %v, want aiStepModelName", m.errStep)
	}
	if cmd == nil {
		t.Fatal("expected a non-nil command starting the connect")
	}
}

func TestAIProviderConfigModel_ModelNameStep_EmptyValueDoesNotAdvance(t *testing.T) {
	entry := registry.LLMEntry{Key: "openai_compatible", DisplayName: "OpenAI-Compatible"}
	m := newAIProviderConfigModel(entry, NewStyles(ThemeDark, 0))
	m.step = aiStepModelName
	m.input.SetValue("")

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.step != aiStepModelName {
		t.Errorf("step = %v, want aiStepModelName (empty model must not advance)", m.step)
	}
}

// --- aiConnectCmd: manual model name bypasses ListModels ---

func TestAiConnectCmd_ManualModelSkipsListModels(t *testing.T) {
	fake := &fakeAIProvider{}
	entry := registry.LLMEntry{
		Key: "openai_compatible",
		New: func(baseURL, apiKey string) llm.AIProvider { return fake },
	}

	msg, ok := aiConnectCmd(entry, "https://example.com/v1", "key", "my-model")().(aiConnectResultMsg)
	if !ok {
		t.Fatalf("expected aiConnectResultMsg")
	}
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if fake.listModelsCalled {
		t.Error("ListModels should not be called when a manual model name is given")
	}
	if len(msg.models) != 1 || msg.models[0].ID != "my-model" {
		t.Errorf("models = %+v, want single model with ID %q", msg.models, "my-model")
	}
}

func TestAiConnectCmd_NoManualModelCallsListModels(t *testing.T) {
	fake := &fakeAIProvider{models: []llm.Model{{ID: "gpt-4o-mini", Name: "gpt-4o-mini"}}}
	entry := registry.LLMEntry{
		Key: "openai",
		New: func(baseURL, apiKey string) llm.AIProvider { return fake },
	}

	msg, ok := aiConnectCmd(entry, "", "key", "")().(aiConnectResultMsg)
	if !ok {
		t.Fatalf("expected aiConnectResultMsg")
	}
	if !fake.listModelsCalled {
		t.Error("expected ListModels to be called when no manual model name is given")
	}
	if len(msg.models) != 1 || msg.models[0].ID != "gpt-4o-mini" {
		t.Errorf("models = %+v", msg.models)
	}
}

func TestAiConnectCmd_ConnectErrorPropagates(t *testing.T) {
	fake := &fakeAIProvider{connectErr: context.DeadlineExceeded}
	entry := registry.LLMEntry{
		New: func(baseURL, apiKey string) llm.AIProvider { return fake },
	}

	msg, ok := aiConnectCmd(entry, "", "key", "")().(aiConnectResultMsg)
	if !ok {
		t.Fatalf("expected aiConnectResultMsg")
	}
	if msg.err == nil {
		t.Error("expected Connect error to propagate")
	}
	if fake.listModelsCalled {
		t.Error("ListModels must not be called when Connect fails")
	}
}
