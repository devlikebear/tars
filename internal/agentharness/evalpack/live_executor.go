package evalpack

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/llm"
)

type LiveExecutor struct {
	Client               llm.Client
	InputCostPerMillion  float64
	OutputCostPerMillion float64
}

func (e LiveExecutor) Execute(ctx context.Context, scenario Scenario) (Metrics, error) {
	metrics, _, err := e.ExecuteDetailed(ctx, scenario)
	return metrics, err
}

func (e LiveExecutor) ExecuteDetailed(ctx context.Context, scenario Scenario) (Metrics, string, error) {
	if e.Client == nil {
		return Metrics{}, "", fmt.Errorf("live provider client is required")
	}
	started := time.Now()
	var firstDelta time.Duration
	var firstDeltaOnce sync.Once
	response, err := e.Client.Chat(ctx, []llm.ChatMessage{
		{Role: "system", Content: "Complete the bounded evaluation task. End the response with the exact requested success token."},
		{Role: "user", Content: scenario.Prompt},
	}, llm.ChatOptions{OnDelta: func(text string) {
		if strings.TrimSpace(text) != "" {
			firstDeltaOnce.Do(func() { firstDelta = time.Since(started) })
		}
	}})
	if err != nil {
		return Metrics{}, "", err
	}
	if firstDelta == 0 {
		firstDelta = time.Since(started)
	}
	content := response.Message.Content
	ok := strings.Contains(content, scenario.SuccessToken)
	inputTokens := response.Usage.InputTokens
	outputTokens := response.Usage.OutputTokens
	cost := float64(inputTokens)*e.InputCostPerMillion/1_000_000 + float64(outputTokens)*e.OutputCostPerMillion/1_000_000
	return Metrics{
		TaskSuccess: ok, VerifierPass: ok,
		TTFTMillis: firstDelta.Milliseconds(), TTFTSource: "provider_stream",
		InputTokens: inputTokens, OutputTokens: outputTokens, EstimatedCostUSD: cost,
	}, "live provider response is checked for the scenario success token; live runs never gate CI", nil
}
