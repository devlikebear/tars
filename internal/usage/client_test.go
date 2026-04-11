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

func TestTrackedClient_LogsSelectionMetadata(t *testing.T) {
	var buf bytes.Buffer
	prev := zlog.Logger
	zlog.Logger = zerolog.New(&buf).Level(zerolog.DebugLevel)
	defer func() {
		zlog.Logger = prev
	}()

	client := NewTrackedClient(&llm.FakeClient{Label: "standard"}, nil, "openai", "gpt-5.4", llm.TierStandard)
	ctx := llm.WithSelectionMetadata(context.Background(), llm.SelectionMetadata{
		Role:      llm.RoleGatewayDefault,
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
		`"role":"gateway_default"`,
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
