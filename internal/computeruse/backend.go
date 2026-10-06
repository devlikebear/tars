package computeruse

import (
	"context"
	"github.com/devlikebear/tars/internal/jev"
)

// DecisionBackend selects one action; only the engine may execute it.
type DecisionBackend interface {
	Decide(context.Context, string, map[string]jev.Question) (Decision, DecisionUsage, error)
}

// DecisionUsage accounts for each call, including rejected model output.
type DecisionUsage struct {
	Backend      string
	Model        string
	InputTokens  int
	OutputTokens int
	EstUSD       float64
	PricingKnown bool
}

type jevBackend struct {
	asker         Asker
	inputTokenUSD float64
}

func (b jevBackend) Decide(ctx context.Context, state string, qs map[string]jev.Question) (Decision, DecisionUsage, error) {
	resp, err := b.asker.Ask(ctx, state, qs)
	spent := DecisionUsage{Backend: "jev", Model: resp.Model, InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens, EstUSD: float64(resp.Usage.InputTokens) * b.inputTokenUSD, PricingKnown: true}
	if err != nil {
		return Decision{}, spent, err
	}
	d, err := ParseDecision(resp)
	return d, spent, err
}
