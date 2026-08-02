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
	SchemaVersion  int
	ID             string
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
	State          WorkState
	Priority       int
	ActorID        string
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CompletedAt    *time.Time
}

type Step struct {
	SchemaVersion  int
	ID             string
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
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CompletedAt    *time.Time
}

type Attempt struct {
	SchemaVersion  int
	ID             string
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
	StartedAt      time.Time
	FinishedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Event struct {
	SchemaVersion  int
	Sequence       int64
	ID             string
	WorkspaceID    string
	WorkID         string
	StepID         string
	AttemptID      string
	Type           EventType
	FromState      WorkState
	ToState        WorkState
	ActorID        string
	CausationID    string
	IdempotencyKey string
	PayloadJSON    json.RawMessage
	CreatedAt      time.Time
}

type Proof struct {
	SchemaVersion  int
	ID             string
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
	CreatedAt      time.Time
}

type Artifact struct {
	SchemaVersion  int
	ID             string
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
	CreatedAt      time.Time
}

type Approval struct {
	SchemaVersion  int
	ID             string
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
	ReviewerID     string
	ExpiresAt      *time.Time
	DecidedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type StepDependency struct {
	WorkspaceID string
	WorkID      string
	StepID      string
	DependsOnID string
	ActorID     string
	CreatedAt   time.Time
}

type WorkProjection struct {
	Work         Work
	Steps        []Step
	Dependencies []StepDependency
	Attempts     []Attempt
	Events       []Event
	Proofs       []Proof
	Artifacts    []Artifact
	Approvals    []Approval
}

type ListWorksFilter struct {
	WorkspaceID string
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
