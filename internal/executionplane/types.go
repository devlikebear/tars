// Package executionplane separates durable scheduling from worker and
// environment implementations.
package executionplane

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/devlikebear/tars/internal/workscheduler"
)

var (
	ErrUnsupported     = errors.New("executionplane: operation is unsupported")
	ErrCredentialScope = errors.New("executionplane: credential is not task scoped")
)

const lifecycleSchemaVersion = 1

type ExecutorCapabilities struct {
	Resume       bool `json:"resume"`
	Steering     bool `json:"steering"`
	Cancellation bool `json:"cancellation"`
	ToolPolicy   bool `json:"tool_policy"`
	Transcript   bool `json:"transcript"`
	Cost         bool `json:"cost"`
	Artifacts    bool `json:"artifacts"`
}

type EnvironmentCapabilities struct {
	Recoverable         bool `json:"recoverable"`
	Snapshot            bool `json:"snapshot"`
	Cleanup             bool `json:"cleanup"`
	FilesystemIsolation bool `json:"filesystem_isolation"`
	CredentialIsolation bool `json:"credential_isolation"`
	EgressPolicy        bool `json:"egress_policy"`
}

type Environment struct {
	SchemaVersion int             `json:"schema_version"`
	ID            string          `json:"environment_id"`
	Kind          string          `json:"kind"`
	RootDir       string          `json:"root_dir"`
	SourceDir     string          `json:"source_dir,omitempty"`
	MetadataJSON  json.RawMessage `json:"metadata"`
	ProvisionedAt time.Time       `json:"provisioned_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type EnvironmentSnapshot struct {
	ID           string          `json:"snapshot_id,omitempty"`
	Digest       string          `json:"digest,omitempty"`
	URI          string          `json:"uri,omitempty"`
	MetadataJSON json.RawMessage `json:"metadata,omitempty"`
	CreatedAt    time.Time       `json:"created_at,omitempty"`
}

type ProvisionRequest struct {
	Execution workscheduler.Execution `json:"execution"`
	SourceDir string                  `json:"source_dir"`
}

type EnvironmentProvider interface {
	Name() string
	Capabilities() EnvironmentCapabilities
	Provision(context.Context, ProvisionRequest) (Environment, error)
	Recover(context.Context, Environment) (Environment, error)
	Sync(context.Context, Environment) (EnvironmentSnapshot, error)
	Destroy(context.Context, Environment) error
}

type CredentialRequest struct {
	Execution   workscheduler.Execution `json:"execution"`
	Environment Environment             `json:"environment"`
	Worker      string                  `json:"worker"`
}

type CredentialGrant struct {
	ID        string            `json:"grant_id"`
	Values    map[string]string `json:"-"`
	ExpiresAt time.Time         `json:"expires_at"`
}

type CredentialBroker interface {
	Issue(context.Context, CredentialRequest) (CredentialGrant, error)
	Revoke(context.Context, CredentialGrant) error
}

type WorkerCheckpoint struct {
	ID          string          `json:"checkpoint_id"`
	ResumeToken string          `json:"resume_token,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedAt   time.Time       `json:"created_at,omitempty"`
}

type WorkerRequest struct {
	Execution   workscheduler.Execution `json:"execution"`
	Environment Environment             `json:"environment"`
	Credentials CredentialGrant         `json:"-"`
}

type TranscriptEntry struct {
	Sequence  int64           `json:"sequence"`
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp time.Time       `json:"timestamp,omitempty"`
}

type WorkerResult struct {
	ExecutionResult workscheduler.ExecutionResult `json:"execution_result"`
	Checkpoint      *WorkerCheckpoint             `json:"checkpoint,omitempty"`
	Transcript      []TranscriptEntry             `json:"transcript,omitempty"`
}

type WorkerClient interface {
	Name() string
	Capabilities() ExecutorCapabilities
	Execute(context.Context, WorkerRequest) (WorkerResult, error)
	Recover(context.Context, WorkerRequest, *WorkerCheckpoint) (WorkerResult, bool, error)
	Cancel(context.Context, WorkerRequest) error
}

type CollectedArtifact struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	URI       string `json:"uri"`
	Digest    string `json:"digest"`
	MediaType string `json:"media_type,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

type CollectRequest struct {
	Execution   workscheduler.Execution `json:"execution"`
	Environment Environment             `json:"environment"`
	Snapshot    EnvironmentSnapshot     `json:"snapshot"`
	Worker      WorkerResult            `json:"worker"`
}

type ArtifactCollector interface {
	Collect(context.Context, CollectRequest) ([]CollectedArtifact, error)
}

type ArtifactSink interface {
	StoreArtifact(context.Context, workscheduler.Execution, CollectedArtifact) error
}

type EventPhase string

const (
	EventEnvironmentProvisioned EventPhase = "environment.provisioned"
	EventCredentialsIssued      EventPhase = "credentials.issued"
	EventWorkerStarted          EventPhase = "worker.started"
	EventCheckpointRecorded     EventPhase = "checkpoint.recorded"
	EventEnvironmentSynced      EventPhase = "environment.synced"
	EventArtifactsCollected     EventPhase = "artifacts.collected"
	EventCredentialsRevoked     EventPhase = "credentials.revoked"
	EventEnvironmentDestroyed   EventPhase = "environment.destroyed"
	EventRecoveryStarted        EventPhase = "recovery.started"
	EventWorkerCancelled        EventPhase = "worker.cancelled"
)

type LifecycleEvent struct {
	Phase         EventPhase              `json:"phase"`
	Execution     workscheduler.Execution `json:"-"`
	Provider      string                  `json:"provider,omitempty"`
	Worker        string                  `json:"worker,omitempty"`
	EnvironmentID string                  `json:"environment_id,omitempty"`
	CredentialID  string                  `json:"credential_id,omitempty"`
	CheckpointID  string                  `json:"checkpoint_id,omitempty"`
	ArtifactCount int                     `json:"artifact_count,omitempty"`
	Snapshot      EnvironmentSnapshot     `json:"snapshot,omitempty"`
	OccurredAt    time.Time               `json:"occurred_at"`
}

type EventSink interface {
	Record(context.Context, LifecycleEvent) error
}

type LifecycleState struct {
	SchemaVersion int                 `json:"schema_version"`
	AttemptID     string              `json:"attempt_id"`
	Phase         EventPhase          `json:"phase"`
	Environment   Environment         `json:"environment"`
	CredentialID  string              `json:"credential_id,omitempty"`
	Checkpoint    *WorkerCheckpoint   `json:"checkpoint,omitempty"`
	Snapshot      EnvironmentSnapshot `json:"snapshot,omitempty"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

type StateStore interface {
	Save(context.Context, LifecycleState) error
	Load(context.Context, string) (LifecycleState, bool, error)
	Delete(context.Context, string) error
}

// Executor is the scheduler-facing lifecycle contract. Implementations also
// expose the narrower workscheduler interfaces for compatibility.
type Executor interface {
	workscheduler.Executor
	workscheduler.RecoverableExecutor
	workscheduler.CancelableExecutor
	Capabilities() ExecutorCapabilities
}
