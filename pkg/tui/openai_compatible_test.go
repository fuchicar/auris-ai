package tui

import (
	"testing"

	"github.com/fuchicar/auris-ai/pkg/config"
)

// --- findLLMEntry resolves dynamic per-instance keys (issue #3) ---

func TestFindLLMEntry_StaticKey(t *testing.T) {
	e, ok := findLLMEntry("ollama")
	if !ok || e.Key != "ollama" || e.DisplayName != "Ollama" {
		t.Fatalf("findLLMEntry(\"ollama\") = %+v, %v", e, ok)
	}
}

func TestFindLLMEntry_DynamicInstanceKey(t *testing.T) {
	e, ok := findLLMEntry("openai_compatible:deepseek")
	if !ok {
		t.Fatal("expected findLLMEntry to resolve a dynamic openai_compatible instance key")
	}
	if e.Key != "openai_compatible:deepseek" {
		t.Errorf("Key = %q, want the full instance key preserved", e.Key)
	}
	if e.DisplayName != "OpenAI-Compatible" {
		t.Errorf("DisplayName = %q, want the base registry entry's display name", e.DisplayName)
	}
	if e.New == nil {
		t.Error("New must be the base driver's factory, not nil")
	}
}

func TestFindLLMEntry_UnknownKey(t *testing.T) {
	if _, ok := findLLMEntry("does_not_exist"); ok {
		t.Fatal("expected ok=false for a completely unknown key")
	}
	if _, ok := findLLMEntry("does_not_exist:either"); ok {
		t.Fatal("expected ok=false when even the prefix doesn't match any registry key")
	}
}

// --- llmEntryFor overrides DisplayName with the user-chosen instance name ---

func TestLlmEntryFor_OverridesDisplayNameWhenNamed(t *testing.T) {
	cfg := &config.AurisConfig{
		AIProviders: map[string]*config.AIProviderConfig{
			"openai_compatible:deepseek": {Name: "DeepSeek"},
		},
	}
	e, ok := llmEntryFor(cfg, "openai_compatible:deepseek")
	if !ok {
		t.Fatal("expected llmEntryFor to resolve the key")
	}
	if e.DisplayName != "DeepSeek" {
		t.Errorf("DisplayName = %q, want %q", e.DisplayName, "DeepSeek")
	}
}

func TestLlmEntryFor_NoOverrideForSingletonProviders(t *testing.T) {
	cfg := &config.AurisConfig{
		AIProviders: map[string]*config.AIProviderConfig{
			"ollama": {BaseURL: "http://localhost:11434"},
		},
	}
	e, ok := llmEntryFor(cfg, "ollama")
	if !ok || e.DisplayName != "Ollama" {
		t.Fatalf("llmEntryFor(\"ollama\") = %+v, %v, want unchanged DisplayName", e, ok)
	}
}

// --- dynamicLLMEntries lists only non-static, sorted instance keys ---

func TestDynamicLLMEntries_ListsOnlyInstanceKeysSorted(t *testing.T) {
	cfg := &config.AurisConfig{
		AIProviders: map[string]*config.AIProviderConfig{
			"ollama":                     {},
			"openai_compatible:groq":     {Name: "Groq"},
			"openai_compatible:deepseek": {Name: "DeepSeek"},
		},
	}
	entries := dynamicLLMEntries(cfg)
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2 (ollama is static and must be excluded)", len(entries))
	}
	if entries[0].Key != "openai_compatible:deepseek" || entries[0].DisplayName != "DeepSeek" {
		t.Errorf("entries[0] = %+v", entries[0])
	}
	if entries[1].Key != "openai_compatible:groq" || entries[1].DisplayName != "Groq" {
		t.Errorf("entries[1] = %+v", entries[1])
	}
}

func TestDynamicLLMEntries_EmptyWhenNoneConfigured(t *testing.T) {
	cfg := &config.AurisConfig{AIProviders: map[string]*config.AIProviderConfig{"anthropic": {}}}
	if entries := dynamicLLMEntries(cfg); len(entries) != 0 {
		t.Errorf("entries = %+v, want empty", entries)
	}
}

// --- newOpenAICompatibleKey: unique key generation, collision handling ---

func TestNewOpenAICompatibleKey_Slugifies(t *testing.T) {
	key := newOpenAICompatibleKey(nil, "DeepSeek")
	if key != "openai_compatible:deepseek" {
		t.Errorf("key = %q, want %q", key, "openai_compatible:deepseek")
	}
}

func TestNewOpenAICompatibleKey_CollisionGetsSuffix(t *testing.T) {
	existing := map[string]*config.AIProviderConfig{
		"openai_compatible:deepseek": {Name: "DeepSeek"},
	}
	key := newOpenAICompatibleKey(existing, "DeepSeek")
	if key != "openai_compatible:deepseek-2" {
		t.Errorf("key = %q, want %q", key, "openai_compatible:deepseek-2")
	}
}

func TestNewOpenAICompatibleKey_EmptyNameFallsBackToInstance(t *testing.T) {
	key := newOpenAICompatibleKey(nil, "!!!")
	if key != "openai_compatible:instance" {
		t.Errorf("key = %q, want %q", key, "openai_compatible:instance")
	}
}
