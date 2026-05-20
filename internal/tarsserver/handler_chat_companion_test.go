package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestCompanionFeedbackAPIUsesLLMWithoutTools(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	mockClient := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{
				Role:    "assistant",
				Content: `{"mood":"spark","message":"LLM 제안입니다.","detail":"실제 모델이 현재 화면을 보고 답합니다."}`,
			},
		},
	}
	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		nil,
		testRouterForClient(t, mockClient),
		zerolog.New(io.Discard),
		8,
		nil,
		"",
		defaultChatToolingOptions(),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/companion", strings.NewReader(`{
		"stimulus":"suggest",
		"route_view":"chat",
		"locale":"ko",
		"fallback_message":"로컬 제안",
		"fallback_detail":"fallback only"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var got struct {
		Mood    string `json:"mood"`
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Source  string `json:"source"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Source != "llm" || got.Mood != "spark" || !strings.Contains(got.Message, "LLM") {
		t.Fatalf("unexpected companion response: %+v", got)
	}
	if mockClient.callCount != 1 {
		t.Fatalf("expected one LLM call, got %d", mockClient.callCount)
	}
	if len(mockClient.seenToolCounts) != 1 || mockClient.seenToolCounts[0] != 0 {
		t.Fatalf("expected no tools, got %+v", mockClient.seenToolCounts)
	}
	if len(mockClient.seenToolChoices) != 1 || mockClient.seenToolChoices[0] != "none" {
		t.Fatalf("expected tool_choice none, got %+v", mockClient.seenToolChoices)
	}
	if len(mockClient.seenReasoning) != 1 || mockClient.seenReasoning[0] != "low" {
		t.Fatalf("expected reasoning effort low, got %+v", mockClient.seenReasoning)
	}
	if len(mockClient.seenMessages) != 1 {
		t.Fatalf("expected captured messages, got %d", len(mockClient.seenMessages))
	}
	prompt := mockClient.seenMessages[0][len(mockClient.seenMessages[0])-1].Content
	if !strings.Contains(prompt, `"stimulus":"suggest"`) || !strings.Contains(prompt, `"route_view":"chat"`) {
		t.Fatalf("expected stimulus and route context in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "Return JSON") {
		t.Fatalf("expected JSON response-format hint in prompt, got %q", prompt)
	}
}

func TestCompanionFeedbackAPIRejectsUnknownStimulus(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	mockClient := &mockLLMClient{}
	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		nil,
		testRouterForClient(t, mockClient),
		zerolog.New(io.Discard),
		8,
		nil,
		"",
		defaultChatToolingOptions(),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/companion", strings.NewReader(`{"stimulus":"dance","route_view":"chat"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", rec.Code, rec.Body.String())
	}
	if mockClient.callCount != 0 {
		t.Fatalf("expected no LLM calls for invalid stimulus, got %d", mockClient.callCount)
	}
}

func TestCompanionFeedbackAPIUsesLightTierWhenRoleDefaultMissing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	standardClient := &mockLLMClient{}
	lightClient := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{Role: "assistant", Content: `{"mood":"focus","message":"light tier"}`},
		},
	}
	entry := func(client llm.Client, model string) llm.TierEntry {
		return llm.TierEntry{Client: client, Provider: "fake", Model: model}
	}
	router, err := llm.NewRouter(llm.RouterConfig{
		Tiers: map[llm.Tier]llm.TierEntry{
			llm.TierHeavy:    entry(standardClient, "heavy"),
			llm.TierStandard: entry(standardClient, "standard"),
			llm.TierLight:    entry(lightClient, "light"),
		},
		DefaultTier: llm.TierStandard,
		RoleDefaults: map[llm.Role]llm.Tier{
			llm.RoleChatMain: llm.TierStandard,
		},
	})
	if err != nil {
		t.Fatalf("build router: %v", err)
	}
	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		nil,
		router,
		zerolog.New(io.Discard),
		8,
		nil,
		"",
		defaultChatToolingOptions(),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/companion", strings.NewReader(`{"stimulus":"poke","route_view":"chat","locale":"en"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if lightClient.callCount != 1 {
		t.Fatalf("expected light tier client call, got %d", lightClient.callCount)
	}
	if standardClient.callCount != 0 {
		t.Fatalf("expected default standard client not to be called, got %d", standardClient.callCount)
	}
}
