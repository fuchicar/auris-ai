package agent

import (
	"strings"
	"testing"

	"github.com/fuchicar/auris-ai/pkg/config"
	"github.com/fuchicar/auris-ai/pkg/llm"
)

// ---- BuildSystemMessage ------------------------------------------------------

func TestBuildSystemMessage_UnknownTask(t *testing.T) {
	msg := BuildSystemMessage(llm.TaskType("unknown_task"), nil)
	if msg != nil {
		t.Errorf("expected nil for unknown task type, got %+v", msg)
	}
}

func TestBuildSystemMessage_NilProfile(t *testing.T) {
	msg := BuildSystemMessage(llm.TaskChat, nil)
	if msg == nil {
		t.Fatal("expected non-nil message for TaskChat")
	}
	if msg.Role != llm.RoleSystem {
		t.Errorf("Role: want %q, got %q", llm.RoleSystem, msg.Role)
	}
	if msg.Content == "" {
		t.Error("Content should not be empty")
	}
	if strings.Contains(msg.Content, "User financial profile") {
		t.Error("profile section should not appear when profile is nil")
	}
}

func TestBuildSystemMessage_EmptyProfile(t *testing.T) {
	// An all-zero FinancialProfile produces no formatted lines, so the
	// profile section should be omitted from the message.
	msg := BuildSystemMessage(llm.TaskChat, &config.FinancialProfile{})
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if strings.Contains(msg.Content, "User financial profile") {
		t.Error("profile section should not appear for an empty profile")
	}
}

func TestBuildSystemMessage_WithProfile(t *testing.T) {
	profile := &config.FinancialProfile{
		LifeStage:       "under35",
		IncomeStability: "stable",
		TimeHorizon:     "long",
		InvestmentGoals: []string{"growth", "income"},
	}
	msg := BuildSystemMessage(llm.TaskChat, profile)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if !strings.Contains(msg.Content, "User financial profile") {
		t.Error("profile section header missing")
	}
	if !strings.Contains(msg.Content, "under35") {
		t.Errorf("expected life stage in content, got:\n%s", msg.Content)
	}
	if !strings.Contains(msg.Content, "growth") || !strings.Contains(msg.Content, "income") {
		t.Errorf("expected investment goals in content, got:\n%s", msg.Content)
	}
}

func TestBuildSystemMessage_BaseContentPreserved(t *testing.T) {
	// The base system prompt must always be present regardless of profile.
	msg := BuildSystemMessage(llm.TaskChat, nil)
	if !strings.Contains(msg.Content, "Auris") {
		t.Error("base system prompt should mention Auris")
	}
}

// ---- formatProfile -----------------------------------------------------------

func TestFormatProfile_AllFields(t *testing.T) {
	p := &config.FinancialProfile{
		LifeStage:           "under35",
		IncomeStability:     "stable",
		EmergencyFund:       "yes",
		InvestmentGoals:     []string{"growth", "income"},
		TimeHorizon:         "long",
		LossScenario:        "hold",
		MaxAcceptableLoss:   "20%",
		FinancialExperience: []string{"stocks", "bonds"},
		InvestmentPriority:  "growth",
		Restrictions:        []string{"no_crypto"},
		RestrictionsCountry: "Spain",
	}
	result := formatProfile(p)

	required := []string{
		"under35", "stable", "yes",
		"growth", "income",
		"long", "hold", "20%",
		"stocks", "bonds",
		"no_crypto", "Spain",
	}
	for _, want := range required {
		if !strings.Contains(result, want) {
			t.Errorf("expected %q in formatted profile, got:\n%s", want, result)
		}
	}
}

func TestFormatProfile_EmptyProfile(t *testing.T) {
	result := formatProfile(&config.FinancialProfile{})
	if result != "" {
		t.Errorf("empty profile should produce empty string, got %q", result)
	}
}

func TestFormatProfile_PartialFields(t *testing.T) {
	// Only LifeStage and InvestmentGoals set — other fields must not appear.
	p := &config.FinancialProfile{
		LifeStage:       "over55",
		InvestmentGoals: []string{"preservation"},
	}
	result := formatProfile(p)
	if !strings.Contains(result, "over55") {
		t.Errorf("expected life stage in output, got %q", result)
	}
	if !strings.Contains(result, "preservation") {
		t.Errorf("expected investment goal in output, got %q", result)
	}
	// Fields not set should produce no output.
	if strings.Contains(result, "stable") || strings.Contains(result, "long") {
		t.Errorf("unset fields should not appear in output, got %q", result)
	}
}

// TestSystemPrompt_FetchNewsAlways verifies that the system prompt uses the same prescriptive
// "ALWAYS" pattern for fetch_news as it does for market data tools.
// Without this, models with "no internet access" training prior ignore the tool.
func TestSystemPrompt_FetchNewsAlways(t *testing.T) {
	msg := BuildSystemMessage(llm.TaskChat, nil)
	if msg == nil {
		t.Fatal("BuildSystemMessage returned nil")
	}
	if !strings.Contains(msg.Content, "fetch_news") {
		t.Error("system prompt must explicitly mention fetch_news")
	}
	fetchNewsIdx := strings.Index(msg.Content, "fetch_news")
	start := fetchNewsIdx - 300
	if start < 0 {
		start = 0
	}
	end := fetchNewsIdx + 300
	if end > len(msg.Content) {
		end = len(msg.Content)
	}
	ctx := msg.Content[start:end]
	if !strings.Contains(ctx, "ALWAYS") {
		t.Error("system prompt must include ALWAYS directive near fetch_news (same pattern as market data)")
	}
}

// TestSystemPrompt_FetchNewsCountersPrior verifies that the system prompt explicitly
// counters the LLM training prior "I don't have access to real-time news".
// Without an explicit counter, models trained to say "I can't access the internet" will
// ignore fetch_news even when it's available as a tool.
func TestSystemPrompt_FetchNewsCountersPrior(t *testing.T) {
	msg := BuildSystemMessage(llm.TaskChat, nil)
	if msg == nil {
		t.Fatal("BuildSystemMessage returned nil")
	}
	lower := strings.ToLower(msg.Content)
	hasCounter := strings.Contains(lower, "never tell") || strings.Contains(lower, "real-time")
	if !hasCounter {
		t.Error("system prompt must counter the 'I don't have access to real-time news' training prior")
	}
}

func TestFormatProfile_RestrictionsCountryOnlyWhenSet(t *testing.T) {
	// RestrictionsCountry has an explicit conditional — test both branches.
	withCountry := &config.FinancialProfile{RestrictionsCountry: "Germany"}
	if !strings.Contains(formatProfile(withCountry), "Germany") {
		t.Error("expected RestrictionsCountry in output when set")
	}

	withoutCountry := &config.FinancialProfile{LifeStage: "under35"}
	if strings.Contains(formatProfile(withoutCountry), "Restrictions country") {
		t.Error("RestrictionsCountry label should not appear when field is empty")
	}
}
