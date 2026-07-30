package tui

import (
	"testing"
	"time"

	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/market"
)

func TestRenderCandleChart_InsufficientData(t *testing.T) {
	s := NewStyles(ThemeDark, 0)
	candles := make([]market.Candle, chartCandlePeriod) // one short of the minimum
	for i := range candles {
		candles[i] = market.Candle{Time: time.Now().AddDate(0, 0, i), Open: 10, High: 11, Low: 9, Close: 10}
	}
	if got := renderCandleChart(candles, s, chartWidth(s), chartHeightMax); got != "" {
		t.Errorf("renderCandleChart with %d candles: want \"\", got non-empty output", len(candles))
	}
}

func TestRenderCandleChart_EnoughData(t *testing.T) {
	s := NewStyles(ThemeDark, 0)
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
	if got := renderCandleChart(candles, s, chartWidth(s), chartHeightMax); got == "" {
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
	s := NewStyles(ThemeDark, 0)
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
	if got := renderCandleChart(candles, s, chartWidth(s), 0); got == "" {
		t.Error("renderCandleChart with height=0: want \"[chart hidden…]\" hint, got \"\"")
	}
	if got := renderCandleChart(candles, s, chartWidth(s), chartHeightMin-1); got == "" {
		t.Errorf("renderCandleChart with height=%d (< chartHeightMin): want hint, got \"\"",
			chartHeightMin-1)
	}
}

// TestChartWidth_FollowsStyles (issue #37) verifies that chartWidth
// tracks Styles.PanelWidth instead of the historical fixed PanelWidth-4
// constant, so the candle canvas shrinks to fit narrow terminals.
//
//	panelWidth(60)  = 56         (60 − panelMargin)
//	chartWidth(60)  = 52         (panelWidth − 4)
//	panelWidth(12)  = 20         (clamped to PanelWidthMin)
//	chartWidth(12)  = 16         (20 − 4)
func TestChartWidth_FollowsStyles(t *testing.T) {
	cases := []struct {
		name string
		maxW int
		want int
	}{
		{"wide terminal falls back to ceiling", 0, PanelWidthMax - boxInnerInset},
		{"100-col terminal clamped to 72", 100, PanelWidthMax - boxInnerInset},
		{"60-col terminal yields 52", 60, 52},
		{"narrow terminal floored at PanelWidthMin", 12, PanelWidthMin - boxInnerInset},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStyles(ThemeDark, tc.maxW)
			if got := chartWidth(s); got != tc.want {
				t.Errorf("chartWidth(NewStyles(ThemeDark, %d)): want %d, got %d", tc.maxW, tc.want, got)
			}
		})
	}
}

func TestRenderSparkline_InsufficientData(t *testing.T) {
	s := NewStyles(ThemeDark, 0)
	if got := renderSparkline(nil, s); got != "" {
		t.Errorf("renderSparkline(nil): want \"\", got %q", got)
	}
	if got := renderSparkline([]float64{1}, s); got != "" {
		t.Errorf("renderSparkline(single value): want \"\", got %q", got)
	}
}

func TestRenderSparkline_EnoughData(t *testing.T) {
	s := NewStyles(ThemeDark, 0)
	if got := renderSparkline([]float64{1, 2, 3, 2, 1}, s); got == "" {
		t.Error("renderSparkline with enough data: want non-empty output, got \"\"")
	}
}
