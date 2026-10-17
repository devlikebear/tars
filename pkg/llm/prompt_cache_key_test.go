package llm

import (
	"strings"
	"testing"
)

func TestPromptCacheKey_HashesAStableBoundedKey(t *testing.T) {
	if got := promptCacheKey("  "); got != "" {
		t.Fatalf("blank key = %q, want none", got)
	}
	key := promptCacheKey("sess-123")
	if key != promptCacheKey(" sess-123 ") {
		t.Fatal("the same session must map to the same key")
	}
	if key == promptCacheKey("sess-124") {
		t.Fatal("different sessions must map to different keys")
	}
	if !strings.HasPrefix(key, promptCacheKeyPrefix) || strings.Contains(key, "sess-123") {
		t.Fatalf("key %q must be prefixed and must not carry the raw id", key)
	}
	// The Responses and Chat Completions APIs cap prompt_cache_key at 64.
	if len(key) > 64 {
		t.Fatalf("key is %d characters, over the 64-character limit", len(key))
	}
}

func TestOpenAICompatibleBuildChatRequest_PromptCacheKeyOnlyForOpenAI(t *testing.T) {
	for _, label := range []string{"openai", "kimi", "gemini"} {
		t.Run(label, func(t *testing.T) {
			client, err := newOpenAICompatibleClientWithConfig(label, "http://127.0.0.1:1/v1", "k", "m", DefaultClientConfig())
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			body, err := client.buildChatRequest([]ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{CacheKey: "sess-1"})
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			got, sent := body["prompt_cache_key"]
			if label == "openai" {
				if got != promptCacheKey("sess-1") {
					t.Fatalf("prompt_cache_key = %v, want %q", got, promptCacheKey("sess-1"))
				}
				return
			}
			// gemini's OpenAI shim and kimi reject fields they do not know.
			if sent {
				t.Fatalf("%s must not send prompt_cache_key, got %v", label, got)
			}
		})
	}
}

func TestOpenAICompatibleBuildChatRequest_NoPromptCacheKeyWithoutOne(t *testing.T) {
	client, err := newOpenAICompatibleClientWithConfig("openai", "http://127.0.0.1:1/v1", "k", "m", DefaultClientConfig())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	body, err := client.buildChatRequest([]ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if _, sent := body["prompt_cache_key"]; sent {
		t.Fatal("a request without a cache key must not send prompt_cache_key")
	}
}

// openai-codex declares CapPromptCacheKey; the conformance stub cannot build
// it (OAuth), so the declaration is checked against the body builder here.
func TestOpenAICodexRequestBody_PromptCacheKey(t *testing.T) {
	if !SupportsCapability("openai-codex", CapPromptCacheKey) {
		t.Fatal("openai-codex must declare prompt_cache_key")
	}
	messages := []ChatMessage{{Role: "user", Content: "hi"}}
	body, err := buildOpenAICodexRequestBody(messages, ChatOptions{CacheKey: "sess-1"}, "gpt-test", true, openAICodexToolNameMap{}, ClientConfig{})
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	if got := body["prompt_cache_key"]; got != promptCacheKey("sess-1") {
		t.Fatalf("prompt_cache_key = %v, want %q", got, promptCacheKey("sess-1"))
	}

	body, err = buildOpenAICodexRequestBody(messages, ChatOptions{}, "gpt-test", true, openAICodexToolNameMap{}, ClientConfig{})
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	if _, sent := body["prompt_cache_key"]; sent {
		t.Fatal("a request without a cache key must not send prompt_cache_key")
	}
}
