package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"auris/pkg/llm"
	"auris/pkg/market"
)

// ---- mock LLM ---------------------------------------------------------------

type mockLLM struct {
	responses []llm.CompletionResponse
	calls     int
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
	return nil, llm.ErrNotSupported
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
}

func (m *mockMarket) Name() string        { return "mock" }
func (m *mockMarket) DocsURL() string     { return "" }
func (m *mockMarket) Description() string { return "" }
func (m *mockMarket) Connect(ctx context.Context) error    { return nil }
func (m *mockMarket) Disconnect(ctx context.Context) error { return nil }
func (m *mockMarket) RefreshToken(ctx context.Context) error { return nil }
func (m *mockMarket) Ping(ctx context.Context) error       { return nil }
func (m *mockMarket) IsConnected() bool                    { return true }

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
	return nil, market.ErrNotSupported
}
func (m *mockMarket) GetTicks(ctx context.Context, symbol string, from, to time.Time) ([]market.Tick, error) {
	return nil, market.ErrNotSupported
}
func (m *mockMarket) GetCorporateActions(ctx context.Context, symbol string, from, to time.Time) ([]market.CorporateAction, error) {
	return nil, market.ErrNotSupported
}
func (m *mockMarket) GetQuote(ctx context.Context, symbol string) (market.Quote, error) {
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
	return m.fundamental, m.fundErr
}

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
	if len(tools) != 25 {
		t.Errorf("expected 25 tools, got %d", len(tools))
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

	result := a.dispatch(context.Background(), llm.ToolCall{
		ID: "c1",
		Function: llm.ToolCallFunction{
			Name:      "market_get_quote",
			Arguments: `{"symbol":"AAPL"}`,
		},
	})

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

	result := a.dispatch(context.Background(), llm.ToolCall{
		ID: "c1",
		Function: llm.ToolCallFunction{
			Name:      "market_get_quote",
			Arguments: `{"symbol":"UNKNOWN"}`,
		},
	})

	if result == "" || result[:6] != "error:" {
		t.Errorf("expected error prefix, got %q", result)
	}
}

func TestDispatch_InvalidArgs(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "market_get_quote",
			Arguments: `not json`,
		},
	})
	if result[:6] != "error:" {
		t.Errorf("expected error prefix, got %q", result)
	}
}

func TestDispatch_UnknownTool(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "market_nonexistent",
			Arguments: `{}`,
		},
	})
	if result[:6] != "error:" {
		t.Errorf("expected error prefix, got %q", result)
	}
}

func TestDispatch_GetCandles_InvalidTime(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "market_get_candles",
			Arguments: `{"symbol":"AAPL","from":"not-a-date","to":"2024-01-01T00:00:00Z","timeframe":"1d"}`,
		},
	})
	if result[:6] != "error:" {
		t.Errorf("expected error prefix, got %q", result)
	}
}
