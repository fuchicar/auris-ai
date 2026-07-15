package agent

import (
	"log"
	"strings"

	"github.com/fuchicar/auris-ai/pkg/llm"
)

// Option configures an Agent at construction time.
type Option func(*Agent)

// WithDebugLogger attaches a logger that records each LLM iteration and tool
// dispatch. Pass nil to disable (the default). Intended for opt-in diagnostics
// via the `-debug <path>` CLI flag.
func WithDebugLogger(l *log.Logger) Option {
	return func(a *Agent) { a.debugLogger = l }
}

// WithPortfolioID sets the default portfolio ID used by portfolio tools when
// no explicit portfolio_id argument is provided. Typically set to the ID of
// the portfolio currently open in the TUI.
func WithPortfolioID(id string) Option {
	return func(a *Agent) { a.currentPortfolioID = id }
}

// debugf writes a formatted line to the debug logger if one is attached.
func (a *Agent) debugf(format string, args ...any) {
	if a == nil || a.debugLogger == nil {
		return
	}
	a.debugLogger.Printf(format, args...)
}

// truncate returns s shortened to at most maxRunes runes, appending "…" when
// truncation occurs. Newlines are collapsed to spaces so each preview fits on
// one log line.
func truncate(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}

// summarizeRoles returns a comma-separated list of message roles, useful for
// logging the shape of the conversation passed to the LLM.
func summarizeRoles(msgs []llm.Message) string {
	parts := make([]string, len(msgs))
	for i, m := range msgs {
		parts[i] = string(m.Role)
	}
	return strings.Join(parts, ",")
}

// summarizeToolNames returns a comma-separated list of tool function names.
func summarizeToolNames(tools []llm.Tool) string {
	parts := make([]string, len(tools))
	for i, t := range tools {
		parts[i] = t.Function.Name
	}
	return strings.Join(parts, ",")
}

// lastUserOrToolPreview returns a short preview of the latest user or tool
// message in msgs, prefixed with its role. Returns "" if none exists.
func lastUserOrToolPreview(msgs []llm.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role == llm.RoleUser || m.Role == llm.RoleTool {
			return string(m.Role) + "=" + truncate(m.Content, 200)
		}
	}
	return ""
}

// summarizeToolCalls returns a compact representation of a list of tool calls
// in the form `name(args), name(args)` with args truncated.
func summarizeToolCalls(calls []llm.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	parts := make([]string, len(calls))
	for i, c := range calls {
		parts[i] = c.Function.Name + "(" + truncate(c.Function.Arguments, 120) + ")"
	}
	return strings.Join(parts, ", ")
}
