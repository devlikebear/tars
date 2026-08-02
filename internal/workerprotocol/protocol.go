// Package workerprotocol defines the versioned control-plane protocol shared
// by in-process and remote TARS workers.
package workerprotocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	ProtocolVersionV1 = "1.0"
	maxEnvelopeBytes  = 4 << 20
)

var (
	ErrVersionUnsupported = errors.New("workerprotocol: protocol version is unsupported")
	ErrInvalidEnvelope    = errors.New("workerprotocol: invalid envelope")
	ErrInvalidPolicy      = errors.New("workerprotocol: invalid execution policy")
	ErrInvalidTransition  = errors.New("workerprotocol: invalid state transition")
	ErrOutOfOrder         = errors.New("workerprotocol: message is out of order")
	ErrConflict           = errors.New("workerprotocol: idempotency conflict")
	ErrNotFound           = errors.New("workerprotocol: record not found")
)

type MessageType string

const (
	MessageRegister   MessageType = "register"
	MessageProvision  MessageType = "provision"
	MessageSync       MessageType = "sync"
	MessageLease      MessageType = "lease"
	MessageHeartbeat  MessageType = "heartbeat"
	MessageExecute    MessageType = "execute"
	MessageStream     MessageType = "stream"
	MessageCheckpoint MessageType = "checkpoint"
	MessageCollect    MessageType = "collect"
	MessageDestroy    MessageType = "destroy"
	MessageLost       MessageType = "lost"
	MessageReclaim    MessageType = "reclaim"
	MessageRehydrate  MessageType = "rehydrate"
)

func (messageType MessageType) String() string { return string(messageType) }

func validMessageType(messageType MessageType) bool {
	switch messageType {
	case MessageRegister, MessageProvision, MessageSync, MessageLease,
		MessageHeartbeat, MessageExecute, MessageStream, MessageCheckpoint,
		MessageCollect, MessageDestroy, MessageLost, MessageReclaim,
		MessageRehydrate:
		return true
	default:
		return false
	}
}

type Envelope struct {
	ProtocolVersion string          `json:"protocol_version"`
	MessageID       string          `json:"message_id"`
	IdempotencyKey  string          `json:"idempotency_key"`
	Type            MessageType     `json:"type"`
	WorkerID        string          `json:"worker_id"`
	PlacementID     string          `json:"placement_id,omitempty"`
	Sequence        int64           `json:"sequence"`
	SentAt          time.Time       `json:"sent_at"`
	Payload         json.RawMessage `json:"payload"`
}

func (envelope Envelope) Validate() error {
	if strings.TrimSpace(envelope.ProtocolVersion) != ProtocolVersionV1 {
		return fmt.Errorf("%w: got %q, want %s", ErrVersionUnsupported, envelope.ProtocolVersion, ProtocolVersionV1)
	}
	if !validMessageType(envelope.Type) || !validProtocolIdentifier(envelope.MessageID) ||
		!validProtocolIdentifier(envelope.IdempotencyKey) || !validProtocolIdentifier(envelope.WorkerID) ||
		envelope.Sequence <= 0 || envelope.SentAt.IsZero() || len(envelope.Payload) > maxEnvelopeBytes {
		return ErrInvalidEnvelope
	}
	if envelope.Type != MessageRegister && envelope.Type != MessageHeartbeat && envelope.Type != MessageLost && !validProtocolIdentifier(envelope.PlacementID) {
		return ErrInvalidEnvelope
	}
	if strings.TrimSpace(envelope.PlacementID) != "" && !validProtocolIdentifier(envelope.PlacementID) {
		return ErrInvalidEnvelope
	}
	if len(envelope.Payload) > 0 && !json.Valid(envelope.Payload) {
		return ErrInvalidEnvelope
	}
	return nil
}

func validProtocolIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 {
		return false
	}
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		case strings.ContainsRune("._:/@+-", char):
		default:
			return false
		}
	}
	return true
}

type RegisterPayload struct {
	Transport    string             `json:"transport"`
	Endpoint     string             `json:"endpoint"`
	Capabilities WorkerCapabilities `json:"capabilities"`
}

type HeartbeatPayload struct {
	LeaseTTLMS int64             `json:"lease_ttl_ms,omitempty"`
	Usage      ResourceUsage     `json:"usage,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type ProvisionPayload struct {
	EnvironmentID string           `json:"environment_id"`
	RootDir       string           `json:"root_dir,omitempty"`
	Manifest      json.RawMessage  `json:"manifest,omitempty"`
	Policy        ExecutionPolicy  `json:"policy,omitempty"`
	Binding       TaskTokenBinding `json:"binding,omitempty"`
}

type SyncPayload struct {
	Mode       SyncMode `json:"mode"`
	Digest     string   `json:"digest"`
	URI        string   `json:"uri,omitempty"`
	FileCount  int      `json:"file_count,omitempty"`
	TotalBytes int64    `json:"total_bytes,omitempty"`
}

type LeasePayload struct {
	LeaseTTLMS int64 `json:"lease_ttl_ms"`
}

type ExecutePayload struct {
	TaskToken      string          `json:"task_token"`
	Resume         bool            `json:"resume,omitempty"`
	CheckpointID   string          `json:"checkpoint_id,omitempty"`
	CheckpointHash string          `json:"checkpoint_digest,omitempty"`
	Request        json.RawMessage `json:"request,omitempty"`
}

type StreamPayload struct {
	Kind    string          `json:"kind"`
	Text    string          `json:"text,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type CheckpointPayload struct {
	ID       string          `json:"checkpoint_id"`
	Digest   string          `json:"digest"`
	URI      string          `json:"uri,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type CollectPayload struct {
	Complete       bool   `json:"complete"`
	Succeeded      bool   `json:"succeeded,omitempty"`
	SnapshotDigest string `json:"snapshot_digest,omitempty"`
	ArtifactCount  int    `json:"artifact_count,omitempty"`
	TaskToken      string `json:"task_token,omitempty"`
}

type DestroyPayload struct {
	Reason    string `json:"reason"`
	TaskToken string `json:"task_token,omitempty"`
}

type LostPayload struct {
	Reason string `json:"reason"`
}

type ReclaimPayload struct {
	Reason string `json:"reason"`
}

type RehydratePayload struct {
	ReplacementWorkerID string           `json:"replacement_worker_id"`
	EnvironmentID       string           `json:"environment_id"`
	SnapshotDigest      string           `json:"snapshot_digest"`
	CheckpointID        string           `json:"checkpoint_id,omitempty"`
	CheckpointDigest    string           `json:"checkpoint_digest,omitempty"`
	LeaseTTLMS          int64            `json:"lease_ttl_ms"`
	Binding             TaskTokenBinding `json:"binding,omitempty"`
	Policy              ExecutionPolicy  `json:"policy,omitempty"`
}
