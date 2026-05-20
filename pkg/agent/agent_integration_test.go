package agent_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"auris/pkg/agent"
	"auris/pkg/drivers/fmp"
	"auris/pkg/drivers/gemini"
	"auris/pkg/drivers/ollama"
	"auris/pkg/llm"
	"auris/pkg/market"
)

// loggingTransport logs each Gemini REST request/response to help diagnose
// "bad request" errors that have empty message bodies.
type loggingTransport struct {
	t    *testing.T
	base http.RoundTripper
}

func (lt *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	lt.t.Logf("→ %s %s  body=%s", req.Method, req.URL.Path, body)
	resp, err := lt.base.RoundTrip(req)
	if err != nil {
		lt.t.Logf("← error: %v", err)
		return resp, err
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body = io.NopCloser(bytes.NewReader(respBody))
	if resp.StatusCode >= 400 {
		lt.t.Logf("← %d ERROR: %s", resp.StatusCode, string(respBody))
	}
	return resp, err
}

// ── credential helpers ────────────────────────────────────────────────────────

func readGeminiKey(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../drivers/gemini/test_data/google_api_key")
	if err != nil {
		t.Skip("no API key in pkg/drivers/gemini/test_data/google_api_key; skipping integration test")
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		t.Skip("empty API key in pkg/drivers/gemini/test_data/google_api_key; skipping integration test")
	}
	return key
}

func readFMPKey(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../drivers/fmp/test_data/fmp_api_key")
	if err != nil {
		t.Skip("no API key in pkg/drivers/fmp/test_data/fmp_api_key; skipping integration test")
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		t.Skip("empty API key in pkg/drivers/fmp/test_data/fmp_api_key; skipping integration test")
	}
	return key
}

func skipIfRateLimited(t *testing.T, err error) {
	t.Helper()
	if errors.Is(err, llm.ErrRateLimit) {
		t.Skipf("API rate limit reached (full error: %v); skipping (re-run after a minute)", err)
	}
}

// cheapestGeminiModel picks the smallest available Gemini model to keep the
// test fast and cheap, while ensuring support for multi-turn function calling.
// Prefers versioned flash models (1.5/2.0) over unversioned aliases, which may
// lack multi-turn function calling support.
func cheapestGeminiModel(t *testing.T, d *gemini.Driver) llm.Model {
	t.Helper()
	models, err := d.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Skip("no models available; skipping integration test")
	}
	for _, m := range models {
		t.Logf("available model: %s (%s)", m.ID, m.Name)
	}
	preferences := []string{
		"3.1-flash-lite",
		"3.0-flash-lite",
		"2.5-flash",
		"2.0-flash",
		"1.5-flash-8b",
		"1.5-flash-latest",
		"1.5-flash",
		"flash-8b",
		"flash-latest",
		"flash-lite",
		"flash",
	}
	// First pass: skip preview models.
	for _, pref := range preferences {
		for _, m := range models {
			id := strings.ToLower(m.ID)
			if strings.Contains(id, pref) && !strings.Contains(id, "preview") {
				return m
			}
		}
	}
	// Fallback: allow preview models.
	for _, pref := range preferences {
		for _, m := range models {
			if strings.Contains(strings.ToLower(m.ID), pref) {
				return m
			}
		}
	}
	return models[0]
}

// ── spyMarket ─────────────────────────────────────────────────────────────────

// spyMarket wraps a real market.ProviderAPI and records whether any data query
// method was called. Connect/Disconnect/Ping lifecycle methods are not tracked.
type spyMarket struct {
	market.ProviderAPI
	t      *testing.T
	called bool
}

func (s *spyMarket) SearchInstrument(ctx context.Context, query string) ([]market.Instrument, error) {
	s.called = true
	return s.ProviderAPI.SearchInstrument(ctx, query)
}

func (s *spyMarket) GetInstrument(ctx context.Context, symbol string) (market.Instrument, error) {
	s.called = true
	return s.ProviderAPI.GetInstrument(ctx, symbol)
}

func (s *spyMarket) ListInstruments(ctx context.Context, at market.AssetType) ([]market.Instrument, error) {
	s.called = true
	return s.ProviderAPI.ListInstruments(ctx, at)
}

func (s *spyMarket) GetCandles(ctx context.Context, symbol string, from, to time.Time, tf market.Timeframe) ([]market.Candle, error) {
	s.called = true
	res, err := s.ProviderAPI.GetCandles(ctx, symbol, from, to, tf)
	if s.t != nil {
		s.t.Logf("GetCandles(%q, %s→%s, %q) → %d candles, err=%v", symbol, from.Format("2006-01-02"), to.Format("2006-01-02"), tf, len(res), err)
	}
	return res, err
}

func (s *spyMarket) GetTicks(ctx context.Context, symbol string, from, to time.Time) ([]market.Tick, error) {
	s.called = true
	return s.ProviderAPI.GetTicks(ctx, symbol, from, to)
}

func (s *spyMarket) GetOrderBook(ctx context.Context, symbol string, depth int) (market.OrderBook, error) {
	s.called = true
	return s.ProviderAPI.GetOrderBook(ctx, symbol, depth)
}

func (s *spyMarket) GetQuote(ctx context.Context, symbol string) (market.Quote, error) {
	s.called = true
	res, err := s.ProviderAPI.GetQuote(ctx, symbol)
	if s.t != nil {
		s.t.Logf("GetQuote(%q) → last=%.2f, err=%v", symbol, res.Last, err)
	}
	return res, err
}

func (s *spyMarket) GetCorporateActions(ctx context.Context, symbol string, from, to time.Time) ([]market.CorporateAction, error) {
	s.called = true
	return s.ProviderAPI.GetCorporateActions(ctx, symbol, from, to)
}

func (s *spyMarket) GetFundamentals(ctx context.Context, symbol string) (market.Fundamental, error) {
	s.called = true
	return s.ProviderAPI.GetFundamentals(ctx, symbol)
}

// requireOllamaRunning skips if Ollama is not reachable at localhost:11434.
func requireOllamaRunning(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:11434/api/tags", nil)
	if err != nil {
		t.Skip("Ollama not running at localhost:11434; skipping integration test")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Skip("Ollama not running at localhost:11434; skipping integration test")
	}
	resp.Body.Close()
}

// readOllamaToolModel reads a model name from a file.
// Not all Ollama models support function calling, so an explicit file is required.
func readOllamaToolModel(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../drivers/ollama/test_data/ollama_tool_model")
	if err != nil {
		t.Skip("no model in pkg/drivers/ollama/test_data/ollama_tool_model; skipping integration test")
	}
	model := strings.TrimSpace(string(data))
	if model == "" {
		t.Skip("empty model in pkg/drivers/ollama/test_data/ollama_tool_model; skipping integration test")
	}
	return model
}

// ── integration test ──────────────────────────────────────────────────────────

func TestAgentChat_AAPLQuote_UsesTools_Ollama(t *testing.T) {
	requireOllamaRunning(t)
	modelID := readOllamaToolModel(t)
	fmpKey := readFMPKey(t)

	ctx := context.Background()

	ollamaDriver := ollama.New()
	if err := ollamaDriver.Connect(ctx); err != nil {
		t.Fatalf("ollama Connect: %v", err)
	}
	t.Cleanup(func() { _ = ollamaDriver.Disconnect(context.Background()) })
	t.Logf("using model: %s", modelID)

	fmpDriver := fmp.New(fmpKey)
	if err := fmpDriver.Connect(ctx); err != nil {
		t.Fatalf("fmp Connect: %v", err)
	}
	t.Cleanup(func() { _ = fmpDriver.Disconnect(context.Background()) })

	spy := &spyMarket{ProviderAPI: fmpDriver, t: t}

	a := agent.New(ollamaDriver, spy, modelID)

	sysMsg := agent.BuildSystemMessage(llm.TaskChat, nil)
	messages := []llm.Message{}
	if sysMsg != nil {
		messages = append(messages, *sysMsg)
	}
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: "Responde únicamente con el valor de la acción de Apple de ayer",
	})

	runCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	resp, err := a.Chat(runCtx, messages)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response content")
	}
	if !spy.called {
		t.Error("agent did not call any market tool; expected at least one tool call to fetch the data")
	}
}

func TestAgentChat_AAPLQuote_UsesTools(t *testing.T) {
	geminiKey := readGeminiKey(t)
	fmpKey := readFMPKey(t)

	ctx := context.Background()

	geminiDriver := gemini.New(geminiKey)
	if err := geminiDriver.Connect(ctx); err != nil {
		t.Fatalf("gemini Connect: %v", err)
	}
	t.Cleanup(func() { _ = geminiDriver.Disconnect(context.Background()) })

	model := cheapestGeminiModel(t, geminiDriver)
	t.Logf("using model: %s", model.ID)

	fmpDriver := fmp.New(fmpKey)
	if err := fmpDriver.Connect(ctx); err != nil {
		t.Fatalf("fmp Connect: %v", err)
	}
	t.Cleanup(func() { _ = fmpDriver.Disconnect(context.Background()) })

	spy := &spyMarket{ProviderAPI: fmpDriver, t: t}

	a := agent.New(geminiDriver, spy, model.ID)

	sysMsg := agent.BuildSystemMessage(llm.TaskChat, nil)
	messages := []llm.Message{}
	if sysMsg != nil {
		messages = append(messages, *sysMsg)
	}
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: "Responde únicamente con el valor de la acción de Apple de ayer",
	})

	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := a.Chat(runCtx, messages)
	skipIfRateLimited(t, err)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response content")
	}
	if !spy.called {
		t.Error("agent did not call any market tool; expected at least one tool call to fetch the data")
	}
}
