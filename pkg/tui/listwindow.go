package tui

import (
	"github.com/fuchicar/auris-ai/pkg/locale"
)

// clampScrollOff adjusts scrollOff so that cursor remains in the visible
// window [scrollOff, scrollOff + maxVis). Caller is responsible for bounding
// scrollOff to total — this helper does no clamping of either input.
//
// When maxVis is 0 (the terminal is too cramped to fit any rows) the helper
// leaves scrollOff unchanged so neither up + down indicators get spuriously
// rendered behind "no rows" content.
func clampScrollOff(cursor, scrollOff, maxVis int) int {
	if maxVis <= 0 {
		return scrollOff
	}
	if cursor < scrollOff {
		return cursor
	}
	if cursor >= scrollOff+maxVis {
		return cursor - maxVis + 1
	}
	return scrollOff
}

// WindowedRowOpts configures one call to windowedRows.
//
// Height is the available terminal height (in rows) for the whole screen. If
// 0, the helper falls back to a 20-row default — the same value most screens
// hardcoded as their maxVisible() fallback before this existed.
//
// ChromeAbove and ChromeBelow together describe how many terminal rows the
// screen spends outside the list itself (title, header, hint, blank lines).
// Each screen knows its own chrome layout, so it passes those numbers in —
// the helper is purely mechanical.
//
// IndicatorReserve is the worst-case number of indicator lines the helper
// might add to the slice (0..2: "↑ more above", "↓ more below", or both).
// Reserving space keeps the slice bounded by Height even when both ends of
// the list are clipped — e.g. when the cursor is in the middle of a long
// list. Set to 2 for a typical list, to 1 if the list is bounded such that
// only one indicator is ever possible (rare), and to 0 only when the screen
// guarantees the list fits. Note visibleCount() floors at 0, not a minimum
// row count — when chrome + this reserve already exceeds Height, the row
// list shrinks to empty rather than forcing rows that would themselves push
// the surrounding chrome off screen.
//
// RenderRow is called for every visible item index. It must be cheap and
// deterministic for a given i.
//
// HintRender is applied to the "↑ more above" / "↓ more below" markers so
// callers can style them with their own lipgloss.Style. Passing nil falls
// back to an identity function (no styling).
type WindowedRowOpts struct {
	Height           int
	ScrollOff        int
	Cursor           int
	Total            int
	ChromeAbove      int
	ChromeBelow      int
	IndicatorReserve int
	RenderRow        func(i int) string
	HintRender       func(text string) string
}

// visibleCount returns the number of rows the visible window can hold, given
// height and chrome budgets. Returns max(0, …) so the row slice itself
// never adds to chrome overflow — a cramped terminal will end up with an
// empty list (chrome already overflows), which is more honest than padding
// the slice to a fixed minimum that would push the surrounding chrome off
// screen. Falls back to min(Total, 20) when height is 0 (the pre-helper
// behavior most screens had).
func (o WindowedRowOpts) visibleCount() int {
	if o.Height == 0 {
		if o.Total < 20 {
			return o.Total
		}
		return 20
	}
	n := o.Height - o.ChromeAbove - o.ChromeBelow - o.IndicatorReserve
	if n < 0 {
		return 0
	}
	if n > o.Total {
		return o.Total
	}
	return n
}

// windowedRows clips a list of rows to fit the terminal height. It returns the
// rendered visible window including optional "↑ more above" / "↓ more below"
// markers at the top and/or bottom when items are clipped off the screen.
// The caller owns chrome rendering (title, header, hint); this helper only
// produces the list slice.
//
// Invariant: whenever total > 0 and at least one row fits the budget
// (visibleCount() > 0), cursor is always inside the returned [first..last)
// window so the cursor never sits on an invisible row. When the budget is 0
// — chrome alone already exceeds Height — no row can satisfy that, and the
// returned slice holds only whichever scroll indicator applies.
//
// hintRender may be nil — the helper falls back to an identity function in
// that case so callers don't have to plumb a no-op renderer.
func windowedRows(opts WindowedRowOpts) []string {
	if opts.Total == 0 {
		return nil
	}
	if opts.HintRender == nil {
		opts.HintRender = func(s string) string { return s }
	}
	vis := opts.visibleCount()

	// Clamp scrollOff into [0, total).
	if opts.ScrollOff < 0 {
		opts.ScrollOff = 0
	}
	if opts.ScrollOff > opts.Total-1 {
		opts.ScrollOff = opts.Total - 1
	}
	// Make sure the cursor is reachable inside the window — without this a
	// stale scrollOff (e.g. after deleting items) could place the cursor
	// outside the visible window.
	opts.ScrollOff = clampScrollOff(opts.Cursor, opts.ScrollOff, vis)
	if opts.ScrollOff < 0 {
		opts.ScrollOff = 0
	}
	if opts.ScrollOff > opts.Total-1 {
		opts.ScrollOff = opts.Total - 1
	}

	end := opts.ScrollOff + vis
	if end > opts.Total {
		end = opts.Total
	}

	rows := make([]string, 0, vis+2)
	if opts.ScrollOff > 0 {
		rows = append(rows, opts.HintRender(locale.T("setup.ai.model.scroll_up")))
	}
	for i := opts.ScrollOff; i < end; i++ {
		rows = append(rows, opts.RenderRow(i))
	}
	if end < opts.Total {
		rows = append(rows, opts.HintRender(locale.T("setup.ai.model.scroll_down")))
	}
	return rows
}
