package tarsserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/usage"
)

// spentThenFailClient fails like a claude-code-cli call cut off by its idle
// timeout after 35 API requests' worth of work: the error carries what the
// requests spent and the session they saved.
type spentThenFailClient struct{}

func (spentThenFailClient) Ask(context.Context, string) (string, error) { return "", nil }

func (spentThenFailClient) Chat(context.Context, []llm.ChatMessage, llm.ChatOptions) (llm.ChatResponse, error) {
	spent := llm.Usage{InputTokens: 120, OutputTokens: 9_000, CacheWriteTokens: 93_092, CacheReadTokens: 2_642_486}
	return llm.ChatResponse{}, &llm.UpstreamSessionError{SessionID: "eb0f0abe", Err: &llm.PartialUsageError{
		Usage:   spent,
		ByModel: map[string]llm.Usage{"claude-sonnet-4-6": spent},
		Err:     errors.New("claude-code-cli request: cli timed out: no output for 5m0s"),
	}}
}

// A turn that ends in an error still counts what it spent toward the
// session's usage and cost.
func TestChatAPI_FailedTurnRecordsSpentUsage(t *testing.T) {
	tracker, err := usage.NewTracker(t.TempDir(), usage.TrackerOptions{})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	client := usage.NewTrackedClient(spentThenFailClient{}, tracker, "claude-code-cli", "sonnet", llm.TierStandard)
	handler, store := newLiveProviderToolHandler(t, client)

	if body := postChatTo(t, handler, `{"message":"fix the tests"}`); !strings.Contains(body, "timed out") {
		t.Fatalf("expected the timeout on the stream, got %q", body)
	}
	sessions, err := store.List()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %+v, err %v", sessions, err)
	}
	summary, err := tracker.SummaryFiltered("month", "provider", usage.SummaryFilter{SessionID: sessions[0].ID})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.TotalCalls != 1 || summary.TotalInput != 120 || summary.TotalOutput != 9_000 {
		t.Fatalf("session usage = %+v", summary)
	}
	// Sonnet rates: 120*3 + 9000*15 + 2,642,486*0.30 + 93,092*3.75 per 1M ≈ $1.278.
	if summary.TotalCostUSD < 1.27 || summary.TotalCostUSD > 1.29 {
		t.Fatalf("session cost = %v, want the estimate ≈ 1.278", summary.TotalCostUSD)
	}
}
