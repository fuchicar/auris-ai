package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fuchicar/auris-ai/pkg/agent"
	"github.com/fuchicar/auris-ai/pkg/llm"
)

// CaseResult is the outcome of running one Case (or the ad-hoc prompt) to
// completion: everything needed to print a trace, an assertion report, and
// feed the end-of-run summary.
type CaseResult struct {
	Name         string
	Prompt       string
	FinalContent string
	Trace        []agent.ToolTraceEvent
	Usage        llm.TokenUsage
	Elapsed      time.Duration
	RunErr       error

	// Assertions only apply in suite mode (Case.ExpectedToolSequence etc. set).
	Failures []string
	Warnings []string

	// Judge is nil unless -judge-provider/-judge-model were configured. It is
	// advisory only — never factors into Passed().
	Judge *JudgeVerdict
}

// Passed reports whether the case ran without error and every assertion held.
// An ad-hoc run (no assertions configured at all) is always "passed" — there
// was nothing to fail — so callers wanting a human verdict should look at the
// trace/final answer, not this flag.
func (r CaseResult) Passed() bool {
	return r.RunErr == nil && len(r.Failures) == 0
}

// FormatTraceLine renders a single completed tool dispatch for real-time
// stdout printing, e.g.:
//
//	[tool] market_get_candles({"symbol":"AAPL","from":"2026-04-01T00:00:00Z",...}) -> 90 candles (312ms)
func FormatTraceLine(e agent.ToolTraceEvent) string {
	return fmt.Sprintf("[tool] %s(%s) -> %s (%dms)",
		e.Name, truncate(e.Args, 200), truncate(e.Result, 200), e.Duration.Milliseconds())
}

func truncate(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}

// evaluateCase applies a Case's assertions to a completed run, returning the
// failures and warnings to attach to the CaseResult. Called after the run
// finishes; toolNames is the ordered list of tool names extracted from trace.
func evaluateCase(c Case, trace []agent.ToolTraceEvent, finalContent string, runErr error) (failures, warnings []string) {
	// runErr is already surfaced via [ERROR] and folded into CaseResult.Passed()
	// directly (see PrintCaseReport) — not duplicated into failures here.
	toolNames := make([]string, len(trace))
	for i, e := range trace {
		toolNames[i] = e.Name
	}

	if len(c.ExpectedToolSequence) > 0 && !isSubsequence(toolNames, c.ExpectedToolSequence) {
		failures = append(failures, fmt.Sprintf(
			"expected tool sequence %v not found as an ordered subsequence of actual calls %v",
			c.ExpectedToolSequence, toolNames))
	}

	lowerContent := strings.ToLower(finalContent)
	for _, phrase := range c.ForbiddenPhrases {
		if strings.Contains(lowerContent, strings.ToLower(phrase)) {
			failures = append(failures, fmt.Sprintf("forbidden phrase found in final answer: %q", phrase))
		}
	}
	for _, phrase := range c.RequiredPhrases {
		if !strings.Contains(lowerContent, strings.ToLower(phrase)) {
			failures = append(failures, fmt.Sprintf("required phrase missing from final answer: %q", phrase))
		}
	}

	seen := make(map[string]bool)
	for _, e := range trace {
		sig := e.Name + "|" + e.Args
		if seen[sig] {
			warnings = append(warnings, fmt.Sprintf("repeated identical tool call: %s(%s)", e.Name, truncate(e.Args, 120)))
		}
		seen[sig] = true
	}

	return failures, warnings
}

// isSubsequence reports whether want appears, in order, within got — not
// necessarily contiguously (e.g. got=[a,b,c,d], want=[a,c] -> true).
func isSubsequence(got, want []string) bool {
	i := 0
	for _, name := range got {
		if i == len(want) {
			break
		}
		if name == want[i] {
			i++
		}
	}
	return i == len(want)
}

// PrintCaseReport prints the assertion outcome, warnings, and stats for a
// completed case. The tool trace itself is printed live as it happens (see
// FormatTraceLine), not repeated here.
func PrintCaseReport(w io.Writer, r CaseResult) {
	fmt.Fprintf(w, "--- final answer (%s) ---\n%s\n", r.Name, r.FinalContent)

	if r.RunErr != nil {
		fmt.Fprintf(w, "[ERROR] agent run failed: %v\n", r.RunErr)
		if hint := contextWindowHint(r.RunErr); hint != "" {
			fmt.Fprintf(w, "[HINT] %s\n", hint)
		}
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "[WARN] %s\n", warn)
	}
	for _, fail := range r.Failures {
		fmt.Fprintf(w, "[FAIL] %s\n", fail)
	}
	if r.Judge != nil {
		fmt.Fprintf(w, "[JUDGE] verdict=%s score=%d reasoning=%q\n", r.Judge.Verdict, r.Judge.Score, r.Judge.Reasoning)
	}

	fmt.Fprintf(w, "[stats] tool_calls=%d prompt_tokens=%d completion_tokens=%d elapsed=%s\n",
		len(r.Trace), r.Usage.PromptTokens, r.Usage.CompletionTokens, r.Elapsed.Round(time.Millisecond))

	verdict := "PASS"
	if !r.Passed() {
		verdict = "FAIL"
	}
	fmt.Fprintf(w, "RESULT [%s]: %s\n\n", r.Name, verdict)
}

// PrintSummary prints an end-of-run table across every case in a suite.
func PrintSummary(w io.Writer, results []CaseResult) {
	fmt.Fprintln(w, "=== summary ===")
	failed := 0
	for _, r := range results {
		verdict := "PASS"
		if !r.Passed() {
			verdict = "FAIL"
			failed++
		}
		fmt.Fprintf(w, "%-6s %s\n", verdict, r.Name)
	}
	fmt.Fprintf(w, "%d/%d cases passed\n", len(results)-failed, len(results))
}

// contextWindowHint returns an actionable suggestion when err looks like a
// context-window overflow (the full Auris prompt + all tool schemas didn't
// fit), or "" otherwise. The match is a heuristic over the error text since
// drivers don't expose a dedicated sentinel for this failure mode.
func contextWindowHint(err error) string {
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "context") {
		return ""
	}
	if !strings.Contains(msg, "window") && !strings.Contains(msg, "length") && !strings.Contains(msg, "exceed") {
		return ""
	}
	return "this looks like a context-window overflow — the full Auris prompt + all tool schemas didn't fit. " +
		"Try increasing the model's context length before judging this prompt " +
		"(LM Studio: `lms load -c <tokens>`; Ollama: set AURIS_OLLAMA_NUM_CTX)."
}

// PrintMatrix prints a PASS/FAIL comparison table across multiple targets
// that all ran the same ordered list of cases (columns are taken from the
// first target's results — every target runs the identical []Case, see
// main.go). Used only when more than one target is configured.
func PrintMatrix(w io.Writer, targetNames []string, resultsByTarget map[string][]CaseResult) {
	fmt.Fprintln(w, "=== matrix ===")
	if len(targetNames) == 0 {
		return
	}
	caseNames := make([]string, 0)
	for _, r := range resultsByTarget[targetNames[0]] {
		caseNames = append(caseNames, r.Name)
	}

	header := fmt.Sprintf("%-24s", "target")
	for _, cn := range caseNames {
		header += fmt.Sprintf(" %-14s", cn)
	}
	header += " pass"
	fmt.Fprintln(w, header)

	for _, name := range targetNames {
		results := resultsByTarget[name]
		row := fmt.Sprintf("%-24s", name)
		passed, judgeTotal, judgeCount := 0, 0, 0
		for _, r := range results {
			verdict := "PASS"
			if !r.Passed() {
				verdict = "FAIL"
			} else {
				passed++
			}
			row += fmt.Sprintf(" %-14s", verdict)
			if r.Judge != nil {
				judgeTotal += r.Judge.Score
				judgeCount++
			}
		}
		row += fmt.Sprintf(" %d/%d", passed, len(results))
		if judgeCount > 0 {
			row += fmt.Sprintf(" (judge avg %.1f)", float64(judgeTotal)/float64(judgeCount))
		}
		fmt.Fprintln(w, row)
	}
}
