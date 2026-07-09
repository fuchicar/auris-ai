package market

// Tool name constants for agent tools that map 1:1 to a ProviderAPI method
// some drivers can never fulfil. Shared between pkg/agent (tool schemas) and
// pkg/drivers/* (capability declarations) so both sides can't drift apart
// through a typo — see FEAT-8 in TODO.md.
const (
	ToolGetOrderBook = "market_get_order_book"
	ToolGetTicks     = "market_get_ticks"
)

// CapabilityReporter is optionally implemented by a ProviderAPI to declare
// agent tool names it can never fulfil — the underlying method always
// returns ErrNotSupported regardless of symbol or account tier. A provider
// that doesn't implement this interface has unknown capabilities, so nothing
// is filtered on its account (the conservative default).
type CapabilityReporter interface {
	UnsupportedTools() []string
}
