package workstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOpenAppliesSchemaWALAndForeignKeys(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")
	store := openTestStore(t, path)

	var journalMode string
	if err := store.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("read journal mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal mode = %q, want wal", journalMode)
	}

	var foreignKeys int
	if err := store.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign key mode: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign keys = %d, want 1", foreignKeys)
	}

	var version int
	var migrationCount int
	if err := store.db.QueryRowContext(ctx, "SELECT MAX(version), COUNT(*) FROM schema_migrations").Scan(&version, &migrationCount); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != schemaVersion || migrationCount != schemaVersion {
		t.Fatalf("schema version/count = %d/%d, want %d/%d", version, migrationCount, schemaVersion, schemaVersion)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat ledger: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("ledger permissions = %o, want 600", got)
	}

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO steps (id, workspace_id, work_id, title, state, actor_id, version, created_at, updated_at)
		VALUES ('stp_orphan', 'workspace-a', 'wrk_missing', 'orphan', 'todo', 'tester', 1, 0, 0)
	`); err == nil {
		t.Fatal("foreign key constraint accepted an orphan step")
	}
}

func TestCreateWorkIsIdempotentConcurrentAndWorkspaceScoped(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	input := CreateWorkInput{
		WorkspaceID:    "workspace-a",
		Kind:           "task",
		Source:         "session",
		SourceID:       "task-42",
		IdempotencyKey: "session:task-42",
		CausationID:    "goal-7",
		Title:          "Persist the task",
		Objective:      "Survive a process restart",
		ActorID:        "scheduler",
	}

	const callers = 12
	results := make(chan Work, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work, err := store.CreateWork(ctx, input)
			results <- work
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent create: %v", err)
		}
	}
	var workID string
	for work := range results {
		if workID == "" {
			workID = work.ID
		}
		if work.ID != workID {
			t.Fatalf("idempotent create returned %q and %q", workID, work.ID)
		}
		if work.State != WorkStateTriage || work.Version != 1 {
			t.Fatalf("created work state/version = %s/%d, want triage/1", work.State, work.Version)
		}
	}

	works, err := store.ListWorks(ctx, ListWorksFilter{WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("list workspace-a: %v", err)
	}
	if len(works) != 1 {
		t.Fatalf("workspace-a work count = %d, want 1", len(works))
	}
	events, err := store.ListEvents(ctx, "workspace-a", workID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 || events[0].Type != EventTypeWorkCreated {
		t.Fatalf("created events = %#v, want one %q", events, EventTypeWorkCreated)
	}

	if _, err := store.GetWork(ctx, "workspace-b", workID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace get error = %v, want ErrNotFound", err)
	}
	other, err := store.CreateWork(ctx, CreateWorkInput{
		WorkspaceID:    "workspace-b",
		Kind:           input.Kind,
		IdempotencyKey: input.IdempotencyKey,
		Title:          input.Title,
		ActorID:        input.ActorID,
	})
	if err != nil {
		t.Fatalf("same idempotency key in another workspace: %v", err)
	}
	if other.ID == workID {
		t.Fatal("workspace-scoped idempotency returned the same work ID")
	}
}

func TestTransitionWorkCommitsStateAndEventAtomically(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "atomic-work")

	transitioned, err := store.TransitionWork(ctx, TransitionWorkInput{
		WorkspaceID:     work.WorkspaceID,
		WorkID:          work.ID,
		ToState:         WorkStateBacklog,
		ExpectedVersion: 1,
		ActorID:         "triager",
		CausationID:     "request-1",
		IdempotencyKey:  "transition:backlog",
		Reason:          "accepted for planning",
	})
	if err != nil {
		t.Fatalf("transition work: %v", err)
	}
	if transitioned.State != WorkStateBacklog || transitioned.Version != 2 {
		t.Fatalf("transition state/version = %s/%d, want backlog/2", transitioned.State, transitioned.Version)
	}
	events, err := store.ListEvents(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	last := events[1]
	if last.Type != EventTypeWorkTransitioned || last.FromState != WorkStateTriage || last.ToState != WorkStateBacklog || last.CausationID != "request-1" {
		t.Fatalf("transition event = %#v", last)
	}
	replayed, err := store.TransitionWork(ctx, TransitionWorkInput{
		WorkspaceID:     work.WorkspaceID,
		WorkID:          work.ID,
		ToState:         WorkStateBacklog,
		ExpectedVersion: 1,
		ActorID:         "triager",
		CausationID:     "request-1",
		IdempotencyKey:  "transition:backlog",
	})
	if err != nil {
		t.Fatalf("replay idempotent transition: %v", err)
	}
	if replayed.Version != 2 || replayed.State != WorkStateBacklog {
		t.Fatalf("replayed transition state/version = %s/%d, want backlog/2", replayed.State, replayed.Version)
	}
	events, err = store.ListEvents(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("list replayed events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("event count after replay = %d, want 2", len(events))
	}

	if _, err := store.TransitionWork(ctx, TransitionWorkInput{
		WorkspaceID:     work.WorkspaceID,
		WorkID:          work.ID,
		ToState:         WorkStateDone,
		ExpectedVersion: 1,
		ActorID:         "stale-client",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale transition error = %v, want ErrConflict", err)
	}
	if _, err := store.TransitionWork(ctx, TransitionWorkInput{
		WorkspaceID:     work.WorkspaceID,
		WorkID:          work.ID,
		ToState:         WorkStateDone,
		ExpectedVersion: 2,
		ActorID:         "triager",
	}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("invalid transition error = %v, want ErrInvalidTransition", err)
	}

	if _, err := store.db.ExecContext(ctx, `
		CREATE TRIGGER fail_transition_event
		BEFORE INSERT ON events
		WHEN NEW.event_type = 'work.transitioned'
		BEGIN
			SELECT RAISE(ABORT, 'forced event failure');
		END
	`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	if _, err := store.TransitionWork(ctx, TransitionWorkInput{
		WorkspaceID:     work.WorkspaceID,
		WorkID:          work.ID,
		ToState:         WorkStateTodo,
		ExpectedVersion: 2,
		ActorID:         "planner",
	}); err == nil {
		t.Fatal("transition succeeded even though event insert failed")
	}
	persisted, err := store.GetWork(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("get work after rollback: %v", err)
	}
	if persisted.State != WorkStateBacklog || persisted.Version != 2 {
		t.Fatalf("rolled-back work state/version = %s/%d, want backlog/2", persisted.State, persisted.Version)
	}
}

func TestCreateWorkMaintainsParentAndTerminalStateInvariants(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	parent := mustCreateWork(t, store, "workspace-a", "parent-work")
	terminal, err := store.CreateWork(ctx, CreateWorkInput{
		WorkspaceID:    parent.WorkspaceID,
		Kind:           "task",
		IdempotencyKey: "completed-child",
		ParentWorkID:   parent.ID,
		Title:          "Imported completed task",
		InitialState:   WorkStateDone,
		ActorID:        "importer",
	})
	if err != nil {
		t.Fatalf("create terminal child: %v", err)
	}
	if terminal.CompletedAt == nil {
		t.Fatal("terminal work has no completion timestamp")
	}

	foreignParent := mustCreateWork(t, store, "workspace-b", "foreign-parent")
	if _, err := store.CreateWork(ctx, CreateWorkInput{
		WorkspaceID:    parent.WorkspaceID,
		Kind:           "task",
		IdempotencyKey: "cross-workspace-child",
		ParentWorkID:   foreignParent.ID,
		Title:          "Invalid child",
		ActorID:        "importer",
	}); err == nil {
		t.Fatal("cross-workspace parent was accepted")
	}
}

func TestProjectionPersistsLedgerRecordsAndGuardsDependencies(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "projection-work")
	first, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		IdempotencyKey: "step:first",
		Title:          "Run command",
		State:          WorkStateDone,
		Position:       1,
		ActorID:        "planner",
	})
	if err != nil {
		t.Fatalf("create first step: %v", err)
	}
	if first.CompletedAt == nil {
		t.Fatal("terminal step has no completion timestamp")
	}
	second, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		IdempotencyKey: "step:second",
		Title:          "Verify output",
		State:          WorkStateTodo,
		Position:       2,
		ActorID:        "planner",
	})
	if err != nil {
		t.Fatalf("create second step: %v", err)
	}
	if err := store.AddStepDependency(ctx, AddStepDependencyInput{
		WorkspaceID: work.WorkspaceID,
		WorkID:      work.ID,
		StepID:      second.ID,
		DependsOnID: first.ID,
		ActorID:     "planner",
	}); err != nil {
		t.Fatalf("add dependency: %v", err)
	}
	if err := store.AddStepDependency(ctx, AddStepDependencyInput{
		WorkspaceID: work.WorkspaceID,
		WorkID:      work.ID,
		StepID:      first.ID,
		DependsOnID: second.ID,
		ActorID:     "planner",
	}); !errors.Is(err, ErrDependencyCycle) {
		t.Fatalf("cycle error = %v, want ErrDependencyCycle", err)
	}

	otherWork := mustCreateWork(t, store, work.WorkspaceID, "other-work")
	otherStep, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID:    otherWork.WorkspaceID,
		WorkID:         otherWork.ID,
		IdempotencyKey: "step:other",
		Title:          "Other work",
		State:          WorkStateTodo,
		ActorID:        "planner",
	})
	if err != nil {
		t.Fatalf("create other step: %v", err)
	}
	if err := store.AddStepDependency(ctx, AddStepDependencyInput{
		WorkspaceID: work.WorkspaceID,
		WorkID:      work.ID,
		StepID:      second.ID,
		DependsOnID: otherStep.ID,
		ActorID:     "planner",
	}); !errors.Is(err, ErrInvalidDependency) {
		t.Fatalf("cross-work dependency error = %v, want ErrInvalidDependency", err)
	}

	attempt, err := store.CreateAttempt(ctx, CreateAttemptInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		StepID:         second.ID,
		IdempotencyKey: "attempt:1",
		Number:         1,
		Adapter:        "local",
		Status:         AttemptStatusRunning,
		ActorID:        "worker-1",
		InputJSON:      []byte(`{"command":"go test ./..."}`),
	})
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	approval, err := store.CreateApproval(ctx, CreateApprovalInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		StepID:         second.ID,
		AttemptID:      attempt.ID,
		IdempotencyKey: "approval:push",
		Authority:      "external-write",
		Status:         ApprovalStatusPending,
		Request:        "Allow git push",
		ActorID:        "worker-1",
	})
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}
	artifact, err := store.CreateArtifact(ctx, CreateArtifactInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		StepID:         second.ID,
		AttemptID:      attempt.ID,
		IdempotencyKey: "artifact:test-log",
		Kind:           "test-log",
		Name:           "go-test.txt",
		URI:            "file:///workspace/go-test.txt",
		Digest:         "sha256:abcd",
		MediaType:      "text/plain",
		SizeBytes:      42,
		ActorID:        "worker-1",
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	proof, err := store.CreateProof(ctx, CreateProofInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		StepID:         second.ID,
		AttemptID:      attempt.ID,
		IdempotencyKey: "proof:test",
		Kind:           "test",
		Status:         ProofStatusPassed,
		Summary:        "all tests passed",
		Verifier:       "go-test",
		Command:        "go test ./...",
		ArtifactID:     artifact.ID,
		ActorID:        "worker-1",
	})
	if err != nil {
		t.Fatalf("create proof: %v", err)
	}

	projection, err := store.GetWorkProjection(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("get projection: %v", err)
	}
	if len(projection.Steps) != 2 || len(projection.Dependencies) != 1 || len(projection.Attempts) != 1 || len(projection.Approvals) != 1 || len(projection.Artifacts) != 1 || len(projection.Proofs) != 1 {
		t.Fatalf("projection counts = steps:%d deps:%d attempts:%d approvals:%d artifacts:%d proofs:%d",
			len(projection.Steps), len(projection.Dependencies), len(projection.Attempts), len(projection.Approvals), len(projection.Artifacts), len(projection.Proofs))
	}
	if projection.Approvals[0].ID != approval.ID || projection.Proofs[0].ID != proof.ID {
		t.Fatalf("projection records do not match created records")
	}
	if projection.Work.SchemaVersion != recordSchemaVersion ||
		projection.Steps[0].SchemaVersion != recordSchemaVersion ||
		projection.Attempts[0].SchemaVersion != recordSchemaVersion ||
		projection.Approvals[0].SchemaVersion != recordSchemaVersion ||
		projection.Artifacts[0].SchemaVersion != recordSchemaVersion ||
		projection.Proofs[0].SchemaVersion != recordSchemaVersion {
		t.Fatalf("projection contains a record with an unexpected schema version")
	}

	indexes := tableIndexes(t, store, "works")
	if !contains(indexes, "idx_works_workspace_state_updated") {
		t.Fatalf("works indexes = %v, missing workspace/state index", indexes)
	}
	eventIndexes := tableIndexes(t, store, "events")
	if !contains(eventIndexes, "idx_events_workspace_work_sequence") {
		t.Fatalf("event indexes = %v, missing workspace/work/sequence index", eventIndexes)
	}
}

func TestOpenReappliesMigrationIdempotently(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")
	store := openTestStore(t, path)
	work := mustCreateWork(t, store, "workspace-a", "reopen-work")
	if err := store.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	reopened := openTestStore(t, path)
	persisted, err := reopened.GetWork(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("get persisted work: %v", err)
	}
	if persisted.ID != work.ID {
		t.Fatalf("persisted work ID = %q, want %q", persisted.ID, work.ID)
	}
	var count int
	if err := reopened.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", schemaVersion).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration row count = %d, want 1", count)
	}
}

func TestRecordCreationCommandsAreIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "idempotent-records")
	stepInput := CreateStepInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		IdempotencyKey: "step:one",
		Title:          "One step",
		ActorID:        "planner",
	}
	step, err := store.CreateStep(ctx, stepInput)
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	replayedStep, err := store.CreateStep(ctx, stepInput)
	if err != nil || replayedStep.ID != step.ID {
		t.Fatalf("replay step = %#v, %v; want ID %s", replayedStep, err, step.ID)
	}

	attemptInput := CreateAttemptInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		StepID:         step.ID,
		IdempotencyKey: "attempt:one",
		Number:         1,
		Adapter:        "local",
		Status:         AttemptStatusPending,
		ActorID:        "worker",
	}
	attempt, err := store.CreateAttempt(ctx, attemptInput)
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	replayedAttempt, err := store.CreateAttempt(ctx, attemptInput)
	if err != nil || replayedAttempt.ID != attempt.ID {
		t.Fatalf("replay attempt = %#v, %v; want ID %s", replayedAttempt, err, attempt.ID)
	}

	expiresAt := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	approvalInput := CreateApprovalInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		StepID:         step.ID,
		AttemptID:      attempt.ID,
		IdempotencyKey: "approval:one",
		Authority:      "network",
		Status:         ApprovalStatusPending,
		Request:        "Allow request",
		ActorID:        "worker",
		ExpiresAt:      &expiresAt,
	}
	approval, err := store.CreateApproval(ctx, approvalInput)
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}
	replayedApproval, err := store.CreateApproval(ctx, approvalInput)
	if err != nil || replayedApproval.ID != approval.ID {
		t.Fatalf("replay approval = %#v, %v; want ID %s", replayedApproval, err, approval.ID)
	}

	artifactInput := CreateArtifactInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		StepID:         step.ID,
		AttemptID:      attempt.ID,
		IdempotencyKey: "artifact:one",
		Kind:           "log",
		Name:           "result.log",
		URI:            "file:///result.log",
		Digest:         "sha256:1234",
		ActorID:        "worker",
	}
	artifact, err := store.CreateArtifact(ctx, artifactInput)
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	replayedArtifact, err := store.CreateArtifact(ctx, artifactInput)
	if err != nil || replayedArtifact.ID != artifact.ID {
		t.Fatalf("replay artifact = %#v, %v; want ID %s", replayedArtifact, err, artifact.ID)
	}

	proofInput := CreateProofInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		StepID:         step.ID,
		AttemptID:      attempt.ID,
		IdempotencyKey: "proof:one",
		Kind:           "test",
		Status:         ProofStatusPassed,
		Summary:        "passed",
		ArtifactID:     artifact.ID,
		ActorID:        "worker",
	}
	proof, err := store.CreateProof(ctx, proofInput)
	if err != nil {
		t.Fatalf("create proof: %v", err)
	}
	replayedProof, err := store.CreateProof(ctx, proofInput)
	if err != nil || replayedProof.ID != proof.ID {
		t.Fatalf("replay proof = %#v, %v; want ID %s", replayedProof, err, proof.ID)
	}

	events, err := store.ListEvents(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 6 {
		t.Fatalf("events after replayed commands = %d, want 6", len(events))
	}
}

func TestValidationFilteringAndDefaultIDs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")
	store, err := Open(ctx, path, Options{})
	if err != nil {
		t.Fatalf("open with defaults: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	triage, err := store.CreateWork(ctx, CreateWorkInput{
		WorkspaceID:    "workspace-a",
		Kind:           "task",
		IdempotencyKey: "default-id",
		Title:          "Default ID",
		ActorID:        "tester",
	})
	if err != nil {
		t.Fatalf("create with default id generator: %v", err)
	}
	if !strings.HasPrefix(triage.ID, "wrk_") || len(triage.ID) != len("wrk_")+32 {
		t.Fatalf("default work ID = %q, want wrk_ plus 32 hex characters", triage.ID)
	}
	if _, err := store.CreateWork(ctx, CreateWorkInput{
		WorkspaceID:    "workspace-a",
		Kind:           "task",
		IdempotencyKey: "backlog",
		Title:          "Backlog",
		InitialState:   WorkStateBacklog,
		ActorID:        "tester",
	}); err != nil {
		t.Fatalf("create backlog work: %v", err)
	}
	filtered, err := store.ListWorks(ctx, ListWorksFilter{
		WorkspaceID: "workspace-a",
		States:      []WorkState{WorkStateBacklog},
		Limit:       1,
		Offset:      -1,
	})
	if err != nil {
		t.Fatalf("filter works: %v", err)
	}
	if len(filtered) != 1 || filtered[0].State != WorkStateBacklog {
		t.Fatalf("filtered works = %#v", filtered)
	}

	if _, err := store.CreateWork(ctx, CreateWorkInput{}); err == nil {
		t.Fatal("empty work input was accepted")
	}
	if _, err := store.CreateWork(ctx, CreateWorkInput{
		WorkspaceID: "workspace-a", Kind: "task", IdempotencyKey: "bad-state",
		Title: "Bad", ActorID: "tester", InitialState: WorkState("unknown"),
	}); err == nil {
		t.Fatal("invalid initial state was accepted")
	}
	if _, err := store.CreateWork(ctx, CreateWorkInput{
		WorkspaceID: "workspace-a", Kind: "task", IdempotencyKey: "bad-json",
		Title: "Bad", ActorID: "tester", ContractJSON: []byte("{"),
	}); err == nil {
		t.Fatal("invalid work JSON was accepted")
	}
	if _, err := store.ListWorks(ctx, ListWorksFilter{}); err == nil {
		t.Fatal("list without workspace was accepted")
	}
	if _, err := store.ListWorks(ctx, ListWorksFilter{WorkspaceID: "workspace-a", States: []WorkState{"unknown"}}); err == nil {
		t.Fatal("list with invalid state was accepted")
	}
	if _, err := store.CreateStep(ctx, CreateStepInput{}); err == nil {
		t.Fatal("empty step input was accepted")
	}
	if _, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: "workspace-a", WorkID: triage.ID, IdempotencyKey: "bad-step",
		Title: "Bad", ActorID: "tester", State: WorkState("unknown"),
	}); err == nil {
		t.Fatal("invalid step state was accepted")
	}
	if _, err := store.CreateAttempt(ctx, CreateAttemptInput{}); err == nil {
		t.Fatal("empty attempt input was accepted")
	}
	if _, err := store.CreateApproval(ctx, CreateApprovalInput{}); err == nil {
		t.Fatal("empty approval input was accepted")
	}
	if _, err := store.CreateArtifact(ctx, CreateArtifactInput{}); err == nil {
		t.Fatal("empty artifact input was accepted")
	}
	if _, err := store.CreateProof(ctx, CreateProofInput{}); err == nil {
		t.Fatal("empty proof input was accepted")
	}
	if _, err := store.TransitionWork(ctx, TransitionWorkInput{}); err == nil {
		t.Fatal("empty transition input was accepted")
	}
	if _, err := store.TransitionWork(ctx, TransitionWorkInput{
		WorkspaceID: "workspace-a", WorkID: triage.ID, ActorID: "tester",
		ToState: WorkState("unknown"), ExpectedVersion: triage.Version,
	}); err == nil {
		t.Fatal("transition to an invalid state was accepted")
	}
}

func TestMigrationChecksumMismatchIsRejected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")
	store := openTestStore(t, path)
	if _, err := store.db.ExecContext(ctx, "UPDATE schema_migrations SET checksum = 'tampered' WHERE version = ?", schemaVersion); err != nil {
		t.Fatalf("tamper migration checksum: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	if _, err := Open(ctx, path, Options{}); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("reopen error = %v, want checksum mismatch", err)
	}
}

func TestOptionalReferencesAndReferenceValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "reference-validation")
	parent, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "parent",
		Title: "Parent", ActorID: "planner",
	})
	if err != nil {
		t.Fatalf("create parent step: %v", err)
	}
	child, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ParentStepID: parent.ID,
		IdempotencyKey: "child", Title: "Child", ActorID: "planner",
	})
	if err != nil {
		t.Fatalf("create child step: %v", err)
	}
	dependency := AddStepDependencyInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: child.ID,
		DependsOnID: parent.ID, ActorID: "planner",
	}
	if err := store.AddStepDependency(ctx, dependency); err != nil {
		t.Fatalf("create dependency: %v", err)
	}
	if err := store.AddStepDependency(ctx, dependency); err != nil {
		t.Fatalf("replay dependency: %v", err)
	}
	if err := store.AddStepDependency(ctx, AddStepDependencyInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: child.ID,
		DependsOnID: child.ID, ActorID: "planner",
	}); !errors.Is(err, ErrDependencyCycle) {
		t.Fatalf("self dependency error = %v, want ErrDependencyCycle", err)
	}
	if _, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ParentStepID: "stp_missing",
		IdempotencyKey: "orphan-child", Title: "Orphan", ActorID: "planner",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing parent step error = %v, want ErrNotFound", err)
	}

	attempt, err := store.CreateAttempt(ctx, CreateAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: parent.ID,
		IdempotencyKey: "attempt", Number: 1, Adapter: "local",
		Status: AttemptStatusRunning, ActorID: "worker",
	})
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	if _, err := store.CreateAttempt(ctx, CreateAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: parent.ID,
		IdempotencyKey: "bad-json-attempt", Number: 2, Adapter: "local",
		Status: AttemptStatusRunning, ActorID: "worker", InputJSON: []byte("{"),
	}); err == nil {
		t.Fatal("attempt with invalid JSON was accepted")
	}
	if _, err := store.CreateApproval(ctx, CreateApprovalInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: child.ID,
		AttemptID: attempt.ID, IdempotencyKey: "mismatched-approval",
		Authority: "network", Status: ApprovalStatusPending,
		Request: "Allow", ActorID: "worker",
	}); !errors.Is(err, ErrInvalidDependency) {
		t.Fatalf("mismatched attempt error = %v, want ErrInvalidDependency", err)
	}
	if _, err := store.CreateProof(ctx, CreateProofInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "missing-artifact",
		Kind: "test", Status: ProofStatusFailed, Summary: "missing",
		ArtifactID: "art_missing", ActorID: "worker",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing artifact error = %v, want ErrNotFound", err)
	}

	if _, err := store.CreateApproval(ctx, CreateApprovalInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "work-approval",
		Authority: "operator", Status: ApprovalStatusApproved,
		Request: "Proceed", ActorID: "operator",
	}); err != nil {
		t.Fatalf("create work-level approval: %v", err)
	}
	if _, err := store.CreateArtifact(ctx, CreateArtifactInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "work-artifact",
		Kind: "report", Name: "report.json", URI: "file:///report.json",
		Digest: "sha256:5678", ActorID: "worker",
	}); err != nil {
		t.Fatalf("create work-level artifact: %v", err)
	}
	if _, err := store.CreateProof(ctx, CreateProofInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "work-proof",
		Kind: "review", Status: ProofStatusInconclusive,
		Summary: "manual review needed", ActorID: "reviewer",
	}); err != nil {
		t.Fatalf("create work-level proof: %v", err)
	}
}

func TestCreateWorkRollsBackWhenEventIDGenerationFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")
	var calls int
	store, err := Open(ctx, path, Options{
		NewID: func(prefix string) (string, error) {
			calls++
			if calls == 1 {
				return prefix + "_record", nil
			}
			return "", errors.New("injected id failure")
		},
	})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.CreateWork(ctx, CreateWorkInput{
		WorkspaceID: "workspace-a", Kind: "task", IdempotencyKey: "rollback",
		Title: "Rollback", ActorID: "tester",
	}); err == nil || !strings.Contains(err.Error(), "injected id failure") {
		t.Fatalf("create error = %v, want injected id failure", err)
	}
	works, err := store.ListWorks(ctx, ListWorksFilter{WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("list rolled-back works: %v", err)
	}
	if len(works) != 0 {
		t.Fatalf("rolled-back work count = %d, want 0", len(works))
	}

	if err := (*Store)(nil).Close(); err != nil {
		t.Fatalf("close nil store: %v", err)
	}
	if _, err := Open(ctx, "", Options{}); err == nil {
		t.Fatal("open with an empty path was accepted")
	}
	var value jsonValue
	if err := value.Scan(nil); err != nil || value != nil {
		t.Fatalf("scan nil json = %q, %v", value, err)
	}
	if err := value.Scan(42); err == nil {
		t.Fatal("unsupported JSON database value was accepted")
	}
}

func TestCanceledContextStopsCommandsAndQueries(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "cancel-context")
	step, err := store.CreateStep(context.Background(), CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "step",
		Title: "Step", ActorID: "planner",
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	attempt, err := store.CreateAttempt(context.Background(), CreateAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		IdempotencyKey: "attempt", Number: 1, Adapter: "local",
		Status: AttemptStatusRunning, ActorID: "worker",
	})
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assertCanceled := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s succeeded with a canceled context", name)
		}
	}
	_, err = store.CreateWork(ctx, CreateWorkInput{
		WorkspaceID: "workspace-a", Kind: "task", IdempotencyKey: "cancel-create",
		Title: "Canceled", ActorID: "tester",
	})
	assertCanceled("create work", err)
	_, err = store.GetWork(ctx, work.WorkspaceID, work.ID)
	assertCanceled("get work", err)
	_, err = store.ListWorks(ctx, ListWorksFilter{WorkspaceID: work.WorkspaceID})
	assertCanceled("list works", err)
	_, err = store.TransitionWork(ctx, TransitionWorkInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ToState: WorkStateBacklog,
		ExpectedVersion: work.Version, ActorID: "tester",
	})
	assertCanceled("transition work", err)
	_, err = store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "cancel-step",
		Title: "Canceled", ActorID: "tester",
	})
	assertCanceled("create step", err)
	err = store.AddStepDependency(ctx, AddStepDependencyInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID,
		StepID: step.ID, DependsOnID: "stp_unused", ActorID: "tester",
	})
	assertCanceled("add dependency", err)
	_, err = store.CreateAttempt(ctx, CreateAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		IdempotencyKey: "cancel-attempt", Number: 2, Adapter: "local",
		Status: AttemptStatusRunning, ActorID: "tester",
	})
	assertCanceled("create attempt", err)
	_, err = store.CreateApproval(ctx, CreateApprovalInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		AttemptID: attempt.ID, IdempotencyKey: "cancel-approval",
		Authority: "network", Status: ApprovalStatusPending,
		Request: "Canceled", ActorID: "tester",
	})
	assertCanceled("create approval", err)
	_, err = store.CreateArtifact(ctx, CreateArtifactInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		AttemptID: attempt.ID, IdempotencyKey: "cancel-artifact",
		Kind: "log", Name: "cancel.log", URI: "file:///cancel.log",
		Digest: "sha256:cancel", ActorID: "tester",
	})
	assertCanceled("create artifact", err)
	_, err = store.CreateProof(ctx, CreateProofInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		AttemptID: attempt.ID, IdempotencyKey: "cancel-proof",
		Kind: "test", Status: ProofStatusInconclusive,
		Summary: "Canceled", ActorID: "tester",
	})
	assertCanceled("create proof", err)
	_, err = store.ListEvents(ctx, work.WorkspaceID, work.ID)
	assertCanceled("list events", err)
	_, err = store.GetWorkProjection(ctx, work.WorkspaceID, work.ID)
	assertCanceled("get projection", err)

	works, err := store.ListWorks(context.Background(), ListWorksFilter{WorkspaceID: work.WorkspaceID})
	if err != nil {
		t.Fatalf("list works after canceled commands: %v", err)
	}
	if len(works) != 1 {
		t.Fatalf("work count after canceled commands = %d, want 1", len(works))
	}
}

func TestIDGenerationFailuresDoNotLeavePartialRecords(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "id-failure")
	first, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "first",
		Title: "First", ActorID: "planner",
	})
	if err != nil {
		t.Fatalf("create first step: %v", err)
	}
	second, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "second",
		Title: "Second", ActorID: "planner",
	})
	if err != nil {
		t.Fatalf("create second step: %v", err)
	}
	attempt, err := store.CreateAttempt(ctx, CreateAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: first.ID,
		IdempotencyKey: "attempt", Number: 1, Adapter: "local",
		Status: AttemptStatusRunning, ActorID: "worker",
	})
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	store.newID = func(string) (string, error) {
		return "", errors.New("id generator unavailable")
	}
	assertIDError := func(name string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "id generator unavailable") {
			t.Fatalf("%s error = %v, want id generator failure", name, err)
		}
	}
	_, err = store.CreateWork(ctx, CreateWorkInput{
		WorkspaceID: work.WorkspaceID, Kind: "task", IdempotencyKey: "failed-work",
		Title: "Failed", ActorID: "tester",
	})
	assertIDError("create work", err)
	_, err = store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "failed-step",
		Title: "Failed", ActorID: "tester",
	})
	assertIDError("create step", err)
	_, err = store.CreateAttempt(ctx, CreateAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: first.ID,
		IdempotencyKey: "failed-attempt", Number: 2, Adapter: "local",
		Status: AttemptStatusRunning, ActorID: "tester",
	})
	assertIDError("create attempt", err)
	_, err = store.CreateApproval(ctx, CreateApprovalInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: first.ID,
		AttemptID: attempt.ID, IdempotencyKey: "failed-approval",
		Authority: "network", Status: ApprovalStatusPending,
		Request: "Failed", ActorID: "tester",
	})
	assertIDError("create approval", err)
	_, err = store.CreateArtifact(ctx, CreateArtifactInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: first.ID,
		AttemptID: attempt.ID, IdempotencyKey: "failed-artifact",
		Kind: "log", Name: "failed.log", URI: "file:///failed.log",
		Digest: "sha256:failed", ActorID: "tester",
	})
	assertIDError("create artifact", err)
	_, err = store.CreateProof(ctx, CreateProofInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: first.ID,
		AttemptID: attempt.ID, IdempotencyKey: "failed-proof",
		Kind: "test", Status: ProofStatusFailed, Summary: "Failed", ActorID: "tester",
	})
	assertIDError("create proof", err)

	err = store.AddStepDependency(ctx, AddStepDependencyInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID,
		StepID: second.ID, DependsOnID: first.ID, ActorID: "planner",
	})
	assertIDError("add dependency event", err)
	_, err = store.TransitionWork(ctx, TransitionWorkInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ToState: WorkStateBacklog,
		ExpectedVersion: work.Version, ActorID: "triager",
	})
	assertIDError("transition event", err)

	projection, err := store.GetWorkProjection(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("get projection: %v", err)
	}
	if projection.Work.State != WorkStateTriage || projection.Work.Version != 1 {
		t.Fatalf("work after failed transition = %s/%d, want triage/1", projection.Work.State, projection.Work.Version)
	}
	if len(projection.Dependencies) != 0 || len(projection.Approvals) != 0 || len(projection.Artifacts) != 0 || len(projection.Proofs) != 0 {
		t.Fatalf("failed commands left partial projection records: %#v", projection)
	}
}

func openTestStore(t *testing.T, path string) *Store {
	t.Helper()

	var mu sync.Mutex
	var sequence int
	fixedNow := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	store, err := Open(context.Background(), path, Options{
		Now: func() time.Time { return fixedNow },
		NewID: func(prefix string) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			sequence++
			return fmt.Sprintf("%s_%04d", prefix, sequence), nil
		},
	})
	if err != nil {
		t.Fatalf("open work store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func mustCreateWork(t *testing.T, store *Store, workspaceID, idempotencyKey string) Work {
	t.Helper()

	work, err := store.CreateWork(context.Background(), CreateWorkInput{
		WorkspaceID:    workspaceID,
		Kind:           "task",
		IdempotencyKey: idempotencyKey,
		Title:          idempotencyKey,
		ActorID:        "tester",
	})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	return work
}

func tableIndexes(t *testing.T, store *Store, table string) []string {
	t.Helper()

	rows, err := store.db.Query("PRAGMA index_list(" + table + ")")
	if err != nil {
		t.Fatalf("list %s indexes: %v", table, err)
	}
	defer closeRows(rows)
	var indexes []string
	for rows.Next() {
		var sequence, unique, partial int
		var name, origin string
		if err := rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan %s index: %v", table, err)
		}
		indexes = append(indexes, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate %s indexes: %v", table, err)
	}
	sort.Strings(indexes)
	return indexes
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
