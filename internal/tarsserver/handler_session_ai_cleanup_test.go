package tarsserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestSessionCleanupSuggestionsAPIArchiveModeUsesLLMAndSafetyRules(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	candidate := mustCreateCleanupSession(t, store, "Old spike")
	pinned := mustCreateCleanupSession(t, store, "Pinned context")
	archived := mustCreateCleanupSession(t, store, "Archived context")
	activePlan := mustCreateCleanupSession(t, store, "Active plan")
	if _, err := store.SetPinned(pinned.ID, true); err != nil {
		t.Fatalf("pin: %v", err)
	}
	if _, err := store.SetArchived(archived.ID, true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := store.SaveTasks(activePlan.ID, session.SessionTasks{
		Plan: &session.Plan{Goal: "Finish active plan", Status: session.PlanStatusExecuting, CreatedAt: session.NowRFC3339()},
	}); err != nil {
		t.Fatalf("save active plan: %v", err)
	}
	old := time.Now().UTC().Add(-20 * 24 * time.Hour)
	rewriteSessionIndexForCleanupTest(t, root, func(index map[string]session.Session) {
		for id, sess := range index {
			sess.CreatedAt = old
			sess.UpdatedAt = old
			if id == archived.ID {
				archivedAt := old
				sess.ArchivedAt = &archivedAt
			}
			index[id] = sess
		}
	})
	if err := session.AppendMessage(store.TranscriptPath(candidate.ID), session.Message{Role: "user", Content: "quick temporary experiment"}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	router, clients, err := llm.NewFakeRouter(llm.TierStandard, map[llm.Role]llm.Tier{
		llm.RoleSessionCleanup: llm.TierLight,
	})
	if err != nil {
		t.Fatalf("fake router: %v", err)
	}
	clients[llm.TierLight].ChatResponse = llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: fmt.Sprintf(`{
		"suggestions": [
				{"session_id": %q, "action": "archive", "confidence": 0.91, "reason": "Temporary spike with no durable follow-up."},
				{"session_id": %q, "action": "archive", "confidence": 0.99, "reason": "Pinned but suggested by mistake."},
				{"session_id": %q, "action": "archive", "confidence": 0.99, "reason": "Already archived."},
				{"session_id": %q, "action": "archive", "confidence": 0.99, "reason": "Active plan must stay visible."},
				{"session_id": "missing", "action": "archive", "confidence": 0.99, "reason": "Unknown session."}
			]
		}`, candidate.ID, pinned.ID, archived.ID, activePlan.ID)}}

	handler := newSessionAPIHandlerFullWithLLM(store, zerolog.Nop(), nil, sessionStyleValues{}, nil, nil, router)
	var resp sessionCleanupSuggestionResponse
	postSessionCleanupSuggestionsForTest(t, handler, `{"mode":"archive","limit":5}`, &resp)

	if clients[llm.TierLight].ChatCalls != 1 {
		t.Fatalf("expected cleanup role to use light LLM once, got %d calls", clients[llm.TierLight].ChatCalls)
	}
	if clients[llm.TierLight].LastChatOptions.ReasoningEffort != "" {
		t.Fatalf("expected cleanup request to use configured tier reasoning effort, got override %q", clients[llm.TierLight].LastChatOptions.ReasoningEffort)
	}
	if clients[llm.TierLight].LastChatOptions.OnDelta == nil {
		t.Fatal("expected cleanup request to keep streaming-compatible OnDelta callback")
	}
	if resp.Mode != "archive" || resp.Action != "archive" {
		t.Fatalf("expected archive mode/action, got %+v", resp)
	}
	if resp.Count != 1 || len(resp.Suggestions) != 1 {
		t.Fatalf("expected exactly one safe archive suggestion, got %+v", resp)
	}
	got := resp.Suggestions[0]
	if got.SessionID != candidate.ID || got.Action != "archive" || got.Confidence < 0.9 {
		t.Fatalf("unexpected archive suggestion: %+v", got)
	}
	if got.Title != "Old spike" || !strings.Contains(got.Reason, "Temporary") {
		t.Fatalf("expected enriched title and reason, got %+v", got)
	}
	if resp.ExcludedCount < 3 {
		t.Fatalf("expected protected sessions to be counted as excluded, got %+v", resp)
	}
}

func TestSessionCleanupSuggestionsAPIDeleteModeOnlyReturnsArchivedSessions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	archived := mustCreateCleanupSession(t, store, "Archived throwaway")
	active := mustCreateCleanupSession(t, store, "Active throwaway")
	if _, err := store.SetArchived(archived.ID, true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	old := time.Now().UTC().Add(-30 * 24 * time.Hour)
	rewriteSessionIndexForCleanupTest(t, root, func(index map[string]session.Session) {
		for id, sess := range index {
			sess.CreatedAt = old
			sess.UpdatedAt = old
			if id == archived.ID {
				archivedAt := old
				sess.ArchivedAt = &archivedAt
			}
			index[id] = sess
		}
	})

	router, clients, err := llm.NewFakeRouter(llm.TierStandard, map[llm.Role]llm.Tier{
		llm.RoleSessionCleanup: llm.TierLight,
	})
	if err != nil {
		t.Fatalf("fake router: %v", err)
	}
	clients[llm.TierLight].ChatResponse = llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: fmt.Sprintf(`{
		"suggestions": [
			{"session_id": %q, "action": "delete", "confidence": 0.88, "reason": "Archived empty scratch session."},
			{"session_id": %q, "action": "delete", "confidence": 0.95, "reason": "Active sessions must not be deleted."}
		]
	}`, archived.ID, active.ID)}}

	handler := newSessionAPIHandlerFullWithLLM(store, zerolog.Nop(), nil, sessionStyleValues{}, nil, nil, router)
	var resp sessionCleanupSuggestionResponse
	postSessionCleanupSuggestionsForTest(t, handler, `{"mode":"delete","limit":5}`, &resp)

	if resp.Mode != "delete" || resp.Action != "delete" {
		t.Fatalf("expected delete mode/action, got %+v", resp)
	}
	if resp.Count != 1 || len(resp.Suggestions) != 1 {
		t.Fatalf("expected one delete suggestion, got %+v", resp)
	}
	if resp.Suggestions[0].SessionID != archived.ID || resp.Suggestions[0].Action != "delete" {
		t.Fatalf("unexpected delete suggestion: %+v", resp.Suggestions[0])
	}
}

func TestSessionCleanupSuggestionsAPIDeleteModeAnalyzesRecentGreetingOnlyArchives(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	greeting := mustCreateCleanupSession(t, store, "안녕?")
	substantive := mustCreateCleanupSession(t, store, "릴리스 체크")
	if _, err := store.SetArchived(greeting.ID, true); err != nil {
		t.Fatalf("archive greeting: %v", err)
	}
	if _, err := store.SetArchived(substantive.ID, true); err != nil {
		t.Fatalf("archive substantive: %v", err)
	}
	recent := time.Now().UTC().Add(-50 * time.Minute)
	rewriteSessionIndexForCleanupTest(t, root, func(index map[string]session.Session) {
		for id, sess := range index {
			sess.CreatedAt = recent
			sess.UpdatedAt = recent
			archivedAt := recent
			sess.ArchivedAt = &archivedAt
			index[id] = sess
		}
	})
	if err := session.AppendMessage(store.TranscriptPath(greeting.ID), session.Message{Role: "user", Content: "안녕?"}); err != nil {
		t.Fatalf("append greeting user: %v", err)
	}
	if err := session.AppendMessage(store.TranscriptPath(greeting.ID), session.Message{Role: "assistant", Content: "안녕하세요. 무엇을 도와드릴까요?"}); err != nil {
		t.Fatalf("append greeting assistant: %v", err)
	}
	if err := session.AppendMessage(store.TranscriptPath(substantive.ID), session.Message{Role: "user", Content: "v0.32.9 릴리스 검증 결과와 Homebrew tap 상태를 비교해줘"}); err != nil {
		t.Fatalf("append substantive user: %v", err)
	}
	if err := session.AppendMessage(store.TranscriptPath(substantive.ID), session.Message{Role: "assistant", Content: "릴리스 검증 결과를 정리했습니다."}); err != nil {
		t.Fatalf("append substantive assistant: %v", err)
	}

	router, clients, err := llm.NewFakeRouter(llm.TierStandard, map[llm.Role]llm.Tier{
		llm.RoleSessionCleanup: llm.TierLight,
	})
	if err != nil {
		t.Fatalf("fake router: %v", err)
	}
	clients[llm.TierLight].ChatResponse = llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: fmt.Sprintf(`{
		"suggestions": [
			{"session_id": %q, "action": "delete", "confidence": 0.9, "reason": "Greeting-only archived conversation."},
			{"session_id": %q, "action": "delete", "confidence": 0.9, "reason": "Recently archived substantive work must stay protected."}
		]
	}`, greeting.ID, substantive.ID)}}

	handler := newSessionAPIHandlerFullWithLLM(store, zerolog.Nop(), nil, sessionStyleValues{}, nil, nil, router)
	var resp sessionCleanupSuggestionResponse
	postSessionCleanupSuggestionsForTest(t, handler, `{"mode":"delete","limit":5}`, &resp)

	if resp.AnalyzedCount != 1 {
		t.Fatalf("expected only the greeting archive to be analyzed, got %+v", resp)
	}
	if resp.Count != 1 || len(resp.Suggestions) != 1 {
		t.Fatalf("expected one greeting delete suggestion, got %+v", resp)
	}
	if resp.Suggestions[0].SessionID != greeting.ID {
		t.Fatalf("expected greeting suggestion, got %+v", resp.Suggestions[0])
	}
	if resp.ExcludedCount < 1 {
		t.Fatalf("expected substantive recent archive to stay protected, got %+v", resp)
	}
}

func TestSessionCleanupTrivialTranscriptClassification(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		summary sessionCleanupTranscript
		want    bool
	}{
		{
			name:  "empty session is trivial",
			title: "New Chat",
			want:  true,
		},
		{
			name:  "greeting punctuation is trivial",
			title: "안녕?",
			summary: sessionCleanupTranscript{
				messageCount: 2,
				firstUser:    "안녕?",
			},
			want: true,
		},
		{
			name:  "generic title with one tiny user message is trivial",
			title: "chat",
			summary: sessionCleanupTranscript{
				messageCount: 1,
				firstUser:    "test",
			},
			want: true,
		},
		{
			name:  "three messages are not trivial",
			title: "안녕?",
			summary: sessionCleanupTranscript{
				messageCount: 3,
				firstUser:    "안녕?",
			},
			want: false,
		},
		{
			name:  "substantive recent archive is not trivial",
			title: "릴리스 체크",
			summary: sessionCleanupTranscript{
				messageCount: 2,
				firstUser:    "v0.32.9 릴리스 검증 결과와 Homebrew tap 상태를 비교해줘",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTrivialSessionCleanupTranscript(tt.title, tt.summary); got != tt.want {
				t.Fatalf("isTrivialSessionCleanupTranscript()=%v want %v", got, tt.want)
			}
		})
	}
	if compactCleanupText(" 안녕?! ") != "안녕" {
		t.Fatalf("expected punctuation-stripped greeting, got %q", compactCleanupText(" 안녕?! "))
	}
	if cleanupTextRuneLen(" 안녕? ") != 3 {
		t.Fatalf("expected trimmed rune length 3, got %d", cleanupTextRuneLen(" 안녕? "))
	}
}

func TestSessionCleanupSuggestionsAPIRequiresLLMRouter(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	handler := newSessionAPIHandlerFullWithLLM(store, zerolog.Nop(), nil, sessionStyleValues{}, nil, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/sessions/cleanup-suggestions", strings.NewReader(`{"mode":"archive"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without LLM router, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSessionCleanupSuggestionsAPIGuardsRequestBoundaries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	router, _, err := fakeCleanupRouterForTest()
	if err != nil {
		t.Fatalf("fake router: %v", err)
	}
	handler := newSessionAPIHandlerFullWithLLM(store, zerolog.Nop(), nil, sessionStyleValues{}, nil, nil, router)

	assertSessionCleanupStatusForTest(t, handler, http.MethodPost, `{"mode":"archive"}`, "", http.StatusForbidden)
	assertSessionCleanupStatusForTest(t, handler, http.MethodGet, ``, "admin", http.StatusMethodNotAllowed)
	assertSessionCleanupStatusForTest(t, handler, http.MethodPost, `{`, "admin", http.StatusBadRequest)
	assertSessionCleanupStatusForTest(t, handler, http.MethodPost, `{"mode":"destroy"}`, "admin", http.StatusBadRequest)
}

func TestSessionCleanupSuggestionsAPIReturns500ForMalformedLLMResponse(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	mustCreateCleanupSession(t, store, "Old malformed response")
	old := time.Now().UTC().Add(-14 * 24 * time.Hour)
	rewriteSessionIndexForCleanupTest(t, root, func(index map[string]session.Session) {
		for id, sess := range index {
			sess.CreatedAt = old
			sess.UpdatedAt = old
			index[id] = sess
		}
	})
	router, clients, err := fakeCleanupRouterForTest()
	if err != nil {
		t.Fatalf("fake router: %v", err)
	}
	clients[llm.TierLight].ChatResponse = llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: `not json`}}
	handler := newSessionAPIHandlerFullWithLLM(store, zerolog.Nop(), nil, sessionStyleValues{}, nil, nil, router)

	assertSessionCleanupStatusForTest(t, handler, http.MethodPost, `{"mode":"archive"}`, "admin", http.StatusInternalServerError)
}

func mustCreateCleanupSession(t *testing.T, store *session.Store, title string) session.Session {
	t.Helper()
	sess, err := store.Create(title)
	if err != nil {
		t.Fatalf("create %q: %v", title, err)
	}
	return sess
}

func fakeCleanupRouterForTest() (llm.Router, map[llm.Tier]*llm.FakeClient, error) {
	return llm.NewFakeRouter(llm.TierStandard, map[llm.Role]llm.Tier{
		llm.RoleSessionCleanup: llm.TierLight,
	})
}

func rewriteSessionIndexForCleanupTest(t *testing.T, root string, apply func(map[string]session.Session)) {
	t.Helper()
	path := filepath.Join(root, "sessions", "sessions.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sessions index: %v", err)
	}
	var index map[string]session.Session
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatalf("decode sessions index: %v", err)
	}
	apply(index)
	encoded, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		t.Fatalf("encode sessions index: %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("write sessions index: %v", err)
	}
}

func postSessionCleanupSuggestionsForTest(t *testing.T, handler http.Handler, body string, out *sessionCleanupSuggestionResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/sessions/cleanup-suggestions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected suggestions 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func assertSessionCleanupStatusForTest(t *testing.T, handler http.Handler, method string, body string, role string, want int) {
	t.Helper()
	req := httptest.NewRequest(method, "/v1/admin/sessions/cleanup-suggestions", strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if role != "" {
		req.Header.Set("Tars-Debug-Auth-Role", role)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("expected status %d, got %d body=%q", want, rec.Code, rec.Body.String())
	}
}
