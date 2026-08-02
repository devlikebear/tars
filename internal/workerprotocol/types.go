package workerprotocol

import (
	"fmt"
	"net"
	"strings"
	"time"
)

type WorkerState string

const (
	WorkerStateRegistered   WorkerState = "registered"
	WorkerStateReady        WorkerState = "ready"
	WorkerStateLeased       WorkerState = "leased"
	WorkerStateExecuting    WorkerState = "executing"
	WorkerStateDisconnected WorkerState = "disconnected"
	WorkerStateLost         WorkerState = "lost"
	WorkerStateDraining     WorkerState = "draining"
	WorkerStateDestroyed    WorkerState = "destroyed"
)

type PlacementState string

const (
	PlacementStatePending      PlacementState = "pending"
	PlacementStateProvisioning PlacementState = "provisioning"
	PlacementStateSyncing      PlacementState = "syncing"
	PlacementStateReady        PlacementState = "ready"
	PlacementStateExecuting    PlacementState = "executing"
	PlacementStateCheckpointed PlacementState = "checkpointed"
	PlacementStateCollecting   PlacementState = "collecting"
	PlacementStateCompleted    PlacementState = "completed"
	PlacementStateFailed       PlacementState = "failed"
	PlacementStateLost         PlacementState = "lost"
	PlacementStateReclaiming   PlacementState = "reclaiming"
	PlacementStateRehydrating  PlacementState = "rehydrating"
	PlacementStateDestroyed    PlacementState = "destroyed"
)

type WorkerCapabilities struct {
	Resume         bool `json:"resume"`
	Streaming      bool `json:"streaming"`
	Checkpoints    bool `json:"checkpoints"`
	EgressPolicy   bool `json:"egress_policy"`
	ResourceLimits bool `json:"resource_limits"`
	ArtifactScan   bool `json:"artifact_scan"`
}

type ResourceLimits struct {
	CPUSeconds     int64 `json:"cpu_seconds"`
	MemoryMB       int64 `json:"memory_mb"`
	DiskMB         int64 `json:"disk_mb"`
	MaxOutputBytes int64 `json:"max_output_bytes"`
}

type ResourceUsage struct {
	CPUSeconds int64 `json:"cpu_seconds,omitempty"`
	MemoryMB   int64 `json:"memory_mb,omitempty"`
	DiskMB     int64 `json:"disk_mb,omitempty"`
}

type EgressMode string

const (
	EgressDeny      EgressMode = "deny"
	EgressAllowlist EgressMode = "allowlist"
)

type EgressPolicy struct {
	Mode       EgressMode `json:"mode"`
	AllowHosts []string   `json:"allow_hosts,omitempty"`
}

type ExecutionPolicy struct {
	Egress EgressPolicy   `json:"egress"`
	Limits ResourceLimits `json:"limits"`
}

func DefaultExecutionPolicy() ExecutionPolicy {
	return ExecutionPolicy{
		Egress: EgressPolicy{Mode: EgressDeny},
		Limits: ResourceLimits{CPUSeconds: 3600, MemoryMB: 2048, DiskMB: 4096, MaxOutputBytes: 64 << 20},
	}
}

func (policy ExecutionPolicy) Validate() error {
	if policy.Limits.CPUSeconds <= 0 || policy.Limits.MemoryMB <= 0 || policy.Limits.DiskMB <= 0 || policy.Limits.MaxOutputBytes <= 0 {
		return fmt.Errorf("%w: positive CPU, memory, disk, and output bounds are required", ErrInvalidPolicy)
	}
	switch policy.Egress.Mode {
	case EgressDeny:
		if len(policy.Egress.AllowHosts) != 0 {
			return fmt.Errorf("%w: deny mode cannot carry allowed hosts", ErrInvalidPolicy)
		}
	case EgressAllowlist:
		if len(policy.Egress.AllowHosts) == 0 {
			return fmt.Errorf("%w: allowlist mode requires at least one host", ErrInvalidPolicy)
		}
		for _, host := range policy.Egress.AllowHosts {
			if !validEgressHost(host) {
				return fmt.Errorf("%w: invalid egress host %q", ErrInvalidPolicy, host)
			}
		}
	default:
		return fmt.Errorf("%w: invalid egress mode %q", ErrInvalidPolicy, policy.Egress.Mode)
	}
	return nil
}

func validEgressHost(raw string) bool {
	host := strings.TrimSpace(strings.ToLower(raw))
	if host == "" || strings.ContainsAny(host, "/:@ \\") {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") || !strings.Contains(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

type SyncMode string

const (
	SyncModeGit       SyncMode = "git"
	SyncModeDirectory SyncMode = "directory"
)

type Ownership string

const (
	OwnerGateway Ownership = "gateway"
	OwnerWorker  Ownership = "worker"
)

type SyncSpec struct {
	Mode           SyncMode  `json:"mode"`
	SourceOwner    Ownership `json:"source_owner"`
	WorkspaceOwner Ownership `json:"workspace_owner"`
	ArtifactOwner  Ownership `json:"artifact_owner"`
	ManifestDigest string    `json:"manifest_digest,omitempty"`
}

func (spec SyncSpec) Validate() error {
	if spec.Mode != SyncModeGit && spec.Mode != SyncModeDirectory {
		return fmt.Errorf("%w: invalid sync mode %q", ErrInvalidPolicy, spec.Mode)
	}
	if spec.SourceOwner != OwnerGateway || spec.WorkspaceOwner != OwnerWorker || spec.ArtifactOwner != OwnerGateway {
		return fmt.Errorf("%w: source, workspace, and artifact ownership must be explicit gateway/worker/gateway", ErrInvalidPolicy)
	}
	return nil
}

type Worker struct {
	ID              string             `json:"id"`
	ProtocolVersion string             `json:"protocol_version"`
	Transport       string             `json:"transport"`
	Endpoint        string             `json:"endpoint"`
	State           WorkerState        `json:"state"`
	Capabilities    WorkerCapabilities `json:"capabilities"`
	LastSequence    int64              `json:"last_sequence"`
	LastSeenAt      time.Time          `json:"last_seen_at"`
	LeaseExpiresAt  *time.Time         `json:"lease_expires_at,omitempty"`
	Version         int                `json:"version"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type Checkpoint struct {
	ID        string    `json:"id"`
	Digest    string    `json:"digest"`
	URI       string    `json:"uri,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Placement struct {
	ID             string          `json:"id"`
	WorkspaceID    string          `json:"workspace_id"`
	WorkID         string          `json:"work_id"`
	StepID         string          `json:"step_id"`
	AttemptID      string          `json:"attempt_id"`
	WorkerID       string          `json:"worker_id"`
	EnvironmentID  string          `json:"environment_id,omitempty"`
	State          PlacementState  `json:"state"`
	Policy         ExecutionPolicy `json:"policy"`
	Sync           SyncSpec        `json:"sync"`
	SnapshotDigest string          `json:"snapshot_digest,omitempty"`
	Checkpoint     *Checkpoint     `json:"checkpoint,omitempty"`
	LeaseExpiresAt *time.Time      `json:"lease_expires_at,omitempty"`
	LastSequence   int64           `json:"last_sequence"`
	RecoveryCount  int             `json:"recovery_count"`
	Version        int             `json:"version"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}
