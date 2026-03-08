package browser

import (
	"context"
)

type playwrightFlowRunner struct {
	cfg Config
}

func newPlaywrightFlowRunner(cfg Config) flowRunner {
	return &playwrightFlowRunner{cfg: cfg}
}

func (r *playwrightFlowRunner) Execute(ctx context.Context, req flowRunRequest) (flowRunResponse, error) {
	return runPlaywrightRequest(ctx, req)
}
