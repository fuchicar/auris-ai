package registry_test

import (
	"testing"

	"auris/pkg/registry"
)

func TestAllMarket_NonEmpty(t *testing.T) {
	entries := registry.AllMarket()
	if len(entries) == 0 {
		t.Fatal("AllMarket() returned no entries; at least one provider (fmp) expected")
	}
}

func TestAllMarket_Metadata(t *testing.T) {
	for _, e := range registry.AllMarket() {
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

func TestAllMarket_FMPEntry(t *testing.T) {
	var found bool
	for _, e := range registry.AllMarket() {
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

func TestAllLLM_NonEmpty(t *testing.T) {
	entries := registry.AllLLM()
	if len(entries) == 0 {
		t.Fatal("AllLLM() returned no entries; at least ollama and gemini expected")
	}
}

func TestAllLLM_Metadata(t *testing.T) {
	for _, e := range registry.AllLLM() {
		if e.Key == "" {
			t.Errorf("LLM entry with empty Key")
		}
		if e.DisplayName == "" {
			t.Errorf("LLM entry %q has empty DisplayName", e.Key)
		}
		if e.New == nil {
			t.Errorf("LLM entry %q has nil New factory", e.Key)
		}
	}
}
