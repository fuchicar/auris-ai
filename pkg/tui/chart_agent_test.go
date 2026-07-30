package tui

import (
	"strings"
	"testing"

	"github.com/fuchicar/auris-ai/pkg/config"
)

func TestRenderHistory_SplicesChartBlock(t *testing.T) {
	m := &AgentModel{
		styles: NewStyles(ThemeDark, 0),
		width:  100,
		session: &config.Session{
			History: []config.ChatTurn{
				{Role: "user", Content: "how has AAPL done?"},
				{Role: "assistant", Content: "Here's the trend.", Charts: []string{"FAKE-CHART-BLOCK"}},
			},
		},
	}
	out := m.renderHistory()
	if !strings.Contains(out, "FAKE-CHART-BLOCK") {
		t.Errorf("expected rendered history to contain the chart block verbatim, got:\n%s", out)
	}
}

// TestRenderHistory_NarrowWidth_HidesChartWithFallback (issue #37) —
// when the terminal is narrower than the chartWidth+4 threshold, the
// chart block is replaced with the "chart too narrow" hint instead of
// being rendered and overflowing the layout. The adaptive PanelWidth
// means a 60-col terminal clamps PanelWidth to ~56 and chartWidth+4 to
// ~56, so the test seeds both m.width and styles with a width below
// that floor to exercise the guard.
func TestRenderHistory_NarrowWidth_HidesChartWithFallback(t *testing.T) {
	m := &AgentModel{
		styles: NewStyles(ThemeDark, 50), // PanelWidth=46 → chartWidth+4=46 (the chart's min threshold)
		width:  40,                       // m.width < 46 → chart hidden
		session: &config.Session{
			History: []config.ChatTurn{
				{Role: "user", Content: "how has AAPL done?"},
				{Role: "assistant", Content: "Here's the trend.", Charts: []string{"FAKE-CHART-BLOCK"}},
			},
		},
	}
	out := m.renderHistory()
	if strings.Contains(out, "FAKE-CHART-BLOCK") {
		t.Errorf("expected chart to be hidden below the width threshold, got:\n%s", out)
	}
}

func TestRenderHistory_NoCharts_NoExtraBlock(t *testing.T) {
	m := &AgentModel{
		styles: NewStyles(ThemeDark, 0),
		width:  100,
		session: &config.Session{
			History: []config.ChatTurn{
				{Role: "user", Content: "hi"},
				{Role: "assistant", Content: "hello"},
			},
		},
	}
	out := m.renderHistory()
	if strings.Contains(out, "FAKE-CHART-BLOCK") {
		t.Errorf("did not expect a chart block when none is attached, got:\n%s", out)
	}
}
