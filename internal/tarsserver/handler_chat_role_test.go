package tarsserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestChatAPIHandler_FiltersHighRiskToolsForUserRole(t *testing.T) {
	seen := runChatAndCollectTools(t, "user", chatToolingOptions{})
	for _, denied := range []string{"exec", "write_file", "edit_file"} {
		if hasToolName(seen, denied) {
			t.Fatalf("expected %s to be filtered for user role, got %+v", denied, seen)
		}
	}
	if !hasToolName(seen, "read_file") {
		t.Fatalf("expected read_file in user tool set, got %+v", seen)
	}
}

func TestChatAPIHandler_ExposesHighRiskToolsForAdminRole(t *testing.T) {
	seen := runChatAndCollectTools(t, "admin", chatToolingOptions{})
	for _, expected := range []string{"exec", "write_file", "edit_file"} {
		if !hasToolName(seen, expected) {
			t.Fatalf("expected %s for admin role, got %+v", expected, seen)
		}
	}
}

func runChatAndCollectTools(t *testing.T, role string, tooling chatToolingOptions) []string {
	t.Helper()
	root := t.TempDir()
	store := session.NewStore(root)
	logger := zerolog.New(io.Discard)
	mockClient := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{
				Role:    "assistant",
				Content: "ok",
			},
		},
	}

	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		mockClient,
		logger,
		4,
		nil,
		"",
		tooling,
	)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tars-Debug-Auth-Role", role)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if len(mockClient.seenTools) == 0 {
		t.Fatalf("expected seen tools to be captured")
	}
	return append([]string(nil), mockClient.seenTools[0]...)
}
