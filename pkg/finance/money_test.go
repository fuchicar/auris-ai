package finance

import (
	"math"
	"testing"
)

func TestCurrencySymbol(t *testing.T) {
	cases := []struct {
		code string
		want string
	}{
		{"USD", "$"},
		{"EUR", "€"},
		{"GBP", "£"},
		{"XYZ", "XYZ "},
		{"", ""},
	}
	for _, c := range cases {
		if got := CurrencySymbol(c.code); got != c.want {
			t.Errorf("CurrencySymbol(%q): want %q, got %q", c.code, c.want, got)
		}
	}
}

func TestFormatMoney(t *testing.T) {
	cases := []struct {
		amount   float64
		currency string
		want     string
	}{
		{1234.5, "USD", "$1,234.50"},
		{1234567.891, "EUR", "€1,234,567.89"},
		{0, "USD", "$0.00"},
		{-500, "USD", "-$500.00"},
		{99.999, "GBP", "£100.00"},
		{42, "XYZ", "XYZ 42.00"},
	}
	for _, c := range cases {
		if got := FormatMoney(c.amount, c.currency); got != c.want {
			t.Errorf("FormatMoney(%v, %q): want %q, got %q", c.amount, c.currency, c.want, got)
		}
	}
}

func TestFormatMoney_NaNInf(t *testing.T) {
	if got := FormatMoney(math.NaN(), "USD"); got != "—" {
		t.Errorf("NaN: want —, got %q", got)
	}
	if got := FormatMoney(math.Inf(1), "USD"); got != "—" {
		t.Errorf("+Inf: want —, got %q", got)
	}
	if got := FormatMoney(math.Inf(-1), "USD"); got != "—" {
		t.Errorf("-Inf: want —, got %q", got)
	}
}

func TestFormatMoneySigned(t *testing.T) {
	cases := []struct {
		amount   float64
		currency string
		want     string
	}{
		{1234.5, "USD", "+$1,234.50"},
		{-1234.5, "USD", "-$1,234.50"},
		{0, "USD", "$0.00"},
	}
	for _, c := range cases {
		if got := FormatMoneySigned(c.amount, c.currency); got != c.want {
			t.Errorf("FormatMoneySigned(%v, %q): want %q, got %q", c.amount, c.currency, c.want, got)
		}
	}
}
