package llm

// Role identifies the speaker of a message in a conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// TaskType is the routing key that maps a use-case to a specific (provider, model) pair in config.
type TaskType string

const (
	TaskChat              TaskType = "chat"
	TaskFinancialAnalysis TaskType = "financial_analysis"
	TaskSummary           TaskType = "summary"
)

// Message is a single turn in a conversation.
type Message struct {
	Role    Role
	Content string
	// ToolCallID links a RoleTool message back to the ToolCall that triggered it.
	ToolCallID string
	// ToolCalls is populated when Role == RoleAssistant and the model invokes tools.
	ToolCalls []ToolCall
	// Extra holds arbitrary provider-specific data that must survive round-trips
	// through the conversation history (e.g. thought signatures). Drivers store
	// data here using package-prefixed keys; all other code ignores this field.
	Extra map[string]any
}

// ToolCall represents one function call requested by the model.
type ToolCall struct {
	ID       string
	Function ToolCallFunction
}

// ToolCallFunction carries the name and arguments chosen by the model.
type ToolCallFunction struct {
	Name      string
	Arguments string // JSON-encoded object; preserved as-is from the driver
}

// Tool describes a callable function the model may invoke.
type Tool struct {
	Type     string       // always "function"
	Function ToolFunction
}

// ToolFunction is the schema of a single callable function.
type ToolFunction struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema object
}

// CompletionRequest is the input to AIProvider.Complete and AIProvider.Stream.
type CompletionRequest struct {
	Model     string    // empty string uses the provider's configured default model
	Messages  []Message
	Tools     []Tool // nil means no tool calling
	MaxTokens int    // 0 uses the provider default
}

// CompletionResponse is the output of AIProvider.Complete.
type CompletionResponse struct {
	Message    Message
	Usage      TokenUsage
	StopReason string // "stop" | "tool_calls" | "length"
}

// StreamChunk is one frame emitted by AIProvider.Stream.
// Done == true marks the terminal frame; the channel is closed immediately after.
// Err != nil always implies Done == true. Consumers only need to check Done to exit.
type StreamChunk struct {
	Content string
	Done    bool
	Usage   TokenUsage // populated only on the final frame

	// ToolCalls and StopReason mirror CompletionResponse.ToolCalls/StopReason
	// and are populated only on the terminal (Done == true) frame; intermediate
	// chunks leave them zero. StopReason is "stop" | "tool_calls" | "length".
	ToolCalls  []ToolCall
	StopReason string

	// Extra carries the same provider-specific round-trip payload as
	// Message.Extra (e.g. "anthropic:content", "gemini:content"), populated
	// only on the terminal frame when ToolCalls is non-empty. Callers that
	// reconstruct a Message from a terminal StreamChunk must copy this
	// straight into Message.Extra so multi-turn tool-calling conversations
	// keep working.
	Extra map[string]any

	Err error
}

// TokenUsage carries prompt and completion token counts.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
}

// Model describes a model available on a provider.
type Model struct {
	ID   string
	Name string
	Size int64 // bytes; 0 if the provider does not report it
}
