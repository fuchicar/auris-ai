package tui

import (
	"strings"
	"testing"

	"github.com/fuchicar/auris-ai/pkg/config"
)

func TestRenderHistory_SplicesChartBlock(t *testing.T) {
	m := &AgentModel{
		styles: NewStyles(ThemeDark),
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

func TestRenderHistory_NarrowWidth_HidesChartWithFallback(t *testing.T) {
	m := &AgentModel{
		styles: NewStyles(ThemeDark),
		width:  40, // narrower than chartWidth+4
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
		styles: NewStyles(ThemeDark),
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
