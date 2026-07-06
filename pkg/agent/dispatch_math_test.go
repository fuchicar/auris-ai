package agent

import (
	"context"
	"encoding/json"
	"testing"

	"auris/pkg/llm"
)

// dispatch_math_test.go exercises the dispatch() handler for every math tool,
// validating argument parsing and JSON output without touching the market layer.

func dispatchMath(t *testing.T, a *Agent, name string, args map[string]any) map[string]any {
	t.Helper()
	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      name,
			Arguments: toolCallArgs(t, args),
		},
	}, &lk)
	var r map[string]any
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("%s: result is not valid JSON: %s — %v", name, result, err)
	}
	return r
}

func dispatchMathErr(t *testing.T, a *Agent, name string, args map[string]any) string {
	t.Helper()
	var lk ProgressKind
	return a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{
			Name:      name,
			Arguments: toolCallArgs(t, args),
		},
	}, &lk)
}

// TestDispatch_MathTools_OK verifies that every math tool returns valid JSON
// containing the primary output field when given valid arguments.
func TestDispatch_MathTools_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")

	tests := []struct {
		tool    string
		args    map[string]any
		wantKey string
	}{
		{
			tool:    "calculate_roi",
			args:    map[string]any{"cost_basis": 100.0, "current_value": 120.0},
			wantKey: "roi_percent",
		},
		{
			tool:    "calculate_cagr",
			args:    map[string]any{"initial_value": 1000.0, "final_value": 2000.0, "years": 5.0},
			wantKey: "cagr_percent",
		},
		{
			tool:    "calculate_volatility",
			args:    map[string]any{"prices": []any{100.0, 102.0, 98.0, 105.0, 103.0}},
			wantKey: "volatility_annual_percent",
		},
		{
			tool: "calculate_sharpe",
			args: map[string]any{
				"returns":               []any{0.01, -0.005, 0.02, -0.01, 0.015},
				"risk_free_rate_annual": 0.04,
			},
			wantKey: "sharpe_ratio",
		},
		{
			tool:    "calculate_max_drawdown",
			args:    map[string]any{"prices": []any{100.0, 120.0, 80.0, 90.0}},
			wantKey: "max_drawdown_percent",
		},
		{
			tool: "calculate_pnl",
			args: map[string]any{
				"entry_price": 100.0, "current_price": 110.0,
				"quantity": 10.0, "position_type": "long",
			},
			wantKey: "pnl_absolute",
		},
		{
			tool: "calculate_sortino",
			args: map[string]any{
				"returns":               []any{0.01, -0.005, 0.02, -0.01, 0.015},
				"risk_free_rate_annual": 0.04,
			},
			wantKey: "sortino_ratio",
		},
		{
			tool: "calculate_beta",
			args: map[string]any{
				"asset_returns":     []any{0.01, -0.02, 0.015, -0.005, 0.02},
				"benchmark_returns": []any{0.01, -0.02, 0.015, -0.005, 0.02},
			},
			wantKey: "beta",
		},
		{
			tool: "calculate_treynor",
			args: map[string]any{
				"returns":               []any{0.01, -0.005, 0.02, -0.01, 0.015},
				"risk_free_rate_annual": 0.04,
				"beta":                  1.2,
			},
			wantKey: "treynor_ratio_percent",
		},
		{
			tool: "calculate_information_ratio",
			args: map[string]any{
				"asset_returns":     []any{0.02, 0.01, 0.03, 0.015, 0.025},
				"benchmark_returns": []any{0.01, 0.005, 0.015, 0.01, 0.012},
			},
			wantKey: "information_ratio",
		},
		{
			tool: "calculate_var",
			args: map[string]any{
				"returns":          []any{0.01, -0.005, 0.02, -0.01, 0.015, -0.02, 0.008, -0.012},
				"confidence_level": 0.95,
				"portfolio_value":  100000.0,
				"method":           "parametric",
			},
			wantKey: "var_absolute",
		},
		{
			tool: "calculate_var",
			args: map[string]any{
				"returns":          []any{0.01, -0.005, 0.02, -0.01, 0.015, -0.02, 0.008, -0.012},
				"confidence_level": 0.95,
				"portfolio_value":  100000.0,
				"method":           "historical",
			},
			wantKey: "var_absolute",
		},
		{
			tool: "calculate_dcf",
			args: map[string]any{
				"free_cash_flows":      []any{100.0, 100.0, 100.0, 100.0, 100.0},
				"discount_rate":        0.10,
				"terminal_growth_rate": 0.03,
				"shares_outstanding":   1000.0,
			},
			wantKey: "intrinsic_value_total",
		},
		{
			tool: "calculate_multiples",
			args: map[string]any{
				"price": 50.0, "eps": 2.5, "book_value_per_share": 20.0,
				"ebitda": 1e9, "enterprise_value": 5e9, "revenue": 2e9,
			},
			wantKey: "summary",
		},
		{
			tool: "convert_currency",
			args: map[string]any{
				"amount": 100.0, "from_currency": "USD",
				"to_currency": "EUR", "exchange_rate": 0.92,
			},
			wantKey: "converted_amount",
		},
		{
			tool: "calculate_compound_interest",
			args: map[string]any{
				"principal": 1000.0, "annual_rate": 0.10,
				"years": 1.0, "compounds_per_year": 12.0,
			},
			wantKey: "final_amount",
		},
		{
			tool: "calculate_stats",
			args: map[string]any{
				"values": []any{1.0, 2.0, 3.0, 4.0, 5.0},
				"label":  "prices",
			},
			wantKey: "mean",
		},
		{
			// REF-3: a legitimate 1000% single-period return (a 10x move)
			// must not be rejected by the plausibility bound.
			tool: "calculate_sharpe",
			args: map[string]any{
				"returns":               []any{0.01, -0.02, 10.0, 0.015},
				"risk_free_rate_annual": 0.04,
			},
			wantKey: "sharpe_ratio",
		},
	}

	for _, tc := range tests {
		t.Run(tc.tool+"/"+tc.wantKey, func(t *testing.T) {
			r := dispatchMath(t, a, tc.tool, tc.args)
			if r[tc.wantKey] == nil {
				t.Errorf("expected key %q in result, got %v", tc.wantKey, r)
			}
		})
	}
}

// TestDispatch_MathTools_Error verifies that each math tool returns an "error:"
// prefix when given arguments that violate its documented constraints.
func TestDispatch_MathTools_Error(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"calculate_roi", map[string]any{"cost_basis": 0.0, "current_value": 100.0}},
		{"calculate_cagr", map[string]any{"initial_value": 1000.0, "final_value": 2000.0, "years": 0.0}},
		{"calculate_volatility", map[string]any{"prices": []any{100.0}}},
		{"calculate_sharpe", map[string]any{
			"returns":               []any{0.005, 0.005, 0.005},
			"risk_free_rate_annual": 0.04,
		}},
		{"calculate_max_drawdown", map[string]any{"prices": []any{100.0}}},
		{"calculate_pnl", map[string]any{
			"entry_price": 100.0, "current_price": 110.0,
			"quantity": 10.0, "position_type": "buy",
		}},
		{"calculate_sortino", map[string]any{
			// All returns comfortably above the risk-free rate → zero
			// downside deviation → undefined ratio.
			"returns":               []any{0.05, 0.06, 0.055},
			"risk_free_rate_annual": 0.01,
		}},
		{"calculate_beta", map[string]any{
			"asset_returns":     []any{0.01, -0.02},
			"benchmark_returns": []any{0.01},
		}},
		{"calculate_treynor", map[string]any{
			"returns":               []any{0.01, -0.005, 0.02, -0.01, 0.015},
			"risk_free_rate_annual": 0.04,
			"beta":                  0.0,
		}},
		{"calculate_information_ratio", map[string]any{
			"asset_returns":     []any{0.01, -0.02, 0.015},
			"benchmark_returns": []any{0.01, -0.02},
		}},
		{"calculate_var", map[string]any{
			"returns":          []any{0.01, -0.01},
			"confidence_level": 1.5,
			"portfolio_value":  100000.0,
			"method":           "parametric",
		}},
		{"calculate_dcf", map[string]any{
			"free_cash_flows":      []any{100.0},
			"discount_rate":        0.03,
			"terminal_growth_rate": 0.05,
			"shares_outstanding":   100.0,
		}},
		{"convert_currency", map[string]any{
			"amount": 100.0, "from_currency": "USD",
			"to_currency": "EUR", "exchange_rate": 0.0,
		}},
		{"calculate_compound_interest", map[string]any{
			"principal": 1000.0, "annual_rate": 0.10,
			"years": 1.0, "compounds_per_year": 0.0,
		}},
		{"calculate_stats", map[string]any{"values": []any{}, "label": "empty"}},
		// REF-3: numeric input validation — implausible/non-finite magnitudes
		// that are representable as JSON numbers (NaN/±Inf can't be, since
		// JSON has no literal for them).
		{"calculate_sharpe", map[string]any{
			"returns":               []any{0.01, 1e6, 0.02},
			"risk_free_rate_annual": 0.04,
		}},
		{"calculate_multiples", map[string]any{
			"price": -50.0, "eps": 2.5, "book_value_per_share": 20.0,
			"ebitda": 1e9, "enterprise_value": 5e9, "revenue": 2e9,
		}},
		{"calculate_max_drawdown", map[string]any{"prices": []any{100.0, 0.0, 90.0}}},
	}

	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			result := dispatchMathErr(t, a, tc.tool, tc.args)
			if len(result) < 6 || result[:6] != "error:" {
				t.Errorf("expected error prefix, got %q", result)
			}
		})
	}
}

// TestDispatch_MathTools_BadArrayArg verifies the "must be an array of numbers"
// guard when a non-array value is passed for an array parameter.
func TestDispatch_MathTools_BadArrayArg(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")

	arrayTools := []struct {
		tool string
		args map[string]any
	}{
		{"calculate_volatility", map[string]any{"prices": "not-an-array"}},
		{"calculate_sharpe", map[string]any{"returns": "bad", "risk_free_rate_annual": 0.04}},
		{"calculate_max_drawdown", map[string]any{"prices": 42.0}},
		{"calculate_sortino", map[string]any{"returns": "bad", "risk_free_rate_annual": 0.04}},
		{"calculate_beta", map[string]any{"asset_returns": "bad", "benchmark_returns": []any{0.01}}},
		{"calculate_treynor", map[string]any{"returns": "bad", "risk_free_rate_annual": 0.04, "beta": 1.2}},
		{"calculate_information_ratio", map[string]any{"asset_returns": "bad", "benchmark_returns": []any{0.01}}},
		{"calculate_var", map[string]any{
			"returns": "bad", "confidence_level": 0.95,
			"portfolio_value": 1000.0, "method": "parametric",
		}},
		{"calculate_dcf", map[string]any{
			"free_cash_flows": "bad", "discount_rate": 0.10,
			"terminal_growth_rate": 0.03, "shares_outstanding": 100.0,
		}},
		{"calculate_stats", map[string]any{"values": "bad", "label": "x"}},
	}

	for _, tc := range arrayTools {
		t.Run(tc.tool, func(t *testing.T) {
			result := dispatchMathErr(t, a, tc.tool, tc.args)
			if len(result) < 5 || result[:5] != "error" {
				t.Errorf("expected error prefix, got %q", result)
			}
		})
	}
}
