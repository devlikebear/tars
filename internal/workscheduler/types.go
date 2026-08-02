// Package workscheduler runs durable workstore steps independently of the
// request that submitted them.
package workscheduler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/devlikebear/tars/internal/workstore"
)

type StepSpec struct {
	Key         string
	Title       string
	Description string
	Position    int
	DependsOn   []string
	Policy      workstore.StepSchedulePolicy
}

type SubmitInput struct {
	WorkspaceID    string
	IdempotencyKey string
	Kind           string
	Source         string
	SourceID       string
	CausationID    string
	ParentWorkID   string
	Title          string
	Objective      string
	ContractJSON   json.RawMessage
	MetadataJSON   json.RawMessage
	Priority       int
	Adapter        string
	ActorID        string
	// CapabilityVersionIDs attributes this Work to reviewed, promoted
	// capability versions that influenced its execution. The scheduler
	// validates the references and records an outcome for every attempt.
	CapabilityVersionIDs []string
	Steps                []StepSpec
}

type Execution struct {
	Work  workstore.Work
	Claim workstore.StepClaim
}

type ExecutionResult struct {
	Succeeded  bool
	OutputJSON json.RawMessage
	Error      string
	Usage      workstore.StepAttemptUsage
}

type VerifierIdentity struct {
	ID              string          `json:"id"`
	EnvironmentJSON json.RawMessage `json:"environment"`
}

type VerificationRequest struct {
	Execution   Execution                  `json:"execution"`
	Result      ExecutionResult            `json:"result"`
	Requirement workstore.ProofRequirement `json:"requirement"`
}

type VerificationResult struct {
	Status              workstore.ProofStatus `json:"status"`
	Summary             string                `json:"summary"`
	Rationale           string                `json:"rationale"`
	SubjectDigest       string                `json:"subject_digest"`
	InputJSON           json.RawMessage       `json:"input"`
	ArtifactDigestsJSON json.RawMessage       `json:"artifact_digests"`
	ObservedAt          *time.Time            `json:"observed_at,omitempty"`
	UsedLLM             bool                  `json:"used_llm,omitempty"`
	Tokens              int64                 `json:"tokens,omitempty"`
	CostUSD             float64               `json:"cost_usd,omitempty"`
}

type Executor interface {
	Adapter() string
	Execute(context.Context, Execution) (ExecutionResult, error)
}

type RecoverableExecutor interface {
	Recover(context.Context, Execution) (ExecutionResult, bool, error)
}

type CancelableExecutor interface {
	Cancel(context.Context, Execution) error
}

type Verifier interface {
	Name() string
	Identity() VerifierIdentity
	Verify(context.Context, VerificationRequest) (VerificationResult, error)
}

type Options struct {
	Store             *workstore.Store
	WorkspaceID       string
	WorkerID          string
	ActorID           string
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	PollInterval      time.Duration
	MaxWorkers        int
	Executors         []Executor
	Verifiers         []Verifier
	OnError           func(error)
}
