package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Case describes a single scripted scenario to run against a configured LLM
// through the real Auris agent tool set, backed by the deterministic
// simulation market driver.
type Case struct {
	// Name identifies the case in output. Required.
	Name string `json:"name"`
	// Prompt is the user message sent to the agent. Required.
	Prompt string `json:"prompt"`
	// SystemPromptFile, if set, overrides the suite-wide -system-prompt-file
	// for this case only. Empty means "use the run's default system prompt".
	SystemPromptFile string `json:"system_prompt_file,omitempty"`
	// ExpectedToolSequence lists tool names that must appear, in this order,
	// as an ordered (not necessarily contiguous) subsequence of the tools the
	// agent actually called.
	ExpectedToolSequence []string `json:"expected_tool_sequence,omitempty"`
	// ForbiddenPhrases must not appear (case-insensitively) in the final
	// answer — e.g. a trained-in denial like "I don't have real-time access".
	ForbiddenPhrases []string `json:"forbidden_phrases,omitempty"`
	// RequiredPhrases must all appear (case-insensitively) in the final answer.
	RequiredPhrases []string `json:"required_phrases,omitempty"`
}

// CaseSuite is the top-level shape of a -cases JSON file.
type CaseSuite struct {
	Cases []Case `json:"cases"`
}

// LoadCaseSuite reads and validates a test-case suite file.
func LoadCaseSuite(path string) (*CaseSuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var suite CaseSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(suite.Cases) == 0 {
		return nil, fmt.Errorf("%s: no cases defined", path)
	}
	for i, c := range suite.Cases {
		if c.Name == "" {
			return nil, fmt.Errorf("%s: cases[%d]: name is required", path, i)
		}
		if c.Prompt == "" {
			return nil, fmt.Errorf("%s: cases[%d] (%s): prompt is required", path, i, c.Name)
		}
	}
	return &suite, nil
}
