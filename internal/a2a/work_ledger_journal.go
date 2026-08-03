package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

type WorkLedgerJournal struct {
	store   *workstore.Store
	actorID string
}

func NewWorkLedgerJournal(store *workstore.Store, actorID string) (*WorkLedgerJournal, error) {
	if store == nil || strings.TrimSpace(actorID) == "" {
		return nil, fmt.Errorf("a2a: work ledger and actor are required")
	}
	return &WorkLedgerJournal{store: store, actorID: strings.TrimSpace(actorID)}, nil
}

func (journal *WorkLedgerJournal) Record(ctx context.Context, execution workscheduler.Execution, event ExternalEvent) error {
	if journal == nil || journal.store == nil {
		return fmt.Errorf("a2a: work ledger journal is not configured")
	}
	eventType, err := workLedgerEventType(event.Kind)
	if err != nil {
		return err
	}
	if !safeTaskID.MatchString(event.TaskID) || event.ContextID != "" && !safeTaskID.MatchString(event.ContextID) ||
		event.AcceptedArtifacts < 0 || event.QuarantinedParts < 0 {
		return fmt.Errorf("a2a: invalid external event")
	}
	payload, err := json.Marshal(struct {
		ProtocolVersion   string    `json:"protocol_version"`
		TaskID            string    `json:"task_id"`
		ContextID         string    `json:"context_id,omitempty"`
		State             TaskState `json:"state,omitempty"`
		AcceptedArtifacts int       `json:"accepted_artifacts,omitempty"`
		QuarantinedParts  int       `json:"quarantined_parts,omitempty"`
	}{
		ProtocolVersion: ProtocolVersion, TaskID: event.TaskID, ContextID: event.ContextID, State: event.State,
		AcceptedArtifacts: event.AcceptedArtifacts, QuarantinedParts: event.QuarantinedParts,
	})
	if err != nil {
		return fmt.Errorf("a2a: encode external event: %w", err)
	}
	claim := execution.Claim
	_, err = journal.store.RecordExecutionEvent(ctx, workstore.RecordExecutionEventInput{
		WorkspaceID: claim.Step.WorkspaceID, WorkID: claim.Step.WorkID, StepID: claim.Step.ID,
		AttemptID: claim.Attempt.ID, Type: eventType, ActorID: journal.actorID,
		CausationID:    claim.Attempt.ID,
		IdempotencyKey: strings.Join([]string{claim.Attempt.ID, string(event.Kind), event.TaskID, string(event.State)}, ":"),
		PayloadJSON:    payload,
	})
	if err != nil {
		return fmt.Errorf("a2a: record external task event: %w", err)
	}
	return nil
}

func (journal *WorkLedgerJournal) Lookup(ctx context.Context, execution workscheduler.Execution) (TaskReference, bool, error) {
	if journal == nil || journal.store == nil {
		return TaskReference{}, false, fmt.Errorf("a2a: work ledger journal is not configured")
	}
	events, err := journal.store.ListEvents(ctx, execution.Work.WorkspaceID, execution.Work.ID)
	if err != nil {
		return TaskReference{}, false, fmt.Errorf("a2a: list external task events: %w", err)
	}
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Type != workstore.EventTypeA2ATaskSubmitted || event.AttemptID != execution.Claim.Attempt.ID {
			continue
		}
		var payload struct {
			ProtocolVersion string `json:"protocol_version"`
			TaskID          string `json:"task_id"`
			ContextID       string `json:"context_id"`
		}
		if err := json.Unmarshal(event.PayloadJSON, &payload); err != nil {
			return TaskReference{}, false, fmt.Errorf("a2a: decode external task reference: %w", err)
		}
		if payload.ProtocolVersion != ProtocolVersion || !safeTaskID.MatchString(payload.TaskID) ||
			payload.ContextID != "" && !safeTaskID.MatchString(payload.ContextID) {
			return TaskReference{}, false, fmt.Errorf("a2a: invalid external task reference")
		}
		return TaskReference{TaskID: payload.TaskID, ContextID: payload.ContextID}, true, nil
	}
	return TaskReference{}, false, nil
}

func workLedgerEventType(kind EventKind) (workstore.EventType, error) {
	switch kind {
	case EventTaskSubmitted:
		return workstore.EventTypeA2ATaskSubmitted, nil
	case EventTaskStateObserved:
		return workstore.EventTypeA2ATaskStateObserved, nil
	case EventArtifactQuarantined:
		return workstore.EventTypeA2AArtifactQuarantined, nil
	case EventTaskCanceled:
		return workstore.EventTypeA2ATaskCanceled, nil
	default:
		return "", fmt.Errorf("a2a: unsupported external event kind %q", kind)
	}
}

var _ Journal = (*WorkLedgerJournal)(nil)
