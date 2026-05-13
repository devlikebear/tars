package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func (r *Runtime) Spawn(ctx context.Context, req SpawnRequest) (Run, error) {
	if err := r.requireEnabled(); err != nil {
		return Run{}, err
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return Run{}, fmt.Errorf("prompt is required")
	}
	sessionStore := r.sessionStoreForWorkspace(req.WorkspaceID)
	if sessionStore == nil {
		return Run{}, fmt.Errorf("session store is not configured")
	}
	selectedAgent, executor, err := r.resolveExecutor(req.Agent)
	if err != nil {
		return Run{}, err
	}

	sessionID, err := resolveSpawnSessionID(sessionStore, req, agentRuntimeAgentInfo(executor), selectedAgent)
	if err != nil {
		return Run{}, err
	}

	runCtx, state := r.newAcceptedRunState(req, prompt, selectedAgent, executor, sessionID)
	if err := r.registerAcceptedRunState(state); err != nil {
		if state.cancel != nil {
			state.cancel()
		}
		return Run{}, err
	}

	go func() {
		defer r.runWG.Done()
		r.executeRun(runCtx, state.run.ID)
	}()
	return state.run, nil
}

func resolveSpawnSessionID(sessionStore *session.Store, req SpawnRequest, info AgentInfo, selectedAgent string) (string, error) {
	sessionID := strings.TrimSpace(req.SessionID)
	switch normalizeSessionRoutingMode(info.SessionRoutingMode) {
	case "new":
		sessionID = ""
	case "fixed":
		sessionID = strings.TrimSpace(info.SessionFixedID)
		if sessionID == "" {
			return "", fmt.Errorf("agent %q is configured with fixed session routing but session_fixed_id is empty", selectedAgent)
		}
	}
	if sessionID == "" {
		if strings.TrimSpace(req.SessionKind) != "" || req.SessionHidden {
			title := strings.TrimSpace(req.Title)
			if title == "" {
				title = strings.TrimSpace(req.SessionKind)
			}
			if title == "" {
				title = "chat"
			}
			s, err := sessionStore.CreateWithOptions(title, strings.TrimSpace(req.SessionKind), req.SessionHidden)
			if err != nil {
				return "", fmt.Errorf("create session: %w", err)
			}
			inheritCriticFromParent(sessionStore, strings.TrimSpace(req.ParentSessionID), s.ID)
			return s.ID, nil
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = "chat"
		}
		s, err := sessionStore.Create(title)
		if err != nil {
			return "", fmt.Errorf("create session: %w", err)
		}
		inheritCriticFromParent(sessionStore, strings.TrimSpace(req.ParentSessionID), s.ID)
		return s.ID, nil
	}
	if _, err := sessionStore.Get(sessionID); err != nil {
		return "", fmt.Errorf("get session: %w", err)
	}
	return sessionID, nil
}

func (r *Runtime) newAcceptedRunState(
	req SpawnRequest,
	prompt string,
	selectedAgent string,
	executor AgentExecutor,
	sessionID string,
) (context.Context, *runState) {
	now := r.nowFn().UTC()
	runID := fmt.Sprintf("run_%d", r.runSeq.Add(1))
	runCtx, cancel := context.WithCancel(context.Background())
	workspaceID := normalizeWorkspaceID(req.WorkspaceID)
	run := Run{
		ID:                        runID,
		WorkspaceID:               workspaceID,
		SessionID:                 sessionID,
		TaskID:                    strings.TrimSpace(req.TaskID),
		SessionKind:               strings.TrimSpace(req.SessionKind),
		Agent:                     selectedAgent,
		Prompt:                    prompt,
		ParentRunID:               strings.TrimSpace(req.ParentRunID),
		RootRunID:                 strings.TrimSpace(req.RootRunID),
		ParentSessionID:           strings.TrimSpace(req.ParentSessionID),
		Depth:                     req.Depth,
		RestartedFromRunID:        strings.TrimSpace(req.RestartedFromRunID),
		RestartedFromCheckpointID: strings.TrimSpace(req.RestartedFromCheckpointID),
		RestartAttempt:            req.RestartAttempt,
		RestartReason:             strings.TrimSpace(req.RestartReason),
		FlowID:                    strings.TrimSpace(req.FlowID),
		StepID:                    strings.TrimSpace(req.StepID),
		Tier:                      resolveRunTier(req.Tier, agentRuntimeAgentInfo(executor).Tier),
		ConsensusMode:             strings.TrimSpace(req.Mode),
		ProviderOverride:          CloneProviderOverride(req.ProviderOverride),
		Status:                    RunStatusAccepted,
		Accepted:                  true,
		CreatedAt:                 now.Format(time.RFC3339),
		UpdatedAt:                 now.Format(time.RFC3339),
	}
	return runCtx, &runState{run: run, req: req, executor: executor, cancel: cancel, done: make(chan struct{})}
}

// inheritCriticFromParent copies the parent session's critic configuration
// onto a freshly-spawned child session so subagents inherit the user's choice
// without a separate API call. Best-effort: missing parent or store errors are
// ignored — the child simply starts with no critic configured.
func inheritCriticFromParent(sessionStore *session.Store, parentID, childID string) {
	if parentID == "" || childID == "" {
		return
	}
	parent, err := sessionStore.Get(parentID)
	if err != nil {
		return
	}
	inherited := session.InheritCriticConfig(parent.Critic)
	if inherited == nil {
		return
	}
	_, _ = sessionStore.SetCritic(childID, inherited)
}

func resolveRunTier(requestTier string, executorTier string) string {
	if tier := strings.ToLower(strings.TrimSpace(requestTier)); tier != "" {
		return tier
	}
	return strings.ToLower(strings.TrimSpace(executorTier))
}

func (r *Runtime) registerAcceptedRunState(state *runState) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return fmt.Errorf("agent runtime is closed")
	}
	r.runs[state.run.ID] = state
	r.runOrder = append(r.runOrder, state.run.ID)
	r.runWG.Add(1)
	r.trimRunHistoryLocked()
	r.stateVersion++
	r.mu.Unlock()
	r.publishRunEvent(state.run.ID, RunEvent{Type: "run_accepted", RunID: state.run.ID, Timestamp: state.run.CreatedAt, Agent: state.run.Agent, Status: string(state.run.Status), Tier: state.run.Tier})
	r.persistSnapshot()
	return nil
}
