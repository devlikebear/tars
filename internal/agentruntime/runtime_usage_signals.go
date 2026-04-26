package agentruntime

import (
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/usage"
)

func (r *Runtime) recordUsageSignal(name string, run Run, dimensions map[string]string) {
	if r == nil || r.opts.UsageTracker == nil {
		return
	}
	source := "agent_run"
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(run.Agent)), "cron") {
		source = "cron"
	}
	_ = r.opts.UsageTracker.RecordSignal(usage.SignalEntry{
		Name:       name,
		Source:     source,
		SessionID:  run.SessionID,
		RunID:      run.ID,
		Dimensions: dimensions,
	})
}

func intDimension(value int) string {
	return fmt.Sprintf("%d", value)
}
