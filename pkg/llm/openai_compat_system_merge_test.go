package llm

import (
	"encoding/json"
	"reflect"
	"testing"
)

func wireRoles(wire []openAIWireMessage) []string {
	roles := make([]string, 0, len(wire))
	for _, m := range wire {
		roles = append(roles, m.Role)
	}
	return roles
}

func TestToOpenAIWireMessages_MergesLeadingSystemMessages(t *testing.T) {
	messages := []ChatMessage{
		{Role: "system", Content: "stable prompt"},
		{Role: "system", Content: "## Current Time\n\nnow"},
		{Role: "user", Content: "hi"},
	}
	wire := toOpenAIWireMessages(messages, false)

	if got, want := wireRoles(wire), []string{"system", "user"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("roles = %v, want %v", got, want)
	}
	if got, want := wire[0].Content, "stable prompt\n\n## Current Time\n\nnow"; got != want {
		t.Fatalf("merged system = %q, want %q", got, want)
	}
	if len(messages) != 3 || messages[0].Content != "stable prompt" {
		t.Fatalf("caller's messages were changed: %+v", messages)
	}
}

// A compaction summary is replayed as a system message right after the
// prompt, so it is part of the opening run; blank ones add no separator.
func TestToOpenAIWireMessages_MergesWholeLeadingRunAndSkipsBlank(t *testing.T) {
	wire := toOpenAIWireMessages([]ChatMessage{
		{Role: "system", Content: "prompt"},
		{Role: "system", Content: "  "},
		{Role: "system", Content: "[COMPACTION SUMMARY]\nearlier"},
		{Role: "user", Content: "hi"},
	}, false)

	if got, want := wireRoles(wire), []string{"system", "user"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("roles = %v, want %v", got, want)
	}
	if got, want := wire[0].Content, "prompt\n\n[COMPACTION SUMMARY]\nearlier"; got != want {
		t.Fatalf("merged system = %q, want %q", got, want)
	}
}

// A system message later in the conversation is neither moved to the front
// nor merged with its neighbours.
func TestToOpenAIWireMessages_KeepsLaterSystemMessagesInPlace(t *testing.T) {
	wire := toOpenAIWireMessages([]ChatMessage{
		{Role: "system", Content: "prompt"},
		{Role: "system", Content: "tail"},
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "answer"},
		{Role: "system", Content: "cron note"},
		{Role: "system", Content: "critic feedback"},
		{Role: "user", Content: "second"},
	}, false)

	wantRoles := []string{"system", "user", "assistant", "system", "system", "user"}
	if got := wireRoles(wire); !reflect.DeepEqual(got, wantRoles) {
		t.Fatalf("roles = %v, want %v", got, wantRoles)
	}
	if wire[0].Content != "prompt\n\ntail" || wire[3].Content != "cron note" || wire[4].Content != "critic feedback" {
		t.Fatalf("unexpected system contents: %+v", wire)
	}
}

func TestToOpenAIWireMessages_LeavesOtherOpeningsAlone(t *testing.T) {
	cases := map[string][]ChatMessage{
		"single system": {
			{Role: "system", Content: "prompt"},
			{Role: "user", Content: "hi"},
		},
		"no system": {
			{Role: "user", Content: "hi"},
			{Role: "system", Content: "note"},
			{Role: "system", Content: "note 2"},
		},
		"system with content blocks": {
			{Role: "system", Content: "prompt"},
			{Role: "system", ContentBlocks: []ContentBlock{{Type: "text", Text: "blocks"}}},
			{Role: "user", Content: "hi"},
		},
	}
	for name, messages := range cases {
		t.Run(name, func(t *testing.T) {
			wire := toOpenAIWireMessages(messages, false)
			if len(wire) != len(messages) {
				t.Fatalf("got %d wire messages, want %d", len(wire), len(messages))
			}
			for i, m := range messages {
				if wire[i].Role != m.Role {
					t.Fatalf("message %d role = %q, want %q", i, wire[i].Role, m.Role)
				}
				if len(m.ContentBlocks) == 0 && wire[i].Content != m.Content {
					t.Fatalf("message %d content = %v, want %q", i, wire[i].Content, m.Content)
				}
			}
		})
	}
}

// Every OpenAI-compatible label sends the merged prompt.
func TestOpenAICompatibleBuildChatRequest_MergesLeadingSystemMessagesForEveryLabel(t *testing.T) {
	for _, label := range []string{"openai", "kimi", "gemini"} {
		t.Run(label, func(t *testing.T) {
			client, err := newOpenAICompatibleClientWithConfig(label, "http://127.0.0.1:1/v1", "k", "m", DefaultClientConfig())
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			body, err := client.buildChatRequest([]ChatMessage{
				{Role: "system", Content: "prompt"},
				{Role: "system", Content: "tail"},
				{Role: "user", Content: "hi"},
			}, ChatOptions{})
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			raw, err := json.Marshal(body["messages"])
			if err != nil {
				t.Fatalf("marshal messages: %v", err)
			}
			want := `[{"role":"system","content":"prompt\n\ntail"},{"role":"user","content":"hi"}]`
			if string(raw) != want {
				t.Fatalf("messages = %s, want %s", raw, want)
			}
		})
	}
}
