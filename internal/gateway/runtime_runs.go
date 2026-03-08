package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func (r *Runtime) getRunState(runID string) (*runState, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.runs[strings.TrimSpace(runID)]
	return state, ok
}

func (r *Runtime) sessionStoreForWorkspace(workspaceID string) *session.Store {
	if r == nil {
		return nil
	}
	if r.opts.SessionStoreForWorkspace != nil {
		if resolved := r.opts.SessionStoreForWorkspace(normalizeWorkspaceID(workspaceID)); resolved != nil {
			return resolved
		}
	}
	return r.opts.SessionStore
}

func (r *Runtime) closeRunDoneLocked(state *runState) {
	if state.closed {
		return
	}
	close(state.done)
	state.closed = true
}

func (r *Runtime) trimRunHistoryLocked() {
	max := r.opts.GatewayRunsMaxRecords
	if max <= 0 {
		return
	}
	for len(r.runOrder) > max {
		id := r.runOrder[0]
		state := r.runs[id]
		if state != nil {
			if state.run.Status == RunStatusAccepted || state.run.Status == RunStatusRunning {
				return
			}
		}
		delete(r.runs, id)
		r.runOrder = r.runOrder[1:]
	}
}

func (r *Runtime) Wait(ctx context.Context, runID string) (Run, error) {
	state, ok := r.getRunState(runID)
	if !ok {
		return Run{}, fmt.Errorf("run not found: %s", strings.TrimSpace(runID))
	}
	select {
	case <-ctx.Done():
		return Run{}, ctx.Err()
	case <-state.done:
		return r.GetOrZero(runID), nil
	}
}

func (r *Runtime) GetOrZero(runID string) Run {
	run, _ := r.Get(runID)
	return run
}

func (r *Runtime) Get(runID string) (Run, bool) {
	if r == nil {
		return Run{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.runs[strings.TrimSpace(runID)]
	if !ok {
		return Run{}, false
	}
	return state.run, true
}

func (r *Runtime) GetByWorkspace(workspaceID, runID string) (Run, bool) {
	if r == nil {
		return Run{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.runs[strings.TrimSpace(runID)]
	if !ok {
		return Run{}, false
	}
	if normalizeWorkspaceID(state.run.WorkspaceID) != normalizeWorkspaceID(workspaceID) {
		return Run{}, false
	}
	return state.run, true
}

func (r *Runtime) List(limit int) []Run {
	return r.listRunsByWorkspace("", limit)
}

func (r *Runtime) ListByWorkspace(workspaceID string, limit int) []Run {
	return r.listRunsByWorkspace(workspaceID, limit)
}

func (r *Runtime) listRunsByWorkspace(workspaceID string, limit int) []Run {
	if r == nil {
		return []Run{}
	}
	if limit <= 0 {
		limit = 50
	}
	targetWorkspaceID := strings.TrimSpace(workspaceID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Run, 0, len(r.runOrder))
	for i := len(r.runOrder) - 1; i >= 0; i-- {
		id := r.runOrder[i]
		state := r.runs[id]
		if state == nil {
			continue
		}
		if targetWorkspaceID != "" && normalizeWorkspaceID(state.run.WorkspaceID) != normalizeWorkspaceID(targetWorkspaceID) {
			continue
		}
		out = append(out, state.run)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (r *Runtime) Cancel(runID string) (Run, error) {
	return r.cancelRun(runID, "")
}

func (r *Runtime) CancelByWorkspace(workspaceID, runID string) (Run, error) {
	return r.cancelRun(runID, workspaceID)
}

func (r *Runtime) cancelRun(runID, workspaceID string) (Run, error) {
	if r == nil {
		return Run{}, fmt.Errorf("gateway runtime is disabled")
	}
	runID = strings.TrimSpace(runID)
	targetWorkspaceID := strings.TrimSpace(workspaceID)
	r.mu.Lock()
	state, ok := r.runs[runID]
	if !ok {
		r.mu.Unlock()
		return Run{}, fmt.Errorf("run not found: %s", runID)
	}
	if targetWorkspaceID != "" && normalizeWorkspaceID(state.run.WorkspaceID) != normalizeWorkspaceID(targetWorkspaceID) {
		r.mu.Unlock()
		return Run{}, fmt.Errorf("run not found: %s", runID)
	}
	if state.run.Status == RunStatusCompleted || state.run.Status == RunStatusFailed || state.run.Status == RunStatusCanceled {
		run := state.run
		r.mu.Unlock()
		return run, nil
	}
	if state.cancel != nil {
		state.cancel()
	}
	now := r.nowFn().UTC().Format(time.RFC3339)
	state.run.Status = RunStatusCanceled
	state.run.Error = "canceled by user"
	state.run.CompletedAt = now
	state.run.UpdatedAt = now
	r.closeRunDoneLocked(state)
	r.trimRunHistoryLocked()
	r.stateVersion++
	run := state.run
	r.mu.Unlock()
	r.persistSnapshot()
	return run, nil
}

func classifyRunDiagnostic(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	reason := strings.TrimSpace(err.Error())
	if reason == "" {
		return "", ""
	}
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "tool not injected for this request"):
		return "policy_tool_blocked", reason
	case strings.Contains(lower, "repeated tool call pattern"):
		return "agent_loop_guard", reason
	case strings.Contains(lower, "context canceled"), strings.Contains(lower, "canceled"):
		return "run_canceled", reason
	default:
		return "run_failed", reason
	}
}
