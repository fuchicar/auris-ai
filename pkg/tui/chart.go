package tui

import (
	"math"

	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	tslc "github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"
	"github.com/NimbleMarkets/ntcharts/sparkline"

	"github.com/fuchicar/auris-ai/pkg/finance"
	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/market"
)

const (
	chartCandlePeriod = 20 // SMA period overlaid on the candle chart
	chartHeightMax    = 14 // generous default (matches the historical fixed chart)
	chartHeightMedium = 8  // compact variant when the screen is tight
	chartHeightMin    = 5  // smallest height the timeseries chart can render meaningfully
	sparklineWidth    = 20
	sparklineHeight   = 3
)

// chartWidth returns the canvas width used by renderCandleChart for the
// given styles. It is derived from Styles.PanelWidth so it follows the
// terminal's current width: 72 on a wide terminal, the same minus 4 on
// any narrower one (issues #37). The 4-col subtraction matches the
// historical PanelWidth-4 layout (border + padding).
func chartWidth(s *Styles) int {
	if s == nil {
		return PanelWidthMax - 4
	}
	return s.PanelWidth - 4
}

// renderCandleChart renders a candlestick chart with a single SMA overlay
// line for candles ordered oldest-first. Returns "" when there isn't enough
// data to draw a meaningful chart (fewer than chartCandlePeriod+1 candles).
//
// When height is 0 (the screen can't spare any rows for the chart) or below
// chartHeightMin, the chart is replaced with the locale "chart hidden" hint
// — same degrade-gracefully precedent used by agent.chart_too_narrow in
// screen_agent.go. width is clamped to chartWidth(s) so callers don't have
// to know the internal canvas size.
func renderCandleChart(candles []market.Candle, s *Styles, width, height int) string {
	if len(candles) < chartCandlePeriod+1 {
		return ""
	}
	if height <= 0 || height < chartHeightMin {
		return s.Hint.Render(locale.T("portfolio.instrument.chart_too_short"))
	}
	cw := chartWidth(s)
	w := width
	if w <= 0 || w > cw {
		w = cw
	}

	minY, maxY := candles[0].Low, candles[0].High
	closes := make([]float64, len(candles))
	for i, c := range candles {
		if c.Low < minY {
			minY = c.Low
		}
		if c.High > maxY {
			maxY = c.High
		}
		closes[i] = c.Close
	}

	chart := tslc.New(w, height,
		tslc.WithTimeRange(candles[0].Time, candles[len(candles)-1].Time),
		tslc.WithYRange(minY, maxY),
	)
	for _, c := range candles {
		chart.PushDataSet("open", tslc.TimePoint{Time: c.Time, Value: c.Open})
		chart.PushDataSet("high", tslc.TimePoint{Time: c.Time, Value: c.High})
		chart.PushDataSet("low", tslc.TimePoint{Time: c.Time, Value: c.Low})
		chart.PushDataSet("close", tslc.TimePoint{Time: c.Time, Value: c.Close})
	}
	chart.DrawCandle("open", "high", "low", "close", s.Bull, s.Bear)

	if sma, err := finance.SMA(closes, chartCandlePeriod); err == nil {
		smaStyle := s.Selected
		for i := 1; i < len(sma); i++ {
			if math.IsNaN(sma[i-1]) || math.IsNaN(sma[i]) {
				continue
			}
			chart.DrawLineWithStyle(
				canvas.Float64Point{X: float64(candles[i-1].Time.Unix()), Y: sma[i-1]},
				canvas.Float64Point{X: float64(candles[i].Time.Unix()), Y: sma[i]},
				runes.ArcLineStyle, smaStyle,
			)
		}
	}

	return chart.View()
}

// renderSparkline renders a small single-color price sparkline from a slice
// of closes ordered oldest-first. Returns "" when there are fewer than 2
// points to draw a line between.
//
// ntcharts' sparkline scales bars from 0 up to the data's max value, not
// from the data's min to its max — for a price series where every value is
// within a couple of percent of the max (the common case for a one-month
// window), that leaves every bar filling nearly the full height, rendering
// as a solid block instead of a shape. Shifting the series down by its own
// minimum before pushing makes the bars span the full height based on the
// series' actual relative movement.
func renderSparkline(closes []float64, s *Styles) string {
	if len(closes) < 2 {
		return ""
	}
	min := closes[0]
	for _, c := range closes[1:] {
		if c < min {
			min = c
		}
	}
	sp := sparkline.New(sparklineWidth, sparklineHeight, sparkline.WithStyle(s.Bull))
	for _, c := range closes {
		sp.Push(c - min)
	}
	sp.Draw()
	return sp.View()
}
