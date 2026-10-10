package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// initiativeLLMFixture builds a long-session observation (the one Decide
// branch that depends on text without needing a console/Telegram
// reachability signal) backed by a real session.Store, so the whole path
// — observer -> RenderState -> router -> provider -> parse -> Decide ->
// ledger/status — runs end to end through a real subprocess-free HTTP
// provider, the same pattern computer_use_llm_e2e_test.go uses.
func initiativeLLMFixture(t *testing.T, dir string, now time.Time) *session.Store {
	t.Helper()
	store := session.NewStore(dir)
	sess, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	path := store.TranscriptPath(sess.ID)
	// Seven messages 30 minutes apart span exactly 3h (the long-session
	// threshold) and the last one is 10 minutes ago (within the
	// long-session tail), with private-looking content a leak check can
	// grep for.
	offsets := []time.Duration{190, 160, 130, 100, 70, 40, 10}
	for i, m := range offsets {
		msg := session.Message{Role: "user", Content: "PRIVATE_MARKER_" + string(rune('A'+i)), Timestamp: now.Add(-m * time.Minute)}
		if err := session.AppendMessage(path, msg); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Touch(sess.ID, now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return store
}

func initiativeLLMConfig(dir, lightProviderAlias string) config.Config {
	return config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: dir},
		Initiative: config.InitiativeConfig{
			Enabled: true, Mode: "shadow", Backend: "llm",
			Tick: "1m", QuietHours: "00:00-00:00", DailyCap: 6, Cooldown: "45m", DailyTextCalls: 60,
		},
		LLMConfig: config.LLMConfig{
			LLMDefaultTier:  "standard",
			LLMRoleDefaults: map[string]string{"initiative": "light"},
			LLMProviders: map[string]config.LLMProviderSettings{
				"shared": {Kind: "openai", BaseURL: "http://unused.invalid", APIKey: "k"},
				"other":  {Kind: "openai", BaseURL: "http://unused.invalid", APIKey: "k2"},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"heavy":    {Provider: "shared", Model: "heavy-model"},
				"standard": {Provider: "shared", Model: "chat-model"},
				"light":    {Provider: lightProviderAlias, Model: "light-model"},
			},
		},
	}
}

// openAIStyleTextSignalServer answers every chat completion with the given
// JSON content, counting requests and recording the last request's model
// and final user message.
type openAIStyleTextSignalServer struct {
	calls      atomic.Int32
	lastModel  string
	lastUser   string
	jsonAnswer string
}

func (s *openAIStyleTextSignalServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.calls.Add(1)
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.lastModel = req.Model
		if n := len(req.Messages); n > 0 {
			s.lastUser = req.Messages[n-1].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]string{"role": "assistant", "content": s.jsonAnswer},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 50, "completion_tokens": 10},
		})
	}
}

func TestInitiativeLLMBackendEndToEnd_SameProviderReadsText(t *testing.T) {
	srv := &openAIStyleTextSignalServer{jsonAnswer: `{"quiet_requested":true,"user_strained":false,"special_day":false}`}
	server := httptest.NewServer(srv.handler())
	defer server.Close()

	dir := t.TempDir()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	store := initiativeLLMFixture(t, dir, now)

	cfg := initiativeLLMConfig(dir, "shared")
	cfg.LLMProviders["shared"] = config.LLMProviderSettings{Kind: "openai", BaseURL: server.URL, APIKey: "k"}
	router, err := buildLLMRouter(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	setup := buildInitiativeRuntime(initiativeSetupInputs{
		Config: cfg, WorkspaceDir: dir, SessionStore: store, Router: router,
		Logger: zerolog.Nop(), Now: func() time.Time { return now },
	})
	if setup.Runtime == nil {
		t.Fatal("expected a runtime")
	}

	entry := setup.Runtime.RunOnce(context.Background())
	if entry.Text.Source != "llm" {
		t.Fatalf("entry.Text.Source = %q, want llm", entry.Text.Source)
	}
	if !entry.Text.QuietRequested || entry.Reason != "quiet_requested" || entry.Intent != "none" {
		t.Fatalf("the backend's quiet_requested=true must override long_session: entry = %+v", entry)
	}
	if srv.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", srv.calls.Load())
	}
	if srv.lastModel != "light-model" {
		t.Fatalf("model = %q, want light-model", srv.lastModel)
	}
	if !strings.Contains(srv.lastUser, "PRIVATE_MARKER_") {
		t.Fatal("same-provider backend must receive the user's own text")
	}

	// A second, unchanged tick must not call the provider again (hash
	// cache), proving the "no change, no call" guarantee at the server
	// path, not just inside the initiative package's own unit tests.
	setup.Runtime.RunOnce(context.Background())
	if srv.calls.Load() != 1 {
		t.Fatalf("an unchanged tick must not call the provider again, calls = %d", srv.calls.Load())
	}

	// Status and the ledger file must agree, and the ledger must never
	// hold the user's own words.
	rec := httptest.NewRecorder()
	setup.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/initiative/status", nil))
	var status struct {
		Backend struct {
			Backend      string `json:"backend"`
			Kind         string `json:"kind"`
			SendsText    bool   `json:"sends_text"`
			TextReason   string `json:"text_reason"`
			CallsToday   int    `json:"calls_today"`
			DailyCallCap int    `json:"daily_call_cap"`
		} `json:"backend"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v, body=%s", err, rec.Body.String())
	}
	if status.Backend.Backend != "llm" || status.Backend.Kind != "openai" || !status.Backend.SendsText ||
		status.Backend.TextReason != "llm_same_provider" || status.Backend.CallsToday != 1 || status.Backend.DailyCallCap != 60 {
		t.Fatalf("status.backend = %+v", status.Backend)
	}

	ledgerPath := filepath.Join(dir, "_shared", "initiative", "ledger.jsonl")
	raw, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if strings.Contains(string(raw), "PRIVATE_MARKER") {
		t.Fatal("ledger must never hold the user's own words")
	}
	if !strings.Contains(string(raw), `"called":true`) {
		t.Fatalf("ledger must record the call: %s", raw)
	}
}

func TestInitiativeLLMBackendEndToEnd_DifferentProviderNeverCalled(t *testing.T) {
	srv := &openAIStyleTextSignalServer{jsonAnswer: `{"quiet_requested":true,"user_strained":false,"special_day":false}`}
	server := httptest.NewServer(srv.handler())
	defer server.Close()

	dir := t.TempDir()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	store := initiativeLLMFixture(t, dir, now)

	// light tier points at "other" (reachable through the same server,
	// but a *different* provider pool alias than chat's "shared") so the
	// request would succeed if made — proving the skip is the alias
	// comparison, not a network failure.
	cfg := initiativeLLMConfig(dir, "other")
	cfg.LLMProviders["shared"] = config.LLMProviderSettings{Kind: "openai", BaseURL: server.URL, APIKey: "k"}
	cfg.LLMProviders["other"] = config.LLMProviderSettings{Kind: "openai", BaseURL: server.URL, APIKey: "k2"}
	router, err := buildLLMRouter(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	setup := buildInitiativeRuntime(initiativeSetupInputs{
		Config: cfg, WorkspaceDir: dir, SessionStore: store, Router: router,
		Logger: zerolog.Nop(), Now: func() time.Time { return now },
	})
	entry := setup.Runtime.RunOnce(context.Background())
	if entry.Text.Source != "skipped" {
		t.Fatalf("entry.Text.Source = %q, want skipped", entry.Text.Source)
	}
	if entry.Reason != "long_session" {
		t.Fatalf("without text, long_session must still check in: entry = %+v", entry)
	}
	if srv.calls.Load() != 0 {
		t.Fatalf("a different provider alias must never be called, calls = %d", srv.calls.Load())
	}

	rec := httptest.NewRecorder()
	setup.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/initiative/status", nil))
	var status struct {
		Backend struct {
			SendsText  bool   `json:"sends_text"`
			TextReason string `json:"text_reason"`
			CallsToday int    `json:"calls_today"`
		} `json:"backend"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v, body=%s", err, rec.Body.String())
	}
	if status.Backend.SendsText || status.Backend.TextReason != "llm_other_provider" || status.Backend.CallsToday != 0 {
		t.Fatalf("status.backend = %+v", status.Backend)
	}
}
