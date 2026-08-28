// Command real-provider is the minimal agent shape built against a real
// provider client, as opposed to examples/min-agent's scripted one.
//
// It exists because a scripted client never type-checks llm.NewProvider: the
// documented "minimal agent shape" could drift from the actual constructor
// signature and every test would still pass. This compiles against the real
// one, so it cannot.
//
// It reads its credential from the environment and makes exactly one request:
//
//	ANTHROPIC_API_KEY=... go run ./examples/real-provider
//	OPENAI_API_KEY=...    go run ./examples/real-provider -provider openai
//
// With no credential it prints what it would have done and exits 0, so it
// stays runnable in a checkout with nothing configured.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/devlikebear/tars/pkg/agentloop"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/devlikebear/tars/pkg/tools"
)

func main() {
	provider := flag.String("provider", "anthropic", "provider kind: anthropic, openai, gemini, gemini-native, kimi")
	model := flag.String("model", "", "model id; empty uses the kind's default")
	prompt := flag.String("prompt", "Use the echo tool with the text 'hello', then tell me what it returned.", "user message")
	flag.Parse()

	apiKey := credentialFor(*provider)
	if apiKey == "" {
		fmt.Printf("no credential in the environment for %q; set %s to run this for real.\n",
			*provider, credentialEnvFor(*provider))
		return
	}

	client, err := llm.NewProvider(llm.ProviderOptions{
		Provider: *provider,
		Model:    *model,
		APIKey:   apiKey,
	})
	if err != nil {
		fail("new provider", err)
	}

	registry := tools.NewRegistry()
	registry.Register(echoTool())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	loop := agentloop.New(client, registry)
	resp, err := loop.Run(ctx, []llm.ChatMessage{
		{Role: "user", Content: *prompt},
	}, agentloop.RunOptions{Tools: registry.Schemas()})
	if err != nil {
		fail("run loop", err)
	}

	fmt.Println(strings.TrimSpace(resp.Message.Content))
	fmt.Fprintf(os.Stderr, "\ntokens: in=%d out=%d cached=%d\n",
		resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.CachedTokens)
}

func echoTool() tools.Tool {
	return tools.Tool{
		Name:        "echo",
		Description: "Echo text back to the model.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
		Execute: func(_ context.Context, params json.RawMessage) (tools.Result, error) {
			var input struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return tools.JSONTextResult(map[string]string{"error": err.Error()}, true), nil
			}
			return tools.JSONTextResult(map[string]string{"echo": input.Text}, false), nil
		},
	}
}

func credentialEnvFor(provider string) string {
	switch provider {
	case "openai":
		return "OPENAI_API_KEY"
	case "gemini", "gemini-native":
		return "GEMINI_API_KEY"
	case "kimi":
		return "KIMI_API_KEY"
	default:
		return "ANTHROPIC_API_KEY"
	}
}

func credentialFor(provider string) string {
	return strings.TrimSpace(os.Getenv(credentialEnvFor(provider)))
}

func fail(what string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", what, err)
	os.Exit(1)
}
