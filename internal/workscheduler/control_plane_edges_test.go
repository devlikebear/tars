package workscheduler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/workstore"
)

func TestSchedulerConstructorRejectsUnsafeControlPlaneOptions(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	valid := Options{
		Store: store, WorkspaceID: "workspace", WorkerID: "worker", ActorID: "actor",
		LeaseDuration: time.Minute, HeartbeatInterval: time.Second,
		Executors: []Executor{&fakeExecutor{adapter: "fake"}},
	}
	cases := []struct {
		name string
		edit func(*Options)
		want string
	}{
		{name: "missing store", edit: func(opts *Options) { opts.Store = nil }, want: "store is required"},
		{name: "missing workspace", edit: func(opts *Options) { opts.WorkspaceID = " " }, want: "workspace, worker, and actor"},
		{name: "missing worker", edit: func(opts *Options) { opts.WorkerID = "" }, want: "workspace, worker, and actor"},
		{name: "missing actor", edit: func(opts *Options) { opts.ActorID = "" }, want: "workspace, worker, and actor"},
		{name: "invalid heartbeat", edit: func(opts *Options) { opts.HeartbeatInterval = opts.LeaseDuration }, want: "heartbeat interval"},
		{name: "no executors", edit: func(opts *Options) { opts.Executors = nil }, want: "at least one executor"},
		{name: "nil executor", edit: func(opts *Options) { opts.Executors = []Executor{nil} }, want: "executor adapter"},
		{name: "blank adapter", edit: func(opts *Options) { opts.Executors = []Executor{&fakeExecutor{adapter: " "}} }, want: "executor adapter"},
		{name: "duplicate adapter", edit: func(opts *Options) {
			opts.Executors = []Executor{&fakeExecutor{adapter: "fake"}, &fakeExecutor{adapter: " fake "}}
		}, want: "duplicate executor"},
		{name: "nil verifier", edit: func(opts *Options) { opts.Verifiers = []Verifier{nil} }, want: "verifier is required"},
		{name: "blank verifier name", edit: func(opts *Options) {
			opts.Verifiers = []Verifier{&fakeVerifier{id: "proof", environment: json.RawMessage(`{"runner":"test"}`)}}
		}, want: "name and identity"},
		{name: "blank verifier identity", edit: func(opts *Options) {
			opts.Verifiers = []Verifier{&fakeVerifier{name: "proof", environment: json.RawMessage(`{"runner":"test"}`)}}
		}, want: "name and identity"},
		{name: "worker verifier identity", edit: func(opts *Options) {
			opts.Verifiers = []Verifier{&fakeVerifier{name: "proof", id: "worker", environment: json.RawMessage(`{"runner":"test"}`)}}
		}, want: "identity separate"},
		{name: "missing verifier environment", edit: func(opts *Options) { opts.Verifiers = []Verifier{&fakeVerifier{name: "proof", id: "proof"}} }, want: "declare its execution environment"},
		{name: "duplicate verifier", edit: func(opts *Options) {
			opts.Verifiers = []Verifier{
				&fakeVerifier{name: "proof", id: "proof-1", environment: json.RawMessage(`{"runner":"one"}`)},
				&fakeVerifier{name: " proof ", id: "proof-2", environment: json.RawMessage(`{"runner":"two"}`)},
			}
		}, want: "duplicate verifier"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			opts := valid
			tc.edit(&opts)
			if _, err := New(opts); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("New() error=%v want containing %q", err, tc.want)
			}
		})
	}

	defaults := valid
	defaults.LeaseDuration = 0
	defaults.HeartbeatInterval = 0
	defaults.PollInterval = 0
	defaults.MaxWorkers = 0
	scheduler, err := New(defaults)
	if err != nil {
		t.Fatalf("New() with safe defaults: %v", err)
	}
	t.Cleanup(scheduler.Close)
	if scheduler.leaseDuration != time.Minute || scheduler.heartbeatInterval != 20*time.Second || scheduler.pollInterval != 250*time.Millisecond || scheduler.maxWorkers != 1 {
		t.Fatalf("unsafe scheduler defaults: lease=%v heartbeat=%v poll=%v workers=%d", scheduler.leaseDuration, scheduler.heartbeatInterval, scheduler.pollInterval, scheduler.maxWorkers)
	}
}

func TestSchedulerSubmissionRejectsMalformedOrUntrustedInputs(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	scheduler := newTestScheduler(t, store, "workspace-submit-edges", &fakeExecutor{adapter: "fake"}, 1)
	valid := SubmitInput{
		WorkspaceID: scheduler.WorkspaceID(), IdempotencyKey: "edge", Title: "Edge",
		Adapter: "fake", ActorID: "planner",
		Steps: []StepSpec{{Key: "run", Title: "Run", Policy: oneAttemptPolicy()}},
	}
	cases := []struct {
		name string
		edit func(*SubmitInput)
		want string
	}{
		{name: "foreign workspace", edit: func(input *SubmitInput) { input.WorkspaceID = "foreign" }, want: "workspace does not match"},
		{name: "unknown adapter", edit: func(input *SubmitInput) { input.Adapter = "missing" }, want: "not configured"},
		{name: "missing idempotency", edit: func(input *SubmitInput) { input.IdempotencyKey = "" }, want: "idempotency key, title, and actor"},
		{name: "missing title", edit: func(input *SubmitInput) { input.Title = "" }, want: "idempotency key, title, and actor"},
		{name: "missing actor", edit: func(input *SubmitInput) { input.ActorID = "" }, want: "idempotency key, title, and actor"},
		{name: "missing steps", edit: func(input *SubmitInput) { input.Steps = nil }, want: "at least one step"},
		{name: "blank step", edit: func(input *SubmitInput) { input.Steps = []StepSpec{{Title: "Run"}} }, want: "key and title"},
		{name: "duplicate step", edit: func(input *SubmitInput) { input.Steps = append(input.Steps, input.Steps[0]) }, want: "duplicate step key"},
		{name: "unknown dependency", edit: func(input *SubmitInput) { input.Steps[0].DependsOn = []string{"missing"} }, want: "depends on unknown"},
		{name: "self dependency", edit: func(input *SubmitInput) { input.Steps[0].DependsOn = []string{"run"} }, want: "cannot depend on itself"},
		{name: "forged metadata", edit: func(input *SubmitInput) { input.MetadataJSON = json.RawMessage(`[]`) }, want: "metadata must be a JSON object"},
		{name: "blank capability", edit: func(input *SubmitInput) { input.CapabilityVersionIDs = []string{" "} }, want: "capability version id is required"},
		{name: "missing capability", edit: func(input *SubmitInput) { input.CapabilityVersionIDs = []string{"missing"} }, want: "resolve capability version"},
		{name: "proof without requirement", edit: func(input *SubmitInput) { input.Steps[0].Policy.Proof.Required = true }, want: "needs at least one verifier"},
		{name: "proof with unknown verifier", edit: func(input *SubmitInput) {
			input.Steps[0].Policy.Proof.Required = true
			input.Steps[0].Policy.Proof.Requirements = []workstore.ProofRequirement{{Kind: "judge", Verifier: "missing"}}
		}, want: "requires unconfigured verifier"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			input := valid
			input.Steps = append([]StepSpec(nil), valid.Steps...)
			tc.edit(&input)
			if _, err := scheduler.Submit(context.Background(), input); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Submit() error=%v want containing %q", err, tc.want)
			}
		})
	}

	var nilScheduler *Scheduler
	if nilScheduler.WorkspaceID() != "" {
		t.Fatal("nil scheduler exposed a workspace")
	}
	if _, err := nilScheduler.Submit(context.Background(), valid); !errors.Is(err, ErrClosed) {
		t.Fatalf("nil scheduler Submit() error=%v want ErrClosed", err)
	}
	scheduler.Close()
	if _, err := scheduler.Submit(context.Background(), valid); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed scheduler Submit() error=%v want ErrClosed", err)
	}
}

func TestSchedulerLifecycleAndOperatorBoundaries(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	errorsSeen := make(chan error, 1)
	scheduler, err := New(Options{
		Store: store, WorkspaceID: "workspace-lifecycle", WorkerID: "worker", ActorID: "actor",
		LeaseDuration: time.Minute, HeartbeatInterval: 10 * time.Millisecond,
		PollInterval: 2 * time.Millisecond, MaxWorkers: 1,
		Executors: []Executor{&fakeExecutor{adapter: "fake"}}, OnError: func(err error) { errorsSeen <- err },
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	t.Cleanup(scheduler.Close)

	if _, err := scheduler.Wait(context.Background(), " "); err == nil {
		t.Fatal("Wait() accepted a blank work ID")
	}
	if _, err := scheduler.Cancel(context.Background(), "", "operator", "reason"); err == nil {
		t.Fatal("Cancel() accepted a blank work ID")
	}
	if _, err := scheduler.Cancel(context.Background(), "work", "", "reason"); err == nil {
		t.Fatal("Cancel() accepted a blank actor")
	}
	if _, err := scheduler.Cancel(context.Background(), "work", "operator", ""); err == nil {
		t.Fatal("Cancel() accepted a blank reason")
	}

	scheduler.reportError(nil)
	sentinel := errors.New("reported")
	scheduler.reportError(sentinel)
	select {
	case got := <-errorsSeen:
		if !errors.Is(got, sentinel) {
			t.Fatalf("reported error=%v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler did not report error")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- scheduler.Run(ctx) }()
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error=%v want context.DeadlineExceeded", err)
	}
	if _, err := scheduler.RunOnce(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("RunOnce() after Run cancellation error=%v want ErrClosed", err)
	}
	if _, err := scheduler.RecoverOnce(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("RecoverOnce() after Run cancellation error=%v want ErrClosed", err)
	}
}

func TestSchedulerMetadataAndStateContracts(t *testing.T) {
	t.Parallel()

	metadata, err := schedulerMetadata(json.RawMessage(`{"owner":"test","capabilities":{"version_ids":["forged"]}}`), " fake ", nil)
	if err != nil {
		t.Fatalf("schedulerMetadata(): %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(metadata, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, forged := decoded["capabilities"]; forged {
		t.Fatalf("typed metadata retained forged capability attribution: %s", metadata)
	}

	metadata, err = schedulerMetadata(metadata, "fake", []string{"v1", "v2"})
	if err != nil {
		t.Fatalf("schedulerMetadata() with capabilities: %v", err)
	}
	versions, err := capabilityVersionIDsFromMetadata(metadata)
	if err != nil || len(versions) != 2 || versions[0] != "v1" || versions[1] != "v2" {
		t.Fatalf("capability versions=%v err=%v", versions, err)
	}
	if versions, err := capabilityVersionIDsFromMetadata(nil); err != nil || versions != nil {
		t.Fatalf("empty capability metadata versions=%v err=%v", versions, err)
	}
	if _, err := capabilityVersionIDsFromMetadata(json.RawMessage(`{`)); err == nil {
		t.Fatal("invalid capability metadata was accepted")
	}

	transitions := []struct {
		from workstore.WorkState
		to   workstore.WorkState
		want workstore.WorkState
		ok   bool
	}{
		{workstore.WorkStateTriage, workstore.WorkStateDone, workstore.WorkStateTodo, true},
		{workstore.WorkStateBacklog, workstore.WorkStateDone, workstore.WorkStateTodo, true},
		{workstore.WorkStateTodo, workstore.WorkStateBlocked, workstore.WorkStateBlocked, true},
		{workstore.WorkStateReady, workstore.WorkStateDone, workstore.WorkStateRunning, true},
		{workstore.WorkStateBlocked, workstore.WorkStateDone, workstore.WorkStateReady, true},
		{workstore.WorkStateReview, workstore.WorkStateDone, workstore.WorkStateDone, true},
		{workstore.WorkStateReview, workstore.WorkStateBlocked, workstore.WorkStateBlocked, true},
		{workstore.WorkStateReview, workstore.WorkStateReady, workstore.WorkStateRunning, true},
		{workstore.WorkStateRunning, workstore.WorkStateDone, workstore.WorkStateDone, true},
		{workstore.WorkStateDone, workstore.WorkStateRunning, "", false},
		{workstore.WorkStateDone, workstore.WorkStateCancelled, workstore.WorkStateCancelled, true},
	}
	for _, tc := range transitions {
		got, ok := nextWorkState(tc.from, tc.to)
		if got != tc.want || ok != tc.ok {
			t.Errorf("nextWorkState(%s,%s)=(%s,%v) want (%s,%v)", tc.from, tc.to, got, ok, tc.want, tc.ok)
		}
	}

	for raw, want := range map[string]bool{
		"": false, "null": false, "{}": false, "[]": false, "{": false,
		`{"runner":"test"}`: true, `["test"]`: true, `true`: true, `0`: true,
	} {
		if got := meaningfulJSON(json.RawMessage(raw)); got != want {
			t.Errorf("meaningfulJSON(%q)=%v want %v", raw, got, want)
		}
	}
	if proofDigest(nil) != proofDigest(json.RawMessage(`{}`)) || !strings.HasPrefix(proofDigest(json.RawMessage(`{"ok":true}`)), "sha256:") {
		t.Fatal("proof digest does not use a stable empty-object SHA-256 contract")
	}
	for _, state := range []workstore.WorkState{workstore.WorkStateReview, workstore.WorkStateBlocked, workstore.WorkStateDone, workstore.WorkStateCancelled} {
		if !isTerminalForWait(state) {
			t.Errorf("terminal state %s was not terminal for Wait", state)
		}
	}
	if isTerminalForWait(workstore.WorkStateRunning) {
		t.Fatal("running state was terminal for Wait")
	}
}

func TestSchedulerReclaimsClaimsThatCannotReconnect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		executors []Executor
	}{
		{name: "adapter unavailable", executors: []Executor{&executeOnlyExecutor{adapter: "other"}}},
		{name: "executor not recoverable", executors: []Executor{&executeOnlyExecutor{adapter: "external"}}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			store := openSchedulerTestStore(t)
			seed := newTestScheduler(t, store, "workspace-reclaim-"+strings.ReplaceAll(tc.name, " ", "-"), &fakeExecutor{adapter: "external"}, 1)
			work, err := seed.Submit(context.Background(), SubmitInput{
				IdempotencyKey: "reclaim", Title: "Reclaim", Adapter: "external", ActorID: "planner",
				Steps: []StepSpec{{Key: "run", Title: "Run", Policy: oneAttemptPolicy()}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.PromoteReadySteps(context.Background(), workstore.PromoteReadyStepsInput{WorkspaceID: work.WorkspaceID, WorkID: work.ID, ActorID: "seed"}); err != nil {
				t.Fatal(err)
			}
			claim, err := store.ClaimReadyStep(context.Background(), workstore.ClaimReadyStepInput{
				WorkspaceID: work.WorkspaceID, WorkID: work.ID, WorkerID: "lost-worker",
				Adapter: "external", LeaseDuration: time.Minute, ActorID: "seed",
			})
			if err != nil {
				t.Fatal(err)
			}
			recovered, err := New(Options{
				Store: store, WorkspaceID: work.WorkspaceID, WorkerID: "replacement", ActorID: "recovery",
				LeaseDuration: time.Minute, HeartbeatInterval: time.Second, Executors: tc.executors,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(recovered.Close)
			if count, err := recovered.RecoverOnce(context.Background()); err != nil || count != 1 {
				t.Fatalf("RecoverOnce() count=%d err=%v", count, err)
			}
			projection, err := store.GetWorkProjection(context.Background(), work.WorkspaceID, work.ID)
			if err != nil || projection.Attempts[0].ID != claim.Attempt.ID || projection.Attempts[0].Status != workstore.AttemptStatusFailed || projection.Steps[0].State != workstore.WorkStateReview {
				t.Fatalf("reclaimed projection=%+v err=%v", projection, err)
			}
			if count, err := recovered.RecoverOnce(context.Background()); err != nil || count != 0 {
				t.Fatalf("idempotent RecoverOnce() count=%d err=%v", count, err)
			}
		})
	}
}

func TestSchedulerProofJudgeFailsClosedAcrossLLMBoundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		proof         workstore.StepProofPolicy
		verification  VerificationResult
		verifyErr     error
		wantStatus    workstore.ProofStatus
		wantRationale string
	}{
		{
			name:         "unapproved llm fallback",
			proof:        workstore.StepProofPolicy{Required: true, FailureState: workstore.WorkStateReview},
			verification: VerificationResult{Status: workstore.ProofStatusPassed, UsedLLM: true, Tokens: 1, CostUSD: 0.01},
			wantStatus:   workstore.ProofStatusFailed, wantRationale: "without an approved fallback",
		},
		{
			name:         "invalid negative usage",
			proof:        workstore.StepProofPolicy{Required: true, FailureState: workstore.WorkStateReview, AllowLLMFallback: true, MaxLLMTokens: 10, MaxLLMCostUSD: 1},
			verification: VerificationResult{Status: workstore.ProofStatusPassed, UsedLLM: true, Tokens: -1},
			wantStatus:   workstore.ProofStatusFailed, wantRationale: "invalid usage",
		},
		{
			name:         "budget exceeded",
			proof:        workstore.StepProofPolicy{Required: true, FailureState: workstore.WorkStateReview, AllowLLMFallback: true, MaxLLMTokens: 1, MaxLLMCostUSD: 0.01},
			verification: VerificationResult{Status: workstore.ProofStatusPassed, UsedLLM: true, Tokens: 2, CostUSD: 0.02},
			wantStatus:   workstore.ProofStatusPending, wantRationale: "budget exceeded",
		},
		{
			name:         "invalid verifier state",
			proof:        workstore.StepProofPolicy{Required: true, FailureState: workstore.WorkStateReview},
			verification: VerificationResult{Status: workstore.ProofStatusReported},
			wantStatus:   workstore.ProofStatusFailed, wantRationale: "invalid terminal proof state",
		},
		{
			name:       "verifier error",
			proof:      workstore.StepProofPolicy{Required: true, FailureState: workstore.WorkStateReview},
			verifyErr:  errors.New("proof process crashed"),
			wantStatus: workstore.ProofStatusFailed, wantRationale: "proof process crashed",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			store := openSchedulerTestStore(t)
			workspaceID := "workspace-proof-edge-" + strings.ReplaceAll(tc.name, " ", "-")
			verifier := &fakeVerifier{
				name: "judge", id: "independent-judge", environment: json.RawMessage(`{"runner":"isolated"}`),
				verify: func(context.Context, VerificationRequest) (VerificationResult, error) {
					return tc.verification, tc.verifyErr
				},
			}
			scheduler, err := New(Options{
				Store: store, WorkspaceID: workspaceID, WorkerID: "worker", ActorID: "scheduler",
				LeaseDuration: time.Minute, HeartbeatInterval: time.Second, PollInterval: time.Millisecond,
				Executors: []Executor{&fakeExecutor{adapter: "fake", execute: func(context.Context, Execution) (ExecutionResult, error) {
					return ExecutionResult{Succeeded: true, OutputJSON: json.RawMessage(`{"ok":true}`)}, nil
				}}}, Verifiers: []Verifier{verifier},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(scheduler.Close)
			policy := tc.proof
			policy.Requirements = []workstore.ProofRequirement{{Kind: "judge", Verifier: "judge"}}
			work, err := scheduler.Submit(context.Background(), SubmitInput{
				IdempotencyKey: "proof", Title: "Proof", Adapter: "fake", ActorID: "planner",
				Steps: []StepSpec{{Key: "run", Title: "Run", Policy: workstore.StepSchedulePolicy{MaxAttempts: 1, EscalationState: workstore.WorkStateReview, Proof: policy}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := scheduler.RunOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			projection, err := scheduler.Wait(context.Background(), work.ID)
			if err != nil || len(projection.Proofs) != 2 {
				t.Fatalf("proof projection=%+v err=%v", projection, err)
			}
			proof := projection.Proofs[0]
			for _, candidate := range projection.Proofs {
				if candidate.Kind == "judge" {
					proof = candidate
					break
				}
			}
			if proof.Status != tc.wantStatus || !strings.Contains(proof.Rationale, tc.wantRationale) {
				t.Fatalf("proof=%+v want status=%s rationale containing %q", proof, tc.wantStatus, tc.wantRationale)
			}
		})
	}
}

func TestSchedulerWaitWatchAndCancelOperatorEdges(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	scheduler := newTestScheduler(t, store, "workspace-operator-edges", &executeOnlyExecutor{adapter: "fake"}, 1)
	work, err := scheduler.Submit(context.Background(), SubmitInput{
		IdempotencyKey: "operator", Title: "Operator", Adapter: "fake", ActorID: "planner",
		Steps: []StepSpec{{Key: "run", Title: "Run", Policy: oneAttemptPolicy()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancelWait := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelWait()
	if _, err := scheduler.Wait(waitCtx, work.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait() timeout error=%v", err)
	}
	events, errs := scheduler.Watch(context.Background(), "missing", 0)
	for range events {
	}
	if err := <-errs; !errors.Is(err, workstore.ErrNotFound) {
		t.Fatalf("Watch() missing error=%v", err)
	}
	projection, err := scheduler.Cancel(context.Background(), work.ID, "operator", "stop")
	if err != nil || projection.Work.State != workstore.WorkStateCancelled {
		t.Fatalf("Cancel() projection=%+v err=%v", projection, err)
	}
	projection, err = scheduler.Cancel(context.Background(), work.ID, "operator", "replay")
	if err != nil || projection.Work.State != workstore.WorkStateCancelled {
		t.Fatalf("idempotent Cancel() projection=%+v err=%v", projection, err)
	}
}

func TestSchedulerShutdownCapacityAndMetadataErrorsRemainObservable(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	scheduler := newTestScheduler(t, store, "workspace-shutdown-edges", &executeOnlyExecutor{adapter: "fake"}, 1)
	work, err := scheduler.Submit(context.Background(), SubmitInput{
		IdempotencyKey: "shutdown", Title: "Shutdown", Adapter: "fake", ActorID: "planner",
		Steps: []StepSpec{{Key: "run", Title: "Run", Policy: oneAttemptPolicy()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	scheduler.Close()
	if _, err := scheduler.Wait(context.Background(), work.ID); !errors.Is(err, ErrClosed) {
		t.Fatalf("Wait() on closed scheduler error=%v", err)
	}
	events, errs := scheduler.Watch(context.Background(), work.ID, 1<<62)
	for range events {
	}
	if err := <-errs; !errors.Is(err, ErrClosed) {
		t.Fatalf("Watch() on closed scheduler error=%v", err)
	}

	running := newTestScheduler(t, store, "workspace-run-close", &executeOnlyExecutor{adapter: "fake"}, 1)
	runDone := make(chan error, 1)
	go func() { runDone <- running.Run(context.Background()) }()
	time.Sleep(10 * time.Millisecond)
	running.Close()
	if err := <-runDone; !errors.Is(err, ErrClosed) {
		t.Fatalf("Run() close error=%v", err)
	}

	capacity := newTestScheduler(t, store, "workspace-capacity-edges", &executeOnlyExecutor{adapter: "fake"}, 1)
	activeCtx, activeCancel := context.WithCancel(context.Background())
	capacity.activeMu.Lock()
	capacity.active["occupied"] = activeExecution{
		cancel: activeCancel, executor: &executeOnlyExecutor{adapter: "fake"},
		execution: Execution{Claim: workstore.StepClaim{Attempt: workstore.Attempt{ID: "occupied"}}},
	}
	capacity.activeMu.Unlock()
	if count, err := capacity.RunOnce(context.Background()); err != nil || count != 0 {
		t.Fatalf("RunOnce() at capacity count=%d err=%v", count, err)
	}
	if count, err := capacity.RecoverOnce(context.Background()); err != nil || count != 0 {
		t.Fatalf("RecoverOnce() at capacity count=%d err=%v", count, err)
	}
	capacity.startActive(workstore.Work{}, workstore.StepClaim{Attempt: workstore.Attempt{ID: "occupied"}}, &executeOnlyExecutor{adapter: "fake"}, func(context.Context, Execution) (ExecutionResult, bool, error) {
		return ExecutionResult{}, true, nil
	})
	activeCancel()
	capacity.activeMu.Lock()
	delete(capacity.active, "occupied")
	capacity.activeMu.Unlock()

	timeoutScheduler := &Scheduler{heartbeatInterval: time.Millisecond, leaseDuration: time.Second}
	if got := timeoutScheduler.heartbeatRequestTimeout(); got != 500*time.Millisecond {
		t.Fatalf("heartbeat request timeout=%v", got)
	}
	if _, err := capacity.executorForWork(workstore.Work{ID: "bad-json", MetadataJSON: json.RawMessage(`{`)}); err == nil {
		t.Fatal("executor metadata accepted malformed JSON")
	}
	if _, err := capacity.executorForWork(workstore.Work{ID: "unknown", MetadataJSON: json.RawMessage(`{"scheduler":{"adapter":"missing"}}`)}); err == nil {
		t.Fatal("executor metadata accepted an unknown adapter")
	}
	if err := capacity.reconcileWork(context.Background(), "missing-work"); !errors.Is(err, workstore.ErrNotFound) {
		t.Fatalf("reconcile missing work error=%v", err)
	}
	if err := capacity.transitionWorkTo(context.Background(), "missing-work", workstore.WorkStateDone, "operator", "missing"); !errors.Is(err, workstore.ErrNotFound) {
		t.Fatalf("transition missing work error=%v", err)
	}
	doneWork, err := store.CreateWork(context.Background(), workstore.CreateWorkInput{
		WorkspaceID: capacity.WorkspaceID(), Kind: "workflow", IdempotencyKey: "done-transition",
		Title: "Done", InitialState: workstore.WorkStateDone, ActorID: "tester",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := capacity.transitionWorkTo(context.Background(), doneWork.ID, workstore.WorkStateRunning, "operator", "invalid"); err == nil {
		t.Fatal("terminal work transitioned back to running")
	}
	if err := capacity.recordCapabilityOutcomes(context.Background(), Execution{
		Work: workstore.Work{MetadataJSON: json.RawMessage(`{`)},
	}, workstore.CapabilityOutcomeFailed, workstore.ProofStatusFailed, 0, time.Millisecond); err == nil {
		t.Fatal("capability outcome accepted malformed metadata")
	}

	_ = activeCtx
}

func TestSchedulerClosedLedgerErrorsRemainObservable(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	scheduler := newTestScheduler(t, store, "workspace-closed-ledger", &executeOnlyExecutor{adapter: "fake"}, 1)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	valid := SubmitInput{
		IdempotencyKey: "closed", Title: "Closed ledger", Adapter: "fake", ActorID: "planner",
		Steps: []StepSpec{{Key: "run", Title: "Run", Policy: oneAttemptPolicy()}},
	}
	if _, err := scheduler.Submit(context.Background(), valid); err == nil {
		t.Fatal("Submit() hid a closed ledger error")
	}
	if err := scheduler.Run(context.Background()); err == nil {
		t.Fatal("Run() hid a closed ledger error")
	}
	if _, err := scheduler.RunOnce(context.Background()); err == nil {
		t.Fatal("RunOnce() hid a closed ledger error")
	}
	if _, err := scheduler.RecoverOnce(context.Background()); err == nil {
		t.Fatal("RecoverOnce() hid a closed ledger error")
	}
	if _, err := scheduler.Wait(context.Background(), "work"); err == nil {
		t.Fatal("Wait() hid a closed ledger error")
	}
	events, errs := scheduler.Watch(context.Background(), "work", 0)
	for range events {
	}
	if err := <-errs; err == nil {
		t.Fatal("Watch() hid a closed ledger error")
	}
	if _, err := scheduler.Cancel(context.Background(), "work", "operator", "stop"); err == nil {
		t.Fatal("Cancel() hid a closed ledger error")
	}
	if _, err := scheduler.Resume(context.Background(), "work", "step", "operator", "retry"); err == nil {
		t.Fatal("Resume() hid a closed ledger error")
	}
	if err := scheduler.recordCapabilityOutcomes(context.Background(), Execution{
		Work: workstore.Work{
			WorkspaceID: scheduler.WorkspaceID(), ID: "work",
			MetadataJSON: json.RawMessage(`{"capabilities":{"version_ids":["missing"]}}`),
		},
		Claim: workstore.StepClaim{Attempt: workstore.Attempt{ID: "attempt"}},
	}, workstore.CapabilityOutcomeFailed, workstore.ProofStatusFailed, 0, time.Millisecond); err == nil {
		t.Fatal("recordCapabilityOutcomes() hid a closed ledger error")
	}

	var nilScheduler *Scheduler
	nilScheduler.Close()
}

func TestSchedulerReconcilesCancelledAndBlockedSteps(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	cancelScheduler := newTestScheduler(t, store, "workspace-reconcile-cancel", &executeOnlyExecutor{adapter: "fake"}, 1)
	cancelWork, err := cancelScheduler.Submit(context.Background(), SubmitInput{
		IdempotencyKey: "cancel", Title: "Cancel", Adapter: "fake", ActorID: "planner",
		Steps: []StepSpec{{Key: "run", Title: "Run", Policy: oneAttemptPolicy()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelProjection, err := store.GetWorkProjection(context.Background(), cancelWork.WorkspaceID, cancelWork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CancelScheduledStep(context.Background(), workstore.CancelScheduledStepInput{
		WorkspaceID: cancelWork.WorkspaceID, WorkID: cancelWork.ID, StepID: cancelProjection.Steps[0].ID,
		ActorID: "operator", Reason: "cancel edge",
	}); err != nil {
		t.Fatal(err)
	}
	if err := cancelScheduler.reconcileWork(context.Background(), cancelWork.ID); err != nil {
		t.Fatal(err)
	}
	cancelProjection, err = store.GetWorkProjection(context.Background(), cancelWork.WorkspaceID, cancelWork.ID)
	if err != nil || cancelProjection.Work.State != workstore.WorkStateCancelled {
		t.Fatalf("cancelled reconciliation projection=%+v err=%v", cancelProjection, err)
	}

	blockedScheduler := newTestScheduler(t, store, "workspace-reconcile-blocked", &fakeExecutor{
		adapter: "blocked", execute: func(context.Context, Execution) (ExecutionResult, error) {
			return ExecutionResult{}, errors.New("simulated blocked execution")
		},
	}, 1)
	blockedWork, err := blockedScheduler.Submit(context.Background(), SubmitInput{
		IdempotencyKey: "blocked", Title: "Blocked", Adapter: "blocked", ActorID: "planner",
		Steps: []StepSpec{{Key: "run", Title: "Run", Policy: workstore.StepSchedulePolicy{
			MaxAttempts: 1, EscalationState: workstore.WorkStateBlocked,
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := blockedScheduler.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	blockedProjection, err := blockedScheduler.Wait(context.Background(), blockedWork.ID)
	if err != nil || blockedProjection.Work.State != workstore.WorkStateBlocked {
		t.Fatalf("blocked reconciliation projection=%+v err=%v", blockedProjection, err)
	}
}

func TestSchedulerRunWatchCloseAndCancelableExecutionHonorCancellation(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	runScheduler := newTestScheduler(t, store, "workspace-run-context-cancel", &executeOnlyExecutor{adapter: "fake"}, 1)
	runScheduler.pollInterval = time.Hour
	runCtx, cancelRun := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- runScheduler.Run(runCtx) }()
	time.Sleep(10 * time.Millisecond)
	cancelRun()
	if err := <-runDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() cancellation error=%v", err)
	}

	scheduler := newTestScheduler(t, store, "workspace-context-cancel", &executeOnlyExecutor{adapter: "fake"}, 1)
	scheduler.pollInterval = time.Hour
	work, err := scheduler.Submit(context.Background(), SubmitInput{
		IdempotencyKey: "watch", Title: "Watch", Adapter: "fake", ActorID: "planner",
		Steps: []StepSpec{{Key: "run", Title: "Run", Policy: oneAttemptPolicy()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	events, errs := scheduler.Watch(watchCtx, work.ID, 1<<62)
	time.Sleep(10 * time.Millisecond)
	cancelWatch()
	for range events {
	}
	if err := <-errs; err != nil {
		t.Fatalf("Watch() cancellation error=%v", err)
	}

	projection, err := store.GetWorkProjection(context.Background(), work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatal(err)
	}
	activeCtx, cancelActive := context.WithCancel(context.Background())
	scheduler.activeMu.Lock()
	scheduler.active["cancelable"] = activeExecution{
		cancel: cancelActive, executor: &cancelErrorExecutor{},
		execution: Execution{
			Work: work,
			Claim: workstore.StepClaim{
				Step:    projection.Steps[0],
				Attempt: workstore.Attempt{ID: "cancelable"},
			},
		},
	}
	scheduler.activeMu.Unlock()
	if cancelled, err := scheduler.Cancel(context.Background(), work.ID, "operator", "stop"); err != nil || cancelled.Work.State != workstore.WorkStateCancelled {
		t.Fatalf("Cancel() projection=%+v err=%v", cancelled, err)
	}
	select {
	case <-activeCtx.Done():
	default:
		t.Fatal("Cancel() did not cancel the active execution context")
	}

	closeCtx, cancelClose := context.WithCancel(context.Background())
	closer := newTestScheduler(t, store, "workspace-close-active", &executeOnlyExecutor{adapter: "fake"}, 1)
	closer.activeMu.Lock()
	closer.active["close-active"] = activeExecution{cancel: cancelClose}
	closer.activeMu.Unlock()
	closer.Close()
	select {
	case <-closeCtx.Done():
	default:
		t.Fatal("Close() did not cancel the active execution context")
	}
}

func TestSchedulerRecoveryMissingResultAndFinalizerFailureAreAudited(t *testing.T) {
	t.Parallel()

	store := openSchedulerTestStore(t)
	seed := newTestScheduler(t, store, "workspace-recovery-missing", &fakeExecutor{adapter: "recover-missing"}, 1)
	work, err := seed.Submit(context.Background(), SubmitInput{
		IdempotencyKey: "recover-missing", Title: "Recover missing", Adapter: "recover-missing", ActorID: "planner",
		Steps: []StepSpec{{Key: "run", Title: "Run", Policy: oneAttemptPolicy()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PromoteReadySteps(context.Background(), workstore.PromoteReadyStepsInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ActorID: "seed",
	}); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimReadyStep(context.Background(), workstore.ClaimReadyStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, WorkerID: "lost-worker",
		Adapter: "recover-missing", LeaseDuration: time.Minute, ActorID: "seed",
	})
	if err != nil {
		t.Fatal(err)
	}
	recovered := newTestScheduler(t, store, work.WorkspaceID, &missingRecoveryExecutor{}, 1)
	if count, err := recovered.RecoverOnce(context.Background()); err != nil || count != 1 {
		t.Fatalf("RecoverOnce() missing result count=%d err=%v", count, err)
	}
	projection, err := recovered.Wait(context.Background(), work.ID)
	if err != nil || projection.Work.State != workstore.WorkStateReview || projection.Attempts[0].ID != claim.Attempt.ID || projection.Attempts[0].Status != workstore.AttemptStatusFailed {
		t.Fatalf("missing recovery projection=%+v err=%v", projection, err)
	}

	finalizerErrors := make(chan error, 4)
	finalizerStore := openSchedulerTestStore(t)
	finalizer, err := New(Options{
		Store: finalizerStore, WorkspaceID: "workspace-finalizer-edge", WorkerID: "worker", ActorID: "scheduler",
		LeaseDuration: time.Minute, HeartbeatInterval: time.Second, PollInterval: time.Millisecond,
		Executors: []Executor{&failingFinalizerExecutor{}}, OnError: func(err error) { finalizerErrors <- err },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(finalizer.Close)
	finalWork, err := finalizer.Submit(context.Background(), SubmitInput{
		IdempotencyKey: "finalizer", Title: "Finalizer", Adapter: "finalizer", ActorID: "planner",
		Steps: []StepSpec{{Key: "run", Title: "Run", Policy: oneAttemptPolicy()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finalizer.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if projection, err := finalizer.Wait(context.Background(), finalWork.ID); err != nil || projection.Work.State != workstore.WorkStateDone {
		t.Fatalf("finalized work projection=%+v err=%v", projection, err)
	}
	select {
	case err := <-finalizerErrors:
		if !strings.Contains(err.Error(), "finalize executor state") {
			t.Fatalf("finalizer error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("finalizer error was not reported")
	}
}

type executeOnlyExecutor struct{ adapter string }

func (executor *executeOnlyExecutor) Adapter() string { return executor.adapter }

func (*executeOnlyExecutor) Execute(context.Context, Execution) (ExecutionResult, error) {
	return ExecutionResult{Succeeded: true}, nil
}

type missingRecoveryExecutor struct{}

func (*missingRecoveryExecutor) Adapter() string { return "recover-missing" }

func (*missingRecoveryExecutor) Execute(context.Context, Execution) (ExecutionResult, error) {
	return ExecutionResult{}, nil
}

func (*missingRecoveryExecutor) Recover(context.Context, Execution) (ExecutionResult, bool, error) {
	return ExecutionResult{}, false, nil
}

type failingFinalizerExecutor struct{}

func (*failingFinalizerExecutor) Adapter() string { return "finalizer" }

func (*failingFinalizerExecutor) Execute(context.Context, Execution) (ExecutionResult, error) {
	return ExecutionResult{Succeeded: true, OutputJSON: json.RawMessage(`{"ok":true}`)}, nil
}

func (*failingFinalizerExecutor) Finalize(context.Context, Execution, ExecutionResult) error {
	return errors.New("simulated finalizer failure")
}

type cancelErrorExecutor struct{}

func (*cancelErrorExecutor) Adapter() string { return "fake" }

func (*cancelErrorExecutor) Execute(context.Context, Execution) (ExecutionResult, error) {
	return ExecutionResult{}, nil
}

func (*cancelErrorExecutor) Cancel(context.Context, Execution) error {
	return errors.New("simulated cancellation notification failure")
}
