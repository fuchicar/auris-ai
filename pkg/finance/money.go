package finance

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// currencySymbols maps a curated set of ISO 4217 codes to their display
// symbol, covering what the FMP/EODHD market drivers realistically return.
var currencySymbols = map[string]string{
	"USD": "$",
	"EUR": "€",
	"GBP": "£",
	"JPY": "¥",
	"CHF": "Fr",
	"CAD": "C$",
	"AUD": "A$",
	"MXN": "$",
	"BRL": "R$",
	"CNY": "¥",
}

// CurrencySymbols returns the ISO 4217 codes CurrencySymbol recognizes, in a
// stable display order (used to populate currency pickers).
func CurrencySymbols() []string {
	return []string{"USD", "EUR", "GBP", "JPY", "CHF", "CAD", "AUD", "MXN", "BRL", "CNY"}
}

// CurrencySymbol returns the display symbol for an ISO 4217 code. An
// unrecognized or empty code falls back to the code itself (or "" if empty)
// followed by a space, so callers always get a usable prefix.
func CurrencySymbol(code string) string {
	if sym, ok := currencySymbols[code]; ok {
		return sym
	}
	if code == "" {
		return ""
	}
	return code + " "
}

// FormatMoney formats amount with thousands separators, 2 decimal places,
// and a currency symbol prefix (e.g. "$1,234.56", "-$500.00"). NaN/±Inf
// render as "—". Formatting is deliberately locale-independent: comma-grouped
// /period-decimal with the symbol always prefixed, regardless of the active
// UI locale.
func FormatMoney(amount float64, currency string) string {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return "—"
	}
	sign := ""
	if amount < 0 {
		sign = "-"
	}
	return sign + CurrencySymbol(currency) + groupThousands(math.Abs(amount))
}

// FormatMoneySigned is FormatMoney with an explicit "+" prefix for positive
// amounts, for rendering P&L figures.
func FormatMoneySigned(amount float64, currency string) string {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return "—"
	}
	sign := ""
	switch {
	case amount > 0:
		sign = "+"
	case amount < 0:
		sign = "-"
	}
	return sign + CurrencySymbol(currency) + groupThousands(math.Abs(amount))
}

// groupThousands formats a non-negative v with 2 decimal places and
// comma-separated thousands groups, e.g. 1234567.5 -> "1,234,567.50".
func groupThousands(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	intPart, decPart, _ := strings.Cut(s, ".")

	var grouped strings.Builder
	n := len(intPart)
	for i, digit := range intPart {
		if i > 0 && (n-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}

	return fmt.Sprintf("%s.%s", grouped.String(), decPart)
}
