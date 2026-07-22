// Command promptlab drives the real Auris agent (LLM + full tool set) against
// the deterministic simulation market driver, to test and refine system
// prompts against locally running LLMs without touching Auris's encrypted
// config, a real market-data API key, or the TUI.
//
// Usage:
//
//	promptlab (-provider <key> -model <model> [-base-url <url>] [-api-key <key>] | -targets <file.json>)
//	          (-prompt <text> | -cases <file.json>)
//	          [-system-prompt-file <path>] [-timeout <duration>] [-json-output <path>]
//	          [-judge-provider <key> -judge-model <model> [-judge-base-url <url>] [-judge-api-key <key>]]
//
// Exactly one of -prompt (a single ad-hoc instruction) or -cases (a JSON
// suite of scripted scenarios with pass/fail assertions, see cases.go) must
// be given. Exactly one of (-provider + -model) or -targets (a JSON list of
// several LLMs to run the same prompt/suite against, see targets.go) must be
// given.
//
// Tips:
//
//   - Context window first. Auris's real prompt plus all ~58 tool schemas is
//     several thousand tokens before the conversation even starts. If a run
//     fails with a context-window error, promptlab prints a [HINT] pointing
//     at how to raise it — do that before judging whether a candidate prompt
//     is any good, or every model will look like it's failing.
//   - Local CPU inference is slow. A single case with 2-3 tool round trips
//     can take several minutes on an unaccelerated local model; -timeout
//     defaults to 5 minutes per case for that reason.
//
// Flags:
//
//	-provider           Registry key: ollama, openai_compatible, gemini, anthropic, minimax, openai.
//	-base-url           Base URL for the provider (e.g. http://localhost:11434 for Ollama,
//	                     http://localhost:1234/v1 for LM Studio via openai_compatible).
//	-api-key            API key, usually empty for local servers.
//	-model              Model ID to request.
//	-targets            Path to a JSON file listing several {name, provider, base_url, api_key, model}
//	                     targets to run the same prompt/suite against, printing a comparison matrix.
//	-system-prompt-file Path to a candidate system prompt (plain text). Omit to use
//	                     Auris's real default prompt (agent.BuildSystemMessage).
//	-prompt             Ad-hoc single instruction to test.
//	-cases              Path to a JSON test-case suite.
//	-timeout            Context timeout per case (default 5m).
//	-json-output        Path to write a structured JSON report, for diffing between runs.
//	-judge-provider     Registry key for an optional LLM-as-judge pass (advisory only,
//	-judge-model         never affects pass/fail). Both flags are required together.
//	-judge-base-url     Base URL / API key for the judge, same conventions as -base-url/-api-key.
//	-judge-api-key
//
// Exit status is 1 if any case (in any target) fails its assertions or errors
// out, 0 otherwise.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/fuchicar/auris-ai/pkg/agent"
	"github.com/fuchicar/auris-ai/pkg/drivers/simulation"
	"github.com/fuchicar/auris-ai/pkg/llm"
	"github.com/fuchicar/auris-ai/pkg/market"
	"github.com/fuchicar/auris-ai/pkg/registry"
)

func main() {
	os.Exit(run())
}

func run() int {
	provider := flag.String("provider", "", "LLM registry key (ollama, openai_compatible, gemini, anthropic, minimax, openai)")
	baseURL := flag.String("base-url", "", "Provider base URL (e.g. http://localhost:11434)")
	apiKey := flag.String("api-key", "", "Provider API key (usually empty for local servers)")
	model := flag.String("model", "", "Model ID to request")
	targetsPath := flag.String("targets", "", "Path to a JSON file of multiple targets to run and compare (alternative to -provider/-model)")
	systemPromptFile := flag.String("system-prompt-file", "", "Path to a candidate system prompt file (default: Auris's real prompt)")
	prompt := flag.String("prompt", "", "Ad-hoc single instruction to test")
	casesPath := flag.String("cases", "", "Path to a JSON test-case suite")
	// 5 minutes: observed local CPU inference needing just over 3 minutes for
	// a single 3-tool-call case.
	timeout := flag.Duration("timeout", 5*time.Minute, "Context timeout per case")
	jsonOutput := flag.String("json-output", "", "Path to write a structured JSON report (for diffing between runs)")
	judgeProvider := flag.String("judge-provider", "", "LLM registry key for an optional advisory LLM-as-judge pass")
	judgeBaseURL := flag.String("judge-base-url", "", "Judge provider base URL")
	judgeAPIKey := flag.String("judge-api-key", "", "Judge provider API key")
	judgeModel := flag.String("judge-model", "", "Judge model ID (required together with -judge-provider)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: promptlab (-provider <key> -model <model> [-base-url <url>] [-api-key <key>] | -targets <file.json>)
                 (-prompt <text> | -cases <file.json>)
                 [-system-prompt-file <path>] [-timeout <duration>] [-json-output <path>]
                 [-judge-provider <key> -judge-model <model> [-judge-base-url <url>] [-judge-api-key <key>]]

Flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	hasSingleTarget := *provider != "" || *model != ""
	hasTargets := *targetsPath != ""
	switch {
	case hasSingleTarget && hasTargets:
		fmt.Fprintln(os.Stderr, "promptlab: cannot combine -targets with -provider/-model")
		flag.Usage()
		return 2
	case !hasSingleTarget && !hasTargets:
		fmt.Fprintln(os.Stderr, "promptlab: specify either -provider and -model, or -targets")
		flag.Usage()
		return 2
	case hasSingleTarget && (*provider == "" || *model == ""):
		fmt.Fprintln(os.Stderr, "promptlab: -provider and -model must be given together")
		flag.Usage()
		return 2
	}
	if (*prompt == "") == (*casesPath == "") {
		fmt.Fprintln(os.Stderr, "promptlab: exactly one of -prompt or -cases is required")
		flag.Usage()
		return 2
	}
	hasJudge := *judgeProvider != "" || *judgeModel != ""
	if hasJudge && (*judgeProvider == "" || *judgeModel == "") {
		fmt.Fprintln(os.Stderr, "promptlab: -judge-provider and -judge-model must be given together")
		flag.Usage()
		return 2
	}

	var targets []Target
	if hasTargets {
		loaded, err := LoadTargets(*targetsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "promptlab: %v\n", err)
			return 2
		}
		targets = loaded
	} else {
		targets = []Target{{Name: *provider + ":" + *model, Provider: *provider, BaseURL: *baseURL, APIKey: *apiKey, Model: *model}}
	}

	var cases []Case
	if *prompt != "" {
		cases = []Case{{Name: "adhoc", Prompt: *prompt}}
	} else {
		suite, err := LoadCaseSuite(*casesPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "promptlab: %v\n", err)
			return 2
		}
		cases = suite.Cases
	}

	ctx := context.Background()

	simDriver := simulation.New()
	if err := simDriver.Connect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "promptlab: connecting simulation driver: %v\n", err)
		return 1
	}

	systemPromptSource := "default"
	if *systemPromptFile != "" {
		systemPromptSource = *systemPromptFile
	}
	defaultSystemContent, err := loadSystemPrompt(*systemPromptFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "promptlab: %v\n", err)
		return 2
	}

	var judgeLLM llm.AIProvider
	if hasJudge {
		entry := findLLMEntry(*judgeProvider)
		if entry == nil {
			fmt.Fprintf(os.Stderr, "promptlab: unknown -judge-provider %q (see registry.AllLLM)\n", *judgeProvider)
			return 2
		}
		judgeLLM = entry.New(*judgeBaseURL, *judgeAPIKey)
		if err := judgeLLM.Connect(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "promptlab: connecting judge %s: %v\n", entry.DisplayName, err)
			return 1
		}
	}

	targetNames := make([]string, 0, len(targets))
	resultsByTarget := make(map[string][]CaseResult, len(targets))
	anyFailed := false

	for _, t := range targets {
		entry := findLLMEntry(t.Provider)
		if entry == nil {
			fmt.Fprintf(os.Stderr, "promptlab: unknown provider %q for target %q\n", t.Provider, t.Name)
			return 2
		}
		llmProvider := entry.New(t.BaseURL, t.APIKey)
		if err := llmProvider.Connect(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "promptlab: connecting to %s (target %s): %v\n", entry.DisplayName, t.Name, err)
			return 1
		}

		var results []CaseResult
		for _, c := range cases {
			systemContent := defaultSystemContent
			if c.SystemPromptFile != "" {
				content, err := loadSystemPrompt(c.SystemPromptFile)
				if err != nil {
					fmt.Fprintf(os.Stderr, "promptlab: case %s: %v\n", c.Name, err)
					return 2
				}
				systemContent = content
			}

			result := runCase(ctx, llmProvider, simDriver, t.Model, systemContent, c.Name, c.Prompt, *timeout)
			result.Failures, result.Warnings = evaluateCase(c, result.Trace, result.FinalContent, result.RunErr)
			if judgeLLM != nil {
				verdict := RunJudge(ctx, judgeLLM, *judgeModel, c.Prompt, result.Trace, result.FinalContent)
				result.Judge = &verdict
			}
			if !result.Passed() {
				anyFailed = true
			}
			results = append(results, result)
			PrintCaseReport(os.Stdout, result)
		}
		if len(cases) > 1 {
			PrintSummary(os.Stdout, results)
		}
		targetNames = append(targetNames, t.Name)
		resultsByTarget[t.Name] = results
	}

	if len(targetNames) > 1 {
		PrintMatrix(os.Stdout, targetNames, resultsByTarget)
	}

	if *jsonOutput != "" {
		report := JSONReport{GeneratedAt: time.Now(), SystemPromptSource: systemPromptSource}
		for _, name := range targetNames {
			jsonCases := make([]JSONCaseResult, 0, len(resultsByTarget[name]))
			for _, r := range resultsByTarget[name] {
				jsonCases = append(jsonCases, buildJSONCaseResult(r))
			}
			report.Targets = append(report.Targets, JSONTargetResult{Name: name, Cases: jsonCases})
		}
		if err := WriteJSONReport(*jsonOutput, report); err != nil {
			fmt.Fprintf(os.Stderr, "promptlab: %v\n", err)
			return 1
		}
	}

	if anyFailed {
		return 1
	}
	return 0
}

// loadSystemPrompt returns the content to use as the system message: the
// file at path if non-empty, otherwise Auris's real default TaskChat prompt.
func loadSystemPrompt(path string) (string, error) {
	if path == "" {
		msg := agent.BuildSystemMessage(llm.TaskChat, nil)
		if msg == nil {
			return "", fmt.Errorf("no default system prompt registered for llm.TaskChat")
		}
		return msg.Content, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read system prompt %s: %w", path, err)
	}
	return string(data), nil
}

// runCase builds a fresh Agent (so each case starts with an empty tool-call
// budget and no cross-case state), sends systemContent + prompt, and prints
// the tool-call trace live via WithToolTrace as it happens.
func runCase(ctx context.Context, llmProvider llm.AIProvider, mp market.ProviderAPI, model, systemContent, name, prompt string, timeout time.Duration) CaseResult {
	fmt.Printf("=== case: %s ===\n", name)

	var trace []agent.ToolTraceEvent
	ag := agent.New(llmProvider, mp, model, agent.WithToolTrace(func(e agent.ToolTraceEvent) {
		trace = append(trace, e)
		fmt.Println(FormatTraceLine(e))
	}))
	if simDriver, ok := mp.(*simulation.Driver); ok {
		ag.SetNewsProvider(simDriver.NewsSource())
	}

	caseCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	finalMsg, err := ag.Chat(caseCtx, []llm.Message{
		{Role: llm.RoleSystem, Content: systemContent},
		{Role: llm.RoleUser, Content: prompt},
	})
	elapsed := time.Since(start)

	return CaseResult{
		Name:         name,
		Prompt:       prompt,
		FinalContent: finalMsg.Content,
		Trace:        trace,
		Usage:        ag.LastUsage(),
		Elapsed:      elapsed,
		RunErr:       err,
	}
}

func findLLMEntry(key string) *registry.LLMEntry {
	for _, e := range registry.AllLLM() {
		if e.Key == key {
			return &e
		}
	}
	return nil
}
