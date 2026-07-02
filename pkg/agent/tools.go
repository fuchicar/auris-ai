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
	"auris/pkg/portfolio"
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
				"asset_returns":     arrNum("Daily returns of the asset in decimal form"),
				"benchmark_returns": arrNum("Daily returns of the benchmark in decimal form (same length as asset_returns)"),
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
				"free_cash_flows":      arrNum("Projected annual free cash flows (year 1 to year N)"),
				"discount_rate":        numProp("Weighted average cost of capital in decimal (e.g. 0.10 = 10%)"),
				"terminal_growth_rate": numProp("Perpetual growth rate in decimal; must be less than discount_rate"),
				"shares_outstanding":   numProp("Number of shares outstanding (use 0 to skip per-share calculation)"),
			}, []string{"free_cash_flows", "discount_rate", "terminal_growth_rate", "shares_outstanding"}),
		),
		tool("calculate_multiples",
			"Calculate market valuation multiples (P/E, P/BV, EV/EBITDA, EV/Revenue, P/S). Any multiple whose denominator is zero is omitted from the result.",
			obj(map[string]any{
				"price":                numProp("Current share price"),
				"eps":                  numProp("Earnings per share (0 to omit P/E)"),
				"book_value_per_share": numProp("Book value per share (0 to omit P/BV)"),
				"ebitda":               numProp("EBITDA (0 to omit EV/EBITDA)"),
				"enterprise_value":     numProp("Enterprise value"),
				"revenue":              numProp("Revenue; use per-share revenue for P/S, total revenue for EV/Revenue"),
			}, []string{"price", "eps", "book_value_per_share", "ebitda", "enterprise_value", "revenue"}),
		),
		tool("calculate_pfcf",
			"Calculate the Price / Free Cash Flow (P/FCF) ratio, a valuation multiple for growth/quality stocks.",
			obj(map[string]any{
				"price":                    numProp("Current share price"),
				"free_cash_flow_per_share": numProp("Free cash flow per share (must be > 0)"),
				"currency":                 str("Currency code, e.g. USD (optional, informational only)"),
			}, []string{"price", "free_cash_flow_per_share"}),
		),
		tool("calculate_peg",
			"Calculate the PEG ratio (P/E ÷ expected annual growth rate). Interpretation: <1 undervalued, 1-2 reasonable, >2 overvalued — treat as a heuristic, not a definitive signal.",
			obj(map[string]any{
				"pe_ratio":            numProp("Price/earnings ratio (must be > 0)"),
				"growth_rate_percent": numProp("Expected annual earnings growth rate in percent, e.g. 15 for 15%"),
			}, []string{"pe_ratio", "growth_rate_percent"}),
		),
		tool("calculate_dividend_yield",
			"Calculate the annual dividend yield on price. Provide either annual_dividend_per_share directly, or quarterly_dividends (last 4 quarterly payments) to derive a trailing-twelve-month yield.",
			obj(map[string]any{
				"price":                     numProp("Current share price (must be > 0)"),
				"annual_dividend_per_share": numProp("Annual dividend per share (optional if quarterly_dividends is provided)"),
				"quarterly_dividends":       arrNum("Last 4 quarterly dividends per share, used to derive TTM yield (optional)"),
			}, []string{"price"}),
		),
		tool("calculate_dividend_growth",
			"Calculate the compound annual growth rate (CAGR) of a chronological dividend-per-share series.",
			obj(map[string]any{
				"dividends": arrNum("Chronological dividends per share (minimum 2 values, first must be > 0)"),
			}, []string{"dividends"}),
		),
		tool("calculate_stress_test",
			"Estimate the resulting value of an asset price or a portfolio's current_value under a list of percentage shocks (e.g. [-10, -20, -30, -40]). For a portfolio, call portfolio_calculate_metrics first and pass its current_value here.",
			obj(map[string]any{
				"current_value":  numProp("Base value to stress: an asset price or a portfolio's current_value (must be > 0)"),
				"shocks_percent": arrNum("List of percentage shocks to apply, e.g. [-10, -20, -30, -40]. Negative = decline, positive = rally."),
				"label":          str("Optional label for context in the summary, e.g. \"AAPL\" or \"portfolio\""),
			}, []string{"current_value", "shocks_percent"}),
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

		// --- Technical indicators ------------------------------------------
		// All five receive a prices[] array directly. The LLM is expected to
		// first call market_get_candles, then pass the close prices in. This
		// keeps the math pure (no provider dependency) and easy to test.
		tool("calculate_sma",
			"Calculate the simple moving average (SMA) of a price series. First period-1 entries of the returned series are NaN to mark the warm-up.",
			obj(map[string]any{
				"prices": arrNum("Chronological closing prices (length >= period)"),
				"period": intProp("Window length, e.g. 20"),
			}, []string{"prices", "period"}),
		),
		tool("calculate_ema",
			"Calculate the exponential moving average (EMA) of a price series. If alpha is omitted, uses Wilder smoothing: alpha = 2 / (period + 1).",
			obj(map[string]any{
				"prices": arrNum("Chronological closing prices"),
				"period": intProp("Lookback window, e.g. 20"),
				"alpha":  numProp("Smoothing factor in (0, 1). Omit for Wilder default."),
			}, []string{"prices", "period"}),
		),
		tool("calculate_rsi",
			"Calculate the Relative Strength Index (Wilder) of a price series. Returns value, interpretation (\"oversold\" <30, \"overbought\" >70, otherwise \"neutral\"), and the full series.",
			obj(map[string]any{
				"prices": arrNum("Chronological closing prices (length >= period+2)"),
				"period": intProp("Lookback window (default 14)"),
			}, []string{"prices"}),
		),
		tool("calculate_macd",
			"Calculate the MACD indicator (fast EMA minus slow EMA, plus signal EMA of the MACD line). Detects bullish/bearish histogram crosses on the latest bar.",
			obj(map[string]any{
				"prices":        arrNum("Chronological closing prices (length >= slow_period + signal_period)"),
				"fast_period":   intProp("Fast EMA window (default 12)"),
				"slow_period":   intProp("Slow EMA window (default 26)"),
				"signal_period": intProp("Signal EMA window (default 9)"),
			}, []string{"prices"}),
		),
		tool("calculate_bollinger_bands",
			"Calculate Bollinger Bands (moving average ± k·σ) for a price series. Returns upper/middle/lower/bandwidth/%b series.",
			obj(map[string]any{
				"prices":  arrNum("Chronological closing prices (length >= period)"),
				"period":  intProp("SMA window (default 20)"),
				"num_std": numProp("Standard deviation multiplier (default 2)"),
			}, []string{"prices"}),
		),
		tool("calculate_correlation_matrix",
			"Calculate the NxN Pearson correlation matrix between named return series. Each series must have the same length. Useful for diversification analysis.",
			obj(map[string]any{
				"series": map[string]any{
					"type":        "object",
					"description": "Map of series name to numeric array, e.g. {\"AAPL\": [0.01, -0.02, ...], \"MSFT\": [0.005, 0.011, ...]}",
					"additionalProperties": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "number"},
					},
				},
			}, []string{"series"}),
		),
		tool("portfolio_calculate_metrics",
			"Aggregate metrics for a portfolio: current value, realised vs unrealised P&L, concentration (HHI + per-symbol weights), weighted dividend yield, weighted beta. If portfolio_id is omitted, uses the current portfolio. Symbols present in the optional quotes snapshot use those values directly; all other holdings fall back to the configured market provider. Symbols with no quote from either source are skipped and reported in missing_quotes.",
			obj(map[string]any{
				"portfolio_id": str("Portfolio ID (optional; omit to use the current portfolio)"),
				"quotes": map[string]any{
					"type":        "object",
					"description": "Optional price snapshot to avoid market provider calls, e.g. {\"AAPL\": {\"last\": 190.5, \"dividend_yield_ttm\": 0.005, \"beta\": 1.2}}. dividend_yield_ttm and beta are optional per symbol.",
					"additionalProperties": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"last":               numProp("Last traded price"),
							"dividend_yield_ttm": numProp("Trailing-twelve-month dividend yield as a fraction, e.g. 0.025 for 2.5%"),
							"beta":               numProp("Asset beta vs benchmark"),
						},
						"required": []string{"last"},
					},
				},
			}, []string{}),
		),

		tool(news.ToolName, news.ToolDescription, news.ToolParams()),

		// Portfolio management tools — read and modify the user's portfolios on disk.
		tool("portfolio_list",
			"List all portfolios. Returns id, name, description, number of instruments, realized P&L, and last-updated timestamp for each.",
			obj(map[string]any{}, []string{}),
		),
		tool("portfolio_create",
			"Create a new portfolio and save it to disk. Returns the new portfolio's id and name.",
			obj(map[string]any{
				"name":        str("Portfolio name"),
				"description": str("Optional portfolio description"),
			}, []string{"name"}),
		),
		tool("portfolio_get",
			"Get full details of a portfolio including all instruments and lots. If portfolio_id is omitted, uses the current portfolio.",
			obj(map[string]any{
				"portfolio_id": str("Portfolio ID (optional; omit to use the current portfolio)"),
			}, []string{}),
		),
		tool("portfolio_add_instrument",
			"Add an instrument (holding or watchlist item) to a portfolio. For holdings, optionally provide quantity, price, and date to record the first lot in one step.",
			obj(map[string]any{
				"portfolio_id":    str("Portfolio ID (optional; omit to use the current portfolio)"),
				"symbol":          str("Ticker symbol, e.g. AAPL"),
				"name":            str("Instrument full name, e.g. Apple Inc."),
				"instrument_type": enum("Whether the user owns the instrument or is only tracking it", []any{"holding", "watchlist"}),
				"quantity":        numProp("Units purchased (optional; only for holdings, creates the first lot)"),
				"price":           numProp("Purchase price per unit (optional; only for holdings, creates the first lot)"),
				"date":            str("Purchase date ISO 8601 or YYYY-MM-DD (optional; defaults to today)"),
			}, []string{"symbol", "name", "instrument_type"}),
		),
		tool("portfolio_add_lot",
			"Add a purchase lot to an existing holding in a portfolio. Fails if the instrument is not a holding.",
			obj(map[string]any{
				"portfolio_id": str("Portfolio ID (optional; omit to use the current portfolio)"),
				"symbol":       str("Ticker symbol of the existing holding"),
				"quantity":     numProp("Number of units purchased"),
				"price":        numProp("Purchase price per unit"),
				"date":         str("Purchase date ISO 8601 or YYYY-MM-DD (optional; defaults to today)"),
			}, []string{"symbol", "quantity", "price"}),
		),
		tool("portfolio_sell",
			"Register a FIFO sale from a holding. Updates remaining lots and accumulates realized P&L on the portfolio.",
			obj(map[string]any{
				"portfolio_id": str("Portfolio ID (optional; omit to use the current portfolio)"),
				"symbol":       str("Ticker symbol of the holding to sell"),
				"quantity":     numProp("Number of units to sell"),
				"sell_price":   numProp("Sale price per unit"),
			}, []string{"symbol", "quantity", "sell_price"}),
		),
		tool("portfolio_remove_instrument",
			"Permanently remove an instrument and all its lots from a portfolio.",
			obj(map[string]any{
				"portfolio_id": str("Portfolio ID (optional; omit to use the current portfolio)"),
				"symbol":       str("Ticker symbol of the instrument to remove"),
			}, []string{"symbol"}),
		),
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
	// Emit a progress event for market, calculation, and portfolio tools (deduplicated).
	var kind ProgressKind
	switch {
	case strings.HasPrefix(call.Function.Name, "market_"):
		kind = ProgressFinancial
	case strings.HasPrefix(call.Function.Name, "calculate_"), call.Function.Name == "convert_currency":
		kind = ProgressCalculation
	case strings.HasPrefix(call.Function.Name, "portfolio_"):
		kind = ProgressPortfolio
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
	case "calculate_pfcf":
		return encode(calcPFCF(numVal("price"), numVal("free_cash_flow_per_share"), str("currency")))
	case "calculate_peg":
		return encode(calcPEG(numVal("pe_ratio"), numVal("growth_rate_percent")))
	case "calculate_dividend_yield":
		quarterly, _ := arrNumVal("quarterly_dividends")
		return encode(calcDividendYield(numVal("price"), numVal("annual_dividend_per_share"), quarterly))
	case "calculate_dividend_growth":
		dividends, ok := arrNumVal("dividends")
		if !ok {
			return `error: dividends must be an array of numbers`
		}
		return encode(calcDividendGrowth(dividends))
	case "calculate_stress_test":
		shocks, ok := arrNumVal("shocks_percent")
		if !ok {
			return `error: shocks_percent must be an array of numbers`
		}
		return encode(calcStressTest(numVal("current_value"), shocks, str("label")))
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

	case "calculate_sma":
		prices, ok := arrNumVal("prices")
		if !ok {
			return `error: prices must be an array of numbers`
		}
		return encode(calcSMA(prices, intVal("period", 20)))

	case "calculate_ema":
		prices, ok := arrNumVal("prices")
		if !ok {
			return `error: prices must be an array of numbers`
		}
		// alpha omitted -> 0, which calcEMA interprets as "use Wilder default".
		return encode(calcEMA(prices, intVal("period", 20), numVal("alpha")))

	case "calculate_rsi":
		prices, ok := arrNumVal("prices")
		if !ok {
			return `error: prices must be an array of numbers`
		}
		return encode(calcRSI(prices, intVal("period", 14)))

	case "calculate_macd":
		prices, ok := arrNumVal("prices")
		if !ok {
			return `error: prices must be an array of numbers`
		}
		return encode(calcMACD(prices,
			intVal("fast_period", 12),
			intVal("slow_period", 26),
			intVal("signal_period", 9)))

	case "calculate_bollinger_bands":
		prices, ok := arrNumVal("prices")
		if !ok {
			return `error: prices must be an array of numbers`
		}
		numStd := numVal("num_std")
		if numStd <= 0 {
			numStd = 2.0 // standard default, consistent with EMA's alpha default
		}
		return encode(calcBollingerBands(prices, intVal("period", 20), numStd))

	case "calculate_correlation_matrix":
		raw, ok := args["series"].(map[string]any)
		if !ok {
			return `error: series must be an object mapping name → number[]`
		}
		series := make(map[string][]float64, len(raw))
		for name, v := range raw {
			arr, ok := v.([]any)
			if !ok {
				return fmt.Sprintf("error: series[%q] must be an array of numbers", name)
			}
			vals := make([]float64, len(arr))
			for i, x := range arr {
				f, ok := x.(float64)
				if !ok {
					return fmt.Sprintf("error: series[%q][%d] must be a number", name, i)
				}
				vals[i] = f
			}
			series[name] = vals
		}
		return encode(calcCorrelationMatrix(series))
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

	// Portfolio tools — operate on persisted portfolio files. portfolio_calculate_metrics
	// additionally needs the market provider for live quotes; it falls back gracefully
	// (and reports missing_quotes) when the provider is unavailable.
	if strings.HasPrefix(call.Function.Name, "portfolio_") {
		// resolvePortfolioID uses the explicit argument or falls back to the current portfolio.
		resolvePortfolioID := func() (string, error) {
			if id := str("portfolio_id"); id != "" {
				return id, nil
			}
			if a.currentPortfolioID != "" {
				return a.currentPortfolioID, nil
			}
			return "", fmt.Errorf("no portfolio_id provided and no current portfolio is set")
		}
		// parseLotDate parses ISO 8601 or YYYY-MM-DD; defaults to now.
		parseLotDate := func(key string) time.Time {
			s := str(key)
			if s == "" {
				return time.Now()
			}
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return t
			}
			if t, err := time.Parse("2006-01-02", s); err == nil {
				return t
			}
			return time.Now()
		}

		switch call.Function.Name {
		case "portfolio_list":
			portfolios, err := portfolio.ListPortfolios()
			if err != nil {
				return encode(nil, err)
			}
			type summary struct {
				ID          string  `json:"id"`
				Name        string  `json:"name"`
				Description string  `json:"description,omitempty"`
				Instruments int     `json:"instruments_count"`
				RealizedPnL float64 `json:"realized_pnl"`
				UpdatedAt   string  `json:"updated_at"`
			}
			out := make([]summary, len(portfolios))
			for i, p := range portfolios {
				out[i] = summary{
					ID:          p.ID,
					Name:        p.Name,
					Description: p.Description,
					Instruments: len(p.Instruments),
					RealizedPnL: p.RealizedPnL,
					UpdatedAt:   p.UpdatedAt.Format(time.RFC3339),
				}
			}
			return encode(out, nil)

		case "portfolio_create":
			name := str("name")
			if name == "" {
				return `error: name is required`
			}
			p := portfolio.NewPortfolio(name)
			p.Description = str("description")
			if err := portfolio.SavePortfolio(p); err != nil {
				return encode(nil, err)
			}
			return encode(map[string]any{"id": p.ID, "name": p.Name, "description": p.Description}, nil)

		case "portfolio_get":
			id, err := resolvePortfolioID()
			if err != nil {
				return encode(nil, err)
			}
			p, err := portfolio.LoadPortfolio(id)
			if err != nil {
				return encode(nil, err)
			}
			if p == nil {
				return `error: portfolio not found`
			}
			return encode(p, nil)

		case "portfolio_add_instrument":
			id, err := resolvePortfolioID()
			if err != nil {
				return encode(nil, err)
			}
			p, err := portfolio.LoadPortfolio(id)
			if err != nil {
				return encode(nil, err)
			}
			if p == nil {
				return `error: portfolio not found`
			}
			symbol := str("symbol")
			if symbol == "" {
				return `error: symbol is required`
			}
			for _, ins := range p.Instruments {
				if ins.Symbol == symbol {
					return `error: instrument already exists in portfolio`
				}
			}
			instType := portfolio.InstrumentType(str("instrument_type"))
			ins := portfolio.Instrument{
				ID:     portfolio.NewInstrumentID(),
				Symbol: symbol,
				Name:   str("name"),
				Type:   instType,
			}
			if instType == portfolio.InstrumentHolding {
				qty := numVal("quantity")
				price := numVal("price")
				if qty > 0 && price > 0 {
					ins.Lots = []portfolio.Lot{portfolio.NewLot(qty, price, parseLotDate("date"))}
				}
			}
			p.Instruments = append(p.Instruments, ins)
			if err := portfolio.SavePortfolio(p); err != nil {
				return encode(nil, err)
			}
			return encode(map[string]any{
				"instrument_id":  ins.ID,
				"symbol":         ins.Symbol,
				"name":           ins.Name,
				"type":           ins.Type,
				"total_quantity": ins.TotalQuantity(),
				"portfolio_id":   p.ID,
			}, nil)

		case "portfolio_add_lot":
			id, err := resolvePortfolioID()
			if err != nil {
				return encode(nil, err)
			}
			p, err := portfolio.LoadPortfolio(id)
			if err != nil {
				return encode(nil, err)
			}
			if p == nil {
				return `error: portfolio not found`
			}
			symbol := str("symbol")
			qty := numVal("quantity")
			price := numVal("price")
			if qty <= 0 {
				return `error: quantity must be positive`
			}
			if price <= 0 {
				return `error: price must be positive`
			}
			for i, ins := range p.Instruments {
				if ins.Symbol != symbol {
					continue
				}
				if ins.Type != portfolio.InstrumentHolding {
					return `error: instrument is not a holding; change its type to holding first`
				}
				p.Instruments[i].Lots = append(p.Instruments[i].Lots, portfolio.NewLot(qty, price, parseLotDate("date")))
				if err := portfolio.SavePortfolio(p); err != nil {
					return encode(nil, err)
				}
				return encode(map[string]any{
					"symbol":         ins.Symbol,
					"total_quantity": p.Instruments[i].TotalQuantity(),
					"lots_count":     len(p.Instruments[i].Lots),
				}, nil)
			}
			return `error: instrument not found in portfolio`

		case "portfolio_sell":
			id, err := resolvePortfolioID()
			if err != nil {
				return encode(nil, err)
			}
			p, err := portfolio.LoadPortfolio(id)
			if err != nil {
				return encode(nil, err)
			}
			if p == nil {
				return `error: portfolio not found`
			}
			symbol := str("symbol")
			qty := numVal("quantity")
			sellPrice := numVal("sell_price")
			if qty <= 0 {
				return `error: quantity must be positive`
			}
			if sellPrice <= 0 {
				return `error: sell_price must be positive`
			}
			for i, ins := range p.Instruments {
				if ins.Symbol != symbol {
					continue
				}
				if ins.Type != portfolio.InstrumentHolding {
					return `error: instrument is not a holding`
				}
				result, err := portfolio.ApplyFIFOSell(ins.Lots, qty, sellPrice)
				if err != nil {
					return encode(nil, err)
				}
				p.Instruments[i].Lots = result.RemainingLots
				p.RealizedPnL += result.RealizedPnL
				if err := portfolio.SavePortfolio(p); err != nil {
					return encode(nil, err)
				}
				return encode(map[string]any{
					"symbol":                       ins.Symbol,
					"realized_pnl":                 result.RealizedPnL,
					"remaining_quantity":           p.Instruments[i].TotalQuantity(),
					"portfolio_total_realized_pnl": p.RealizedPnL,
				}, nil)
			}
			return `error: instrument not found in portfolio`

		case "portfolio_remove_instrument":
			id, err := resolvePortfolioID()
			if err != nil {
				return encode(nil, err)
			}
			p, err := portfolio.LoadPortfolio(id)
			if err != nil {
				return encode(nil, err)
			}
			if p == nil {
				return `error: portfolio not found`
			}
			symbol := str("symbol")
			filtered := p.Instruments[:0]
			found := false
			for _, ins := range p.Instruments {
				if ins.Symbol == symbol {
					found = true
					continue
				}
				filtered = append(filtered, ins)
			}
			if !found {
				return `error: instrument not found in portfolio`
			}
			p.Instruments = filtered
			if err := portfolio.SavePortfolio(p); err != nil {
				return encode(nil, err)
			}
			return encode(map[string]any{"removed": symbol, "portfolio_id": p.ID}, nil)

		case "portfolio_calculate_metrics":
			id, err := resolvePortfolioID()
			if err != nil {
				return encode(nil, err)
			}
			p, err := portfolio.LoadPortfolio(id)
			if err != nil {
				return encode(nil, err)
			}
			if p == nil {
				return `error: portfolio not found`
			}
			// Preload any snapshot quotes the caller already knows, so the
			// auto-fetch loop below skips those symbols (it already checks
			// "seen" per symbol).
			quotes := make(map[string]portfolio.Quote, len(p.Instruments))
			if raw, ok := args["quotes"].(map[string]any); ok {
				for symbol, v := range raw {
					entry, ok := v.(map[string]any)
					if !ok {
						continue
					}
					last, ok := entry["last"].(float64)
					if !ok || last <= 0 {
						continue
					}
					pq := portfolio.Quote{Last: last}
					if dy, ok := entry["dividend_yield_ttm"].(float64); ok {
						pq.DividendYieldTTM = dy
					}
					if beta, ok := entry["beta"].(float64); ok {
						pq.Beta = beta
					}
					quotes[symbol] = pq
				}
			}
			// Auto-fetch a quote per holding not already covered by the
			// snapshot above. Symbols whose quote fails or returns Last=0 end
			// up in MissingQuotes and are excluded from valuation,
			// concentration, and dividend/beta aggregates.
			if a.market != nil {
				for _, ins := range p.Instruments {
					if ins.Type != portfolio.InstrumentHolding {
						continue
					}
					if _, seen := quotes[ins.Symbol]; seen {
						continue
					}
					q, qErr := a.market.GetQuote(ctx, ins.Symbol)
					if qErr != nil || q.Last <= 0 {
						continue
					}
					pq := portfolio.Quote{Last: q.Last}
					// Enrich with dividend yield TTM and beta from fundamentals.
					// Failure is non-fatal: missing fields stay zero and are
					// excluded from the weighted averages by ComputeMetrics.
					if f, fErr := a.market.GetFundamentals(ctx, ins.Symbol); fErr == nil {
						pq.DividendYieldTTM = f.DividendYieldTTM
						pq.Beta = f.Beta
					}
					quotes[ins.Symbol] = pq
				}
			}
			m, err := portfolio.ComputeMetrics(p, quotes)
			if err != nil {
				return encode(nil, err)
			}
			m.ComputedAt = time.Now().UTC().Format(time.RFC3339)
			if a.market == nil {
				// Surface the cause of the missing valuation so the LLM can warn the user.
				m.Summary = m.Summary + " [WARNING: no market provider configured — only realised P&L and cost basis are accurate]"
			}
			return encode(m, nil)
		}
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
