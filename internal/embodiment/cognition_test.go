package embodiment

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
)

type fakeCognitionRuntime struct {
	mu     sync.Mutex
	spawns []agentruntime.SpawnRequest
	waitCh chan struct{}
}

func (f *fakeCognitionRuntime) Spawn(_ context.Context, req agentruntime.SpawnRequest) (agentruntime.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spawns = append(f.spawns, req)
	return agentruntime.Run{ID: "run_1", SessionID: req.SessionID, Status: agentruntime.RunStatusAccepted, Accepted: true}, nil
}

func (f *fakeCognitionRuntime) Wait(ctx context.Context, runID string) (agentruntime.Run, error) {
	select {
	case <-f.waitCh:
		return agentruntime.Run{ID: runID, Status: agentruntime.RunStatusCompleted}, nil
	case <-ctx.Done():
		return agentruntime.Run{}, ctx.Err()
	}
}

func (f *fakeCognitionRuntime) spawnCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.spawns)
}

func (f *fakeCognitionRuntime) lastSpawn() agentruntime.SpawnRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.spawns[len(f.spawns)-1]
}

func TestCognitionTrigger(t *testing.T) {
	rt := &fakeCognitionRuntime{waitCh: make(chan struct{})}
	cog := NewCognition(rt, CognitionConfig{
		DefaultSessionID: "sess_main",
		DefaultAgent:     "embodied",
		WaitTimeout:      time.Second,
	})
	percept := Percept{
		Provider:      "stackchan",
		Modality:      ModalityAudio,
		Owner:         OwnerOwner,
		Summary:       "Owner asked for a status update.",
		Trigger:       "event",
		IsSelfSensory: true,
		CapturedAt:    time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
	}
	decision := GateDecision{Trigger: true, Mode: GateModeDirective, Reason: GateReasonOwnerVoice}

	result, err := cog.Trigger(context.Background(), percept, decision)
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if !result.Triggered || result.RunID != "run_1" {
		t.Fatalf("unexpected trigger result: %+v", result)
	}
	req := rt.lastSpawn()
	if req.SessionID != "sess_main" || req.Agent != "embodied" {
		t.Fatalf("spawn routing = session %q agent %q", req.SessionID, req.Agent)
	}
	if !strings.Contains(req.Prompt, "Owner asked for a status update.") {
		t.Fatalf("prompt missing percept summary: %q", req.Prompt)
	}
	if !strings.Contains(req.SystemPromptAppend, "너는 몸이 있다") || !strings.Contains(req.SystemPromptAppend, "owner") {
		t.Fatalf("system prompt append missing embodied context: %q", req.SystemPromptAppend)
	}

	suppressed, err := cog.Trigger(context.Background(), percept, decision)
	if err != nil {
		t.Fatalf("second Trigger: %v", err)
	}
	if suppressed.Triggered || suppressed.Reason != CognitionReasonInFlight {
		t.Fatalf("expected in-flight suppression, got %+v", suppressed)
	}

	close(rt.waitCh)
	eventually(t, time.Second, func() bool { return !cog.InFlight("stackchan", "sess_main") })
}

func eventually(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition was not met within %s", timeout)
}
