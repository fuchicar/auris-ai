package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fuchicar/auris-ai/pkg/llm"
	"github.com/fuchicar/auris-ai/pkg/market"
	"github.com/fuchicar/auris-ai/pkg/news"
)

// ---- mock LLM ---------------------------------------------------------------

type mockLLM struct {
	responses []llm.CompletionResponse
	calls     int

	// streamResponses configures Stream: each call consumes the next slice in
	// order and replays it verbatim on the returned channel (the last element
	// of each slice should have Done: true). Mirrors responses/calls above.
	streamResponses [][]llm.StreamChunk
	streamCalls     int
}

func (m *mockLLM) Name() string        { return "mock" }
func (m *mockLLM) Description() string { return "" }
func (m *mockLLM) Connect(ctx context.Context) error {
	return nil
}
func (m *mockLLM) Disconnect(ctx context.Context) error { return nil }
func (m *mockLLM) IsConnected() bool                    { return true }
func (m *mockLLM) Ping(ctx context.Context) error       { return nil }
func (m *mockLLM) ListModels(ctx context.Context) ([]llm.Model, error) {
	return nil, nil
}
func (m *mockLLM) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamChunk, error) {
	if m.streamCalls >= len(m.streamResponses) {
		return nil, errors.New("mock: no more stream responses")
	}
	chunks := m.streamResponses[m.streamCalls]
	m.streamCalls++
	ch := make(chan llm.StreamChunk, len(chunks))
	go func() {
		defer close(ch)
		for _, c := range chunks {
			select {
			case ch <- c:
			case <-ctx.Done():
				return
			}
			if c.Done {
				return
			}
		}
	}()
	return ch, nil
}
func (m *mockLLM) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	if m.calls >= len(m.responses) {
		return llm.CompletionResponse{}, errors.New("mock: no more responses")
	}
	resp := m.responses[m.calls]
	m.calls++
	return resp, nil
}

// ---- mock market provider ---------------------------------------------------

type mockMarket struct {
	quote       market.Quote
	fundamental market.Fundamental
	quoteErr    error
	fundErr     error
	// quotesBySymbol, when set, takes precedence over `quote` for GetQuote.
	// Lets individual tests return per-symbol prices without changing the
	// shape of the mock.
	quotesBySymbol map[string]market.Quote
	// quoteErrBySymbol, when set, overrides quoteErr for that symbol.
	quoteErrBySymbol map[string]error
	// fundamentalsBySymbol, when set, takes precedence over `fundamental`.
	fundamentalsBySymbol map[string]market.Fundamental
	// candlesBySymbol, when set, takes precedence over candlesErr for that
	// symbol's GetCandles call regardless of the requested range.
	candlesBySymbol map[string][]market.Candle
	// candlesErr is returned by GetCandles for symbols not in candlesBySymbol;
	// defaults to market.ErrNotSupported (the zero value) like the real FMP-less case.
	candlesErr error
}

func (m *mockMarket) Name() string                           { return "mock" }
func (m *mockMarket) DocsURL() string                        { return "" }
func (m *mockMarket) Description() string                    { return "" }
func (m *mockMarket) Connect(ctx context.Context) error      { return nil }
func (m *mockMarket) Disconnect(ctx context.Context) error   { return nil }
func (m *mockMarket) RefreshToken(ctx context.Context) error { return nil }
func (m *mockMarket) Ping(ctx context.Context) error         { return nil }
func (m *mockMarket) IsConnected() bool                      { return true }

func (m *mockMarket) SearchInstrument(ctx context.Context, query string) ([]market.Instrument, error) {
	return nil, market.ErrNotSupported
}
func (m *mockMarket) GetInstrument(ctx context.Context, symbol string) (market.Instrument, error) {
	return market.Instrument{}, market.ErrNotSupported
}
func (m *mockMarket) ListInstruments(ctx context.Context, at market.AssetType) ([]market.Instrument, error) {
	return nil, market.ErrNotSupported
}
func (m *mockMarket) GetCandles(ctx context.Context, symbol string, from, to time.Time, tf market.Timeframe) ([]market.Candle, error) {
	if c, ok := m.candlesBySymbol[symbol]; ok {
		return c, nil
	}
	if m.candlesErr != nil {
		return nil, m.candlesErr
	}
	return nil, market.ErrNotSupported
}
func (m *mockMarket) GetTicks(ctx context.Context, symbol string, from, to time.Time) ([]market.Tick, error) {
	return nil, market.ErrNotSupported
}
func (m *mockMarket) GetCorporateActions(ctx context.Context, symbol string, from, to time.Time) ([]market.CorporateAction, error) {
	return nil, market.ErrNotSupported
}
func (m *mockMarket) GetQuote(ctx context.Context, symbol string) (market.Quote, error) {
	if q, ok := m.quotesBySymbol[symbol]; ok {
		if err, has := m.quoteErrBySymbol[symbol]; has {
			return q, err
		}
		return q, nil
	}
	return m.quote, m.quoteErr
}
func (m *mockMarket) GetOrderBook(ctx context.Context, symbol string, depth int) (market.OrderBook, error) {
	return market.OrderBook{}, market.ErrNotSupported
}
func (m *mockMarket) SubscribeQuotes(ctx context.Context, symbol string) (<-chan market.Quote, error) {
	return nil, market.ErrNotSupported
}
func (m *mockMarket) SubscribeTrades(ctx context.Context, symbol string) (<-chan market.Tick, error) {
	return nil, market.ErrNotSupported
}
func (m *mockMarket) SubscribeOrderBook(ctx context.Context, symbol string, depth int) (<-chan market.OrderBook, error) {
	return nil, market.ErrNotSupported
}
func (m *mockMarket) GetFundamentals(ctx context.Context, symbol string) (market.Fundamental, error) {
	if f, ok := m.fundamentalsBySymbol[symbol]; ok {
		return f, nil
	}
	return m.fundamental, m.fundErr
}

// capabilityMockMarket layers market.CapabilityReporter on top of mockMarket,
// so New() wiring tests can construct a provider that declares unsupported
// tools without changing mockMarket's default (unknown-capability) shape.
type capabilityMockMarket struct {
	mockMarket
	unsupported []string
}

func (m *capabilityMockMarket) UnsupportedTools() []string { return m.unsupported }

// ---- helpers ----------------------------------------------------------------

func toolCallArgs(t *testing.T, args map[string]any) string {
	t.Helper()
	b, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ---- tests ------------------------------------------------------------------

func TestBuildTools_Count(t *testing.T) {
	tools := buildTools()
	if len(tools) != 58 {
		t.Errorf("expected 58 tools, got %d", len(tools))
	}
}

func TestBuildTools_NamesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range buildTools() {
		name := tool.Function.Name
		if seen[name] {
			t.Errorf("duplicate tool name: %s", name)
		}
		seen[name] = true
	}
}

func TestBuildTools_AllHaveType(t *testing.T) {
	for _, tool := range buildTools() {
		if tool.Type != "function" {
			t.Errorf("tool %s has type %q, want \"function\"", tool.Function.Name, tool.Type)
		}
	}
}

func TestFilterTools_RemovesUnsupported(t *testing.T) {
	tools := []llm.Tool{
		{Function: llm.ToolFunction{Name: "a"}},
		{Function: llm.ToolFunction{Name: "b"}},
		{Function: llm.ToolFunction{Name: "c"}},
	}
	got := filterTools(tools, []string{"b"})
	if len(got) != 2 || got[0].Function.Name != "a" || got[1].Function.Name != "c" {
		t.Errorf("expected [a c], got %v", got)
	}
}

func TestFilterTools_NilOrEmptyUnsupportedIsNoOp(t *testing.T) {
	tools := []llm.Tool{{Function: llm.ToolFunction{Name: "a"}}}
	if got := filterTools(tools, nil); len(got) != 1 {
		t.Errorf("expected no-op with nil, got %v", got)
	}
	if got := filterTools(tools, []string{}); len(got) != 1 {
		t.Errorf("expected no-op with empty slice, got %v", got)
	}
}

func TestNew_FiltersToolsWhenMarketDeclaresCapabilities(t *testing.T) {
	mp := &capabilityMockMarket{unsupported: []string{market.ToolGetOrderBook, market.ToolGetTicks}}
	a := New(&mockLLM{}, mp, "")
	if len(a.tools) != 56 {
		t.Errorf("expected 56 tools, got %d", len(a.tools))
	}
	for _, tool := range a.tools {
		if tool.Function.Name == market.ToolGetOrderBook || tool.Function.Name == market.ToolGetTicks {
			t.Errorf("expected %s to be filtered out, found in tools", tool.Function.Name)
		}
	}
}

func TestNew_NoFilterWhenMarketDoesNotDeclareCapabilities(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	if len(a.tools) != 58 {
		t.Errorf("expected all 58 tools when capabilities are unknown, got %d", len(a.tools))
	}
}

func TestChat_DirectAnswer(t *testing.T) {
	a := New(&mockLLM{responses: []llm.CompletionResponse{
		{
			Message:    llm.Message{Role: llm.RoleAssistant, Content: "AAPL is trading well."},
			StopReason: "stop",
		},
	}}, &mockMarket{}, "")

	msg, err := a.Chat(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "How is AAPL doing?"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "AAPL is trading well." {
		t.Errorf("unexpected content: %q", msg.Content)
	}
}

func TestChat_SingleToolCall(t *testing.T) {
	quote := market.Quote{Last: 182.50}
	mp := &mockMarket{quote: quote}

	mlm := &mockLLM{responses: []llm.CompletionResponse{
		{
			Message: llm.Message{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{{
					ID: "call_1",
					Function: llm.ToolCallFunction{
						Name:      "market_get_quote",
						Arguments: toolCallArgs(t, map[string]any{"symbol": "AAPL"}),
					},
				}},
			},
			StopReason: "tool_calls",
		},
		{
			Message:    llm.Message{Role: llm.RoleAssistant, Content: "AAPL last: 182.50"},
			StopReason: "stop",
		},
	}}

	a := New(mlm, mp, "")
	msg, err := a.Chat(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "What is the current price of AAPL?"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "AAPL last: 182.50" {
		t.Errorf("unexpected content: %q", msg.Content)
	}
	if mlm.calls != 2 {
		t.Errorf("expected 2 LLM calls, got %d", mlm.calls)
	}
}

func TestChat_MaxIterations(t *testing.T) {
	// LLM always returns tool_calls; agent must stop after maxLoopIterations.
	endless := make([]llm.CompletionResponse, maxLoopIterations+2)
	for i := range endless {
		endless[i] = llm.CompletionResponse{
			Message: llm.Message{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{{
					ID: "call_loop",
					Function: llm.ToolCallFunction{
						Name:      "market_get_quote",
						Arguments: toolCallArgs(t, map[string]any{"symbol": "X"}),
					},
				}},
			},
			StopReason: "tool_calls",
		}
	}

	a := New(&mockLLM{responses: endless}, &mockMarket{}, "")
	_, err := a.Chat(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "loop"},
	})
	if err == nil {
		t.Error("expected error when max iterations exceeded")
	}
}

func TestDispatch_GetQuote_OK(t *testing.T) {
	ts := time.Now().UTC().Truncate(time.Second)
	mp := &mockMarket{quote: market.Quote{Time: ts, Bid: 100, Ask: 101, Last: 100.5}}
	a := New(&mockLLM{}, mp, "")

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		ID: "c1",
		Function: llm.ToolCallFunction{
			Name:      "market_get_quote",
			Arguments: `{"symbol":"AAPL"}`,
		},
	}, &lk)

	var q market.Quote
	if err := json.Unmarshal([]byte(result), &q); err != nil {
		t.Fatalf("result is not valid JSON: %s — %v", result, err)
	}
	if q.Last != 100.5 {
		t.Errorf("expected Last=100.5, got %v", q.Last)
	}
}

func TestDispatch_GetQuote_MarketError(t *testing.T) {
	mp := &mockMarket{quoteErr: market.ErrNotFound}
	a := New(&mockLLM{}, mp, "")

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		ID: "c1",
		Function: llm.ToolCallFunction{
			Name:      "market_get_quote",
			Arguments: `{"symbol":"UNKNOWN"}`,
		},
	}, &lk)

	if result == "" || result[:6] != "error:" {
		t.Errorf("expected error prefix, got %q", result)
	}
}

func TestDispatch_InvalidArgs(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "market_get_quote",
			Arguments: `not json`,
		},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("expected error prefix, got %q", result)
	}
}

func TestDispatch_UnknownTool(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "market_nonexistent",
			Arguments: `{}`,
		},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("expected error prefix, got %q", result)
	}
}

func TestDispatch_FetchNews_NoProvider(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "fetch_news",
			Arguments: `{"keywords":["IBEX"]}`,
		},
	}, &lk)
	if result != `{"error":"no news provider configured"}` {
		t.Errorf("unexpected result when news provider is nil: %q", result)
	}
}

// mockNewsSource returns a fixed set of articles, used to check that the
// fetch_news dispatch wraps external RSS content with an untrusted-content
// reminder (the injection vector for this tool: article text comes from
// third-party feeds, not from the user or the system prompt).
type mockNewsSource struct {
	items []news.NewsItem
}

func (m *mockNewsSource) HandleFetchNews(_ context.Context, _ news.FetchNewsParams) ([]news.NewsItem, error) {
	return m.items, nil
}

func TestDispatch_FetchNews_WarnsUntrustedContent(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	a.SetNewsProvider(&mockNewsSource{items: []news.NewsItem{
		{Title: "Markets rally on rate-cut hopes", Summary: "Stocks rose today."},
	}})
	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "fetch_news",
			Arguments: `{"keywords":[]}`,
		},
	}, &lk)

	if !containsFold(result, "not instructions") {
		t.Errorf("expected untrusted-content reminder in result, got %q", result)
	}
	if !containsFold(result, "Markets rally on rate-cut hopes") {
		t.Errorf("expected article data to still be present in result, got %q", result)
	}
}

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func TestDispatch_ToolTrace_FiresWithNameArgsResult(t *testing.T) {
	var events []ToolTraceEvent
	a := New(&mockLLM{}, &mockMarket{}, "", WithToolTrace(func(e ToolTraceEvent) {
		events = append(events, e)
	}))
	var lk ProgressKind
	a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "time_now", Arguments: "{}"},
	}, &lk)

	if len(events) != 1 {
		t.Fatalf("expected 1 trace event, got %d", len(events))
	}
	ev := events[0]
	if ev.Name != "time_now" {
		t.Errorf("expected Name=time_now, got %q", ev.Name)
	}
	if ev.Args != "{}" {
		t.Errorf("expected Args={}, got %q", ev.Args)
	}
	if ev.Result == "" {
		t.Errorf("expected non-empty Result")
	}
}

func TestDispatch_ToolTrace_NilIsNoOp(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// Must not panic when no WithToolTrace was configured.
	a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "time_now", Arguments: "{}"},
	}, &lk)
}

func TestDispatch_GetCandles_InvalidTime(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "market_get_candles",
			Arguments: `{"symbol":"AAPL","from":"not-a-date","to":"2024-01-01T00:00:00Z","timeframe":"1d"}`,
		},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("expected error prefix, got %q", result)
	}
}
