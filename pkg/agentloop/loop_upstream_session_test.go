package agentloop

import (
	"context"
	"errors"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/tool"
)

// sessionThenFailClient answers the first call with an upstream session and
// fails every later one with err.
type sessionThenFailClient struct {
	calls int
	err   error
}

func (c *sessionThenFailClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *sessionThenFailClient) Chat(context.Context, []llm.ChatMessage, llm.ChatOptions) (llm.ChatResponse, error) {
	c.calls++
	if c.calls == 1 {
		return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "part one"}, SessionID: "sess-1"}, nil
	}
	return llm.ChatResponse{}, c.err
}

func runContinuingOnce(t *testing.T, client llm.Client, opts RunOptions) error {
	t.Helper()
	injected := false
	opts.OnTurnEnd = func(context.Context, llm.ChatResponse) (string, error) {
		if injected {
			return "", nil
		}
		injected = true
		return "keep going", nil
	}
	_, err := NewLoop(client, tool.NewRegistry()).Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "go"}}, opts)
	return err
}

// A later iteration that fails leaves the session an earlier one saved: the
// caller learns it from the error and resumes it next turn.
func TestLoop_Run_FailedIterationReportsSavedSession(t *testing.T) {
	cause := errors.New("cli timed out")
	err := runContinuingOnce(t, &sessionThenFailClient{err: cause}, RunOptions{PersistUpstreamSession: true})
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want the call's error", err)
	}
	if got := llm.UpstreamSessionIDFromError(err); got != "sess-1" {
		t.Fatalf("session on error = %q, want sess-1", got)
	}
}

// The provider's own report wins: it names the session the failed call
// actually used.
func TestLoop_Run_ProviderReportedSessionWins(t *testing.T) {
	cause := &llm.UpstreamSessionError{SessionID: "sess-2", Err: errors.New("boom")}
	err := runContinuingOnce(t, &sessionThenFailClient{err: cause}, RunOptions{PersistUpstreamSession: true})
	if got := llm.UpstreamSessionIDFromError(err); got != "sess-2" {
		t.Fatalf("session on error = %q, want sess-2", got)
	}
}

// Without persistence the earlier session was never saved.
func TestLoop_Run_FailedIterationWithoutPersistenceReportsNoSession(t *testing.T) {
	err := runContinuingOnce(t, &sessionThenFailClient{err: errors.New("boom")}, RunOptions{})
	if got := llm.UpstreamSessionIDFromError(err); got != "" {
		t.Fatalf("session on error = %q, want none", got)
	}
}
