package tui

import "testing"

func TestFormatToolCall_SingleArg(t *testing.T) {
	got := formatToolCall("market_get_quote", `{"symbol":"AAPL"}`)
	want := "market_get_quote(AAPL)"
	if got != want {
		t.Errorf("formatToolCall() = %q, want %q", got, want)
	}
}

func TestFormatToolCall_MultipleArgsSortedByKey(t *testing.T) {
	got := formatToolCall("portfolio_add_lot", `{"symbol":"AAPL","quantity":5,"price":100}`)
	want := "portfolio_add_lot(price=100, quantity=5, symbol=AAPL)"
	if got != want {
		t.Errorf("formatToolCall() = %q, want %q", got, want)
	}
}

func TestFormatToolCall_NoArgs(t *testing.T) {
	for _, args := range []string{`{}`, ``, `not-json`} {
		got := formatToolCall("time_now", args)
		want := "time_now()"
		if got != want {
			t.Errorf("formatToolCall(%q) = %q, want %q", args, got, want)
		}
	}
}

func TestFormatToolCall_ArrayArg(t *testing.T) {
	got := formatToolCall("calculate_sma", `{"prices":[1,2,3,4,5],"period":3}`)
	want := "calculate_sma(period=3, prices=[5])"
	if got != want {
		t.Errorf("formatToolCall() = %q, want %q", got, want)
	}
}
