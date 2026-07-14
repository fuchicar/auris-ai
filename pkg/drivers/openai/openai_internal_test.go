package openai

import (
	"testing"

	sdk "github.com/openai/openai-go"

	"auris/pkg/llm"
)

// BUG-11: Complete (via mapResponse) used to discard the assistant's text
// when the response also carried tool calls, even though Stream preserved it
// via the SDK accumulator. Verify parity.
func TestMapResponse_PreservesContentAlongsideToolCalls(t *testing.T) {
	resp := &sdk.ChatCompletion{
		Choices: []sdk.ChatCompletionChoice{
			{
				FinishReason: "tool_calls",
				Message: sdk.ChatCompletionMessage{
					Content: "Let me check that for you.",
					ToolCalls: []sdk.ChatCompletionMessageToolCall{
						{
							ID: "call_1",
							Function: sdk.ChatCompletionMessageToolCallFunction{
								Name:      "get_weather",
								Arguments: `{"location":"Madrid"}`,
							},
						},
					},
				},
			},
		},
	}

	got := mapResponse(resp)

	if got.Message.Content != "Let me check that for you." {
		t.Errorf("Content = %q, want assistant text preserved alongside tool calls", got.Message.Content)
	}
	if len(got.Message.ToolCalls) != 1 || got.Message.ToolCalls[0].ID != "call_1" {
		t.Errorf("ToolCalls = %+v, want one call with ID call_1", got.Message.ToolCalls)
	}
}

// buildAssistantMessage is the inverse of mapResponse: when a stored
// assistant llm.Message carries both Content and ToolCalls (as mapResponse
// now produces), round-tripping it back into the SDK param must not drop
// the text either.
func TestBuildAssistantMessage_PreservesContentAlongsideToolCalls(t *testing.T) {
	msg := llm.Message{
		Role:    llm.RoleAssistant,
		Content: "Let me check that for you.",
		ToolCalls: []llm.ToolCall{
			{
				ID: "call_1",
				Function: llm.ToolCallFunction{
					Name:      "get_weather",
					Arguments: `{"location":"Madrid"}`,
				},
			},
		},
	}

	got := buildAssistantMessage(msg)

	if got.OfAssistant == nil {
		t.Fatal("expected OfAssistant to be set")
	}
	content := got.OfAssistant.Content.OfString
	if !content.Valid() || content.Value != "Let me check that for you." {
		t.Errorf("Content = %+v, want assistant text preserved alongside tool calls", content)
	}
	if len(got.OfAssistant.ToolCalls) != 1 || got.OfAssistant.ToolCalls[0].ID != "call_1" {
		t.Errorf("ToolCalls = %+v, want one call with ID call_1", got.OfAssistant.ToolCalls)
	}
}
