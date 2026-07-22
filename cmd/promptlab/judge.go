package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fuchicar/auris-ai/pkg/agent"
	"github.com/fuchicar/auris-ai/pkg/llm"
)

// JudgeVerdict is an advisory, LLM-produced assessment of one case's final
// answer. It never gates CaseResult.Passed() — only the deterministic
// assertions in evaluateCase do that — so a flaky or wrong judge call can
// only add information, never silently fail a case.
type JudgeVerdict struct {
	Verdict   string `json:"verdict"` // PASS | FAIL | UNSURE
	Score     int    `json:"score"`   // 1-5
	Reasoning string `json:"reasoning"`
}

const judgeSystemPrompt = `You are a strict evaluator of an AI financial assistant's response.
You will be given the user's request, the tools the assistant called (with their arguments and results), and the assistant's final answer.

Judge whether the final answer is accurate, is properly grounded in the tool results (not invented or hallucinated), and used tools appropriately for the request.

Respond with ONLY a single JSON object, no markdown code fences, no extra text, in this exact shape:
{"verdict":"PASS or FAIL","score":1-5,"reasoning":"one or two sentences"}`

// RunJudge asks judgeLLM to score a completed case. It never returns an
// error: any call failure or unparseable response becomes a JudgeVerdict
// with Verdict "UNSURE" so a bad judge never aborts the run it's evaluating.
func RunJudge(ctx context.Context, judgeLLM llm.AIProvider, judgeModel, userPrompt string, trace []agent.ToolTraceEvent, finalAnswer string) JudgeVerdict {
	var sb strings.Builder
	fmt.Fprintf(&sb, "User request:\n%s\n\nTool calls made:\n", userPrompt)
	if len(trace) == 0 {
		sb.WriteString("(none)\n")
	}
	for _, e := range trace {
		fmt.Fprintf(&sb, "- %s(%s) -> %s\n", e.Name, truncate(e.Args, 300), truncate(e.Result, 300))
	}
	fmt.Fprintf(&sb, "\nFinal answer:\n%s\n", finalAnswer)

	resp, err := judgeLLM.Complete(ctx, llm.CompletionRequest{
		Model: judgeModel,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: judgeSystemPrompt},
			{Role: llm.RoleUser, Content: sb.String()},
		},
	})
	if err != nil {
		return JudgeVerdict{Verdict: "UNSURE", Reasoning: "judge call failed: " + err.Error()}
	}

	var v JudgeVerdict
	if err := json.Unmarshal([]byte(extractJSONObject(resp.Message.Content)), &v); err != nil {
		return JudgeVerdict{Verdict: "UNSURE", Reasoning: truncate(resp.Message.Content, 300)}
	}
	return v
}

// extractJSONObject strips a possible ```json ... ``` fence and returns the
// substring from the first '{' to the last '}', tolerating models that add
// prose around the JSON object despite being told not to.
func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end < start {
		return s
	}
	return s[start : end+1]
}
