package llm

import "testing"

// Gemini rejects a history where a model turn's parallel function calls are
// answered by separate user turns: "the number of function response parts is
// equal to the number of function call parts of the function call turn". All
// the answers to one turn go back in one user turn, each carrying its call's id.
func TestToGeminiNativeContents_GroupsParallelFunctionResponses(t *testing.T) {
	contents := toGeminiNativeContents([]ChatMessage{
		{Role: "user", Content: "read both scenes"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{ID: "call_a", Name: "read_scene", Arguments: `{"id":"1"}`},
				{ID: "call_b", Name: "read_scene", Arguments: `{"id":"2"}`},
				{ID: "call_c", Name: "list_notes", Arguments: `{}`},
			},
		},
		{Role: "tool", ToolCallID: "call_a", Content: `{"text":"one"}`},
		{Role: "tool", ToolCallID: "call_b", Content: `{"text":"two"}`},
		{Role: "tool", ToolCallID: "call_c", Content: `[]`},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_d", Name: "list_notes", Arguments: `{}`}}},
		{Role: "tool", ToolCallID: "call_d", Content: `[]`},
	})

	if len(contents) != 5 {
		t.Fatalf("expected user, model, user(3 responses), model, user(1 response); got %d contents", len(contents))
	}
	grouped := contents[2]
	if grouped.Role != "user" || len(grouped.Parts) != 3 {
		t.Fatalf("expected one user turn with 3 function responses, got role %q with %d parts", grouped.Role, len(grouped.Parts))
	}
	wantIDs := []string{"call_a", "call_b", "call_c"}
	wantNames := []string{"read_scene", "read_scene", "list_notes"}
	for i, part := range grouped.Parts {
		if part.FunctionResponse == nil {
			t.Fatalf("part %d is not a function response", i)
		}
		if part.FunctionResponse.ID != wantIDs[i] || part.FunctionResponse.Name != wantNames[i] {
			t.Fatalf("part %d = %s/%s, want %s/%s", i, part.FunctionResponse.ID, part.FunctionResponse.Name, wantIDs[i], wantNames[i])
		}
	}
	if last := contents[4]; last.Role != "user" || len(last.Parts) != 1 || last.Parts[0].FunctionResponse.ID != "call_d" {
		t.Fatalf("the next turn's response must start a user turn of its own, got %+v", last)
	}
}

// A user message between tool results is a new turn, not more answers.
func TestToGeminiNativeContents_UserMessageEndsFunctionResponseGroup(t *testing.T) {
	contents := toGeminiNativeContents([]ChatMessage{
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_a", Name: "read_scene", Arguments: `{}`}}},
		{Role: "tool", ToolCallID: "call_a", Content: `ok`},
		{Role: "user", Content: "also check notes"},
		{Role: "tool", ToolCallID: "call_a", Content: `late`},
	})
	if len(contents) != 4 {
		t.Fatalf("expected 4 contents, got %d", len(contents))
	}
	if len(contents[1].Parts) != 1 || len(contents[3].Parts) != 1 {
		t.Fatalf("responses on either side of a user message must not merge")
	}
}
