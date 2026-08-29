package apptool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

func TestDurableSubagentsOrchestrateReturnsWorkAndOutlivesRequest(t *testing.T) {
	started := make(chan string, 3)
	releaseResearch := make(chan struct{})
	runtime, _ := newAgentRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, _ []string, _ string) (string, error) {
		started <- prompt
		if prompt == "inspect backend" || prompt == "inspect docs" {
			<-releaseResearch
			return strings.TrimPrefix(prompt, "inspect ") + " findings", nil
		}
		return "durable report", nil
	})
	ledger := openDurableToolLedger(t)
	capability := createPromotedDurableToolCapability(t, ledger, "ws-durable", "research-helper")
	scheduler := startDurableToolScheduler(t, ledger, "ws-durable", runtime)
	mirrorConfig, sessionID, sessionStore := newSubagentTaskMirrorConfigForTest(t)
	ctx, cancelRequest := context.WithCancel(serverauth.WithWorkspaceID(context.Background(), "ws-durable"))
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source: "chat", SessionID: sessionID, CapabilityVersionIDs: []string{capability.ID},
	})
	tool := NewDurableSubagentsOrchestrateTool(runtime, scheduler, mirrorConfig)
	result, err := tool.Execute(ctx, json.RawMessage(`{
		"flow_id":"flow-durable",
		"steps":[
			{"id":"research","mode":"parallel","tasks":[
				{"id":"backend","prompt":"inspect backend"},
				{"id":"docs","prompt":"inspect docs"}
			]},
			{"id":"report","mode":"sequential","tasks":[
				{"id":"combine","prompt":"combine {{task.backend.summary}} and {{task.docs.summary}}"}
			]}
		]
	}`))
	if err != nil || result.IsError {
		t.Fatalf("submit durable flow result=%s err=%v", result.Text(), err)
	}
	var accepted struct {
		WorkID  string `json:"work_id"`
		FlowID  string `json:"flow_id"`
		Durable bool   `json:"durable"`
	}
	if err := json.Unmarshal([]byte(result.Text()), &accepted); err != nil {
		t.Fatalf("decode durable acceptance: %v", err)
	}
	if accepted.WorkID == "" || accepted.FlowID != "flow-durable" || !accepted.Durable {
		t.Fatalf("durable acceptance = %+v", accepted)
	}
	cancelRequest()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case prompt := <-started:
			seen[prompt] = true
		case <-time.After(subagentTestEventTimeout):
			t.Fatalf("durable research tasks did not start: %v", seen)
		}
	}
	if !seen["inspect backend"] || !seen["inspect docs"] {
		t.Fatalf("durable parallel prompts = %v", seen)
	}
	close(releaseResearch)
	select {
	case prompt := <-started:
		if prompt != "combine backend findings and docs findings" {
			t.Fatalf("durable placeholder prompt=%q", prompt)
		}
	case <-time.After(subagentTestEventTimeout):
		t.Fatal("durable dependent task did not start")
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), subagentTestEventTimeout)
	defer waitCancel()
	projection, err := scheduler.Wait(waitCtx, accepted.WorkID)
	if err != nil {
		t.Fatalf("wait durable flow: %v", err)
	}
	if projection.Work.State != workstore.WorkStateDone || len(projection.Attempts) != 3 {
		t.Fatalf("durable projection = %+v", projection)
	}
	outcomes, err := ledger.ListCapabilityOutcomes(context.Background(), capability.WorkspaceID, capability.ID)
	if err != nil {
		t.Fatalf("list durable capability outcomes: %v", err)
	}
	if len(outcomes) != 3 {
		t.Fatalf("durable capability outcome count=%d want 3", len(outcomes))
	}
	for _, outcome := range outcomes {
		if outcome.WorkID != accepted.WorkID || outcome.Status != workstore.CapabilityOutcomeSucceeded {
			t.Fatalf("durable capability outcome = %+v", outcome)
		}
	}
	legacyTasks, err := sessionStore.GetTasks(sessionID)
	if err != nil {
		t.Fatalf("get legacy task mirror: %v", err)
	}
	if legacyTasks.Plan != nil || len(legacyTasks.Tasks) != 0 {
		t.Fatalf("durable path mutated independent session task mirror = %+v", legacyTasks)
	}
}

func TestDurableSubagentsOrchestrateSupportsShortSynchronousWrapper(t *testing.T) {
	runtime, _ := newAgentRuntimeForSubagentToolTests(t, 2, 1, func(_ context.Context, _ string, prompt string, _ []string, _ string) (string, error) {
		return "completed " + prompt, nil
	})
	ledger := openDurableToolLedger(t)
	scheduler := startDurableToolScheduler(t, ledger, "ws-wrapper", runtime)
	ctx := usage.WithCallMeta(serverauth.WithWorkspaceID(context.Background(), "ws-wrapper"), usage.CallMeta{SessionID: "session-wrapper"})
	result, err := NewDurableSubagentsOrchestrateTool(runtime, scheduler).Execute(ctx, json.RawMessage(`{
		"flow_id":"flow-wrapper",
		"wait_for_completion":true,
		"timeout_ms":2000,
		"steps":[{"id":"one","mode":"sequential","tasks":[{"id":"task","prompt":"inspect"}]}]
	}`))
	if err != nil || result.IsError {
		t.Fatalf("durable wrapper result=%s err=%v", result.Text(), err)
	}
	var payload struct {
		WorkID string              `json:"work_id"`
		Status workstore.WorkState `json:"status"`
		Steps  []struct {
			Tasks []subagentTaskOutput `json:"tasks"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(result.Text()), &payload); err != nil {
		t.Fatalf("decode durable wrapper: %v", err)
	}
	if payload.WorkID == "" || payload.Status != workstore.WorkStateDone || len(payload.Steps) != 1 || len(payload.Steps[0].Tasks) != 1 || payload.Steps[0].Tasks[0].Response != "completed inspect" {
		t.Fatalf("durable wrapper payload = %+v", payload)
	}
}

func openDurableToolLedger(t *testing.T) *workstore.Store {
	t.Helper()
	store, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open durable tool ledger: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func createPromotedDurableToolCapability(t *testing.T, store *workstore.Store, workspaceID, name string) workstore.CapabilityVersion {
	t.Helper()
	work, err := store.CreateWork(context.Background(), workstore.CreateWorkInput{
		WorkspaceID: workspaceID, Kind: "capability_review", Source: "skill_inbox",
		IdempotencyKey: "capability:" + name, Title: "Review " + name,
		InitialState: workstore.WorkStateReview, ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create durable capability work: %v", err)
	}
	version, err := store.CreateCapabilityVersion(context.Background(), workstore.CreateCapabilityVersionInput{
		WorkspaceID: workspaceID, WorkID: work.ID, CandidateID: "candidate-" + name,
		CapabilityName: name, InitialState: workstore.CapabilityStatePromoted,
		ContentDigest: "sha256:" + name, SnapshotJSON: json.RawMessage(`{"files":[]}`),
		ProvenanceJSON: json.RawMessage(`{"source":"test"}`), PermissionsJSON: json.RawMessage(`[]`),
		ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create durable promoted capability: %v", err)
	}
	return version
}

func startDurableToolScheduler(t *testing.T, ledger *workstore.Store, workspaceID string, agentRuntime *agentruntime.Runtime) *workscheduler.Scheduler {
	t.Helper()
	scheduler, err := workscheduler.New(workscheduler.Options{
		Store: ledger, WorkspaceID: workspaceID, WorkerID: "tool-scheduler", ActorID: "tool-scheduler",
		// The lease only exists to reclaim a worker that has actually died, so it
		// has to outlast any stall these tests can suffer on a loaded runner. At
		// one second a starved heartbeat goroutine was enough to lapse the lease
		// on a live attempt, and the reclaim came back as a retry: the step ran
		// twice, the prompt picked up "Retry the task and correct the previous
		// failure.", and the assertions on response text and attempt count failed
		// on Windows CI. A minute matches both the scheduler's own default and
		// every other scheduler test in the repo.
		LeaseDuration: time.Minute, HeartbeatInterval: 100 * time.Millisecond,
		PollInterval: 5 * time.Millisecond, MaxWorkers: 4,
		Executors: []workscheduler.Executor{NewAgentRuntimeWorkExecutor(agentRuntime, ledger)},
	})
	if err != nil {
		t.Fatalf("new durable tool scheduler: %v", err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- scheduler.Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		scheduler.Close()
		select {
		case <-done:
		case <-time.After(subagentTestEventTimeout):
			t.Fatal("durable scheduler did not stop")
		}
	})
	return scheduler
}
