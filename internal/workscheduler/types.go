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
	Steps          []StepSpec
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
	OnError           func(error)
}
