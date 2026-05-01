package autofix

import (
	"context"
	"testing"
)

type fakeChatAutoContinuer struct {
	result ChatAutoContinueResult
	err    error
	calls  int
}

func (f *fakeChatAutoContinuer) AutoContinueStalledChats(ctx context.Context) (ChatAutoContinueResult, error) {
	f.calls++
	return f.result, f.err
}

func TestAutoContinueChatRunsConfiguredContinuer(t *testing.T) {
	continuer := &fakeChatAutoContinuer{
		result: ChatAutoContinueResult{
			Resumed:    1,
			Skipped:    2,
			Escalated:  1,
			SessionIDs: []string{"sess_1"},
		},
	}
	fixer := &AutoContinueChat{Continuer: continuer}

	result, err := fixer.Run(context.Background())
	if err != nil {
		t.Fatalf("run auto continue: %v", err)
	}
	if continuer.calls != 1 {
		t.Fatalf("calls = %d, want 1", continuer.calls)
	}
	if !result.Changed {
		t.Fatalf("expected changed result, got %+v", result)
	}
	if result.Name != AutoContinueChatName {
		t.Fatalf("name = %q", result.Name)
	}
	if result.Details["resumed"] != 1 {
		t.Fatalf("details = %+v", result.Details)
	}
}
