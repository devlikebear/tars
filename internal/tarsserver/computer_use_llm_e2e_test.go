package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/computeruse"
	"github.com/devlikebear/tars/internal/config"
)

func TestComputerUseLLMEndToEndWithExistingRouter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX driver fixture")
	}
	fixture, err := filepath.Abs(filepath.Join("..", "computeruse", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "cua-driver")
	calls := filepath.Join(dir, "calls")
	script := `#!/bin/sh
printf '%s\n' "$1" >> '` + calls + `'
case "$1" in
 list_windows) cat '` + fixture + `/list_windows.json';;
 get_window_state) cat '` + fixture + `/get_window_state_calculator.json';;
 *) printf '%s' '{"structuredContent":{"effect":"confirmed"}}';;
esac
`
	if err := os.WriteFile(stub, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "light-test" {
			t.Errorf("wrong model: %s", request.Model)
		}
		prompts = append(prompts, request.Messages[len(request.Messages)-1].Content)
		content := `{"op":"click","target":"e15","input_key":"none","risky":false,"done":false}`
		if len(prompts) > 1 {
			content = `{"op":"done","target":"none","input_key":"none","risky":false,"done":true}`
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": content}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 20}})
	}))
	defer server.Close()
	cfg := config.Default()
	cfg.LLMDefaultTier = "standard"
	cfg.LLMRoleDefaults = map[string]string{"computer_use": "light"}
	cfg.WorkspaceDir = dir
	cfg.ToolsComputerUseCuaDriverPath = stub
	cfg.LLMProviders = map[string]config.LLMProviderSettings{"test": {Kind: "openai", BaseURL: server.URL, APIKey: "test-only"}}
	cfg.LLMTiers = map[string]config.LLMTierBinding{"heavy": {Provider: "test", Model: "heavy-test"}, "standard": {Provider: "test", Model: "standard-test"}, "light": {Provider: "test", Model: "light-test"}}
	router, err := buildLLMRouter(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	var res computeruse.Result
	for _, tool := range buildOptionalChatTools(cfg, nil, router, nil) {
		if tool.Name != "computer_use" {
			continue
		}
		result, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"Press 2","inputs":{"password":"private-input-value"},"max_steps":4}`))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(result.Content[0].Text), &res); err != nil {
			t.Fatal(err)
		}
	}
	if res.Status != computeruse.StatusDone || res.Steps != 2 {
		t.Fatalf("result: %+v", res)
	}
	if res.Usage.Backend != "llm" || res.Usage.InputTokens != 200 || res.Usage.OutputTokens != 40 || res.Usage.JevInputTokens != 0 {
		t.Fatalf("usage: %+v", res.Usage)
	}
	if res.Usage.EstUSD != 0 {
		t.Fatalf("unpriced LLM must not use Jev price: %+v", res.Usage)
	}
	if len(prompts) != 2 {
		t.Fatalf("calls: %d", len(prompts))
	}
	for _, prompt := range prompts {
		if strings.Contains(prompt, "private-input-value") {
			t.Fatal("input leaked")
		}
	}
	if !strings.Contains(prompts[1], "RECENT ACTIONS") {
		t.Fatal("missing action history")
	}
	log, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(log)) != "list_windows\nlist_windows\nget_window_state\nclick\nget_window_state" {
		t.Fatalf("driver log: %s", log)
	}
}
