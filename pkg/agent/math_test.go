package agent

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"auris/pkg/finance"
	"auris/pkg/llm"
	"auris/pkg/market"
	"auris/pkg/portfolio"
)

// ---- helpers -----------------------------------------------------------------

func approxEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func TestDispatch_CalculateSMA_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"prices": []float64{1, 2, 3, 4, 5}, "period": 3, "label": "AAPL"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_sma", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.SmaResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.Last != 4 {
		t.Errorf("SMA last: want 4, got %v", r.Last)
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if !strings.Contains(r.InputsSummary, "AAPL") {
		t.Errorf("InputsSummary must echo the label, got %q", r.InputsSummary)
	}
}

func TestDispatch_CalculateEMA_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// Omit alpha to exercise the default-Wilder branch.
	args := toolCallArgs(t, map[string]any{"prices": []float64{10, 11, 12, 13}, "period": 3, "label": "AAPL"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_ema", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.EmaResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if !approxEqual(r.Alpha, 0.5, 1e-9) {
		t.Errorf("Wilder alpha for period=3: want 0.5, got %v", r.Alpha)
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if !strings.Contains(r.InputsSummary, "AAPL") {
		t.Errorf("InputsSummary must echo the label, got %q", r.InputsSummary)
	}
}

func TestDispatch_CalculateRSI_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	prices := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27}
	args := toolCallArgs(t, map[string]any{"prices": prices, "period": 14, "label": "AAPL"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_rsi", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.RsiResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.Value != 100 {
		t.Errorf("all-gains RSI: want 100, got %v", r.Value)
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if !strings.Contains(r.InputsSummary, "AAPL") {
		t.Errorf("InputsSummary must echo the label, got %q", r.InputsSummary)
	}
}

func TestDispatch_CalculateMACD_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	prices := make([]float64, 40)
	for i := range prices {
		prices[i] = 100 + float64(i)
	}
	args := toolCallArgs(t, map[string]any{
		"prices":        prices,
		"fast_period":   12,
		"slow_period":   26,
		"signal_period": 9,
		"label":         "AAPL",
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_macd", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.MacdResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.LastMACD <= 0 {
		t.Errorf("monotonically rising series must yield positive MACD, got %v", r.LastMACD)
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if !strings.Contains(r.InputsSummary, "AAPL") {
		t.Errorf("InputsSummary must echo the label, got %q", r.InputsSummary)
	}
}

func TestDispatch_CalculateBollinger_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"prices":  []float64{10, 11, 12, 13, 14, 15},
		"period":  3,
		"num_std": 2,
		"label":   "AAPL",
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_bollinger_bands", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.BollingerResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.LastPrice != 15 {
		t.Errorf("LastPrice: want 15, got %v", r.LastPrice)
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if !strings.Contains(r.InputsSummary, "AAPL") {
		t.Errorf("InputsSummary must echo the label, got %q", r.InputsSummary)
	}
}

func TestDispatch_CalculateBollinger_MissingNumStd(t *testing.T) {
	// Omitting num_std must apply the default of 2.0 (consistent with EMA's
	// alpha default) instead of returning an error.
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"prices": []float64{10, 11, 12, 13, 14, 15},
		"period": 3,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_bollinger_bands", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("missing num_std should default to 2.0, got error: %s", result)
	}
	var r finance.BollingerResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.NumStd != 2.0 {
		t.Errorf("default num_std: want 2.0, got %v", r.NumStd)
	}
}

func TestDispatch_CalculateCorrelationMatrix_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"series": map[string]any{
			"AAPL": []float64{0.01, -0.02, 0.03},
			"MSFT": []float64{0.02, -0.04, 0.06},
		},
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_correlation_matrix", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.CorrelationMatrixResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if len(r.Matrix) != 2 {
		t.Errorf("matrix size: want 2x2, got %d", len(r.Matrix))
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if !strings.Contains(r.InputsSummary, "AAPL") || !strings.Contains(r.InputsSummary, "MSFT") {
		t.Errorf("InputsSummary must list the series names, got %q", r.InputsSummary)
	}
}

func TestDispatch_CalculateCorrelationMatrix_BadArgType(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// Pass an array where the schema expects an object.
	args := toolCallArgs(t, map[string]any{"series": []float64{1, 2}})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_correlation_matrix", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("non-object series should produce an error, got %s", result)
	}
}

func TestDispatch_CalculatePFCF_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"price": 150.0, "free_cash_flow_per_share": 12.5})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_pfcf", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.PfcfResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if !approxEqual(r.PFCF, 12.0, 0.01) {
		t.Errorf("PFCF: want ~12.0, got %v", r.PFCF)
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if r.InputsSummary == "" {
		t.Error("InputsSummary must be populated by the dispatcher")
	}
}

func TestDispatch_CalculatePEG_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"pe_ratio": 20.0, "growth_rate_percent": 15.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_peg", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.PegResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.Interpretation != "reasonable" {
		t.Errorf("Interpretation: want reasonable, got %q", r.Interpretation)
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if r.InputsSummary == "" {
		t.Error("InputsSummary must be populated by the dispatcher")
	}
}

func TestDispatch_CalculateDividendYield_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"price": 100.0, "annual_dividend_per_share": 2.5, "label": "AAPL"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_dividend_yield", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.DividendYieldResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if !approxEqual(r.YieldPercent, 2.5, 0.001) {
		t.Errorf("YieldPercent: want 2.5, got %v", r.YieldPercent)
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if !strings.Contains(r.InputsSummary, "AAPL") {
		t.Errorf("InputsSummary must echo the label, got %q", r.InputsSummary)
	}
}

func TestDispatch_CalculateDividendYield_MissingBoth(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"price": 100.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_dividend_yield", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("missing both dividend inputs should produce an error, got %s", result)
	}
}

func TestDispatch_CalculateDividendGrowth_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"dividends": []float64{1.00, 1.10, 1.21}, "label": "AAPL"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_dividend_growth", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.DividendGrowthResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if !approxEqual(r.CAGRPercent, 10.0, 0.01) {
		t.Errorf("CAGRPercent: want ~10.0, got %v", r.CAGRPercent)
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if !strings.Contains(r.InputsSummary, "AAPL") {
		t.Errorf("InputsSummary must echo the label, got %q", r.InputsSummary)
	}
}

func TestDispatch_CalculateDividendGrowth_BadType(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"dividends": "not an array"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_dividend_growth", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("non-array dividends should produce an error, got %s", result)
	}
}

func TestDispatch_CalculateStressTest_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"current_value":  10000.0,
		"shocks_percent": []float64{-10, -20, -30, -40},
		"label":          "portfolio",
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_stress_test", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.StressTestResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if len(r.Scenarios) != 4 {
		t.Errorf("Scenarios: want 4, got %d", len(r.Scenarios))
	}
	if !approxEqual(r.WorstCase.ResultingValue, 6000, 0.01) {
		t.Errorf("WorstCase.ResultingValue: want 6000, got %v", r.WorstCase.ResultingValue)
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if !strings.Contains(r.InputsSummary, "portfolio") {
		t.Errorf("InputsSummary must echo the label, got %q", r.InputsSummary)
	}
}

func TestDispatch_CalculateMonteCarloSimulation_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"last_price":        100.0,
		"drift_annual":      0.08,
		"volatility_annual": 0.25,
		"days":              90.0,
		"num_simulations":   5000.0,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_monte_carlo_simulation", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.MonteCarloResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.Days != 90 {
		t.Errorf("Days: want 90, got %d", r.Days)
	}
	if r.NumSimulations != 5000 {
		t.Errorf("NumSimulations: want 5000, got %d", r.NumSimulations)
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if r.InputsSummary == "" {
		t.Error("InputsSummary must be populated by the dispatcher")
	}
}

func TestDispatch_CalculateMonteCarloSimulation_InvalidInput(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"last_price":        0.0,
		"drift_annual":      0.08,
		"volatility_annual": 0.25,
		"days":              90.0,
		"num_simulations":   5000.0,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_monte_carlo_simulation", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("last_price <= 0 should produce an error, got %s", result)
	}
}

func TestDispatch_CalculateStressTest_BadShocksType(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"current_value": 10000.0, "shocks_percent": "not an array"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_stress_test", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("non-array shocks_percent should produce an error, got %s", result)
	}
}

func TestDispatch_CalculateStressTest_ZeroCurrentValue(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"current_value": 0.0, "shocks_percent": []float64{-10}})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_stress_test", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("current_value <= 0 should produce an error, got %s", result)
	}
}

func TestDispatch_CalculateSMA_BadPrices(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// Pass a string where the schema expects an array.
	args := toolCallArgs(t, map[string]any{"prices": "not an array", "period": 3})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_sma", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("string prices should produce an error, got %s", result)
	}
}

// TestDispatch_CalculateComputedAtAndInputsSummary covers the
// calculate_*/convert_currency tools that don't otherwise have a dedicated
// dispatch test in this file (their business logic is already covered
// directly in pkg/finance), to confirm REF-2/REF-5: every one of these tools
// stamps its JSON result with a dispatcher-populated, RFC3339-parseable
// computed_at, and a non-empty inputs_summary. Cases that pass a label (or
// asset_label/benchmark_label) also assert it's echoed back in inputs_summary.
func TestDispatch_CalculateComputedAtAndInputsSummary(t *testing.T) {
	cases := []struct {
		name          string
		args          map[string]any
		wantInSummary []string
	}{
		{"calculate_roi", map[string]any{"cost_basis": 100.0, "current_value": 120.0}, nil},
		{"calculate_cagr", map[string]any{"initial_value": 100.0, "final_value": 150.0, "years": 3.0}, nil},
		{"calculate_volatility", map[string]any{"prices": []float64{100, 101, 99, 102, 105}, "label": "AAPL"}, []string{"AAPL"}},
		{"calculate_sharpe", map[string]any{"returns": []float64{0.01, -0.005, 0.02, 0.01, -0.01}, "risk_free_rate_annual": 0.03, "label": "AAPL"}, []string{"AAPL"}},
		{"calculate_sortino", map[string]any{"returns": []float64{0.01, -0.005, 0.02, 0.01, -0.01}, "risk_free_rate_annual": 0.03}, nil},
		{"calculate_max_drawdown", map[string]any{"prices": []float64{100, 110, 90, 95}}, nil},
		{"calculate_pnl", map[string]any{"entry_price": 100.0, "current_price": 110.0, "quantity": 10.0, "position_type": "long"}, nil},
		{"calculate_beta", map[string]any{"asset_returns": []float64{0.01, 0.02, -0.01, 0.03}, "benchmark_returns": []float64{0.008, 0.015, -0.005, 0.02}, "asset_label": "AAPL", "benchmark_label": "SPY"}, []string{"AAPL", "SPY"}},
		{"calculate_treynor", map[string]any{"returns": []float64{0.01, -0.005, 0.02, 0.01, -0.01}, "risk_free_rate_annual": 0.03, "beta": 1.1}, nil},
		{"calculate_information_ratio", map[string]any{"asset_returns": []float64{0.01, 0.02, -0.01, 0.03}, "benchmark_returns": []float64{0.008, 0.015, -0.005, 0.02}}, nil},
		{"calculate_var", map[string]any{"returns": []float64{0.01, -0.02, 0.015, -0.03, 0.02}, "confidence_level": 0.95, "portfolio_value": 10000.0, "method": "historical"}, nil},
		{"calculate_dcf", map[string]any{"free_cash_flows": []float64{100, 110, 120}, "discount_rate": 0.1, "terminal_growth_rate": 0.02, "shares_outstanding": 1000.0, "label": "AAPL"}, []string{"AAPL"}},
		{"calculate_multiples", map[string]any{"price": 50.0, "eps": 5.0, "book_value_per_share": 20.0, "ebitda": 100.0, "enterprise_value": 500.0, "revenue": 300.0}, nil},
		{"convert_currency", map[string]any{"amount": 100.0, "from_currency": "USD", "to_currency": "EUR", "exchange_rate": 0.9}, nil},
		{"calculate_compound_interest", map[string]any{"principal": 1000.0, "annual_rate": 0.05, "years": 5.0, "compounds_per_year": 12.0}, nil},
		{"calculate_stats", map[string]any{"values": []float64{1, 2, 3, 4, 5}, "label": "AAPL daily returns"}, []string{"AAPL daily returns"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := New(&mockLLM{}, &mockMarket{}, "")
			var lk ProgressKind
			args := toolCallArgs(t, tc.args)
			result := a.dispatch(context.Background(), llm.ToolCall{
				Function: llm.ToolCallFunction{Name: tc.name, Arguments: args},
			}, &lk)
			if result[:6] == "error:" {
				t.Fatalf("unexpected error: %s", result)
			}
			var r map[string]any
			if err := json.Unmarshal([]byte(result), &r); err != nil {
				t.Fatalf("invalid JSON: %s — %v", result, err)
			}
			computedAt, _ := r["computed_at"].(string)
			if computedAt == "" {
				t.Fatal("computed_at must be populated by the dispatcher")
			}
			if _, err := time.Parse(time.RFC3339, computedAt); err != nil {
				t.Errorf("computed_at %q is not RFC3339/ISO 8601: %v", computedAt, err)
			}
			inputsSummary, _ := r["inputs_summary"].(string)
			if inputsSummary == "" {
				t.Fatal("inputs_summary must be populated by the dispatcher")
			}
			for _, want := range tc.wantInSummary {
				if !strings.Contains(inputsSummary, want) {
					t.Errorf("inputs_summary %q must contain %q", inputsSummary, want)
				}
			}
		})
	}
}

// --- portfolio_calculate_metrics dispatch -----------------------------------

func TestDispatch_PortfolioSetTargetAllocation_OK(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"target_allocation": map[string]any{"AAPL": 0.4, "MSFT": 0.3, "GOOG": 0.3},
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_set_target_allocation", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.TargetAllocation) != 3 {
		t.Fatalf("TargetAllocation: want 3 symbols, got %d", len(reloaded.TargetAllocation))
	}
	if !approxEqual(reloaded.TargetAllocation["AAPL"], 0.4, 0.0001) {
		t.Errorf("TargetAllocation[AAPL]: want 0.4, got %v", reloaded.TargetAllocation["AAPL"])
	}
}

func TestDispatch_PortfolioSetTargetAllocation_Replaces(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.TargetAllocation = map[string]float64{"OLD": 1.0}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"target_allocation": map[string]any{"AAPL": 1.0}})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_set_target_allocation", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, stillThere := reloaded.TargetAllocation["OLD"]; stillThere {
		t.Error("TargetAllocation should be fully replaced, but OLD symbol survived")
	}
	if len(reloaded.TargetAllocation) != 1 {
		t.Errorf("TargetAllocation: want 1 symbol after replace, got %d", len(reloaded.TargetAllocation))
	}
}

func TestDispatch_PortfolioSetTargetAllocation_EmptyObject(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"target_allocation": map[string]any{}})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_set_target_allocation", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("empty target_allocation should produce an error, got %s", result)
	}
}

func TestDispatch_PortfolioSetTargetAllocation_NegativeWeight(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"target_allocation": map[string]any{"AAPL": -0.1}})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_set_target_allocation", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("negative weight should produce an error, got %s", result)
	}
}

func TestDispatch_PortfolioSetTargetAllocation_SumWarning(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"target_allocation": map[string]any{"AAPL": 0.5}})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_set_target_allocation", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	if !strings.Contains(result, "WARNING") {
		t.Errorf("result should warn that weights don't sum to ~1.0, got %s", result)
	}
}

func TestDispatch_PortfolioCalculateMetrics_OK(t *testing.T) {
	// Override the portfolios dir so we can write a deterministic portfolio
	// without touching the user's real config directory.
	tmp := t.TempDir()
	orig := portfolio.PortfoliosDir
	_ = orig                         // documented: we set the override via the package-internal var
	t.Setenv("XDG_CONFIG_HOME", tmp) // belt-and-braces; the override below is what counts

	// Set the package-level override directly.
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	now := time.Now().UTC().Truncate(time.Second)
	for _, sym := range []string{"AAPL", "MSFT"} {
		p.Instruments = append(p.Instruments, portfolio.Instrument{
			ID:     "ins-" + sym,
			Symbol: sym,
			Name:   sym,
			Type:   portfolio.InstrumentHolding,
			Lots:   []portfolio.Lot{{ID: "lot-" + sym, Quantity: 10, Price: 100, Date: now}},
		})
	}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	mp := &mockMarket{
		quotesBySymbol: map[string]market.Quote{
			"AAPL": {Last: 150},
			"MSFT": {Last: 200},
		},
		fundamentalsBySymbol: map[string]market.Fundamental{
			"AAPL": {DividendYieldTTM: 0.005, Beta: 1.2},
			"MSFT": {DividendYieldTTM: 0.008, Beta: 0.9},
		},
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_metrics", Arguments: "{}"},
	}, &lk)

	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var m portfolio.PortfolioMetrics
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if m.CurrentValue != 3500 {
		t.Errorf("CurrentValue: want 3500, got %v", m.CurrentValue)
	}
	if len(m.WeightBySymbol) != 2 {
		t.Errorf("WeightBySymbol: want 2 entries, got %d", len(m.WeightBySymbol))
	}
	if m.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	// Weighted beta: AAPL 1500/3500 * 1.2 + MSFT 2000/3500 * 0.9 ≈ 1.028.
	if m.WeightedBeta == 0 {
		t.Error("WeightedBeta must be non-zero when fundamentals are available")
	}
	// Dividend yield must be non-zero when fundamentals are available.
	if m.DividendYield == 0 {
		t.Error("DividendYield must be non-zero when fundamentals are available")
	}
}

func TestDispatch_PortfolioSuggestRebalance_OK(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	now := time.Now()
	p.Instruments = append(p.Instruments, portfolio.Instrument{
		ID:     "ins-AAPL",
		Symbol: "AAPL",
		Name:   "AAPL",
		Type:   portfolio.InstrumentHolding,
		Lots:   []portfolio.Lot{{ID: "lot-AAPL", Quantity: 10, Price: 100, Date: now}},
	})
	p.TargetAllocation = map[string]float64{"AAPL": 0.5}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	mp := &mockMarket{
		quotesBySymbol: map[string]market.Quote{"AAPL": {Last: 150}},
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_suggest_rebalance", Arguments: "{}"},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r portfolio.RebalanceResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if len(r.Operations) != 1 {
		t.Fatalf("Operations: want 1, got %d: %+v", len(r.Operations), r.Operations)
	}
	if r.Operations[0].Action != "sell" {
		t.Errorf("Action: want sell (AAPL overweight vs 50%% target), got %s", r.Operations[0].Action)
	}
}

func TestDispatch_PortfolioSuggestRebalance_NoTargetAllocation(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_suggest_rebalance", Arguments: "{}"},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("missing target_allocation should produce an error, got %s", result)
	}
}

func TestDispatch_PortfolioSuggestRebalance_MaxDriftPercent(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	now := time.Now()
	p.Instruments = append(p.Instruments, portfolio.Instrument{
		ID:     "ins-AAPL",
		Symbol: "AAPL",
		Name:   "AAPL",
		Type:   portfolio.InstrumentHolding,
		Lots:   []portfolio.Lot{{ID: "lot-AAPL", Quantity: 10, Price: 100, Date: now}},
	})
	// AAPL is exactly at its target weight (100%, no cash) — zero drift.
	p.TargetAllocation = map[string]float64{"AAPL": 1.0}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	mp := &mockMarket{quotesBySymbol: map[string]market.Quote{"AAPL": {Last: 150}}}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"max_drift_percent": 5.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_suggest_rebalance", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r portfolio.RebalanceResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if len(r.Operations) != 0 {
		t.Errorf("Operations: want 0 (within tolerance), got %d: %+v", len(r.Operations), r.Operations)
	}
}

// BUG-6: smaResult.Previous, emaResult.Previous, rsiResult.PreviousValue were
// typed float64, not Float. When the input is the minimum valid size, "previous"
// is NaN (warm-up slot); json.Marshal rejects NaN with UnsupportedValueError and
// the encode helper returned "" silently. Fixed by switching those fields to Float.
func TestDispatch_CalculateSMA_MinimumInput_ValidJSON(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// period=3, exactly 3 prices → previous will be NaN (warm-up).
	args := toolCallArgs(t, map[string]any{"prices": []float64{10, 11, 12}, "period": 3})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_sma", Arguments: args},
	}, &lk)
	if result == "" {
		t.Fatal("BUG-6 regression: dispatch returned empty string (json.Marshal NaN silently failed)")
	}
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.SmaResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	// previous must be null (NaN → null via Float.MarshalJSON).
	if !math.IsNaN(float64(r.Previous)) {
		t.Errorf("Previous: want NaN (marshalled as null), got %v", float64(r.Previous))
	}
}

func TestDispatch_CalculateRSI_MinimumInput_ValidJSON(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// period=3 → minimum 5 prices (period+2).
	prices := []float64{10, 11, 12, 11, 12}
	args := toolCallArgs(t, map[string]any{"prices": prices, "period": 3})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_rsi", Arguments: args},
	}, &lk)
	if result == "" {
		t.Fatal("BUG-6 regression: dispatch returned empty string (json.Marshal NaN silently failed)")
	}
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.RsiResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	// PreviousValue must be null (NaN → null via Float.MarshalJSON).
	if !math.IsNaN(float64(r.PreviousValue)) {
		t.Errorf("PreviousValue: want NaN (marshalled as null), got %v", float64(r.PreviousValue))
	}
}

// BUG-6 (EMA leg): emaResult.Previous was float64, not Float. When only 1
// price is supplied (the minimum that calcEMA accepts), previous stays at its
// initial math.NaN() because the len(values) >= 2 branch is not entered.
// json.Marshal then silently returned "" via the ignored error in encode().
func TestRegression_BUG6_EMAMinimumInputValidJSON(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// 1 price is the minimum calcEMA accepts (period=5 is fine here; EMA seeds
	// from the first observation so len(prices)==1 is valid).
	args := toolCallArgs(t, map[string]any{"prices": []float64{100.0}, "period": 5})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_ema", Arguments: args},
	}, &lk)
	if result == "" {
		t.Fatal("BUG-6 regression (EMA): dispatch returned empty string — json.Marshal silently rejected NaN in Previous")
	}
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r finance.EmaResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	// With a single price the EMA equals that price and Previous is null.
	if !approxEqual(r.Last, 100.0, 1e-9) {
		t.Errorf("Last: want 100.0, got %v", r.Last)
	}
	if !math.IsNaN(float64(r.Previous)) {
		t.Errorf("Previous: want NaN (serialised as JSON null), got %v", float64(r.Previous))
	}
}

// ---- regression tests: inconsistencies ----------------------------------------
//
// These tests guard against the two design inconsistencies fixed after the
// initial Tier-1 indicator commit. Run them in isolation with:
//
//	go test ./pkg/agent/... -run TestRegression_INCON

// INCON-1: calculate_bollinger_bands had an inconsistency with calculate_ema:
// omitting `num_std` returned an error ("num_std must be positive, got 0.0000")
// even though the tool description advertised "(default 2)" and `num_std` was
// not in the required list. Fixed: dispatch now applies 2.0 when the value is
// absent (≤ 0), matching how calculate_ema handles a missing alpha.
func TestRegression_INCON1_BollingerOmittedNumStdDefaultsToTwo(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// Deliberately omit num_std.
	args := toolCallArgs(t, map[string]any{
		"prices": []float64{10, 11, 12, 13, 14, 15, 16},
		"period": 3,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_bollinger_bands", Arguments: args},
	}, &lk)
	if result == "" || result[:6] == "error:" {
		t.Fatalf("INCON-1 regression: omitting num_std should default to 2.0, got: %s", result)
	}
	var r finance.BollingerResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.NumStd != 2.0 {
		t.Errorf("INCON-1 regression: want NumStd=2.0, got %v", r.NumStd)
	}
	// Sanity-check band structure: upper > middle > lower on every valid bar.
	for i, u := range r.Upper {
		m, l := r.Middle[i], r.Lower[i]
		if math.IsNaN(float64(u)) {
			continue // warm-up NaN — expected
		}
		if float64(u) < float64(m) || float64(m) < float64(l) {
			t.Errorf("band ordering violated at index %d: upper=%.4f middle=%.4f lower=%.4f",
				i, float64(u), float64(m), float64(l))
		}
	}
}

// INCON-1b: also verify that a zero num_std supplied explicitly is defaulted to
// 2.0 (the dispatch guard is `<= 0`, not just `== 0`).
func TestRegression_INCON1_BollingerZeroNumStdDefaultsToTwo(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"prices":  []float64{10, 11, 12, 13, 14, 15},
		"period":  3,
		"num_std": 0.0,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_bollinger_bands", Arguments: args},
	}, &lk)
	if result == "" || result[:6] == "error:" {
		t.Fatalf("INCON-1b regression: num_std=0 should default to 2.0, got: %s", result)
	}
	var r finance.BollingerResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.NumStd != 2.0 {
		t.Errorf("INCON-1b regression: want NumStd=2.0, got %v", r.NumStd)
	}
}

// INCON-2: portfolio_calculate_metrics always returned dividend_yield=0 and
// weighted_beta=0 because the dispatch only populated portfolio.Quote.Last and
// left DividendYieldTTM and Beta at their zero values. Fixed: the dispatch now
// calls GetFundamentals per holding and propagates those two fields.
func TestRegression_INCON2_PortfolioMetricsDividendAndBetaPropagate(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	now := time.Now().UTC().Truncate(time.Second)
	p := portfolio.NewPortfolio("regression-incon2")
	for _, sym := range []string{"AAPL", "MSFT"} {
		p.Instruments = append(p.Instruments, portfolio.Instrument{
			ID:     "ins-" + sym,
			Symbol: sym,
			Name:   sym,
			Type:   portfolio.InstrumentHolding,
			Lots:   []portfolio.Lot{{ID: "lot-" + sym, Quantity: 10, Price: 100, Date: now}},
		})
	}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	// AAPL: price=150, div_yield=0.5%, beta=1.2
	// MSFT: price=200, div_yield=0.8%, beta=0.9
	// Weights: AAPL=1500/3500≈0.4286, MSFT=2000/3500≈0.5714
	// Weighted beta  = 0.4286*1.2 + 0.5714*0.9 = 0.5143 + 0.5143 = 1.0286
	// Weighted yield = (0.005*1500 + 0.008*2000) / 3500 = (7.5+16) / 3500 ≈ 0.6714%
	mp := &mockMarket{
		quotesBySymbol: map[string]market.Quote{
			"AAPL": {Last: 150},
			"MSFT": {Last: 200},
		},
		fundamentalsBySymbol: map[string]market.Fundamental{
			"AAPL": {DividendYieldTTM: 0.005, Beta: 1.2},
			"MSFT": {DividendYieldTTM: 0.008, Beta: 0.9},
		},
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_metrics", Arguments: "{}"},
	}, &lk)
	if result == "" || result[:6] == "error:" {
		t.Fatalf("unexpected error or empty result: %s", result)
	}
	var m portfolio.PortfolioMetrics
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}

	// Weighted beta ≈ 1.029 (rounded to 4 decimal places by ComputeMetrics).
	wantBeta := (1500.0/3500.0)*1.2 + (2000.0/3500.0)*0.9
	if !approxEqual(m.WeightedBeta, math.Round(wantBeta*10000)/10000, 1e-3) {
		t.Errorf("INCON-2 regression: WeightedBeta want ≈%.4f, got %.4f", wantBeta, m.WeightedBeta)
	}
	if m.WeightedBeta == 0 {
		t.Error("INCON-2 regression: WeightedBeta is 0 — fundamentals not propagated to portfolio.Quote")
	}

	// Weighted dividend yield (stored as percent by ComputeMetrics).
	wantYieldFrac := (0.005*1500 + 0.008*2000) / 3500
	wantYieldPct := math.Round(wantYieldFrac*100*10000) / 10000 // round4 then *100
	if !approxEqual(m.DividendYield, wantYieldPct, 1e-3) {
		t.Errorf("INCON-2 regression: DividendYield want ≈%.4f%%, got %.4f%%", wantYieldPct, m.DividendYield)
	}
	if m.DividendYield == 0 {
		t.Error("INCON-2 regression: DividendYield is 0 — fundamentals not propagated to portfolio.Quote")
	}
}

// INCON-2b: when GetFundamentals fails for a holding, the rest of the portfolio
// metrics (cost basis, current value, HHI) must still be correct and the
// dividend_yield / weighted_beta for that holding must gracefully be 0.
func TestRegression_INCON2_PortfolioMetricsFundamentalsFailureIsNonFatal(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	now := time.Now().UTC().Truncate(time.Second)
	p := portfolio.NewPortfolio("regression-incon2b")
	p.Instruments = append(p.Instruments, portfolio.Instrument{
		ID:     "ins-AAPL",
		Symbol: "AAPL",
		Name:   "AAPL",
		Type:   portfolio.InstrumentHolding,
		Lots:   []portfolio.Lot{{ID: "lot-AAPL", Quantity: 5, Price: 200, Date: now}},
	})
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	// GetFundamentals returns an error; GetQuote succeeds.
	mp := &mockMarket{
		quotesBySymbol: map[string]market.Quote{"AAPL": {Last: 250}},
		fundErr:        market.ErrNotSupported,
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_metrics", Arguments: "{}"},
	}, &lk)
	if result == "" || result[:6] == "error:" {
		t.Fatalf("INCON-2b regression: fundamentals failure must not abort metrics: %s", result)
	}
	var m portfolio.PortfolioMetrics
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	// Current value and cost basis must still be computed correctly.
	if m.CurrentValue != 1250 { // 5 * 250
		t.Errorf("INCON-2b regression: CurrentValue want 1250, got %v", m.CurrentValue)
	}
	if m.CostBasis != 1000 { // 5 * 200
		t.Errorf("INCON-2b regression: CostBasis want 1000, got %v", m.CostBasis)
	}
	// dividend_yield and weighted_beta must be 0 when fundamentals are absent.
	if m.DividendYield != 0 {
		t.Errorf("INCON-2b regression: DividendYield want 0 when fundamentals fail, got %v", m.DividendYield)
	}
	if m.WeightedBeta != 0 {
		t.Errorf("INCON-2b regression: WeightedBeta want 0 when fundamentals fail, got %v", m.WeightedBeta)
	}
}

// DD-2: a quotes snapshot passed by the caller must be used directly and
// must NOT trigger a market provider auto-fetch for the symbols it covers.
// AAPL's mock quote is configured to error out, so if the dispatch tried to
// auto-fetch it despite the snapshot, AAPL would end up in MissingQuotes and
// its value would be excluded — this test would then fail.
func TestRegression_DD2_PortfolioMetricsQuotesSnapshotSkipsAutoFetch(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	now := time.Now().UTC().Truncate(time.Second)
	p := portfolio.NewPortfolio("regression-dd2")
	p.Instruments = append(p.Instruments,
		portfolio.Instrument{
			ID: "ins-AAPL", Symbol: "AAPL", Name: "AAPL", Type: portfolio.InstrumentHolding,
			Lots: []portfolio.Lot{{ID: "lot-AAPL", Quantity: 10, Price: 100, Date: now}},
		},
		portfolio.Instrument{
			ID: "ins-MSFT", Symbol: "MSFT", Name: "MSFT", Type: portfolio.InstrumentHolding,
			Lots: []portfolio.Lot{{ID: "lot-MSFT", Quantity: 5, Price: 100, Date: now}},
		},
	)
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	mp := &mockMarket{
		// AAPL would fail if the dispatch ever tried to auto-fetch it.
		quoteErrBySymbol: map[string]error{"AAPL": market.ErrNotFound},
		// MSFT has no snapshot entry, so it must come from auto-fetch.
		quotesBySymbol: map[string]market.Quote{"MSFT": {Last: 200}},
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"quotes": map[string]any{
			"AAPL": map[string]any{"last": 150.0, "dividend_yield_ttm": 0.005, "beta": 1.2},
		},
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_metrics", Arguments: args},
	}, &lk)
	if result == "" || result[:6] == "error:" {
		t.Fatalf("DD-2 regression: unexpected error or empty result: %s", result)
	}
	var m portfolio.PortfolioMetrics
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	for _, sym := range m.MissingQuotes {
		if sym == "AAPL" {
			t.Fatalf("DD-2 regression: AAPL should be valued from the snapshot, not auto-fetched (missing_quotes=%v)", m.MissingQuotes)
		}
	}
	// AAPL: 10 * 150 = 1500, MSFT: 5 * 200 = 1000 → current_value = 2500.
	if m.CurrentValue != 2500 {
		t.Errorf("DD-2 regression: CurrentValue want 2500 (snapshot AAPL + auto-fetched MSFT), got %v", m.CurrentValue)
	}
}

func TestDispatch_PortfolioCalculateMetrics_NoMarketProvider(t *testing.T) {
	// With a nil market provider, the tool still runs but flags the
	// limitation in the summary.
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	now := time.Now().UTC().Truncate(time.Second)
	p.Instruments = append(p.Instruments, portfolio.Instrument{
		ID:     "ins-AAPL",
		Symbol: "AAPL",
		Name:   "AAPL",
		Type:   portfolio.InstrumentHolding,
		Lots:   []portfolio.Lot{{ID: "lot-AAPL", Quantity: 10, Price: 100, Date: now}},
	})
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, nil, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_metrics", Arguments: "{}"},
	}, &lk)

	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	if !strings.Contains(result, "no market provider configured") {
		t.Errorf("summary must warn about missing market provider, got: %s", result)
	}
}

// ---- REF-3: numeric input validation ------------------------------------------
