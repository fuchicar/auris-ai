// Package finance holds pure numeric indicator functions shared between the
// agent's math tools (pkg/agent/math.go) and the TUI's chart rendering
// (pkg/tui), so the TUI can compute overlays without going through the LLM.
package finance

import (
	"fmt"
	"math"
)

// SMA computes the simple moving average of values over a sliding window of
// length period. It returns a slice the same length as values where the
// first period-1 entries are NaN (warm-up period).
func SMA(values []float64, period int) ([]float64, error) {
	if period <= 0 {
		return nil, fmt.Errorf("period must be greater than zero, got %d", period)
	}
	if len(values) < period {
		return nil, fmt.Errorf("at least %d values required for SMA, got %d", period, len(values))
	}

	out := make([]float64, len(values))
	for i := range out {
		out[i] = math.NaN()
	}
	var sum float64
	for i := 0; i < period; i++ {
		sum += values[i]
	}
	out[period-1] = sum / float64(period)
	for i := period; i < len(values); i++ {
		sum += values[i] - values[i-period]
		out[i] = sum / float64(period)
	}
	return out, nil
}
