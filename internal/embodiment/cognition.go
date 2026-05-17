package embodiment

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
)

type AgentRuntime interface {
	Spawn(context.Context, agentruntime.SpawnRequest) (agentruntime.Run, error)
	Wait(context.Context, string) (agentruntime.Run, error)
}

type ActionRouter interface {
	RouteAll(context.Context, []BodyAction, ProviderDescriptor) []RouteResult
}

type CognitionConfig struct {
	DefaultSessionID string
	DefaultAgent     string
	WaitTimeout      time.Duration
	ActionRouter     ActionRouter
	ProviderResolver func(string) (ProviderDescriptor, bool)
}

type Cognition struct {
	runtime AgentRuntime
	cfg     CognitionConfig

	mu       sync.Mutex
	inFlight map[string]string
}

func NewCognition(runtime AgentRuntime, cfg CognitionConfig) *Cognition {
	return &Cognition{
		runtime:  runtime,
		cfg:      cfg,
		inFlight: map[string]string{},
	}
}

func (c *Cognition) Trigger(ctx context.Context, percept Percept, decision GateDecision) (CognitionResult, error) {
	if c == nil || c.runtime == nil || !decision.Trigger {
		return CognitionResult{Reason: CognitionReasonSkipped}, nil
	}
	sessionID := strings.TrimSpace(percept.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(c.cfg.DefaultSessionID)
	}
	key := cognitionKey(percept.Provider, sessionID)

	c.mu.Lock()
	if _, exists := c.inFlight[key]; exists {
		c.mu.Unlock()
		return CognitionResult{Reason: CognitionReasonInFlight}, nil
	}
	c.inFlight[key] = ""
	c.mu.Unlock()

	req := agentruntime.SpawnRequest{
		SessionID:          sessionID,
		Title:              "Embodied cognition: " + strings.TrimSpace(percept.Provider),
		Prompt:             BuildCognitionPrompt(percept, decision),
		Agent:              strings.TrimSpace(c.cfg.DefaultAgent),
		SessionKind:        "embodiment",
		SystemPromptAppend: BuildSystemPromptBlock(percept, decision),
	}
	run, err := c.runtime.Spawn(ctx, req)
	if err != nil {
		c.clearInFlight(key)
		return CognitionResult{}, err
	}
	c.mu.Lock()
	c.inFlight[key] = strings.TrimSpace(run.ID)
	c.mu.Unlock()
	go c.waitAndClear(key, run.ID, percept)
	return CognitionResult{Triggered: true, RunID: run.ID, Reason: CognitionReasonTriggered}, nil
}

func (c *Cognition) InFlight(provider, sessionID string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.inFlight[cognitionKey(provider, sessionID)]
	return ok
}

func (c *Cognition) waitAndClear(key, runID string, percept Percept) {
	timeout := c.cfg.WaitTimeout
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	run, err := c.runtime.Wait(ctx, runID)
	if err == nil {
		c.routeRunActions(ctx, percept, run.Response)
	}
	c.clearInFlight(key)
}

func (c *Cognition) routeRunActions(ctx context.Context, percept Percept, response string) {
	if c == nil || c.cfg.ActionRouter == nil || strings.TrimSpace(response) == "" {
		return
	}
	actions, err := ExtractBodyActions(response)
	if err != nil || len(actions) == 0 {
		return
	}
	if c.cfg.ProviderResolver == nil {
		return
	}
	provider, ok := c.cfg.ProviderResolver(percept.Provider)
	if !ok {
		return
	}
	c.cfg.ActionRouter.RouteAll(ctx, actions, provider)
}

func (c *Cognition) clearInFlight(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.inFlight, key)
}

func cognitionKey(provider, sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		sessionID = "default"
	}
	return normalizeName(provider) + ":" + strings.TrimSpace(sessionID)
}
