// Package registry maintains the list of available market data and AI providers.
// Each provider is described by an Entry that carries metadata and a factory
// function to construct a live driver instance.
package registry

import (
	"auris/pkg/drivers/fmp"
	"auris/pkg/market"
)

// MarketEntry describes a registered market data provider.
type MarketEntry struct {
	// Key is the stable lowercase identifier stored in the config file (e.g. "fmp").
	Key string
	// DisplayName is the human-readable provider name shown in the TUI.
	DisplayName string
	// DocsURL is the URL where users can obtain an API key for this provider.
	DocsURL string
	// New constructs a fresh driver instance configured with the given API key.
	New func(apiKey string) market.ProviderAPI
}

// AllMarket returns the ordered list of all registered market data provider entries.
// The order determines how they appear in the setup wizard.
func AllMarket() []MarketEntry {
	return []MarketEntry{
		newMarketEntry("fmp", func(apiKey string) market.ProviderAPI {
			return fmp.New(apiKey)
		}),
	}
}

// newMarketEntry builds a MarketEntry by calling the factory with an empty key to read
// the provider's metadata. fmp.New("") has no network side-effects.
func newMarketEntry(key string, factory func(string) market.ProviderAPI) MarketEntry {
	sentinel := factory("")
	return MarketEntry{
		Key:         key,
		DisplayName: sentinel.Name(),
		DocsURL:     sentinel.DocsURL(),
		New:         factory,
	}
}
