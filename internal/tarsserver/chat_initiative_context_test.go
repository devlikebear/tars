package tarsserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// TestMostRecentUnansweredInitiativeMessage covers the pure lookup in
// isolation: an initiative message found before any user message is
// unanswered; one found after a user message already replied is not.
func TestMostRecentUnansweredInitiativeMessage(t *testing.T) {
	store := session.NewStore(t.TempDir())
	sess, err := store.Create("s")
	if err != nil {
		t.Fatal(err)
	}
	path := store.TranscriptPath(sess.ID)

	note, err := mostRecentUnansweredInitiativeMessage(path)
	if err != nil || note != nil {
		t.Fatalf("empty transcript: note=%v err=%v", note, err)
	}

	if err := session.AppendMessage(path, session.Message{
		Role: "assistant", Content: "Welcome back!", Timestamp: time.Now(),
		Initiative: &session.MessageInitiative{Intent: "greet", EntryID: "e1"},
	}); err != nil {
		t.Fatal(err)
	}
	note, err = mostRecentUnansweredInitiativeMessage(path)
	if err != nil || note == nil || note.Content != "Welcome back!" {
		t.Fatalf("after initiative message: note=%v err=%v", note, err)
	}

	if err := session.AppendMessage(path, session.Message{Role: "user", Content: "thanks", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	note, err = mostRecentUnansweredInitiativeMessage(path)
	if err != nil || note != nil {
		t.Fatalf("after the user replied: note=%v err=%v", note, err)
	}
}

func TestAppendInitiativeContextSkipsSlashCommandsAndNilNote(t *testing.T) {
	if got := appendInitiativeContext("/goal do the thing", &session.Message{Content: "hi"}); got != "/goal do the thing" {
		t.Fatalf("slash command must stay untouched, got %q", got)
	}
	if got := appendInitiativeContext("hello", nil); got != "hello" {
		t.Fatalf("nil note must leave the message untouched, got %q", got)
	}
	note := &session.Message{Content: "Welcome back!", Timestamp: time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)}
	got := appendInitiativeContext("thanks", note)
	if !strings.HasPrefix(got, "thanks\n\n"+initiativeContextOpen) || !strings.HasSuffix(got, initiativeContextClose) ||
		!strings.Contains(got, "Welcome back!") {
		t.Fatalf("block = %q", got)
	}
}

func TestMaybeAppendInitiativeContextGatesOnProviderAndUpstreamSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	sess, err := store.Create("s")
	if err != nil {
		t.Fatal(err)
	}
	path := store.TranscriptPath(sess.ID)
	if err := session.AppendMessage(path, session.Message{
		Role: "assistant", Content: "Welcome back!", Timestamp: time.Now(),
		Initiative: &session.MessageInitiative{Intent: "greet"},
	}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		kind       string
		upstreamID string
		wantBlock  bool
	}{
		{"resume provider with upstream session", "claude-code-cli", "sess-1", true},
		{"antigravity-cli also resumes", "antigravity-cli", "sess-1", true},
		{"native provider never resumes", "anthropic", "sess-1", false},
		{"resume provider but no upstream session yet", "claude-code-cli", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := maybeAppendInitiativeContext("thanks", path, c.kind, c.upstreamID, zerolog.Nop())
			hasBlock := strings.Contains(got, initiativeContextOpen)
			if hasBlock != c.wantBlock {
				t.Fatalf("got %q, want block=%v", got, c.wantBlock)
			}
		})
	}
}

// fakeKindRouter is a minimal llm.Router whose resolved TierResolution
// reports a fixed provider kind, so handler-level tests can force the
// resume/native branch maybeAppendInitiativeContext takes.
type fakeKindRouter struct {
	client llm.Client
	kind   string
}

func (r *fakeKindRouter) ClientFor(llm.Role) (llm.Client, llm.TierResolution, error) {
	return r.client, llm.TierResolution{Provider: r.kind}, nil
}
func (r *fakeKindRouter) ClientForTier(llm.Tier) (llm.Client, llm.TierResolution, error) {
	return r.client, llm.TierResolution{Provider: r.kind}, nil
}
func (r *fakeKindRouter) TierForRole(llm.Role) llm.Tier { return llm.TierStandard }
func (r *fakeKindRouter) DefaultTier() llm.Tier         { return llm.TierStandard }

// recordingResumeClient records every Chat call's last user message
// content and the ResumeSessionID it was given, and answers with a fixed
// reply while reporting SessionID so the turn can persist an upstream
// session like claude-code-cli does.
type recordingResumeClient struct {
	calls []struct {
		lastUserContent string
		resumeID        string
	}
}

func (c *recordingResumeClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *recordingResumeClient) Chat(_ context.Context, msgs []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	last := ""
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			last = msgs[i].Content
			break
		}
	}
	c.calls = append(c.calls, struct {
		lastUserContent string
		resumeID        string
	}{last, opts.ResumeSessionID})
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "got it"}, SessionID: "sess-upstream"}, nil
}

// TestChatAPI_InitiativeContextRidesOnceOnAResumingProvidersNextTurn is the
// handler-level proof for tars#1220's hidden-context requirement: a
// session already resuming an upstream session (claude-code-cli) with an
// unanswered initiative message gets the hidden block on the next user
// turn's message (not the whole transcript — ResumeSessionID stays set),
// and never again on the turn after that.
func TestChatAPI_InitiativeContextRidesOnceOnAResumingProvidersNextTurn(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("s")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.AppendMessage(store.TranscriptPath(sess.ID), session.Message{
		Role: "assistant", Content: "Welcome back!", Timestamp: time.Now(),
		Initiative: &session.MessageInitiative{Intent: "greet", EntryID: "e1"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetUpstreamSessionID(sess.ID, "sess-upstream"); err != nil {
		t.Fatal(err)
	}

	client := &recordingResumeClient{}
	router := &fakeKindRouter{client: client, kind: "claude-code-cli"}
	handler := newChatAPIHandlerWithRuntimeConfig(root, store, nil, router, zerolog.Nop(), 2, nil, "", defaultChatToolingOptions())

	post := func(msg string) {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(
			`{"session_id":"`+sess.ID+`","message":"`+msg+`"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	post("thanks")
	if len(client.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(client.calls))
	}
	if !strings.Contains(client.calls[0].lastUserContent, initiativeContextOpen) || !strings.Contains(client.calls[0].lastUserContent, "Welcome back!") {
		t.Fatalf("first turn message = %q, want the hidden block", client.calls[0].lastUserContent)
	}
	if client.calls[0].resumeID != "sess-upstream" {
		t.Fatalf("resumeID = %q, want sess-upstream (resume, not full replay)", client.calls[0].resumeID)
	}

	post("and again")
	if len(client.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(client.calls))
	}
	if strings.Contains(client.calls[1].lastUserContent, initiativeContextOpen) {
		t.Fatalf("second turn message = %q, must not repeat the hidden block", client.calls[1].lastUserContent)
	}
}

// TestChatAPI_InitiativeContextNeverAppendsForNativeProvider checks a
// native provider (which already replays the full transcript) never gets
// the hidden block, even with an unanswered initiative message and an
// upstream session id on the session.
func TestChatAPI_InitiativeContextNeverAppendsForNativeProvider(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("s")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.AppendMessage(store.TranscriptPath(sess.ID), session.Message{
		Role: "assistant", Content: "Welcome back!", Timestamp: time.Now(),
		Initiative: &session.MessageInitiative{Intent: "greet"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetUpstreamSessionID(sess.ID, "sess-upstream"); err != nil {
		t.Fatal(err)
	}

	client := &recordingResumeClient{}
	router := &fakeKindRouter{client: client, kind: "anthropic"}
	handler := newChatAPIHandlerWithRuntimeConfig(root, store, nil, router, zerolog.Nop(), 2, nil, "", defaultChatToolingOptions())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(
		`{"session_id":"`+sess.ID+`","message":"thanks"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if len(client.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(client.calls))
	}
	if strings.Contains(client.calls[0].lastUserContent, initiativeContextOpen) {
		t.Fatalf("native provider must never get the hidden block, got %q", client.calls[0].lastUserContent)
	}
}
