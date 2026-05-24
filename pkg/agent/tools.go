package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"auris/pkg/llm"
	"auris/pkg/market"
	"auris/pkg/news"
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
	numProp := func(desc string) map[string]any {
		return map[string]any{"type": "number", "description": desc}
	}
	numEnum := func(desc string, vals []any) map[string]any {
		return map[string]any{"type": "number", "description": desc, "enum": vals}
	}
	arrNum := func(desc string) map[string]any {
		return map[string]any{"type": "array", "description": desc, "items": map[string]any{"type": "number"}}
	}
	intEnum := func(desc string, vals []any) map[string]any {
		return map[string]any{"type": "integer", "description": desc, "enum": vals}
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

		// Math / finance tools — local calculations, no market provider required.
		tool("calculate_roi",
			"Calculate return on investment (ROI) for a position.",
			obj(map[string]any{
				"cost_basis":    numProp("Initial cost or investment amount"),
				"current_value": numProp("Current value of the investment"),
			}, []string{"cost_basis", "current_value"}),
		),
		tool("calculate_cagr",
			"Calculate compound annual growth rate (CAGR) between two values over a number of years.",
			obj(map[string]any{
				"initial_value": numProp("Starting value (must be > 0)"),
				"final_value":   numProp("Ending value"),
				"years":         numProp("Number of years elapsed (must be > 0)"),
			}, []string{"initial_value", "final_value", "years"}),
		),
		tool("calculate_volatility",
			"Calculate annualised volatility (std dev of log returns × √252) from a chronological series of closing prices.",
			obj(map[string]any{
				"prices": arrNum("Chronological closing prices (minimum 2 values)"),
			}, []string{"prices"}),
		),
		tool("calculate_sharpe",
			"Calculate the annualised Sharpe ratio from daily returns and an annual risk-free rate.",
			obj(map[string]any{
				"returns":               arrNum("Daily returns in decimal form (e.g. 0.01 = 1%)"),
				"risk_free_rate_annual": numProp("Annual risk-free rate in decimal form (e.g. 0.04 = 4%)"),
			}, []string{"returns", "risk_free_rate_annual"}),
		),
		tool("calculate_max_drawdown",
			"Calculate the maximum peak-to-trough drawdown from a chronological series of prices.",
			obj(map[string]any{
				"prices": arrNum("Chronological prices (minimum 2 values)"),
			}, []string{"prices"}),
		),
		tool("calculate_pnl",
			"Calculate profit and loss for a long or short trading position.",
			obj(map[string]any{
				"entry_price":   numProp("Price at which the position was opened"),
				"current_price": numProp("Current market price"),
				"quantity":      numProp("Number of units held"),
				"position_type": enum("Direction of the position", []any{"long", "short"}),
			}, []string{"entry_price", "current_price", "quantity", "position_type"}),
		),
		tool("calculate_beta",
			"Calculate asset beta and correlation relative to a benchmark using daily return series of equal length.",
			obj(map[string]any{
				"asset_returns":      arrNum("Daily returns of the asset in decimal form"),
				"benchmark_returns":  arrNum("Daily returns of the benchmark in decimal form (same length as asset_returns)"),
			}, []string{"asset_returns", "benchmark_returns"}),
		),
		tool("calculate_var",
			"Calculate Value at Risk (VaR) and Conditional VaR (Expected Shortfall) for a portfolio.",
			obj(map[string]any{
				"returns":          arrNum("Daily returns in decimal form"),
				"confidence_level": numEnum("Confidence level for VaR", []any{0.90, 0.95, 0.99}),
				"portfolio_value":  numProp("Current portfolio value in currency units"),
				"method":           enum("Calculation method", []any{"parametric", "historical"}),
			}, []string{"returns", "confidence_level", "portfolio_value", "method"}),
		),
		tool("calculate_dcf",
			"Calculate intrinsic value using discounted cash flow (DCF) with Gordon Growth Model terminal value.",
			obj(map[string]any{
				"free_cash_flows":     arrNum("Projected annual free cash flows (year 1 to year N)"),
				"discount_rate":       numProp("Weighted average cost of capital in decimal (e.g. 0.10 = 10%)"),
				"terminal_growth_rate": numProp("Perpetual growth rate in decimal; must be less than discount_rate"),
				"shares_outstanding":  numProp("Number of shares outstanding (use 0 to skip per-share calculation)"),
			}, []string{"free_cash_flows", "discount_rate", "terminal_growth_rate", "shares_outstanding"}),
		),
		tool("calculate_multiples",
			"Calculate market valuation multiples (P/E, P/BV, EV/EBITDA, EV/Revenue, P/S). Any multiple whose denominator is zero is omitted from the result.",
			obj(map[string]any{
				"price":               numProp("Current share price"),
				"eps":                 numProp("Earnings per share (0 to omit P/E)"),
				"book_value_per_share": numProp("Book value per share (0 to omit P/BV)"),
				"ebitda":              numProp("EBITDA (0 to omit EV/EBITDA)"),
				"enterprise_value":    numProp("Enterprise value"),
				"revenue":             numProp("Revenue; use per-share revenue for P/S, total revenue for EV/Revenue"),
			}, []string{"price", "eps", "book_value_per_share", "ebitda", "enterprise_value", "revenue"}),
		),
		tool("convert_currency",
			"Convert an amount between two currencies using an explicit exchange rate. Fetch the rate from market data first.",
			obj(map[string]any{
				"amount":        numProp("Amount to convert"),
				"from_currency": str("Source currency code, e.g. USD"),
				"to_currency":   str("Target currency code, e.g. EUR"),
				"exchange_rate": numProp("Units of to_currency per 1 unit of from_currency"),
			}, []string{"amount", "from_currency", "to_currency", "exchange_rate"}),
		),
		tool("calculate_compound_interest",
			"Calculate compound interest with configurable compounding frequency.",
			obj(map[string]any{
				"principal":          numProp("Initial principal amount"),
				"annual_rate":        numProp("Annual interest rate in decimal (e.g. 0.05 = 5%)"),
				"years":              numProp("Investment duration in years"),
				"compounds_per_year": intEnum("Compounding frequency (1=annual, 4=quarterly, 12=monthly, 365=daily)", []any{1, 4, 12, 365}),
			}, []string{"principal", "annual_rate", "years", "compounds_per_year"}),
		),
		tool("calculate_stats",
			"Calculate descriptive statistics (mean, median, std dev, min, max, quartiles) for a numeric series.",
			obj(map[string]any{
				"values": arrNum("Numeric series to analyse"),
				"label":  str("Name of the series used in the summary (e.g. \"AAPL daily returns\")"),
			}, []string{"values", "label"}),
		),

		tool(news.ToolName, news.ToolDescription, news.ToolParams()),
	}
}

// dispatch executes a single tool call and returns the result as a JSON string,
// or an error description the model can reason about. lastKind tracks the last
// ProgressKind emitted this turn so consecutive same-category events are skipped.
func (a *Agent) dispatch(ctx context.Context, call llm.ToolCall, lastKind *ProgressKind) string {
	a.debugf("[dispatch] tool=%s args=%s", call.Function.Name, truncate(call.Function.Arguments, 300))
	start := time.Now()
	result := a.dispatchInner(ctx, call, lastKind)
	a.debugf("[dispatch] tool=%s done duration=%dms result=%s",
		call.Function.Name, time.Since(start).Milliseconds(), truncate(result, 300))
	return result
}

// dispatchInner is the actual tool dispatch logic; dispatch wraps it with logging.
func (a *Agent) dispatchInner(ctx context.Context, call llm.ToolCall, lastKind *ProgressKind) string {
	// Emit a progress event for market and calculation tools (deduplicated).
	var kind ProgressKind
	switch {
	case strings.HasPrefix(call.Function.Name, "market_"):
		kind = ProgressFinancial
	case strings.HasPrefix(call.Function.Name, "calculate_"), call.Function.Name == "convert_currency":
		kind = ProgressCalculation
	}
	if kind != "" && kind != *lastKind {
		*lastKind = kind
		if a.progressCh != nil {
			select {
			case a.progressCh <- ProgressEvent{Kind: kind}:
			default:
			}
		}
	}

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

	// Parse arguments — needed by both math and market tools.
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return fmt.Sprintf("error: invalid arguments: %s", err)
	}

	str := func(key string) string {
		v, _ := args[key].(string)
		return v
	}
	numVal := func(key string) float64 {
		v, _ := args[key].(float64)
		return v
	}
	arrNumVal := func(key string) ([]float64, bool) {
		raw, ok := args[key].([]any)
		if !ok {
			return nil, false
		}
		out := make([]float64, len(raw))
		for i, v := range raw {
			f, ok := v.(float64)
			if !ok {
				return nil, false
			}
			out[i] = f
		}
		return out, true
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

	// Math tools — no market provider needed.
	switch call.Function.Name {
	case "calculate_roi":
		return encode(calcROI(numVal("cost_basis"), numVal("current_value")))
	case "calculate_cagr":
		return encode(calcCAGR(numVal("initial_value"), numVal("final_value"), numVal("years")))
	case "calculate_volatility":
		prices, ok := arrNumVal("prices")
		if !ok {
			return `error: prices must be an array of numbers`
		}
		return encode(calcVolatility(prices))
	case "calculate_sharpe":
		returns, ok := arrNumVal("returns")
		if !ok {
			return `error: returns must be an array of numbers`
		}
		return encode(calcSharpe(returns, numVal("risk_free_rate_annual")))
	case "calculate_max_drawdown":
		prices, ok := arrNumVal("prices")
		if !ok {
			return `error: prices must be an array of numbers`
		}
		return encode(calcMaxDrawdown(prices))
	case "calculate_pnl":
		return encode(calcPnL(numVal("entry_price"), numVal("current_price"), numVal("quantity"), str("position_type")))
	case "calculate_beta":
		ar, ok1 := arrNumVal("asset_returns")
		br, ok2 := arrNumVal("benchmark_returns")
		if !ok1 || !ok2 {
			return `error: asset_returns and benchmark_returns must be arrays of numbers`
		}
		return encode(calcBeta(ar, br))
	case "calculate_var":
		returns, ok := arrNumVal("returns")
		if !ok {
			return `error: returns must be an array of numbers`
		}
		return encode(calcVaR(returns, numVal("confidence_level"), numVal("portfolio_value"), str("method")))
	case "calculate_dcf":
		fcf, ok := arrNumVal("free_cash_flows")
		if !ok {
			return `error: free_cash_flows must be an array of numbers`
		}
		return encode(calcDCF(fcf, numVal("discount_rate"), numVal("terminal_growth_rate"), numVal("shares_outstanding")))
	case "calculate_multiples":
		r := calcMultiples(numVal("price"), numVal("eps"), numVal("book_value_per_share"),
			numVal("ebitda"), numVal("enterprise_value"), numVal("revenue"))
		return encode(r, nil)
	case "convert_currency":
		return encode(calcCurrencyConversion(numVal("amount"), str("from_currency"), str("to_currency"), numVal("exchange_rate")))
	case "calculate_compound_interest":
		return encode(calcCompoundInterest(numVal("principal"), numVal("annual_rate"), numVal("years"), intVal("compounds_per_year", 1)))
	case "calculate_stats":
		vals, ok := arrNumVal("values")
		if !ok {
			return `error: values must be an array of numbers`
		}
		return encode(calcStats(vals, str("label")))
	}

	// News tool — no market provider needed.
	if call.Function.Name == news.ToolName {
		if kind := ProgressNews; kind != *lastKind {
			*lastKind = kind
			if a.progressCh != nil {
				select {
				case a.progressCh <- ProgressEvent{Kind: kind}:
				default:
				}
			}
		}
		if a.news == nil {
			return `{"error":"no news provider configured"}`
		}
		var params news.FetchNewsParams
		if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil {
			return fmt.Sprintf("error: invalid arguments: %s", err)
		}
		items, err := a.news.HandleFetchNews(ctx, params)
		if err != nil {
			b, _ := json.Marshal(map[string]any{"status": "error", "message": err.Error()})
			return string(b)
		}
		if len(items) == 0 {
			b, _ := json.Marshal(map[string]any{
				"status":  "no_results",
				"message": "No se encontraron noticias recientes con los criterios especificados.",
			})
			return string(b)
		}
		b, _ := json.Marshal(items)
		return string(b)
	}

	if a.market == nil {
		return `{"error":"no market data provider configured"}`
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
