package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func codexInputText(t *testing.T, messages []ChatMessage, rf *ResponseFormat) string {
	t.Helper()
	body, err := buildOpenAICodexRequestBody(messages, ChatOptions{ResponseFormat: rf}, "gpt-test", false, openAICodexToolNameMap{}, ClientConfig{})
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	raw, err := json.Marshal(body["input"])
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	return string(raw)
}

// The Responses API refuses a json_object request unless an input message
// says "json"; a system prompt does not count because it travels as
// instructions.
func TestCodexRequestAddsJSONHintWhenOnlyTheSystemPromptMentionsIt(t *testing.T) {
	messages := []ChatMessage{
		{Role: "system", Content: "Reply with a single JSON object."},
		{Role: "user", Content: "Goal: ship it"},
	}
	input := codexInputText(t, messages, &ResponseFormat{Type: ResponseFormatJSONObject})
	if !strings.Contains(input, codexJSONObjectHint) {
		t.Fatalf("expected the JSON hint in the input, got %s", input)
	}
}

func TestCodexRequestLeavesInputAloneWhenNoHintIsNeeded(t *testing.T) {
	jsonObject := &ResponseFormat{Type: ResponseFormatJSONObject}
	cases := map[string]struct {
		messages []ChatMessage
		format   *ResponseFormat
	}{
		"an input message already says json": {[]ChatMessage{{Role: "user", Content: "Return JSON please"}}, jsonObject},
		"no response format":                 {[]ChatMessage{{Role: "user", Content: "hello"}}, nil},
		"plain text format":                  {[]ChatMessage{{Role: "user", Content: "hello"}}, &ResponseFormat{Type: ResponseFormatText}},
	}
	for name, tc := range cases {
		if input := codexInputText(t, tc.messages, tc.format); strings.Contains(input, codexJSONObjectHint) {
			t.Fatalf("%s: unexpected JSON hint in %s", name, input)
		}
	}
}
