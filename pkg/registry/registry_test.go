package registry_test

import (
	"testing"

	"auris/pkg/registry"
)

func TestAll_NonEmpty(t *testing.T) {
	entries := registry.All()
	if len(entries) == 0 {
		t.Fatal("All() returned no entries; at least one provider (fmp) expected")
	}
}

func TestAll_Metadata(t *testing.T) {
	for _, e := range registry.All() {
		if e.Key == "" {
			t.Errorf("entry with empty Key")
		}
		if e.DisplayName == "" {
			t.Errorf("entry %q has empty DisplayName", e.Key)
		}
		if e.DocsURL == "" {
			t.Errorf("entry %q has empty DocsURL", e.Key)
		}
		if e.New == nil {
			t.Errorf("entry %q has nil New factory", e.Key)
		}
	}
}

func TestAll_FMPEntry(t *testing.T) {
	var found bool
	for _, e := range registry.All() {
		if e.Key == "fmp" {
			found = true
			if e.DisplayName != "Financial Modeling Prep" {
				t.Errorf("fmp DisplayName = %q, want %q", e.DisplayName, "Financial Modeling Prep")
			}
			if e.DocsURL == "" {
				t.Error("fmp DocsURL is empty")
			}
		}
	}
	if !found {
		t.Error("fmp entry not found in registry")
	}
}
