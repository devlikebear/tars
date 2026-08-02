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
	ErrNoReadyStep       = errors.New("workstore: no ready step")
	ErrClaimConflict     = errors.New("workstore: step claim conflict")
	ErrClaimExpired      = errors.New("workstore: step claim expired")
	ErrEffectConflict    = errors.New("workstore: effect receipt conflict")
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
	ProofStatusReported ProofStatus = "reported"
	ProofStatusPending  ProofStatus = "pending"
	ProofStatusPassed   ProofStatus = "passed"
	ProofStatusFailed   ProofStatus = "failed"
	ProofStatusStale    ProofStatus = "stale"

	// ProofStatusInconclusive is kept as a source-compatible alias. New records
	// persist the explicit pending state instead of the ambiguous old value.
	ProofStatusInconclusive = ProofStatusPending
)

type ProofOrigin string

const (
	ProofOriginWorkerReport        ProofOrigin = "worker_report"
	ProofOriginIndependentVerifier ProofOrigin = "independent_verifier"
	ProofOriginLegacy              ProofOrigin = "legacy"
)

type EffectReceiptStatus string

const (
	EffectReceiptStatusPending   EffectReceiptStatus = "pending"
	EffectReceiptStatusCommitted EffectReceiptStatus = "committed"
)

type EventType string

const (
	EventTypeWorkCreated            EventType = "work.created"
	EventTypeWorkTransitioned       EventType = "work.transitioned"
	EventTypeStepCreated            EventType = "step.created"
	EventTypeStepDependencyAdded    EventType = "step.dependency_added"
	EventTypeAttemptCreated         EventType = "attempt.created"
	EventTypeApprovalCreated        EventType = "approval.created"
	EventTypeProofCreated           EventType = "proof.created"
	EventTypeProofTransitioned      EventType = "proof.transitioned"
	EventTypeArtifactCreated        EventType = "artifact.created"
	EventTypeStepScheduleConfigured EventType = "step.schedule_configured"
	EventTypeStepReady              EventType = "step.ready"
	EventTypeStepClaimed            EventType = "step.claimed"
	EventTypeStepHeartbeat          EventType = "step.heartbeat"
	EventTypeAttemptCompleted       EventType = "attempt.completed"
	EventTypeStepCompleted          EventType = "step.completed"
	EventTypeStepRetryScheduled     EventType = "step.retry_scheduled"
	EventTypeStepReplanScheduled    EventType = "step.replan_scheduled"
	EventTypeStepDecomposeScheduled EventType = "step.decompose_scheduled"
	EventTypeStepReleased           EventType = "step.released"
	EventTypeStepReclaimed          EventType = "step.reclaimed"
	EventTypeStepReviewRequested    EventType = "step.review_requested"
	EventTypeStepBlocked            EventType = "step.blocked"
	EventTypeStepResumed            EventType = "step.resumed"
	EventTypeStepCancelled          EventType = "step.cancelled"
	EventTypeEffectStarted          EventType = "effect.started"
	EventTypeEffectCommitted        EventType = "effect.committed"
)

type StepExecutionAction string

const (
	StepExecutionActionExecute   StepExecutionAction = "execute"
	StepExecutionActionRetry     StepExecutionAction = "retry"
	StepExecutionActionReplan    StepExecutionAction = "replan"
	StepExecutionActionDecompose StepExecutionAction = "decompose"
)

type StepDisposition string

const (
	StepDispositionDone      StepDisposition = "done"
	StepDispositionRetry     StepDisposition = "retry"
	StepDispositionReplan    StepDisposition = "replan"
	StepDispositionDecompose StepDisposition = "decompose"
	StepDispositionReview    StepDisposition = "review"
	StepDispositionBlocked   StepDisposition = "blocked"
)

type StepSchedulePolicy struct {
	MaxAttempts     int             `json:"max_attempts"`
	RetryLimit      int             `json:"retry_limit"`
	ReplanLimit     int             `json:"replan_limit"`
	DecomposeLimit  int             `json:"decompose_limit"`
	MaxIterations   int             `json:"max_iterations,omitempty"`
	MaxTokens       int64           `json:"max_tokens,omitempty"`
	MaxCostUSD      float64         `json:"max_cost_usd,omitempty"`
	EscalationState WorkState       `json:"escalation_state"`
	Proof           StepProofPolicy `json:"proof,omitempty"`
}

type ProofRequirement struct {
	Kind      string          `json:"kind"`
	Verifier  string          `json:"verifier"`
	Command   string          `json:"command,omitempty"`
	Paths     []string        `json:"paths,omitempty"`
	URL       string          `json:"url,omitempty"`
	InputJSON json.RawMessage `json:"input,omitempty"`
}

type StepProofPolicy struct {
	Required             bool               `json:"required,omitempty"`
	Requirements         []ProofRequirement `json:"requirements,omitempty"`
	MinIndependentPasses int                `json:"min_independent_passes,omitempty"`
	FailureState         WorkState          `json:"failure_state,omitempty"`
	AllowLLMFallback     bool               `json:"allow_llm_fallback,omitempty"`
	MaxLLMTokens         int64              `json:"max_llm_tokens,omitempty"`
	MaxLLMCostUSD        float64            `json:"max_llm_cost_usd,omitempty"`
}

type StepSchedule struct {
	SchemaVersion       int                 `json:"schema_version"`
	WorkspaceID         string              `json:"workspace_id"`
	WorkID              string              `json:"work_id"`
	StepID              string              `json:"step_id"`
	Policy              StepSchedulePolicy  `json:"policy"`
	LeaseOwner          string              `json:"lease_owner,omitempty"`
	LeaseExpiresAt      *time.Time          `json:"lease_expires_at,omitempty"`
	LastHeartbeatAt     *time.Time          `json:"last_heartbeat_at,omitempty"`
	ActiveAttemptID     string              `json:"active_attempt_id,omitempty"`
	AttemptCount        int                 `json:"attempt_count"`
	CycleAttemptCount   int                 `json:"cycle_attempt_count"`
	ConsumedIterations  int                 `json:"consumed_iterations"`
	ConsumedTokens      int64               `json:"consumed_tokens"`
	ConsumedCostUSD     float64             `json:"consumed_cost_usd"`
	NextAction          StepExecutionAction `json:"next_action"`
	LastDisposition     StepDisposition     `json:"last_disposition,omitempty"`
	BlockedReason       string              `json:"blocked_reason,omitempty"`
	HumanResumeRequired bool                `json:"human_resume_required"`
	UpdatedAt           time.Time           `json:"updated_at"`
}

type ConfigureStepScheduleInput struct {
	WorkspaceID string
	WorkID      string
	StepID      string
	Policy      StepSchedulePolicy
	ActorID     string
}

type PromoteReadyStepsInput struct {
	WorkspaceID string
	WorkID      string
	ActorID     string
}

type ClaimReadyStepInput struct {
	WorkspaceID   string
	WorkID        string
	WorkerID      string
	Adapter       string
	LeaseDuration time.Duration
	ActorID       string
}

type StepClaim struct {
	Step     Step         `json:"step"`
	Attempt  Attempt      `json:"attempt"`
	Schedule StepSchedule `json:"schedule"`
}

type HeartbeatStepClaimInput struct {
	WorkspaceID   string
	WorkID        string
	StepID        string
	AttemptID     string
	WorkerID      string
	LeaseDuration time.Duration
	ActorID       string
}

type ReleaseStepClaimInput struct {
	WorkspaceID string
	WorkID      string
	StepID      string
	AttemptID   string
	WorkerID    string
	ActorID     string
	Reason      string
}

type StepAttemptUsage struct {
	Iterations int     `json:"iterations"`
	Tokens     int64   `json:"tokens"`
	CostUSD    float64 `json:"cost_usd"`
}

type CompleteStepAttemptInput struct {
	WorkspaceID string
	WorkID      string
	StepID      string
	AttemptID   string
	WorkerID    string
	Succeeded   bool
	OutputJSON  json.RawMessage
	ErrorText   string
	Usage       StepAttemptUsage
	ActorID     string
}

type StepResolution struct {
	Step        Step            `json:"step"`
	Attempt     Attempt         `json:"attempt"`
	Schedule    StepSchedule    `json:"schedule"`
	Disposition StepDisposition `json:"disposition"`
}

type ReclaimExpiredStepClaimsInput struct {
	WorkspaceID string
	WorkID      string
	ActorID     string
	Reason      string
}

type ReclaimStepClaimInput struct {
	WorkspaceID string
	WorkID      string
	StepID      string
	AttemptID   string
	WorkerID    string
	ActorID     string
	Reason      string
}

type ResumeScheduledStepInput struct {
	WorkspaceID string
	WorkID      string
	StepID      string
	ActorID     string
	Reason      string
}

type CancelScheduledStepInput struct {
	WorkspaceID string
	WorkID      string
	StepID      string
	ActorID     string
	Reason      string
}

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
	SchemaVersion       int             `json:"schema_version"`
	ID                  string          `json:"id"`
	WorkspaceID         string          `json:"workspace_id"`
	WorkID              string          `json:"work_id"`
	StepID              string          `json:"step_id,omitempty"`
	AttemptID           string          `json:"attempt_id,omitempty"`
	IdempotencyKey      string          `json:"idempotency_key"`
	CausationID         string          `json:"causation_id,omitempty"`
	Kind                string          `json:"kind"`
	Status              ProofStatus     `json:"status"`
	Origin              ProofOrigin     `json:"origin"`
	Summary             string          `json:"summary"`
	ReporterID          string          `json:"reporter_id,omitempty"`
	VerifierID          string          `json:"verifier_id,omitempty"`
	Verifier            string          `json:"verifier,omitempty"`
	Command             string          `json:"command,omitempty"`
	ArtifactID          string          `json:"artifact_id,omitempty"`
	EnvironmentJSON     json.RawMessage `json:"environment"`
	InputJSON           json.RawMessage `json:"input"`
	ArtifactDigestsJSON json.RawMessage `json:"artifact_digests"`
	SubjectDigest       string          `json:"subject_digest,omitempty"`
	Rationale           string          `json:"rationale,omitempty"`
	ActorID             string          `json:"actor_id"`
	ObservedAt          *time.Time      `json:"observed_at,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
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
	Work           Work             `json:"work"`
	Steps          []Step           `json:"steps"`
	Schedules      []StepSchedule   `json:"schedules"`
	Dependencies   []StepDependency `json:"dependencies"`
	Attempts       []Attempt        `json:"attempts"`
	Events         []Event          `json:"events"`
	Proofs         []Proof          `json:"proofs"`
	Artifacts      []Artifact       `json:"artifacts"`
	Approvals      []Approval       `json:"approvals"`
	EffectReceipts []EffectReceipt  `json:"effect_receipts"`
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
	WorkspaceID         string
	WorkID              string
	StepID              string
	AttemptID           string
	IdempotencyKey      string
	CausationID         string
	Kind                string
	Status              ProofStatus
	Origin              ProofOrigin
	Summary             string
	ReporterID          string
	VerifierID          string
	Verifier            string
	Command             string
	ArtifactID          string
	EnvironmentJSON     json.RawMessage
	InputJSON           json.RawMessage
	ArtifactDigestsJSON json.RawMessage
	SubjectDigest       string
	Rationale           string
	ActorID             string
	ObservedAt          *time.Time
}

type TransitionProofInput struct {
	WorkspaceID         string
	WorkID              string
	ProofID             string
	ExpectedStatus      ProofStatus
	ToStatus            ProofStatus
	InputJSON           json.RawMessage
	ArtifactDigestsJSON json.RawMessage
	SubjectDigest       string
	Rationale           string
	ActorID             string
	ObservedAt          *time.Time
}

type DetectStaleProofInput struct {
	WorkspaceID          string
	WorkID               string
	ProofID              string
	CurrentSubjectDigest string
	ActorID              string
	Rationale            string
}

type EffectReceipt struct {
	SchemaVersion     int                 `json:"schema_version"`
	ID                string              `json:"id"`
	WorkspaceID       string              `json:"workspace_id"`
	WorkID            string              `json:"work_id"`
	StepID            string              `json:"step_id,omitempty"`
	AttemptID         string              `json:"attempt_id,omitempty"`
	IdempotencyKey    string              `json:"idempotency_key"`
	CausationID       string              `json:"causation_id,omitempty"`
	EffectType        string              `json:"effect_type"`
	Target            string              `json:"target,omitempty"`
	RequestDigest     string              `json:"request_digest"`
	Status            EffectReceiptStatus `json:"status"`
	OutcomeJSON       json.RawMessage     `json:"outcome"`
	ExternalReference string              `json:"external_reference,omitempty"`
	ActorID           string              `json:"actor_id"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
	CommittedAt       *time.Time          `json:"committed_at,omitempty"`
}

type BeginEffectReceiptInput struct {
	WorkspaceID    string
	WorkID         string
	StepID         string
	AttemptID      string
	IdempotencyKey string
	CausationID    string
	EffectType     string
	Target         string
	RequestDigest  string
	ActorID        string
}

type CommitEffectReceiptInput struct {
	WorkspaceID       string
	WorkID            string
	IdempotencyKey    string
	RequestDigest     string
	OutcomeJSON       json.RawMessage
	ExternalReference string
	ActorID           string
}
