package tui

import (
	"testing"
	"time"

	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/market"
)

func TestRenderCandleChart_InsufficientData(t *testing.T) {
	s := NewStyles(ThemeDark)
	candles := make([]market.Candle, chartCandlePeriod) // one short of the minimum
	for i := range candles {
		candles[i] = market.Candle{Time: time.Now().AddDate(0, 0, i), Open: 10, High: 11, Low: 9, Close: 10}
	}
	if got := renderCandleChart(candles, s, chartWidth, chartHeightMax); got != "" {
		t.Errorf("renderCandleChart with %d candles: want \"\", got non-empty output", len(candles))
	}
}

func TestRenderCandleChart_EnoughData(t *testing.T) {
	s := NewStyles(ThemeDark)
	candles := make([]market.Candle, chartCandlePeriod+1)
	for i := range candles {
		candles[i] = market.Candle{
			Time:  time.Now().AddDate(0, 0, i),
			Open:  100 + float64(i),
			High:  101 + float64(i),
			Low:   99 + float64(i),
			Close: 100 + float64(i),
		}
	}
	if got := renderCandleChart(candles, s, chartWidth, chartHeightMax); got == "" {
		t.Error("renderCandleChart with enough data: want non-empty output, got \"\"")
	}
}

// TestRenderCandleChart_HeightZeroReturnsHint covers the issue #36
// degrade-gracefully path: when the screen can't spare any rows for the
// chart (height <= 0 or below chartHeightMin), the helper renders the
// "chart hidden" hint instead of an empty string — same precedent as
// agent.chart_too_narrow in screen_agent.go.
func TestRenderCandleChart_HeightZeroReturnsHint(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	s := NewStyles(ThemeDark)
	candles := make([]market.Candle, chartCandlePeriod+1)
	for i := range candles {
		candles[i] = market.Candle{
			Time:  time.Now().AddDate(0, 0, i),
			Open:  100 + float64(i),
			High:  101 + float64(i),
			Low:   99 + float64(i),
			Close: 100 + float64(i),
		}
	}
	if got := renderCandleChart(candles, s, chartWidth, 0); got == "" {
		t.Error("renderCandleChart with height=0: want \"[chart hidden…]\" hint, got \"\"")
	}
	if got := renderCandleChart(candles, s, chartWidth, chartHeightMin-1); got == "" {
		t.Errorf("renderCandleChart with height=%d (< chartHeightMin): want hint, got \"\"",
			chartHeightMin-1)
	}
}

func TestRenderSparkline_InsufficientData(t *testing.T) {
	s := NewStyles(ThemeDark)
	if got := renderSparkline(nil, s); got != "" {
		t.Errorf("renderSparkline(nil): want \"\", got %q", got)
	}
	if got := renderSparkline([]float64{1}, s); got != "" {
		t.Errorf("renderSparkline(single value): want \"\", got %q", got)
	}
}

func TestRenderSparkline_EnoughData(t *testing.T) {
	s := NewStyles(ThemeDark)
	if got := renderSparkline([]float64{1, 2, 3, 2, 1}, s); got == "" {
		t.Error("renderSparkline with enough data: want non-empty output, got \"\"")
	}
}
