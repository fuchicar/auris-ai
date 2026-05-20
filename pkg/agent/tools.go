package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"auris/pkg/llm"
	"auris/pkg/market"
)

func buildTools() []llm.Tool {
	const timeDesc = "ISO 8601 date-time, e.g. 2024-01-15T00:00:00Z"

	assetEnum := []any{"stock", "etf", "future", "forex", "crypto"}
	tfEnum := []any{"1m", "5m", "15m", "1h", "4h", "1d"}

	obj := func(props map[string]any, req []string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": req}
	}
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	enum := func(desc string, vals []any) map[string]any {
		return map[string]any{"type": "string", "description": desc, "enum": vals}
	}
	intProp := func(desc string) map[string]any {
		return map[string]any{"type": "integer", "description": desc}
	}
	tool := func(name, desc string, params map[string]any) llm.Tool {
		return llm.Tool{Type: "function", Function: llm.ToolFunction{Name: name, Description: desc, Parameters: params}}
	}

	return []llm.Tool{
		tool("market_search_instrument",
			"Search for financial instruments by name, ticker symbol, or ISIN.",
			obj(map[string]any{
				"query": str("Search term: company name, ticker, or ISIN"),
			}, []string{"query"}),
		),
		tool("market_get_instrument",
			"Get full metadata for a specific instrument by ticker symbol.",
			obj(map[string]any{
				"symbol": str("Ticker symbol, e.g. AAPL"),
			}, []string{"symbol"}),
		),
		tool("market_list_instruments",
			"List available instruments filtered by asset type.",
			obj(map[string]any{
				"asset_type": enum("Asset class to list", assetEnum),
			}, []string{"asset_type"}),
		),
		tool("market_get_candles",
			"Get historical OHLCV candlestick data for a symbol.",
			obj(map[string]any{
				"symbol":    str("Ticker symbol"),
				"from":      str("Start of range: " + timeDesc),
				"to":        str("End of range: " + timeDesc),
				"timeframe": enum("Bar granularity", tfEnum),
			}, []string{"symbol", "from", "to", "timeframe"}),
		),
		tool("market_get_quote",
			"Get the current bid/ask/last price for a symbol.",
			obj(map[string]any{
				"symbol": str("Ticker symbol, e.g. AAPL"),
			}, []string{"symbol"}),
		),
		tool("market_get_order_book",
			"Get the current order book depth for a symbol.",
			obj(map[string]any{
				"symbol": str("Ticker symbol"),
				"depth":  intProp("Number of price levels per side (default 10)"),
			}, []string{"symbol"}),
		),
		tool("market_get_ticks",
			"Get tick-by-tick trade data for a symbol in a time range.",
			obj(map[string]any{
				"symbol": str("Ticker symbol"),
				"from":   str("Start of range: " + timeDesc),
				"to":     str("End of range: " + timeDesc),
			}, []string{"symbol", "from", "to"}),
		),
		tool("market_get_corporate_actions",
			"Get dividends, splits, and other corporate events for a symbol.",
			obj(map[string]any{
				"symbol": str("Ticker symbol"),
				"from":   str("Start of range: " + timeDesc),
				"to":     str("End of range: " + timeDesc),
			}, []string{"symbol", "from", "to"}),
		),
		tool("market_get_fundamentals",
			"Get fundamental financial ratios for a symbol (P/E, EPS, market cap).",
			obj(map[string]any{
				"symbol": str("Ticker symbol, e.g. AAPL"),
			}, []string{"symbol"}),
		),

		// Time tools — allow the model to orient itself in time.
		tool("time_now",
			"Get the current date and time in UTC. Returns ISO 8601 datetime, date, time, day of week, year, month, and yesterday's date.",
			obj(map[string]any{}, []string{}),
		),
		tool("time_today",
			"Get today's date in YYYY-MM-DD format (UTC).",
			obj(map[string]any{}, []string{}),
		),
		tool("time_yesterday",
			"Get yesterday's date in YYYY-MM-DD format (UTC).",
			obj(map[string]any{}, []string{}),
		),
	}
}

// dispatch executes a single tool call and returns the result as a JSON string,
// or an error description the model can reason about.
func (a *Agent) dispatch(ctx context.Context, call llm.ToolCall) string {
	// Time tools — no market provider needed.
	switch call.Function.Name {
	case "time_now":
		now := time.Now().UTC()
		b, _ := json.Marshal(map[string]any{
			"iso8601":     now.Format(time.RFC3339),
			"date":        now.Format("2006-01-02"),
			"time":        now.Format("15:04:05"),
			"day_of_week": now.Weekday().String(),
			"year":        now.Year(),
			"month":       now.Month().String(),
			"yesterday":   now.AddDate(0, 0, -1).Format("2006-01-02"),
		})
		return string(b)
	case "time_today":
		return `"` + time.Now().UTC().Format("2006-01-02") + `"`
	case "time_yesterday":
		return `"` + time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02") + `"`
	}

	if a.market == nil {
		return `{"error":"no market data provider configured"}`
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return fmt.Sprintf("error: invalid arguments: %s", err)
	}

	str := func(key string) string {
		v, _ := args[key].(string)
		return v
	}
	parseTime := func(key string) (time.Time, error) {
		t, err := time.Parse(time.RFC3339, str(key))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid %s %q: %w", key, str(key), err)
		}
		return t, nil
	}
	intVal := func(key string, def int) int {
		if v, ok := args[key].(float64); ok {
			return int(v)
		}
		return def
	}
	encode := func(v any, err error) string {
		if err != nil {
			return fmt.Sprintf("error: %s", err)
		}
		b, _ := json.Marshal(v)
		return string(b)
	}

	switch call.Function.Name {
	case "market_search_instrument":
		res, err := a.market.SearchInstrument(ctx, str("query"))
		return encode(res, err)

	case "market_get_instrument":
		res, err := a.market.GetInstrument(ctx, str("symbol"))
		return encode(res, err)

	case "market_list_instruments":
		res, err := a.market.ListInstruments(ctx, market.AssetType(str("asset_type")))
		return encode(res, err)

	case "market_get_candles":
		from, err := parseTime("from")
		if err != nil {
			return fmt.Sprintf("error: %s", err)
		}
		to, err := parseTime("to")
		if err != nil {
			return fmt.Sprintf("error: %s", err)
		}
		res, err := a.market.GetCandles(ctx, str("symbol"), from, to, market.Timeframe(str("timeframe")))
		return encode(res, err)

	case "market_get_quote":
		res, err := a.market.GetQuote(ctx, str("symbol"))
		return encode(res, err)

	case "market_get_order_book":
		res, err := a.market.GetOrderBook(ctx, str("symbol"), intVal("depth", 10))
		return encode(res, err)

	case "market_get_ticks":
		from, err := parseTime("from")
		if err != nil {
			return fmt.Sprintf("error: %s", err)
		}
		to, err := parseTime("to")
		if err != nil {
			return fmt.Sprintf("error: %s", err)
		}
		res, err := a.market.GetTicks(ctx, str("symbol"), from, to)
		return encode(res, err)

	case "market_get_corporate_actions":
		from, err := parseTime("from")
		if err != nil {
			return fmt.Sprintf("error: %s", err)
		}
		to, err := parseTime("to")
		if err != nil {
			return fmt.Sprintf("error: %s", err)
		}
		res, err := a.market.GetCorporateActions(ctx, str("symbol"), from, to)
		return encode(res, err)

	case "market_get_fundamentals":
		res, err := a.market.GetFundamentals(ctx, str("symbol"))
		return encode(res, err)

	default:
		return fmt.Sprintf("error: unknown tool %q", call.Function.Name)
	}
}
