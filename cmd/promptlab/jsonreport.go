package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// JSONReport is the top-level shape written by -json-output: everything
// gathered during a run, structured so two runs (different prompt, different
// model) can be diffed with plain `diff`/`jq` instead of scraping stdout.
type JSONReport struct {
	GeneratedAt        time.Time          `json:"generated_at"`
	SystemPromptSource string             `json:"system_prompt_source"` // "default" or the -system-prompt-file path
	Targets            []JSONTargetResult `json:"targets"`
}

type JSONTargetResult struct {
	Name  string           `json:"name"`
	Cases []JSONCaseResult `json:"cases"`
}

type JSONCaseResult struct {
	Name             string         `json:"name"`
	Prompt           string         `json:"prompt"`
	FinalAnswer      string         `json:"final_answer"`
	ToolCalls        []JSONToolCall `json:"tool_calls"`
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
	ElapsedMS        int64          `json:"elapsed_ms"`
	Passed           bool           `json:"passed"`
	Failures         []string       `json:"failures,omitempty"`
	Warnings         []string       `json:"warnings,omitempty"`
	Error            string         `json:"error,omitempty"`
	Judge            *JudgeVerdict  `json:"judge,omitempty"`
}

type JSONToolCall struct {
	Name       string `json:"name"`
	Args       string `json:"args"`
	Result     string `json:"result"`
	DurationMS int64  `json:"duration_ms"`
}

// buildJSONCaseResult converts an already-run CaseResult into its JSON
// shape. CaseResult already holds everything needed — no extra state has to
// be threaded through runCase for this.
func buildJSONCaseResult(r CaseResult) JSONCaseResult {
	calls := make([]JSONToolCall, len(r.Trace))
	for i, e := range r.Trace {
		calls[i] = JSONToolCall{Name: e.Name, Args: e.Args, Result: e.Result, DurationMS: e.Duration.Milliseconds()}
	}
	errMsg := ""
	if r.RunErr != nil {
		errMsg = r.RunErr.Error()
	}
	return JSONCaseResult{
		Name:             r.Name,
		Prompt:           r.Prompt,
		FinalAnswer:      r.FinalContent,
		ToolCalls:        calls,
		PromptTokens:     r.Usage.PromptTokens,
		CompletionTokens: r.Usage.CompletionTokens,
		ElapsedMS:        r.Elapsed.Milliseconds(),
		Passed:           r.Passed(),
		Failures:         r.Failures,
		Warnings:         r.Warnings,
		Error:            errMsg,
		Judge:            r.Judge,
	}
}

// WriteJSONReport marshals report as indented JSON to path.
func WriteJSONReport(path string, report JSONReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json report: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write json report %s: %w", path, err)
	}
	return nil
}
