// Package registry maintains the list of available market data providers.
// Each provider is described by an [Entry] that carries metadata and a factory
// function to construct a live driver instance.
package registry

import (
	"auris/pkg/drivers/fmp"
	"auris/pkg/providers"
)

// Entry describes a registered market data provider.
type Entry struct {
	// Key is the stable lowercase identifier stored in the config file (e.g. "fmp").
	Key string
	// DisplayName is the human-readable provider name shown in the TUI.
	DisplayName string
	// DocsURL is the URL where users can obtain an API key for this provider.
	DocsURL string
	// New constructs a fresh driver instance configured with the given API key.
	New func(apiKey string) providers.ProviderAPI
}

// All returns the ordered list of all registered provider entries.
// The order determines how they appear in the setup wizard.
func All() []Entry {
	return []Entry{
		newEntry("fmp", func(apiKey string) providers.ProviderAPI {
			return fmp.New(apiKey)
		}),
	}
}

// newEntry builds an Entry by calling the factory with an empty key to read
// the provider's metadata. fmp.New("") has no network side-effects.
func newEntry(key string, factory func(string) providers.ProviderAPI) Entry {
	sentinel := factory("")
	return Entry{
		Key:         key,
		DisplayName: sentinel.Name(),
		DocsURL:     sentinel.DocsURL(),
		New:         factory,
	}
}
