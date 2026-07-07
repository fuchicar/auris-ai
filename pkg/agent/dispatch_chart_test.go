package agent

import (
	"context"
	"encoding/json"
	"testing"

	"auris/pkg/llm"
	"auris/pkg/market"
)

func TestDispatch_RenderPriceChart_OK_SendsChartEvent(t *testing.T) {
	candles := []market.Candle{
		{Close: 100}, {Close: 101}, {Close: 99},
	}
	a := New(&mockLLM{}, &mockMarket{candlesBySymbol: map[string][]market.Candle{"AAPL": candles}}, "")

	ch := make(chan ChartEvent, 1)
	a.SetChartCh(ch)

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "market_render_price_chart",
			Arguments: toolCallArgs(t, map[string]any{"symbol": "AAPL"}),
		},
	}, &lk)

	var got []market.Candle
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("result is not valid JSON candle array: %s — %v", result, err)
	}
	if len(got) != len(candles) {
		t.Fatalf("candles: want %d, got %d", len(candles), len(got))
	}

	select {
	case ev := <-ch:
		if ev.Symbol != "AAPL" {
			t.Errorf("ChartEvent.Symbol: want AAPL, got %q", ev.Symbol)
		}
		if len(ev.Candles) != len(candles) {
			t.Errorf("ChartEvent.Candles: want %d, got %d", len(candles), len(ev.Candles))
		}
	default:
		t.Fatal("expected a ChartEvent on the chart channel, got none")
	}
}

func TestDispatch_RenderPriceChart_MarketError_NoChartEvent(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "") // no candlesBySymbol entry -> GetCandles returns ErrNotSupported

	ch := make(chan ChartEvent, 1)
	a.SetChartCh(ch)

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "market_render_price_chart",
			Arguments: toolCallArgs(t, map[string]any{"symbol": "UNKNOWN"}),
		},
	}, &lk)

	if len(result) < 6 || result[:6] != "error:" {
		t.Errorf("expected an error string, got %q", result)
	}

	select {
	case ev := <-ch:
		t.Fatalf("expected no ChartEvent on market error, got %+v", ev)
	default:
	}
}

func TestDispatch_RenderPriceChart_NilChartCh_NoPanic(t *testing.T) {
	candles := []market.Candle{{Close: 100}}
	a := New(&mockLLM{}, &mockMarket{candlesBySymbol: map[string][]market.Candle{"AAPL": candles}}, "")

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      "market_render_price_chart",
			Arguments: toolCallArgs(t, map[string]any{"symbol": "AAPL"}),
		},
	}, &lk)

	var got []market.Candle
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("result is not valid JSON candle array: %s — %v", result, err)
	}
}

// TestChat_RenderPriceChart_FullLoop_SendsChartEvent exercises the tool
// end-to-end through Agent.Chat -> runLoop -> dispatch, not just a direct
// dispatch() call, mirroring TestChat_SingleToolCall's pattern. This is what
// actually proves the ReAct loop and the chart channel wiring work together.
func TestChat_RenderPriceChart_FullLoop_SendsChartEvent(t *testing.T) {
	candles := []market.Candle{{Close: 100}, {Close: 105}}
	mp := &mockMarket{candlesBySymbol: map[string][]market.Candle{"AAPL": candles}}

	mlm := &mockLLM{responses: []llm.CompletionResponse{
		{
			Message: llm.Message{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{{
					ID: "call_1",
					Function: llm.ToolCallFunction{
						Name:      "market_render_price_chart",
						Arguments: toolCallArgs(t, map[string]any{"symbol": "AAPL"}),
					},
				}},
			},
			StopReason: "tool_calls",
		},
		{
			Message:    llm.Message{Role: llm.RoleAssistant, Content: "Here's the chart."},
			StopReason: "stop",
		},
	}}

	a := New(mlm, mp, "")
	ch := make(chan ChartEvent, 1)
	a.SetChartCh(ch)

	msg, err := a.Chat(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "Show me AAPL's chart"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "Here's the chart." {
		t.Errorf("unexpected content: %q", msg.Content)
	}

	select {
	case ev := <-ch:
		if ev.Symbol != "AAPL" || len(ev.Candles) != len(candles) {
			t.Errorf("unexpected ChartEvent: %+v", ev)
		}
	default:
		t.Fatal("expected a ChartEvent to have been sent during the loop")
	}
}
