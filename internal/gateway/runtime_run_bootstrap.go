package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/session"
)

func (r *Runtime) Spawn(ctx context.Context, req SpawnRequest) (Run, error) {
	if r == nil || !r.opts.Enabled {
		return Run{}, fmt.Errorf("gateway runtime is disabled")
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

	sessionID, err := resolveSpawnSessionID(sessionStore, req, gatewayAgentInfo(executor), selectedAgent)
	if err != nil {
		return Run{}, err
	}
	projectID, err := resolveSpawnProjectID(sessionStore, sessionID, req.ProjectID)
	if err != nil {
		return Run{}, err
	}

	runCtx, state := r.newAcceptedRunState(req, prompt, selectedAgent, executor, sessionID, projectID)
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
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = "chat"
		}
		s, err := sessionStore.Create(title)
		if err != nil {
			return "", fmt.Errorf("create session: %w", err)
		}
		return s.ID, nil
	}
	if _, err := sessionStore.Get(sessionID); err != nil {
		return "", fmt.Errorf("get session: %w", err)
	}
	return sessionID, nil
}

func resolveSpawnProjectID(sessionStore *session.Store, sessionID, requestProjectID string) (string, error) {
	projectID := strings.TrimSpace(requestProjectID)
	if strings.TrimSpace(sessionID) == "" || sessionStore == nil {
		return projectID, nil
	}
	sess, err := sessionStore.Get(sessionID)
	if err != nil {
		return projectID, nil
	}
	if projectID == "" {
		return strings.TrimSpace(sess.ProjectID), nil
	}
	if strings.TrimSpace(sess.ProjectID) != projectID {
		_ = sessionStore.SetProjectID(sessionID, projectID)
	}
	return projectID, nil
}

func (r *Runtime) newAcceptedRunState(
	req SpawnRequest,
	prompt string,
	selectedAgent string,
	executor AgentExecutor,
	sessionID string,
	projectID string,
) (context.Context, *runState) {
	now := r.nowFn().UTC()
	runID := fmt.Sprintf("run_%d", r.runSeq.Add(1))
	runCtx, cancel := context.WithCancel(context.Background())
	workspaceID := normalizeWorkspaceID(req.WorkspaceID)
	run := Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		SessionID:   sessionID,
		ProjectID:   strings.TrimSpace(projectID),
		Agent:       selectedAgent,
		Prompt:      prompt,
		Status:      RunStatusAccepted,
		Accepted:    true,
		CreatedAt:   now.Format(time.RFC3339),
		UpdatedAt:   now.Format(time.RFC3339),
	}
	return runCtx, &runState{run: run, executor: executor, cancel: cancel, done: make(chan struct{})}
}

func (r *Runtime) registerAcceptedRunState(state *runState) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return fmt.Errorf("gateway runtime is closed")
	}
	r.runs[state.run.ID] = state
	r.runOrder = append(r.runOrder, state.run.ID)
	r.runWG.Add(1)
	r.trimRunHistoryLocked()
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
	return nil
}
