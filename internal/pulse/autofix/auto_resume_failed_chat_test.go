package autofix

import (
	"context"
	"errors"
	"testing"
)

type fakeFailedChatResumer struct {
	result FailedChatResumeResult
	err    error
	calls  int
}

func (f *fakeFailedChatResumer) ResumeFailedChats(ctx context.Context) (FailedChatResumeResult, error) {
	f.calls++
	return f.result, f.err
}

func TestAutoResumeFailedChat_RunsConfiguredResumer(t *testing.T) {
	resumer := &fakeFailedChatResumer{
		result: FailedChatResumeResult{Resumed: 2, Skipped: 1, SessionIDs: []string{"sess_a", "sess_b"}},
	}
	fixer := &AutoResumeFailedChat{Resumer: resumer}

	got, err := fixer.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if resumer.calls != 1 {
		t.Fatalf("calls = %d, want 1", resumer.calls)
	}
	if !got.Changed {
		t.Fatalf("expected changed result: %+v", got)
	}
	if got.Name != AutoResumeFailedChatName {
		t.Fatalf("name = %q", got.Name)
	}
	if got.Details["resumed"] != 2 {
		t.Fatalf("details = %+v", got.Details)
	}
}

func TestAutoResumeFailedChat_NotConfigured(t *testing.T) {
	fixer := &AutoResumeFailedChat{}
	if _, err := fixer.Run(context.Background()); err == nil {
		t.Fatalf("expected error when resumer is nil")
	}
}

func TestAutoResumeFailedChat_PropagatesResumerError(t *testing.T) {
	resumer := &fakeFailedChatResumer{err: errors.New("boom")}
	fixer := &AutoResumeFailedChat{Resumer: resumer}
	if _, err := fixer.Run(context.Background()); err == nil {
		t.Fatalf("expected resumer error to propagate")
	}
}
