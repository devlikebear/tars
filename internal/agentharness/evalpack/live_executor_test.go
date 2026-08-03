package evalpack

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
)

type fakeLiveClient struct {
	response  llm.ChatResponse
	err       error
	emitDelta bool
	delay     time.Duration
	messages  []llm.ChatMessage
}

func (f *fakeLiveClient) Ask(context.Context, string) (string, error) {
	return "", errors.New("not implemented")
}

func (f *fakeLiveClient) Chat(_ context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	f.messages = append([]llm.ChatMessage(nil), messages...)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.emitDelta && opts.OnDelta != nil {
		opts.OnDelta("first")
	}
	return f.response, f.err
}

func TestLiveExecutorRecordsStreamUsageAndCost(t *testing.T) {
	client := &fakeLiveClient{
		emitDelta: true,
		response: llm.ChatResponse{
			Message: llm.ChatMessage{Role: "assistant", Content: "done LIVE_OK"},
			Usage:   llm.Usage{InputTokens: 250, OutputTokens: 100},
		},
	}
	executor := LiveExecutor{Client: client, InputCostPerMillion: 2, OutputCostPerMillion: 8}
	metrics, details, err := executor.ExecuteDetailed(context.Background(), Scenario{
		Prompt: "Return LIVE_OK", SuccessToken: "LIVE_OK",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !metrics.TaskSuccess || !metrics.VerifierPass || metrics.TTFTSource != "provider_stream" {
		t.Fatalf("unexpected quality metrics: %+v", metrics)
	}
	if metrics.InputTokens != 250 || metrics.OutputTokens != 100 || metrics.EstimatedCostUSD != 0.0013 {
		t.Fatalf("unexpected usage metrics: %+v", metrics)
	}
	if len(client.messages) != 2 || client.messages[0].Role != "system" || !strings.Contains(client.messages[1].Content, "LIVE_OK") {
		t.Fatalf("unexpected messages: %+v", client.messages)
	}
	if !strings.Contains(details, "never gate CI") {
		t.Fatalf("unexpected details: %q", details)
	}
}

func TestLiveExecutorUsesCompletionLatencyWithoutDelta(t *testing.T) {
	client := &fakeLiveClient{
		delay:    2 * time.Millisecond,
		response: llm.ChatResponse{Message: llm.ChatMessage{Content: "missing token"}},
	}
	metrics, err := (LiveExecutor{Client: client}).Execute(context.Background(), Scenario{SuccessToken: "EXPECTED"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if metrics.TaskSuccess || metrics.VerifierPass || metrics.TTFTMillis < 1 {
		t.Fatalf("unexpected fallback metrics: %+v", metrics)
	}
}

func TestLiveExecutorReportsConfigurationAndProviderErrors(t *testing.T) {
	if _, _, err := (LiveExecutor{}).ExecuteDetailed(context.Background(), Scenario{}); err == nil {
		t.Fatal("expected missing client error")
	}
	want := errors.New("provider unavailable")
	client := &fakeLiveClient{err: want}
	if _, _, err := (LiveExecutor{Client: client}).ExecuteDetailed(context.Background(), Scenario{}); !errors.Is(err, want) {
		t.Fatalf("expected provider error, got %v", err)
	}
}
