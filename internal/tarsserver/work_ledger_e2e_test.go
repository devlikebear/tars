package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func TestWorkLedgerEndToEndSessionTaskAttemptProofTimeline(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionStore := session.NewStore(t.TempDir())
	sess, err := sessionStore.EnsureMain()
	if err != nil {
		t.Fatalf("ensure session: %v", err)
	}
	ledger := openWorkLedgerHandlerTestStore(t)
	sessionHandler := newSessionAPIHandlerWithWorkLedger(sessionStore, ledger, zerolog.Nop())

	for _, body := range []string{
		`{"action":"plan_set","goal":"Prove the durable lifecycle"}`,
		`{"action":"add","title":"Run the acceptance check","description":"Capture proof from the E2E flow"}`,
	} {
		response := sessionTasksRequest(t, sessionHandler, http.MethodPost, sess.ID, body)
		if response.Code != http.StatusOK {
			t.Fatalf("mutate session tasks body=%s status=%d response=%s", body, response.Code, response.Body.String())
		}
	}

	works, err := ledger.ListWorks(ctx, workstore.ListWorksFilter{
		WorkspaceID: defaultWorkspaceID,
		Source:      string(workstore.ImportSourceLegacySession),
		SourceID:    sess.ID,
	})
	if err != nil {
		t.Fatalf("list synchronized session work: %v", err)
	}
	var lifecycle workstore.WorkProjection
	for _, work := range works {
		projection, projectionErr := ledger.GetWorkProjection(ctx, defaultWorkspaceID, work.ID)
		if projectionErr != nil {
			t.Fatalf("get synchronized work projection: %v", projectionErr)
		}
		if len(projection.Steps) == 1 {
			lifecycle = projection
			break
		}
	}
	if lifecycle.Work.ID == "" || lifecycle.Steps[0].Title != "Run the acceptance check" {
		t.Fatalf("session task was not normalized into work and step: %+v", lifecycle)
	}

	step := lifecycle.Steps[0]
	attempt, err := ledger.CreateAttempt(ctx, workstore.CreateAttemptInput{
		WorkspaceID:    defaultWorkspaceID,
		WorkID:         lifecycle.Work.ID,
		StepID:         step.ID,
		IdempotencyKey: "e2e:attempt:1",
		CausationID:    "e2e:session-task-attempt-proof",
		Number:         1,
		Adapter:        "local-e2e",
		Status:         workstore.AttemptStatusSucceeded,
		ActorID:        "e2e-worker",
		InputJSON:      json.RawMessage(`{"command":"go test ./internal/tarsserver"}`),
		OutputJSON:     json.RawMessage(`{"exit_code":0}`),
	})
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	artifact, err := ledger.CreateArtifact(ctx, workstore.CreateArtifactInput{
		WorkspaceID:    defaultWorkspaceID,
		WorkID:         lifecycle.Work.ID,
		StepID:         step.ID,
		AttemptID:      attempt.ID,
		IdempotencyKey: "e2e:artifact:test-log",
		CausationID:    "e2e:session-task-attempt-proof",
		Kind:           "test-log",
		Name:           "tarsserver-e2e.log",
		URI:            "artifact://work-ledger-e2e/tarsserver-e2e.log",
		Digest:         "sha256:e2e-passed",
		MediaType:      "text/plain",
		SizeBytes:      17,
		ActorID:        "e2e-worker",
	})
	if err != nil {
		t.Fatalf("create proof artifact: %v", err)
	}
	proof, err := ledger.CreateProof(ctx, workstore.CreateProofInput{
		WorkspaceID:    defaultWorkspaceID,
		WorkID:         lifecycle.Work.ID,
		StepID:         step.ID,
		AttemptID:      attempt.ID,
		IdempotencyKey: "e2e:proof:test",
		CausationID:    "e2e:session-task-attempt-proof",
		Kind:           "test",
		Status:         workstore.ProofStatusPassed,
		Summary:        "session task lifecycle passed",
		Verifier:       "go-test",
		Command:        "go test ./internal/tarsserver",
		ArtifactID:     artifact.ID,
		ActorID:        "e2e-worker",
	})
	if err != nil {
		t.Fatalf("create proof: %v", err)
	}

	timelineHandler := newWorkLedgerAPIHandler(ledger, zerolog.Nop())
	timeline := httptest.NewRecorder()
	timelineHandler.ServeHTTP(timeline, httptest.NewRequest(http.MethodGet, "/v1/work/works/"+lifecycle.Work.ID+"/timeline", nil))
	if timeline.Code != http.StatusOK {
		t.Fatalf("read timeline status=%d body=%s", timeline.Code, timeline.Body.String())
	}
	var got workstore.WorkProjection
	if err := json.Unmarshal(timeline.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	if len(got.Steps) != 1 || len(got.Attempts) != 1 || len(got.Artifacts) != 1 || len(got.Proofs) != 1 {
		t.Fatalf("timeline lifecycle counts steps=%d attempts=%d artifacts=%d proofs=%d", len(got.Steps), len(got.Attempts), len(got.Artifacts), len(got.Proofs))
	}
	if got.Attempts[0].ID != attempt.ID || got.Proofs[0].ID != proof.ID || got.Proofs[0].ArtifactID != artifact.ID {
		t.Fatalf("timeline lost attempt/proof linkage: %+v", got)
	}
	for _, eventType := range []workstore.EventType{
		workstore.EventTypeWorkCreated,
		workstore.EventTypeStepCreated,
		workstore.EventTypeAttemptCreated,
		workstore.EventTypeArtifactCreated,
		workstore.EventTypeProofCreated,
	} {
		if !workLedgerTimelineHasEvent(got.Events, eventType) {
			t.Fatalf("timeline missing event %q: %+v", eventType, got.Events)
		}
	}
}

func workLedgerTimelineHasEvent(events []workstore.Event, want workstore.EventType) bool {
	for _, event := range events {
		if event.Type == want {
			return true
		}
	}
	return false
}
