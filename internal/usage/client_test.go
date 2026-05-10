package usage

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

type fakeCodexClient struct {
	llm.FakeClient
	snap CodexSnap
}

type CodexSnap struct {
	llm.CodexRateLimitSnapshot
	present bool
}

func (f *fakeCodexClient) LastCodexRateLimit() (llm.CodexRateLimitSnapshot, bool) {
	return f.snap.CodexRateLimitSnapshot, f.snap.present
}

func TestTrackedClient_LastCodexRateLimit_Passthrough(t *testing.T) {
	primary := &llm.CodexRateLimitWindow{UsedPercent: 50, WindowMinutes: 300}
	inner := &fakeCodexClient{
		FakeClient: llm.FakeClient{Label: "standard"},
		snap: CodexSnap{
			CodexRateLimitSnapshot: llm.CodexRateLimitSnapshot{Primary: primary},
			present:                true,
		},
	}
	tracked := NewTrackedClient(inner, nil, "openai-codex", "gpt-5.3-codex", llm.TierStandard)

	got, ok := tracked.LastCodexRateLimit()
	if !ok {
		t.Fatal("expected snapshot to be present")
	}
	if got.Primary == nil || got.Primary.UsedPercent != 50 {
		t.Errorf("primary mismatch: %+v", got.Primary)
	}
}

func TestTrackedClient_LastCodexRateLimit_NonCodexInner(t *testing.T) {
	tracked := NewTrackedClient(&llm.FakeClient{Label: "x"}, nil, "anthropic", "claude", llm.TierHeavy)
	if _, ok := tracked.LastCodexRateLimit(); ok {
		t.Error("expected ok=false when inner does not implement CodexRateLimitSource")
	}
}

func TestTrackedClient_LastCodexRateLimit_NilSafe(t *testing.T) {
	var tracked *TrackedClient
	if _, ok := tracked.LastCodexRateLimit(); ok {
		t.Error("expected ok=false on nil receiver")
	}
}

func TestTrackedClient_LogsSelectionMetadata(t *testing.T) {
	var buf bytes.Buffer
	prev := zlog.Logger
	zlog.Logger = zerolog.New(&buf).Level(zerolog.DebugLevel)
	defer func() {
		zlog.Logger = prev
	}()

	client := NewTrackedClient(&llm.FakeClient{Label: "standard"}, nil, "openai", "gpt-5.4", llm.TierStandard)
	ctx := llm.WithSelectionMetadata(context.Background(), llm.SelectionMetadata{
		Role:      llm.RoleAgentRuntimeDefault,
		Tier:      llm.TierStandard,
		Provider:  "openai",
		Model:     "gpt-5.4",
		Source:    "role",
		SessionID: "sess-1",
		RunID:     "run-1",
		AgentName: "explorer",
		FlowID:    "flow-1",
		StepID:    "step-1",
	})

	if _, err := client.Chat(ctx, []llm.ChatMessage{{Role: "user", Content: "hello"}}, llm.ChatOptions{}); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	logs := buf.String()
	for _, want := range []string{
		`"message":"llm selection"`,
		`"tier":"standard"`,
		`"provider":"openai"`,
		`"model":"gpt-5.4"`,
		`"role":"agentruntime_default"`,
		`"source":"role"`,
		`"session_id":"sess-1"`,
		`"run_id":"run-1"`,
		`"agent_name":"explorer"`,
		`"flow_id":"flow-1"`,
		`"step_id":"step-1"`,
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected log %q in %s", want, logs)
		}
	}
}
