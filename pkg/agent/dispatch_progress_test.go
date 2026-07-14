package agent

import (
	"context"
	"testing"

	"auris/pkg/llm"
)

// ---- ProgressEvent emission (FEAT-21) ----------------------------------------

func TestDispatch_ProgressEvent_PortfolioToolSurfaces(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	ch := make(chan ProgressEvent, 4)
	a.SetProgressCh(ch)

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"symbol": "AAPL"})
	a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_remove_instrument", Arguments: args},
	}, &lk)

	select {
	case ev := <-ch:
		if ev.Name != "portfolio_remove_instrument" || ev.Args != args {
			t.Errorf("event = %+v, want Name=portfolio_remove_instrument Args=%s", ev, args)
		}
	default:
		t.Fatal("expected a ProgressEvent for a portfolio_ tool call, got none")
	}
}

func TestDispatch_ProgressEvent_DistinctCallsBothSurface(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	ch := make(chan ProgressEvent, 4)
	a.SetProgressCh(ch)

	var lk ProgressKind
	quoteArgs := toolCallArgs(t, map[string]any{"symbol": "AAPL"})
	a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "market_get_quote", Arguments: quoteArgs},
	}, &lk)

	candlesArgs := toolCallArgs(t, map[string]any{"symbol": "AAPL", "from": "2024-01-01T00:00:00Z", "to": "2024-06-01T00:00:00Z", "timeframe": "1d"})
	a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "market_get_candles", Arguments: candlesArgs},
	}, &lk)

	first := <-ch
	if first.Name != "market_get_quote" {
		t.Errorf("first event Name = %q, want market_get_quote", first.Name)
	}
	second := <-ch
	if second.Name != "market_get_candles" {
		t.Errorf("second event Name = %q, want market_get_candles", second.Name)
	}
}

func TestDispatch_ProgressEvent_ExactRepeatSuppressed(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	ch := make(chan ProgressEvent, 4)
	a.SetProgressCh(ch)

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"symbol": "AAPL"})
	for i := 0; i < 2; i++ {
		a.dispatch(context.Background(), llm.ToolCall{
			Function: llm.ToolCallFunction{Name: "market_get_quote", Arguments: args},
		}, &lk)
	}

	if len(ch) != 1 {
		t.Fatalf("channel has %d events, want exactly 1 (exact repeat should be deduplicated)", len(ch))
	}
}
