package workscheduler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/workstore"
)

var ErrClosed = errors.New("workscheduler: scheduler is closed")

type Scheduler struct {
	store             *workstore.Store
	workspaceID       string
	workerID          string
	actorID           string
	leaseDuration     time.Duration
	heartbeatInterval time.Duration
	pollInterval      time.Duration
	maxWorkers        int
	executors         map[string]Executor
	verifiers         map[string]Verifier
	onError           func(error)

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	runMu     sync.Mutex
	activeMu  sync.Mutex
	active    map[string]activeExecution
	wg        sync.WaitGroup
}

type activeExecution struct {
	cancel    context.CancelFunc
	executor  Executor
	execution Execution
}

func New(opts Options) (*Scheduler, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("workscheduler: store is required")
	}
	workspaceID := strings.TrimSpace(opts.WorkspaceID)
	workerID := strings.TrimSpace(opts.WorkerID)
	actorID := strings.TrimSpace(opts.ActorID)
	if workspaceID == "" || workerID == "" || actorID == "" {
		return nil, fmt.Errorf("workscheduler: workspace, worker, and actor are required")
	}
	if opts.LeaseDuration <= 0 {
		opts.LeaseDuration = time.Minute
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = opts.LeaseDuration / 3
	}
	if opts.HeartbeatInterval >= opts.LeaseDuration {
		return nil, fmt.Errorf("workscheduler: heartbeat interval must be shorter than lease duration")
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 250 * time.Millisecond
	}
	if opts.MaxWorkers <= 0 {
		opts.MaxWorkers = 1
	}
	executors := make(map[string]Executor, len(opts.Executors))
	for _, executor := range opts.Executors {
		if executor == nil || strings.TrimSpace(executor.Adapter()) == "" {
			return nil, fmt.Errorf("workscheduler: executor adapter is required")
		}
		adapter := strings.TrimSpace(executor.Adapter())
		if _, exists := executors[adapter]; exists {
			return nil, fmt.Errorf("workscheduler: duplicate executor adapter %q", adapter)
		}
		executors[adapter] = executor
	}
	if len(executors) == 0 {
		return nil, fmt.Errorf("workscheduler: at least one executor is required")
	}
	verifiers := make(map[string]Verifier, len(opts.Verifiers))
	for _, verifier := range opts.Verifiers {
		if verifier == nil {
			return nil, fmt.Errorf("workscheduler: verifier is required")
		}
		name := strings.TrimSpace(verifier.Name())
		identity := verifier.Identity()
		identity.ID = strings.TrimSpace(identity.ID)
		if name == "" || identity.ID == "" {
			return nil, fmt.Errorf("workscheduler: verifier name and identity are required")
		}
		if identity.ID == workerID {
			return nil, fmt.Errorf("workscheduler: verifier %q must use an identity separate from worker %q", name, workerID)
		}
		if !meaningfulJSON(identity.EnvironmentJSON) {
			return nil, fmt.Errorf("workscheduler: verifier %q must declare its execution environment", name)
		}
		if _, exists := verifiers[name]; exists {
			return nil, fmt.Errorf("workscheduler: duplicate verifier %q", name)
		}
		verifiers[name] = verifier
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		store: opts.Store, workspaceID: workspaceID, workerID: workerID, actorID: actorID,
		leaseDuration: opts.LeaseDuration, heartbeatInterval: opts.HeartbeatInterval,
		pollInterval: opts.PollInterval, maxWorkers: opts.MaxWorkers, executors: executors,
		onError: opts.OnError, verifiers: verifiers, ctx: ctx, cancel: cancel, active: make(map[string]activeExecution),
	}, nil
}

// WorkspaceID identifies the isolation boundary owned by this scheduler.
func (s *Scheduler) WorkspaceID() string {
	if s == nil {
		return ""
	}
	return s.workspaceID
}

func (s *Scheduler) Submit(ctx context.Context, input SubmitInput) (workstore.Work, error) {
	if s == nil {
		return workstore.Work{}, ErrClosed
	}
	if err := s.ctx.Err(); err != nil {
		return workstore.Work{}, ErrClosed
	}
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	if input.WorkspaceID == "" {
		input.WorkspaceID = s.workspaceID
	}
	if input.WorkspaceID != s.workspaceID {
		return workstore.Work{}, fmt.Errorf("workscheduler: submission workspace does not match scheduler workspace")
	}
	input.Adapter = strings.TrimSpace(input.Adapter)
	if _, ok := s.executors[input.Adapter]; !ok {
		return workstore.Work{}, fmt.Errorf("workscheduler: executor adapter %q is not configured", input.Adapter)
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.ActorID) == "" {
		return workstore.Work{}, fmt.Errorf("workscheduler: idempotency key, title, and actor are required")
	}
	if len(input.Steps) == 0 {
		return workstore.Work{}, fmt.Errorf("workscheduler: at least one step is required")
	}
	if err := validateStepSpecs(input.Steps); err != nil {
		return workstore.Work{}, err
	}
	if err := s.validateProofVerifiers(input.Steps); err != nil {
		return workstore.Work{}, err
	}
	metadataJSON, err := schedulerMetadata(input.MetadataJSON, input.Adapter)
	if err != nil {
		return workstore.Work{}, err
	}
	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		kind = "workflow"
	}
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = "scheduler"
	}
	work, err := s.store.CreateWork(ctx, workstore.CreateWorkInput{
		WorkspaceID: input.WorkspaceID, Kind: kind, Source: source,
		SourceID: strings.TrimSpace(input.SourceID), IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
		CausationID: strings.TrimSpace(input.CausationID), ParentWorkID: strings.TrimSpace(input.ParentWorkID),
		Title: strings.TrimSpace(input.Title), Objective: strings.TrimSpace(input.Objective),
		ContractJSON: input.ContractJSON, MetadataJSON: metadataJSON,
		InitialState: workstore.WorkStateTodo, Priority: input.Priority, ActorID: strings.TrimSpace(input.ActorID),
	})
	if err != nil {
		return workstore.Work{}, err
	}
	if work.State != workstore.WorkStateTodo {
		return work, nil
	}
	stepsByKey := make(map[string]workstore.Step, len(input.Steps))
	for index, spec := range input.Steps {
		position := spec.Position
		if position <= 0 {
			position = index + 1
		}
		step, createErr := s.store.CreateStep(ctx, workstore.CreateStepInput{
			WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: strings.TrimSpace(spec.Key),
			CausationID: work.ID, Title: strings.TrimSpace(spec.Title), Description: strings.TrimSpace(spec.Description),
			State: workstore.WorkStateTodo, Position: position, ActorID: strings.TrimSpace(input.ActorID),
		})
		if createErr != nil {
			return workstore.Work{}, createErr
		}
		stepsByKey[strings.TrimSpace(spec.Key)] = step
	}
	for _, spec := range input.Steps {
		step := stepsByKey[strings.TrimSpace(spec.Key)]
		for _, dependencyKey := range spec.DependsOn {
			dependency := stepsByKey[strings.TrimSpace(dependencyKey)]
			if err := s.store.AddStepDependency(ctx, workstore.AddStepDependencyInput{
				WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
				DependsOnID: dependency.ID, ActorID: strings.TrimSpace(input.ActorID),
			}); err != nil {
				return workstore.Work{}, err
			}
		}
		if _, err := s.store.ConfigureStepSchedule(ctx, workstore.ConfigureStepScheduleInput{
			WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
			Policy: spec.Policy, ActorID: strings.TrimSpace(input.ActorID),
		}); err != nil {
			return workstore.Work{}, err
		}
	}
	work, err = s.store.GetWork(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		return workstore.Work{}, err
	}
	if work.State != workstore.WorkStateTodo {
		return work, nil
	}
	return s.store.TransitionWork(ctx, workstore.TransitionWorkInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ToState: workstore.WorkStateRunning,
		ExpectedVersion: work.Version, ActorID: strings.TrimSpace(input.ActorID),
		CausationID: work.ID, Reason: "durable scheduler accepted work",
	})
}

func (s *Scheduler) Run(ctx context.Context) error {
	if _, err := s.RecoverOnce(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.Close()
			return ctx.Err()
		case <-s.ctx.Done():
			return ErrClosed
		case <-ticker.C:
			if _, err := s.RunOnce(ctx); err != nil {
				s.reportError(err)
			}
		}
	}
}

func (s *Scheduler) RunOnce(ctx context.Context) (int, error) {
	if s == nil || s.ctx.Err() != nil {
		return 0, ErrClosed
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	reclaimed, err := s.store.ReclaimExpiredStepClaims(ctx, workstore.ReclaimExpiredStepClaimsInput{
		WorkspaceID: s.workspaceID, ActorID: s.actorID, Reason: "scheduler lease expired",
	})
	if err != nil {
		return 0, err
	}
	for _, resolution := range reclaimed {
		if err := s.reconcileWork(ctx, resolution.Step.WorkID); err != nil {
			return 0, err
		}
	}
	works, err := s.store.ListWorks(ctx, workstore.ListWorksFilter{
		WorkspaceID: s.workspaceID, States: []workstore.WorkState{workstore.WorkStateRunning}, Limit: 1000,
	})
	if err != nil {
		return 0, err
	}
	for _, work := range works {
		if _, err := s.store.PromoteReadySteps(ctx, workstore.PromoteReadyStepsInput{
			WorkspaceID: s.workspaceID, WorkID: work.ID, ActorID: s.actorID,
		}); err != nil {
			return 0, err
		}
	}
	capacity := s.maxWorkers - s.activeCount()
	if capacity <= 0 {
		return 0, nil
	}
	claimed := 0
	for claimed < capacity {
		progress := false
		for _, work := range works {
			if claimed >= capacity {
				break
			}
			executor, resolveErr := s.executorForWork(work)
			if resolveErr != nil {
				return claimed, resolveErr
			}
			claim, claimErr := s.store.ClaimReadyStep(ctx, workstore.ClaimReadyStepInput{
				WorkspaceID: s.workspaceID, WorkID: work.ID, WorkerID: s.workerID,
				Adapter: executor.Adapter(), LeaseDuration: s.leaseDuration, ActorID: s.actorID,
			})
			if errors.Is(claimErr, workstore.ErrNoReadyStep) {
				continue
			}
			if claimErr != nil {
				return claimed, claimErr
			}
			s.startExecution(work, claim, executor)
			claimed++
			progress = true
		}
		if !progress {
			break
		}
	}
	return claimed, nil
}

func (s *Scheduler) RecoverOnce(ctx context.Context) (int, error) {
	if s == nil || s.ctx.Err() != nil {
		return 0, ErrClosed
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	claims, err := s.store.ListActiveStepClaims(ctx, s.workspaceID, "")
	if err != nil {
		return 0, err
	}
	capacity := s.maxWorkers - s.activeCount()
	if capacity <= 0 {
		return 0, nil
	}
	count := 0
	for _, claim := range claims {
		if count >= capacity {
			break
		}
		work, getErr := s.store.GetWork(ctx, claim.Step.WorkspaceID, claim.Step.WorkID)
		if getErr != nil {
			return count, getErr
		}
		executor, ok := s.executors[strings.TrimSpace(claim.Attempt.Adapter)]
		if !ok {
			if _, reclaimErr := s.reclaimClaim(ctx, claim, "executor adapter is unavailable after restart"); reclaimErr != nil {
				return count, reclaimErr
			}
			if err := s.reconcileWork(ctx, work.ID); err != nil {
				return count, err
			}
			count++
			continue
		}
		recoverable, ok := executor.(RecoverableExecutor)
		if !ok {
			if _, reclaimErr := s.reclaimClaim(ctx, claim, "executor does not support reconnect"); reclaimErr != nil {
				return count, reclaimErr
			}
			if err := s.reconcileWork(ctx, work.ID); err != nil {
				return count, err
			}
			count++
			continue
		}
		s.startRecovery(work, claim, executor, recoverable)
		count++
	}
	return count, nil
}

func (s *Scheduler) Wait(ctx context.Context, workID string) (workstore.WorkProjection, error) {
	workID = strings.TrimSpace(workID)
	if workID == "" {
		return workstore.WorkProjection{}, fmt.Errorf("workscheduler: work id is required")
	}
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		projection, err := s.store.GetWorkProjection(ctx, s.workspaceID, workID)
		if err != nil {
			return workstore.WorkProjection{}, err
		}
		if isTerminalForWait(projection.Work.State) {
			return projection, nil
		}
		select {
		case <-ctx.Done():
			return workstore.WorkProjection{}, ctx.Err()
		case <-s.ctx.Done():
			return workstore.WorkProjection{}, ErrClosed
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) Watch(ctx context.Context, workID string, afterSequence int64) (<-chan workstore.Event, <-chan error) {
	events := make(chan workstore.Event, 16)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		ticker := time.NewTicker(s.pollInterval)
		defer ticker.Stop()
		for {
			listed, err := s.store.ListEvents(ctx, s.workspaceID, strings.TrimSpace(workID))
			if err != nil {
				errs <- err
				return
			}
			for _, event := range listed {
				if event.Sequence <= afterSequence {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case events <- event:
					afterSequence = event.Sequence
				}
			}
			work, err := s.store.GetWork(ctx, s.workspaceID, strings.TrimSpace(workID))
			if err != nil {
				errs <- err
				return
			}
			if isTerminalForWait(work.State) {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-s.ctx.Done():
				errs <- ErrClosed
				return
			case <-ticker.C:
			}
		}
	}()
	return events, errs
}

func (s *Scheduler) Cancel(ctx context.Context, workID, actorID, reason string) (workstore.WorkProjection, error) {
	if strings.TrimSpace(workID) == "" || strings.TrimSpace(actorID) == "" || strings.TrimSpace(reason) == "" {
		return workstore.WorkProjection{}, fmt.Errorf("workscheduler: work, actor, and reason are required to cancel")
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	projection, err := s.store.GetWorkProjection(ctx, s.workspaceID, strings.TrimSpace(workID))
	if err != nil {
		return workstore.WorkProjection{}, err
	}
	if projection.Work.State == workstore.WorkStateCancelled {
		return projection, nil
	}
	for _, step := range projection.Steps {
		if step.State == workstore.WorkStateDone || step.State == workstore.WorkStateCancelled {
			continue
		}
		if active, ok := s.activeForStep(step.ID); ok {
			if cancelable, ok := active.executor.(CancelableExecutor); ok {
				if cancelErr := cancelable.Cancel(ctx, active.execution); cancelErr != nil {
					s.reportError(cancelErr)
				}
			}
			active.cancel()
		}
		if _, err := s.store.CancelScheduledStep(ctx, workstore.CancelScheduledStepInput{
			WorkspaceID: s.workspaceID, WorkID: projection.Work.ID, StepID: step.ID,
			ActorID: strings.TrimSpace(actorID), Reason: strings.TrimSpace(reason),
		}); err != nil && !errors.Is(err, workstore.ErrInvalidTransition) {
			return workstore.WorkProjection{}, err
		}
	}
	if err := s.transitionWorkTo(ctx, projection.Work.ID, workstore.WorkStateCancelled, strings.TrimSpace(actorID), strings.TrimSpace(reason)); err != nil {
		return workstore.WorkProjection{}, err
	}
	return s.store.GetWorkProjection(ctx, s.workspaceID, projection.Work.ID)
}

func (s *Scheduler) Resume(ctx context.Context, workID, stepID, actorID, reason string) (workstore.WorkProjection, error) {
	if _, err := s.store.ResumeScheduledStep(ctx, workstore.ResumeScheduledStepInput{
		WorkspaceID: s.workspaceID, WorkID: strings.TrimSpace(workID), StepID: strings.TrimSpace(stepID),
		ActorID: strings.TrimSpace(actorID), Reason: strings.TrimSpace(reason),
	}); err != nil {
		return workstore.WorkProjection{}, err
	}
	if err := s.transitionWorkTo(ctx, strings.TrimSpace(workID), workstore.WorkStateRunning, strings.TrimSpace(actorID), strings.TrimSpace(reason)); err != nil {
		return workstore.WorkProjection{}, err
	}
	return s.store.GetWorkProjection(ctx, s.workspaceID, strings.TrimSpace(workID))
}

func (s *Scheduler) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.cancel()
		s.activeMu.Lock()
		for _, active := range s.active {
			active.cancel()
		}
		s.activeMu.Unlock()
		s.wg.Wait()
	})
}

func (s *Scheduler) startExecution(work workstore.Work, claim workstore.StepClaim, executor Executor) {
	s.startActive(work, claim, executor, func(ctx context.Context, execution Execution) (ExecutionResult, bool, error) {
		result, err := executor.Execute(ctx, execution)
		return result, true, err
	})
}

func (s *Scheduler) startRecovery(work workstore.Work, claim workstore.StepClaim, executor Executor, recoverable RecoverableExecutor) {
	s.startActive(work, claim, executor, recoverable.Recover)
}

func (s *Scheduler) startActive(
	work workstore.Work,
	claim workstore.StepClaim,
	executor Executor,
	run func(context.Context, Execution) (ExecutionResult, bool, error),
) {
	execCtx, cancel := context.WithCancel(s.ctx)
	execution := Execution{Work: work, Claim: claim}
	s.activeMu.Lock()
	if _, exists := s.active[claim.Attempt.ID]; exists {
		s.activeMu.Unlock()
		cancel()
		return
	}
	s.active[claim.Attempt.ID] = activeExecution{cancel: cancel, executor: executor, execution: execution}
	s.activeMu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			cancel()
			s.activeMu.Lock()
			delete(s.active, claim.Attempt.ID)
			s.activeMu.Unlock()
		}()
		heartbeatDone := make(chan struct{})
		go func() {
			defer close(heartbeatDone)
			s.heartbeat(execCtx, claim, cancel)
		}()
		result, found, err := run(execCtx, execution)
		wasCanceled := execCtx.Err() != nil
		if !found {
			cancel()
			<-heartbeatDone
			if _, reclaimErr := s.reclaimClaim(context.Background(), claim, "executor could not reconnect after restart"); reclaimErr != nil {
				s.reportError(reclaimErr)
			}
			if reconcileErr := s.reconcileWork(context.Background(), work.ID); reconcileErr != nil {
				s.reportError(reconcileErr)
			}
			return
		}
		if err != nil {
			result.Succeeded = false
			if strings.TrimSpace(result.Error) == "" {
				result.Error = err.Error()
			}
		}
		if wasCanceled {
			cancel()
			<-heartbeatDone
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, releaseErr := s.store.ReleaseStepClaim(releaseCtx, workstore.ReleaseStepClaimInput{
				WorkspaceID: claim.Step.WorkspaceID, WorkID: claim.Step.WorkID, StepID: claim.Step.ID,
				AttemptID: claim.Attempt.ID, WorkerID: claim.Schedule.LeaseOwner,
				ActorID: s.actorID, Reason: "scheduler execution stopped",
			})
			releaseCancel()
			if releaseErr != nil && !errors.Is(releaseErr, workstore.ErrClaimConflict) && !errors.Is(releaseErr, workstore.ErrClaimExpired) {
				s.reportError(releaseErr)
			}
			return
		}
		if result.Succeeded {
			if proofErr := s.recordVerificationProofs(execCtx, execution, result); proofErr != nil {
				s.reportError(proofErr)
			}
		}
		completeCtx, completeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, completeErr := s.store.CompleteStepAttempt(completeCtx, workstore.CompleteStepAttemptInput{
			WorkspaceID: claim.Step.WorkspaceID, WorkID: claim.Step.WorkID, StepID: claim.Step.ID,
			AttemptID: claim.Attempt.ID, WorkerID: claim.Schedule.LeaseOwner,
			Succeeded: result.Succeeded, OutputJSON: result.OutputJSON, ErrorText: strings.TrimSpace(result.Error),
			Usage: result.Usage, ActorID: s.actorID,
		})
		completeCancel()
		cancel()
		<-heartbeatDone
		if completeErr != nil && !errors.Is(completeErr, workstore.ErrClaimConflict) {
			s.reportError(completeErr)
		}
		if reconcileErr := s.reconcileWork(context.Background(), work.ID); reconcileErr != nil {
			s.reportError(reconcileErr)
		}
	}()
}

func (s *Scheduler) recordVerificationProofs(ctx context.Context, execution Execution, result ExecutionResult) error {
	claim := execution.Claim
	resultDigest := proofDigest(result.OutputJSON)
	resultInput, _ := json.Marshal(map[string]any{"output_digest": resultDigest})
	if _, err := s.store.CreateProof(ctx, workstore.CreateProofInput{
		WorkspaceID: claim.Step.WorkspaceID, WorkID: claim.Step.WorkID,
		StepID: claim.Step.ID, AttemptID: claim.Attempt.ID,
		IdempotencyKey: claim.Attempt.ID + ":worker-report", CausationID: claim.Attempt.ID,
		Kind: "worker-result", Status: workstore.ProofStatusReported,
		Origin: workstore.ProofOriginWorkerReport, Summary: "worker reported successful execution",
		ReporterID: claim.Schedule.LeaseOwner, InputJSON: resultInput,
		SubjectDigest: resultDigest, ActorID: s.actorID,
	}); err != nil {
		return fmt.Errorf("workscheduler: record worker success report: %w", err)
	}
	if !claim.Schedule.Policy.Proof.Required {
		return nil
	}
	for _, requirement := range claim.Schedule.Policy.Proof.Requirements {
		verifier, ok := s.verifiers[requirement.Verifier]
		if !ok {
			return fmt.Errorf("workscheduler: verifier %q is not configured", requirement.Verifier)
		}
		identity := verifier.Identity()
		requestInput, err := json.Marshal(map[string]any{
			"requirement":          requirement,
			"worker_output_digest": resultDigest,
		})
		if err != nil {
			return fmt.Errorf("workscheduler: encode verification input: %w", err)
		}
		pending, err := s.store.CreateProof(ctx, workstore.CreateProofInput{
			WorkspaceID: claim.Step.WorkspaceID, WorkID: claim.Step.WorkID,
			StepID: claim.Step.ID, AttemptID: claim.Attempt.ID,
			IdempotencyKey: claim.Attempt.ID + ":verification:" + requirement.Kind,
			CausationID:    claim.Attempt.ID, Kind: requirement.Kind,
			Status: workstore.ProofStatusPending, Origin: workstore.ProofOriginIndependentVerifier,
			Summary: "independent verification pending", ReporterID: claim.Schedule.LeaseOwner,
			VerifierID: strings.TrimSpace(identity.ID), Verifier: requirement.Verifier,
			Command: requirement.Command, EnvironmentJSON: identity.EnvironmentJSON,
			InputJSON: requestInput, SubjectDigest: resultDigest, ActorID: s.actorID,
		})
		if err != nil {
			return fmt.Errorf("workscheduler: create pending proof %q: %w", requirement.Kind, err)
		}
		if pending.Status == workstore.ProofStatusPassed || pending.Status == workstore.ProofStatusFailed {
			continue
		}
		if pending.Status == workstore.ProofStatusStale {
			pending, err = s.store.TransitionProof(ctx, workstore.TransitionProofInput{
				WorkspaceID: pending.WorkspaceID, WorkID: pending.WorkID, ProofID: pending.ID,
				ExpectedStatus: workstore.ProofStatusStale, ToStatus: workstore.ProofStatusPending,
				ActorID: s.actorID, Rationale: "reverification requested",
			})
			if err != nil {
				return fmt.Errorf("workscheduler: reopen stale proof %q: %w", requirement.Kind, err)
			}
		}
		verified, verifyErr := verifier.Verify(ctx, VerificationRequest{
			Execution: execution, Result: result, Requirement: requirement,
		})
		if verified.Status == "" {
			verified.Status = workstore.ProofStatusFailed
		}
		if verifyErr != nil {
			verified.Status = workstore.ProofStatusFailed
			if strings.TrimSpace(verified.Rationale) == "" {
				verified.Rationale = verifyErr.Error()
			}
		}
		if verified.UsedLLM {
			switch {
			case !claim.Schedule.Policy.Proof.AllowLLMFallback:
				verified.Status = workstore.ProofStatusFailed
				verified.Rationale = "LLM judge was used without an approved fallback policy"
			case verified.Tokens < 0 || verified.CostUSD < 0:
				verified.Status = workstore.ProofStatusFailed
				verified.Rationale = "LLM judge returned invalid usage"
			case verified.Tokens > claim.Schedule.Policy.Proof.MaxLLMTokens || verified.CostUSD > claim.Schedule.Policy.Proof.MaxLLMCostUSD:
				verified.Status = workstore.ProofStatusPending
				verified.Rationale = "LLM proof budget exceeded; operator review required"
			}
		}
		if verified.Status == workstore.ProofStatusPending {
			if _, err := s.store.TransitionProof(ctx, workstore.TransitionProofInput{
				WorkspaceID: pending.WorkspaceID, WorkID: pending.WorkID, ProofID: pending.ID,
				ExpectedStatus: workstore.ProofStatusPending, ToStatus: workstore.ProofStatusPending,
				Summary: verified.Summary, InputJSON: verified.InputJSON,
				ArtifactDigestsJSON: verified.ArtifactDigestsJSON,
				SubjectDigest:       verified.SubjectDigest, Rationale: verified.Rationale,
				ActorID: identity.ID, ObservedAt: verified.ObservedAt,
			}); err != nil {
				return fmt.Errorf("workscheduler: update pending proof %q: %w", requirement.Kind, err)
			}
			continue
		}
		if verified.Status != workstore.ProofStatusPassed && verified.Status != workstore.ProofStatusFailed {
			verified.Status = workstore.ProofStatusFailed
			verified.Rationale = "verifier returned an invalid terminal proof state"
		}
		if strings.TrimSpace(verified.SubjectDigest) == "" {
			verified.SubjectDigest = resultDigest
		}
		if strings.TrimSpace(verified.Summary) == "" {
			verified.Summary = "independent verification completed"
		}
		if strings.TrimSpace(verified.Rationale) == "" {
			verified.Rationale = verified.Summary
		}
		if _, err := s.store.TransitionProof(ctx, workstore.TransitionProofInput{
			WorkspaceID: pending.WorkspaceID, WorkID: pending.WorkID, ProofID: pending.ID,
			ExpectedStatus: workstore.ProofStatusPending, ToStatus: verified.Status, Summary: verified.Summary,
			InputJSON: verified.InputJSON, ArtifactDigestsJSON: verified.ArtifactDigestsJSON,
			SubjectDigest: verified.SubjectDigest, Rationale: verified.Rationale,
			ActorID: identity.ID, ObservedAt: verified.ObservedAt,
		}); err != nil {
			return fmt.Errorf("workscheduler: finalize proof %q: %w", requirement.Kind, err)
		}
	}
	return nil
}

func (s *Scheduler) heartbeat(ctx context.Context, claim workstore.StepClaim, cancel context.CancelFunc) {
	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()
	leaseExpiresAt := time.Time{}
	if claim.Schedule.LeaseExpiresAt != nil {
		leaseExpiresAt = *claim.Schedule.LeaseExpiresAt
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			heartbeatCtx, heartbeatCancel := context.WithTimeout(context.Background(), s.heartbeatRequestTimeout())
			schedule, err := s.store.HeartbeatStepClaim(heartbeatCtx, workstore.HeartbeatStepClaimInput{
				WorkspaceID: claim.Step.WorkspaceID, WorkID: claim.Step.WorkID, StepID: claim.Step.ID,
				AttemptID: claim.Attempt.ID, WorkerID: claim.Schedule.LeaseOwner,
				LeaseDuration: s.leaseDuration, ActorID: s.actorID,
			})
			heartbeatCancel()
			if err != nil {
				s.reportError(fmt.Errorf("workscheduler: heartbeat claim %s: %w", claim.Attempt.ID, err))
				if shouldStopHeartbeat(err, leaseExpiresAt, time.Now()) {
					cancel()
					return
				}
				continue
			}
			if schedule.LeaseExpiresAt != nil {
				leaseExpiresAt = *schedule.LeaseExpiresAt
			}
		}
	}
}

func (s *Scheduler) heartbeatRequestTimeout() time.Duration {
	timeout := s.heartbeatInterval
	floor := time.Second
	if halfLease := s.leaseDuration / 2; halfLease > 0 && halfLease < floor {
		floor = halfLease
	}
	if timeout < floor {
		timeout = floor
	}
	return timeout
}

func shouldStopHeartbeat(err error, leaseExpiresAt, now time.Time) bool {
	if errors.Is(err, workstore.ErrClaimConflict) || errors.Is(err, workstore.ErrClaimExpired) {
		return true
	}
	return !leaseExpiresAt.IsZero() && !now.Before(leaseExpiresAt)
}

func (s *Scheduler) reclaimClaim(ctx context.Context, claim workstore.StepClaim, reason string) (workstore.StepResolution, error) {
	return s.store.ReclaimStepClaim(ctx, workstore.ReclaimStepClaimInput{
		WorkspaceID: claim.Step.WorkspaceID, WorkID: claim.Step.WorkID, StepID: claim.Step.ID,
		AttemptID: claim.Attempt.ID, WorkerID: claim.Schedule.LeaseOwner,
		ActorID: s.actorID, Reason: reason,
	})
}

func (s *Scheduler) reconcileWork(ctx context.Context, workID string) error {
	projection, err := s.store.GetWorkProjection(ctx, s.workspaceID, workID)
	if err != nil {
		return err
	}
	if len(projection.Steps) == 0 || projection.Work.State == workstore.WorkStateCancelled || projection.Work.State == workstore.WorkStateDone {
		return nil
	}
	target := workstore.WorkStateRunning
	allDone := true
	anyCancelled := false
	anyReview := false
	anyBlocked := false
	for _, step := range projection.Steps {
		allDone = allDone && step.State == workstore.WorkStateDone
		anyCancelled = anyCancelled || step.State == workstore.WorkStateCancelled
		anyReview = anyReview || step.State == workstore.WorkStateReview
		anyBlocked = anyBlocked || step.State == workstore.WorkStateBlocked
	}
	switch {
	case anyCancelled:
		target = workstore.WorkStateCancelled
	case anyReview:
		target = workstore.WorkStateReview
	case anyBlocked:
		target = workstore.WorkStateBlocked
	case allDone:
		target = workstore.WorkStateDone
	}
	return s.transitionWorkTo(ctx, workID, target, s.actorID, "scheduler reconciled step states")
}

func (s *Scheduler) transitionWorkTo(ctx context.Context, workID string, target workstore.WorkState, actorID, reason string) error {
	for range 8 {
		work, err := s.store.GetWork(ctx, s.workspaceID, workID)
		if err != nil {
			return err
		}
		if work.State == target {
			return nil
		}
		next, ok := nextWorkState(work.State, target)
		if !ok {
			return fmt.Errorf("workscheduler: cannot transition work %s from %s to %s", workID, work.State, target)
		}
		_, err = s.store.TransitionWork(ctx, workstore.TransitionWorkInput{
			WorkspaceID: s.workspaceID, WorkID: workID, ToState: next,
			ExpectedVersion: work.Version, ActorID: actorID, CausationID: workID, Reason: reason,
		})
		if errors.Is(err, workstore.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return workstore.ErrConflict
}

func nextWorkState(current, target workstore.WorkState) (workstore.WorkState, bool) {
	if target == workstore.WorkStateCancelled {
		return target, true
	}
	switch current {
	case workstore.WorkStateTriage, workstore.WorkStateBacklog:
		return workstore.WorkStateTodo, true
	case workstore.WorkStateTodo, workstore.WorkStateReady:
		if target == workstore.WorkStateBlocked {
			return target, true
		}
		return workstore.WorkStateRunning, true
	case workstore.WorkStateBlocked:
		return workstore.WorkStateReady, true
	case workstore.WorkStateReview:
		if target == workstore.WorkStateDone || target == workstore.WorkStateBlocked {
			return target, true
		}
		return workstore.WorkStateRunning, true
	case workstore.WorkStateRunning:
		return target, true
	default:
		return "", false
	}
}

func (s *Scheduler) executorForWork(work workstore.Work) (Executor, error) {
	var metadata struct {
		Scheduler struct {
			Adapter string `json:"adapter"`
		} `json:"scheduler"`
	}
	if err := json.Unmarshal(work.MetadataJSON, &metadata); err != nil {
		return nil, fmt.Errorf("workscheduler: decode work scheduler metadata: %w", err)
	}
	adapter := strings.TrimSpace(metadata.Scheduler.Adapter)
	executor, ok := s.executors[adapter]
	if !ok {
		return nil, fmt.Errorf("workscheduler: executor adapter %q is not configured for work %s", adapter, work.ID)
	}
	return executor, nil
}

func schedulerMetadata(raw json.RawMessage, adapter string) (json.RawMessage, error) {
	metadata := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return nil, fmt.Errorf("workscheduler: metadata must be a JSON object: %w", err)
		}
	}
	metadata["scheduler"] = map[string]any{"adapter": strings.TrimSpace(adapter), "schema_version": 1}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("workscheduler: encode scheduler metadata: %w", err)
	}
	return encoded, nil
}

func validateStepSpecs(steps []StepSpec) error {
	seen := make(map[string]struct{}, len(steps))
	graph := make(map[string][]string, len(steps))
	for index, step := range steps {
		key := strings.TrimSpace(step.Key)
		if key == "" || strings.TrimSpace(step.Title) == "" {
			return fmt.Errorf("workscheduler: step %d key and title are required", index+1)
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("workscheduler: duplicate step key %q", key)
		}
		seen[key] = struct{}{}
		graph[key] = nil
	}
	for _, step := range steps {
		key := strings.TrimSpace(step.Key)
		for _, dependency := range step.DependsOn {
			dependency = strings.TrimSpace(dependency)
			if _, exists := seen[dependency]; !exists {
				return fmt.Errorf("workscheduler: step %q depends on unknown step %q", strings.TrimSpace(step.Key), dependency)
			}
			if dependency == strings.TrimSpace(step.Key) {
				return fmt.Errorf("workscheduler: step %q cannot depend on itself", dependency)
			}
			graph[key] = append(graph[key], dependency)
		}
	}
	states := make(map[string]uint8, len(graph))
	var visit func(string) bool
	visit = func(key string) bool {
		if states[key] == 1 {
			return true
		}
		if states[key] == 2 {
			return false
		}
		states[key] = 1
		for _, dependency := range graph[key] {
			if visit(dependency) {
				return true
			}
		}
		states[key] = 2
		return false
	}
	for key := range graph {
		if visit(key) {
			return fmt.Errorf("%w: step %q closes a cycle", workstore.ErrDependencyCycle, key)
		}
	}
	return nil
}

func (s *Scheduler) validateProofVerifiers(steps []StepSpec) error {
	for _, step := range steps {
		if !step.Policy.Proof.Required {
			continue
		}
		if len(step.Policy.Proof.Requirements) == 0 {
			return fmt.Errorf("workscheduler: proof-gated step %q needs at least one verifier requirement", strings.TrimSpace(step.Key))
		}
		for _, requirement := range step.Policy.Proof.Requirements {
			name := strings.TrimSpace(requirement.Verifier)
			if _, ok := s.verifiers[name]; !ok {
				return fmt.Errorf("workscheduler: proof-gated step %q requires unconfigured verifier %q", strings.TrimSpace(step.Key), name)
			}
		}
	}
	return nil
}

func meaningfulJSON(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	switch typed := value.(type) {
	case map[string]any:
		return len(typed) > 0
	case []any:
		return len(typed) > 0
	default:
		return typed != nil
	}
}

func proofDigest(raw json.RawMessage) string {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	digest := sha256.Sum256(raw)
	return fmt.Sprintf("sha256:%x", digest[:])
}

func (s *Scheduler) activeCount() int {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	return len(s.active)
}

func (s *Scheduler) activeForStep(stepID string) (activeExecution, bool) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	for _, active := range s.active {
		if active.execution.Claim.Step.ID == stepID {
			return active, true
		}
	}
	return activeExecution{}, false
}

func (s *Scheduler) reportError(err error) {
	if err != nil && s.onError != nil {
		s.onError(err)
	}
}

func isTerminalForWait(state workstore.WorkState) bool {
	switch state {
	case workstore.WorkStateReview, workstore.WorkStateBlocked, workstore.WorkStateDone, workstore.WorkStateCancelled:
		return true
	default:
		return false
	}
}
