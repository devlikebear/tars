package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
)

// failThenResumeClient fails its first call after the upstream session was
// saved, then answers, recording the resume ID of every call.
type failThenResumeClient struct {
	resumeIDs []string
	firstErr  error
}

func (c *failThenResumeClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *failThenResumeClient) Chat(_ context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.resumeIDs = append(c.resumeIDs, opts.ResumeSessionID)
	if len(c.resumeIDs) == 1 {
		return llm.ChatResponse{}, c.firstErr
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "picked up"}, SessionID: opts.ResumeSessionID}, nil
}

func postChatTo(t *testing.T, handler http.Handler, body string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Body.String()
}

// A first turn that times out after the CLI saved its session keeps that
// session: the next turn resumes it instead of starting over.
func TestChatAPI_FailedTurnKeepsUpstreamSessionForNextTurn(t *testing.T) {
	client := &failThenResumeClient{firstErr: &llm.UpstreamSessionError{
		SessionID: "sess-saved",
		Err:       errors.New("claude-code-cli request: cli timed out: no output for 15m0s"),
	}}
	handler, store := newLiveProviderToolHandler(t, client)

	if body := postChatTo(t, handler, `{"message":"read the code"}`); !strings.Contains(body, "timed out") {
		t.Fatalf("expected the timeout on the stream, got %q", body)
	}
	sessions, err := store.List()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %+v, err %v", sessions, err)
	}
	if got := sessions[0].UpstreamSessionID; got != "sess-saved" {
		t.Fatalf("UpstreamSessionID = %q, want sess-saved", got)
	}

	postChatTo(t, handler, `{"session_id":"`+sessions[0].ID+`","message":"continue"}`)
	if len(client.resumeIDs) != 2 || client.resumeIDs[1] != "sess-saved" {
		t.Fatalf("resume IDs = %q, want the second turn to resume sess-saved", client.resumeIDs)
	}
}

// A failure that saved nothing leaves the stored session alone.
func TestChatAPI_FailedTurnWithoutSessionKeepsStoredID(t *testing.T) {
	client := &failThenResumeClient{firstErr: errors.New("boom")}
	handler, store := newLiveProviderToolHandler(t, client)
	postChatTo(t, handler, `{"message":"hi"}`)
	sessions, _ := store.List()
	if len(sessions) != 1 || sessions[0].UpstreamSessionID != "" {
		t.Fatalf("sessions = %+v, want no upstream session", sessions)
	}
}
