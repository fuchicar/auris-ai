package tui

import (
	"strings"
	"testing"

	"github.com/fuchicar/auris-ai/pkg/locale"
)

// Regression tests for issue #35: windowedRows must always keep the cursor
// inside the visible window, and must surface "↑ more above" / "↓ more below"
// indicators when items are clipped.

func init() {
	// The scroll-up/down indicators are locale strings; tests need them
	// resolved to the English source instead of the raw key, so the assertions
	// below can match on human-readable text.
	_ = locale.Init("en")
}

func TestWindowedRows_EmptyTotalReturnsNil(t *testing.T) {
	got := windowedRows(WindowedRowOpts{Total: 0, RenderRow: func(i int) string { return "x" }})
	if got != nil {
		t.Fatalf("want nil for empty list, got %#v", got)
	}
}

func TestWindowedRows_HeightZeroUsesFallbackBudget(t *testing.T) {
	render := func(i int) string { return "r" }
	got := windowedRows(WindowedRowOpts{Total: 100, ScrollOff: 0, Cursor: 0, RenderRow: render})
	// Fallback is min(total, 20) → 20 visible rows + 1 down indicator (more below).
	if len(got) != 21 {
		t.Fatalf("height=0 fallback should render 20 rows + 1 down indicator = 21, got %d", len(got))
	}
	for i, line := range got {
		if i == len(got)-1 {
			if !strings.Contains(line, "more below") {
				t.Fatalf("last line should be down indicator, got %q", line)
			}
			continue
		}
		if line != "r" {
			t.Fatalf("row %d content %q, expected %q", i, line, "r")
		}
	}
}

func TestWindowedRows_AllRowsFit_NoIndicators(t *testing.T) {
	render := func(i int) string { return strings.Repeat("x", i+1) }
	got := windowedRows(WindowedRowOpts{
		Height:      50,
		ChromeAbove: 0,
		ChromeBelow: 0,
		Total:       5,
		ScrollOff:   0,
		Cursor:      0,
		RenderRow:   render,
	})
	if len(got) != 5 {
		t.Fatalf("want 5 rows, got %d", len(got))
	}
	for i, line := range got {
		want := strings.Repeat("x", i+1)
		if line != want {
			t.Fatalf("row %d = %q, want %q", i, line, want)
		}
	}
}

func TestWindowedRows_OverflowShowsDownIndicator(t *testing.T) {
	render := func(i int) string { return "r" }
	// height 10, chrome 2 above + 2 below → 6 visible.
	got := windowedRows(WindowedRowOpts{
		Height:      10,
		ChromeAbove: 2,
		ChromeBelow: 2,
		Total:       50,
		ScrollOff:   0,
		Cursor:      0,
		RenderRow:   render,
	})
	// Expect 1 down indicator + 6 items + nothing above = 7.
	if len(got) != 7 {
		t.Fatalf("want 7 lines, got %d (%v)", len(got), got)
	}
	if !strings.Contains(got[len(got)-1], "more below") {
		t.Fatalf("last line should be ↓ more below indicator, got %q", got[len(got)-1])
	}
}

func TestWindowedRows_OverflowShowsUpAndDownIndicators(t *testing.T) {
	render := func(i int) string { return "r" }
	// Cursor parked far into the middle so both ends are clipped.
	got := windowedRows(WindowedRowOpts{
		Height:      10,
		ChromeAbove: 2,
		ChromeBelow: 2,
		Total:       50,
		ScrollOff:   10,
		Cursor:      25,
		RenderRow:   render,
	})
	if len(got) < 3 {
		t.Fatalf("expected several lines, got %v", got)
	}
	if !strings.Contains(got[0], "more above") {
		t.Fatalf("first line should be ↑ more above indicator, got %q", got[0])
	}
	if !strings.Contains(got[len(got)-1], "more below") {
		t.Fatalf("last line should be ↓ more below indicator, got %q", got[len(got)-1])
	}
}

// Walking the cursor from 0 to total-1 must always keep it in the visible
// window — otherwise the cursor "disappears" above/below the viewport.
func TestWindowedRows_CursorAlwaysVisible(t *testing.T) {
	const total = 100
	render := func(i int) string { return "r" }

	for height := 6; height <= 40; height += 2 {
		for cursor := 0; cursor < total; cursor++ {
			opts := WindowedRowOpts{
				Height:      height,
				ChromeAbove: 2,
				ChromeBelow: 2,
				Total:       total,
				ScrollOff:   cursor, // simulate a stale scrollOff from a previous state
				Cursor:      cursor,
				RenderRow:   render,
			}
			got := windowedRows(opts)

			// Figure out which lines are real rows vs indicators. Indicators
			// contain "more above" / "more below"; row entries are bare "r".
			firstReal := 0
			if strings.Contains(got[0], "more above") {
				firstReal = 1
			}
			lastReal := len(got) - 1
			if strings.Contains(got[lastReal], "more below") {
				lastReal--
			}

			visibleCount := lastReal - firstReal + 1
			visibleStart := opts.ScrollOff
			visibleEnd := visibleStart + visibleCount

			if cursor < visibleStart || cursor >= visibleEnd {
				t.Fatalf("height=%d cursor=%d: cursor outside visible [%d,%d)", height, cursor, visibleStart, visibleEnd)
			}
		}
	}
}

func TestWindowedRows_HintRenderApplied(t *testing.T) {
	render := func(i int) string { return "r" }
	hintRender := func(text string) string { return "<" + text + ">" }

	got := windowedRows(WindowedRowOpts{
		Height:      10,
		ChromeAbove: 2,
		ChromeBelow: 2,
		Total:       100,
		ScrollOff:   50,
		Cursor:      55,
		RenderRow:   render,
		HintRender:  hintRender,
	})

	if !strings.HasPrefix(got[0], "<") || !strings.HasSuffix(got[0], ">") {
		t.Fatalf("first line should be wrapped in <> via hintRender, got %q", got[0])
	}
	if !strings.HasPrefix(got[len(got)-1], "<") || !strings.HasSuffix(got[len(got)-1], ">") {
		t.Fatalf("last line should be wrapped in <> via hintRender, got %q", got[len(got)-1])
	}
}

// TinyHeight_ClampsToZero pins the post-fix contract: when chrome + indicator
// reserves exceed the terminal height, the helper returns at most the
// indicators themselves — never padding rows that would push the surrounding
// chrome off-screen. The earlier "floor at 3" choice was a regression risk
// flagged by the issue #35 review.
func TestWindowedRows_TinyHeight_ClampsToZero(t *testing.T) {
	render := func(i int) string { return "r" }
	got := windowedRows(WindowedRowOpts{
		Height:           4, // smaller than chrome (2 + 2 + 2 indicator reserve)
		ChromeAbove:      2,
		ChromeBelow:      2,
		IndicatorReserve: 2,
		Total:            100,
		ScrollOff:        0,
		Cursor:           0,
		RenderRow:        render,
	})
	// Both clampScrollOff and IndicatorReserve interact here: scrollOff
	// becomes 1 after clamping, but with vis=0 the loop runs zero times,
	// so only the down indicator is added (no up indicator — vis=0 means
	// the up indicator isn't meaningful since no rows were clipped off).
	if len(got) != 1 {
		t.Fatalf("tiny height with 2+2+2 chrome/indicators should render only the down indicator, got %d (%v)", len(got), got)
	}
	if !strings.Contains(got[0], "more below") {
		t.Fatalf("expected down indicator, got %q", got[0])
	}
}

// ZeroBudget_ReturnsEmptySlice_NoIndicators covers the corner case where
// the available height exactly equals chrome + indicator reserves (visible
// count = 0). With vis=0, clampScrollOff keeps scrollOff unchanged, so the
// up indicator isn't spuriously added; only the down indicator remains,
// signalling that content exists below even though nothing fits.
func TestWindowedRows_ZeroBudget(t *testing.T) {
	render := func(i int) string { return "r" }
	got := windowedRows(WindowedRowOpts{
		Height:           6, // 2 chrome + 2 chrome + 2 indicator = 6, leaving 0 for rows
		ChromeAbove:      2,
		ChromeBelow:      2,
		IndicatorReserve: 2,
		Total:            100,
		ScrollOff:        0,
		Cursor:           0,
		RenderRow:        render,
	})
	if len(got) != 1 {
		t.Fatalf("zero budget should render only the down indicator, got %d (%v)", len(got), got)
	}
	if !strings.Contains(got[0], "more below") {
		t.Fatalf("expected down indicator, got %q", got[0])
	}
}

func TestWindowedRows_StaleScrollOffPulledBackToFitCursor(t *testing.T) {
	render := func(i int) string { return "r" }
	// scrollOff points way past the cursor — would normally hide the cursor
	// above the viewport. The helper must clamp scrollOff back so cursor is
	// visible. After that, scrollOff lands at 3, which is > 0, so a single
	// "↑ more above" indicator is shown (the items 0..2 above the cursor).
	got := windowedRows(WindowedRowOpts{
		Height:      12,
		ChromeAbove: 2,
		ChromeBelow: 2,
		Total:       100,
		ScrollOff:   50,
		Cursor:      3,
		RenderRow:   render,
	})
	if len(got) < 1 {
		t.Fatalf("expected rows, got %v", got)
	}
	if !strings.Contains(got[0], "more above") {
		t.Fatalf("cursor=3 with stale scrollOff=50 should pull back and surface an up indicator, got %v", got)
	}
	// Cursor must be inside the visible window (not on the up-indicator line).
	firstReal := 1 // up-indicator is index 0
	lastReal := len(got) - 1
	if strings.Contains(got[lastReal], "more below") {
		lastReal--
	}
	if 3 < firstReal || 3 > lastReal {
		t.Fatalf("cursor=3 not in visible window [%d,%d] after pull-back, got %v", firstReal, lastReal, got)
	}
}

func TestClampScrollOff_CursorAtTop(t *testing.T) {
	if got := clampScrollOff(0, 5, 4); got != 0 {
		t.Fatalf("cursor=0 should pull scrollOff back to 0, got %d", got)
	}
}

func TestClampScrollOff_CursorPastWindow(t *testing.T) {
	if got := clampScrollOff(10, 0, 4); got != 7 {
		t.Fatalf("cursor=10 with maxVis=4 should slide scrollOff to 7, got %d", got)
	}
}

func TestClampScrollOff_CursorInsideWindow_Unchanged(t *testing.T) {
	if got := clampScrollOff(3, 1, 4); got != 1 {
		t.Fatalf("cursor inside window should not move scrollOff, got %d", got)
	}
}
