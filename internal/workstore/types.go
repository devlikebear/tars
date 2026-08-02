// Package workstore persists the durable, auditable execution ledger used by
// TARS schedulers, workers, and operator surfaces.
package workstore

import (
	"encoding/json"
	"errors"
	"time"
)

const recordSchemaVersion = 1

var (
	ErrNotFound          = errors.New("workstore: record not found")
	ErrConflict          = errors.New("workstore: version conflict")
	ErrInvalidTransition = errors.New("workstore: invalid state transition")
	ErrDependencyCycle   = errors.New("workstore: dependency cycle")
	ErrInvalidDependency = errors.New("workstore: invalid dependency")
)

type WorkState string

const (
	WorkStateTriage    WorkState = "triage"
	WorkStateBacklog   WorkState = "backlog"
	WorkStateTodo      WorkState = "todo"
	WorkStateReady     WorkState = "ready"
	WorkStateRunning   WorkState = "running"
	WorkStateReview    WorkState = "review"
	WorkStateBlocked   WorkState = "blocked"
	WorkStateDone      WorkState = "done"
	WorkStateCancelled WorkState = "cancelled"
)

type AttemptStatus string

const (
	AttemptStatusPending   AttemptStatus = "pending"
	AttemptStatusRunning   AttemptStatus = "running"
	AttemptStatusSucceeded AttemptStatus = "succeeded"
	AttemptStatusFailed    AttemptStatus = "failed"
	AttemptStatusCancelled AttemptStatus = "cancelled"
)

type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusDenied   ApprovalStatus = "denied"
	ApprovalStatusExpired  ApprovalStatus = "expired"
)

type ProofStatus string

const (
	ProofStatusPassed       ProofStatus = "passed"
	ProofStatusFailed       ProofStatus = "failed"
	ProofStatusInconclusive ProofStatus = "inconclusive"
)

type EventType string

const (
	EventTypeWorkCreated         EventType = "work.created"
	EventTypeWorkTransitioned    EventType = "work.transitioned"
	EventTypeStepCreated         EventType = "step.created"
	EventTypeStepDependencyAdded EventType = "step.dependency_added"
	EventTypeAttemptCreated      EventType = "attempt.created"
	EventTypeApprovalCreated     EventType = "approval.created"
	EventTypeProofCreated        EventType = "proof.created"
	EventTypeArtifactCreated     EventType = "artifact.created"
)

type Work struct {
	SchemaVersion  int             `json:"schema_version"`
	ID             string          `json:"id"`
	WorkspaceID    string          `json:"workspace_id"`
	Kind           string          `json:"kind"`
	Source         string          `json:"source,omitempty"`
	SourceID       string          `json:"source_id,omitempty"`
	IdempotencyKey string          `json:"idempotency_key"`
	CausationID    string          `json:"causation_id,omitempty"`
	ParentWorkID   string          `json:"parent_work_id,omitempty"`
	Title          string          `json:"title"`
	Objective      string          `json:"objective,omitempty"`
	ContractJSON   json.RawMessage `json:"contract"`
	MetadataJSON   json.RawMessage `json:"metadata"`
	State          WorkState       `json:"state"`
	Priority       int             `json:"priority"`
	ActorID        string          `json:"actor_id"`
	Version        int             `json:"version"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
}

type Step struct {
	SchemaVersion  int        `json:"schema_version"`
	ID             string     `json:"id"`
	WorkspaceID    string     `json:"workspace_id"`
	WorkID         string     `json:"work_id"`
	ParentStepID   string     `json:"parent_step_id,omitempty"`
	IdempotencyKey string     `json:"idempotency_key"`
	CausationID    string     `json:"causation_id,omitempty"`
	Title          string     `json:"title"`
	Description    string     `json:"description,omitempty"`
	State          WorkState  `json:"state"`
	Position       int        `json:"position"`
	ActorID        string     `json:"actor_id"`
	Version        int        `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

type Attempt struct {
	SchemaVersion  int             `json:"schema_version"`
	ID             string          `json:"id"`
	WorkspaceID    string          `json:"workspace_id"`
	WorkID         string          `json:"work_id"`
	StepID         string          `json:"step_id,omitempty"`
	IdempotencyKey string          `json:"idempotency_key"`
	CausationID    string          `json:"causation_id,omitempty"`
	Number         int             `json:"number"`
	Adapter        string          `json:"adapter"`
	Status         AttemptStatus   `json:"status"`
	ActorID        string          `json:"actor_id"`
	InputJSON      json.RawMessage `json:"input"`
	OutputJSON     json.RawMessage `json:"output"`
	ErrorText      string          `json:"error,omitempty"`
	StartedAt      time.Time       `json:"started_at"`
	FinishedAt     *time.Time      `json:"finished_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type Event struct {
	SchemaVersion  int             `json:"schema_version"`
	Sequence       int64           `json:"sequence"`
	ID             string          `json:"id"`
	WorkspaceID    string          `json:"workspace_id"`
	WorkID         string          `json:"work_id"`
	StepID         string          `json:"step_id,omitempty"`
	AttemptID      string          `json:"attempt_id,omitempty"`
	Type           EventType       `json:"type"`
	FromState      WorkState       `json:"from_state,omitempty"`
	ToState        WorkState       `json:"to_state,omitempty"`
	ActorID        string          `json:"actor_id"`
	CausationID    string          `json:"causation_id,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	PayloadJSON    json.RawMessage `json:"payload"`
	CreatedAt      time.Time       `json:"created_at"`
}

type Proof struct {
	SchemaVersion  int         `json:"schema_version"`
	ID             string      `json:"id"`
	WorkspaceID    string      `json:"workspace_id"`
	WorkID         string      `json:"work_id"`
	StepID         string      `json:"step_id,omitempty"`
	AttemptID      string      `json:"attempt_id,omitempty"`
	IdempotencyKey string      `json:"idempotency_key"`
	CausationID    string      `json:"causation_id,omitempty"`
	Kind           string      `json:"kind"`
	Status         ProofStatus `json:"status"`
	Summary        string      `json:"summary"`
	Verifier       string      `json:"verifier,omitempty"`
	Command        string      `json:"command,omitempty"`
	ArtifactID     string      `json:"artifact_id,omitempty"`
	ActorID        string      `json:"actor_id"`
	CreatedAt      time.Time   `json:"created_at"`
}

type Artifact struct {
	SchemaVersion  int       `json:"schema_version"`
	ID             string    `json:"id"`
	WorkspaceID    string    `json:"workspace_id"`
	WorkID         string    `json:"work_id"`
	StepID         string    `json:"step_id,omitempty"`
	AttemptID      string    `json:"attempt_id,omitempty"`
	IdempotencyKey string    `json:"idempotency_key"`
	CausationID    string    `json:"causation_id,omitempty"`
	Kind           string    `json:"kind"`
	Name           string    `json:"name"`
	URI            string    `json:"uri"`
	Digest         string    `json:"digest"`
	MediaType      string    `json:"media_type,omitempty"`
	SizeBytes      int64     `json:"size_bytes"`
	ActorID        string    `json:"actor_id"`
	CreatedAt      time.Time `json:"created_at"`
}

type Approval struct {
	SchemaVersion  int            `json:"schema_version"`
	ID             string         `json:"id"`
	WorkspaceID    string         `json:"workspace_id"`
	WorkID         string         `json:"work_id"`
	StepID         string         `json:"step_id,omitempty"`
	AttemptID      string         `json:"attempt_id,omitempty"`
	IdempotencyKey string         `json:"idempotency_key"`
	CausationID    string         `json:"causation_id,omitempty"`
	Authority      string         `json:"authority"`
	Status         ApprovalStatus `json:"status"`
	Request        string         `json:"request"`
	Reason         string         `json:"reason,omitempty"`
	ActorID        string         `json:"actor_id"`
	ReviewerID     string         `json:"reviewer_id,omitempty"`
	ExpiresAt      *time.Time     `json:"expires_at,omitempty"`
	DecidedAt      *time.Time     `json:"decided_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type StepDependency struct {
	WorkspaceID string    `json:"workspace_id"`
	WorkID      string    `json:"work_id"`
	StepID      string    `json:"step_id"`
	DependsOnID string    `json:"depends_on_id"`
	ActorID     string    `json:"actor_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type WorkProjection struct {
	Work         Work             `json:"work"`
	Steps        []Step           `json:"steps"`
	Dependencies []StepDependency `json:"dependencies"`
	Attempts     []Attempt        `json:"attempts"`
	Events       []Event          `json:"events"`
	Proofs       []Proof          `json:"proofs"`
	Artifacts    []Artifact       `json:"artifacts"`
	Approvals    []Approval       `json:"approvals"`
}

type ListWorksFilter struct {
	WorkspaceID string
	Source      string
	SourceID    string
	States      []WorkState
	Limit       int
	Offset      int
}

type CreateWorkInput struct {
	WorkspaceID    string
	Kind           string
	Source         string
	SourceID       string
	IdempotencyKey string
	CausationID    string
	ParentWorkID   string
	Title          string
	Objective      string
	ContractJSON   json.RawMessage
	MetadataJSON   json.RawMessage
	InitialState   WorkState
	Priority       int
	ActorID        string
}

type TransitionWorkInput struct {
	WorkspaceID     string
	WorkID          string
	ToState         WorkState
	ExpectedVersion int
	ActorID         string
	CausationID     string
	IdempotencyKey  string
	Reason          string
}

type CreateStepInput struct {
	WorkspaceID    string
	WorkID         string
	ParentStepID   string
	IdempotencyKey string
	CausationID    string
	Title          string
	Description    string
	State          WorkState
	Position       int
	ActorID        string
}

type AddStepDependencyInput struct {
	WorkspaceID string
	WorkID      string
	StepID      string
	DependsOnID string
	ActorID     string
	CausationID string
}

type CreateAttemptInput struct {
	WorkspaceID    string
	WorkID         string
	StepID         string
	IdempotencyKey string
	CausationID    string
	Number         int
	Adapter        string
	Status         AttemptStatus
	ActorID        string
	InputJSON      json.RawMessage
	OutputJSON     json.RawMessage
	ErrorText      string
	StartedAt      *time.Time
	FinishedAt     *time.Time
}

type CreateApprovalInput struct {
	WorkspaceID    string
	WorkID         string
	StepID         string
	AttemptID      string
	IdempotencyKey string
	CausationID    string
	Authority      string
	Status         ApprovalStatus
	Request        string
	Reason         string
	ActorID        string
	ExpiresAt      *time.Time
}

type CreateArtifactInput struct {
	WorkspaceID    string
	WorkID         string
	StepID         string
	AttemptID      string
	IdempotencyKey string
	CausationID    string
	Kind           string
	Name           string
	URI            string
	Digest         string
	MediaType      string
	SizeBytes      int64
	ActorID        string
}

type CreateProofInput struct {
	WorkspaceID    string
	WorkID         string
	StepID         string
	AttemptID      string
	IdempotencyKey string
	CausationID    string
	Kind           string
	Status         ProofStatus
	Summary        string
	Verifier       string
	Command        string
	ArtifactID     string
	ActorID        string
}
