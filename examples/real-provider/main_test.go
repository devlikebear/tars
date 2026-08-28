package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCredentialEnvFor(t *testing.T) {
	cases := map[string]string{
		"anthropic":     "ANTHROPIC_API_KEY",
		"openai":        "OPENAI_API_KEY",
		"gemini":        "GEMINI_API_KEY",
		"gemini-native": "GEMINI_API_KEY",
		"kimi":          "KIMI_API_KEY",
		// An unrecognized kind falls back to the default provider's variable
		// rather than an empty string, so the "set X to run this" message
		// always names something actionable.
		"not-a-provider": "ANTHROPIC_API_KEY",
	}
	for provider, want := range cases {
		if got := credentialEnvFor(provider); got != want {
			t.Errorf("credentialEnvFor(%q) = %q, want %q", provider, got, want)
		}
	}
}

func TestCredentialFor_TrimsAndReadsTheEnvironment(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "  sk-test  ")
	if got := credentialFor("anthropic"); got != "sk-test" {
		t.Fatalf("credentialFor = %q, want the trimmed value", got)
	}

	t.Setenv("OPENAI_API_KEY", "   ")
	if got := credentialFor("openai"); got != "" {
		t.Fatalf("a whitespace-only credential should read as absent, got %q", got)
	}
}

func TestEchoTool_RoundTripsItsInput(t *testing.T) {
	tool := echoTool()
	if tool.Name != "echo" {
		t.Fatalf("tool name = %q", tool.Name)
	}

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res)
	}
	if !strings.Contains(res.Content[0].Text, "hello") {
		t.Fatalf("echo did not return its input: %+v", res)
	}
}

func TestEchoTool_ReportsBadInputAsAToolError(t *testing.T) {
	// A malformed argument must come back as an error *result*, not a Go
	// error: the loop shows the model what went wrong instead of aborting.
	res, err := echoTool().Execute(context.Background(), json.RawMessage(`{"text":`))
	if err != nil {
		t.Fatalf("execute returned a Go error rather than an error result: %v", err)
	}
	if !res.IsError {
		t.Fatalf("malformed input did not produce an error result: %+v", res)
	}
}
