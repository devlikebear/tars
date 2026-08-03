package workerprotocol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

const remoteRunStateSchemaVersion = 1

var safeRemoteRunID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,255}$`)

type RemoteRunState struct {
	SchemaVersion int              `json:"schema_version"`
	AttemptID     string           `json:"attempt_id"`
	Input         RemoteRunInput   `json:"input"`
	Result        *RemoteRunResult `json:"result,omitempty"`
}

type RemoteRunStore interface {
	Prepare(context.Context, RemoteRunInput) error
	RecordResult(context.Context, RemoteRunInput, RemoteRunResult) error
	Load(context.Context, string) (RemoteRunState, bool, error)
	Delete(context.Context, string) error
}

type RemoteResultRecorder interface {
	RecordResult(context.Context, RemoteRunInput, RemoteRunResult) error
}

type FileRemoteRunStore struct {
	rootDir string
	mu      sync.Mutex
}

func NewFileRemoteRunStore(rootDir string) (*FileRemoteRunStore, error) {
	root := strings.TrimSpace(rootDir)
	if root == "" {
		return nil, fmt.Errorf("workerprotocol: remote run state root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workerprotocol: resolve remote run state root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("workerprotocol: create remote run state root: %w", err)
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("workerprotocol: secure remote run state root: %w", err)
	}
	return &FileRemoteRunStore{rootDir: absolute}, nil
}

func (store *FileRemoteRunStore) Prepare(ctx context.Context, input RemoteRunInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if store == nil || !safeRemoteRunID.MatchString(input.AttemptID) || len(input.RedactValues) != 0 {
		return fmt.Errorf("workerprotocol: invalid durable remote run input")
	}
	if err := validateRemoteRunInput(input); err != nil {
		return err
	}
	state := RemoteRunState{SchemaVersion: remoteRunStateSchemaVersion, AttemptID: input.AttemptID, Input: cloneRemoteRunInput(input)}
	store.mu.Lock()
	defer store.mu.Unlock()
	existing, found, err := store.loadLocked(input.AttemptID)
	if err != nil {
		return err
	}
	if found {
		if remoteRunInputEqual(existing.Input, state.Input) {
			return nil
		}
		return fmt.Errorf("%w: remote run input changed for attempt %s", ErrConflict, input.AttemptID)
	}
	return store.saveLocked(state)
}

func (store *FileRemoteRunStore) RecordResult(ctx context.Context, input RemoteRunInput, result RemoteRunResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if store == nil || !safeRemoteRunID.MatchString(input.AttemptID) {
		return fmt.Errorf("workerprotocol: invalid durable remote result")
	}
	if err := validatePersistedRemoteResult(result); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	state, found, err := store.loadLocked(input.AttemptID)
	if err != nil {
		return err
	}
	if !found || !remoteRunInputEqual(state.Input, cloneRemoteRunInput(input)) {
		return fmt.Errorf("%w: remote run input is not durably prepared", ErrConflict)
	}
	if state.Result != nil {
		if remoteRunResultEqual(*state.Result, result) {
			return nil
		}
		return fmt.Errorf("%w: remote result changed for attempt %s", ErrConflict, input.AttemptID)
	}
	copy := cloneRemoteRunResult(result)
	state.Result = &copy
	return store.saveLocked(state)
}

func (store *FileRemoteRunStore) Load(ctx context.Context, attemptID string) (RemoteRunState, bool, error) {
	if err := ctx.Err(); err != nil {
		return RemoteRunState{}, false, err
	}
	if store == nil || !safeRemoteRunID.MatchString(strings.TrimSpace(attemptID)) {
		return RemoteRunState{}, false, fmt.Errorf("workerprotocol: invalid remote run attempt id")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.loadLocked(strings.TrimSpace(attemptID))
}

func (store *FileRemoteRunStore) Delete(ctx context.Context, attemptID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if store == nil || !safeRemoteRunID.MatchString(strings.TrimSpace(attemptID)) {
		return fmt.Errorf("workerprotocol: invalid remote run attempt id")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	err := os.Remove(store.statePath(strings.TrimSpace(attemptID)))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("workerprotocol: delete remote run state: %w", err)
	}
	return nil
}

func (store *FileRemoteRunStore) loadLocked(attemptID string) (RemoteRunState, bool, error) {
	raw, err := os.ReadFile(store.statePath(attemptID))
	if os.IsNotExist(err) {
		return RemoteRunState{}, false, nil
	}
	if err != nil {
		return RemoteRunState{}, false, fmt.Errorf("workerprotocol: read remote run state: %w", err)
	}
	var state RemoteRunState
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return RemoteRunState{}, false, fmt.Errorf("workerprotocol: decode remote run state: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return RemoteRunState{}, false, fmt.Errorf("%w: remote run state has trailing data", ErrWireContract)
	}
	if state.SchemaVersion != remoteRunStateSchemaVersion || state.AttemptID != attemptID ||
		state.Input.AttemptID != attemptID || len(state.Input.RedactValues) != 0 {
		return RemoteRunState{}, false, fmt.Errorf("%w: incompatible remote run state", ErrWireContract)
	}
	if err := validateRemoteRunInput(state.Input); err != nil {
		return RemoteRunState{}, false, fmt.Errorf("%w: invalid remote run input: %v", ErrWireContract, err)
	}
	if state.Result != nil {
		if err := validatePersistedRemoteResult(*state.Result); err != nil {
			return RemoteRunState{}, false, err
		}
	}
	return cloneRemoteRunState(state), true, nil
}

func (store *FileRemoteRunStore) saveLocked(state RemoteRunState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("workerprotocol: encode remote run state: %w", err)
	}
	path := store.statePath(state.AttemptID)
	if err := atomicwrite.Write(path, append(raw, '\n')); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("workerprotocol: secure remote run state: %w", err)
	}
	return nil
}

func (store *FileRemoteRunStore) statePath(attemptID string) string {
	return filepath.Join(store.rootDir, attemptID+".json")
}

func validatePersistedRemoteResult(result RemoteRunResult) error {
	if len(result.Payload) > 0 && !json.Valid(result.Payload) {
		return fmt.Errorf("%w: invalid remote result payload", ErrWireContract)
	}
	for _, artifact := range result.Artifacts {
		if strings.TrimSpace(artifact.Name) == "" || strings.TrimSpace(artifact.URI) == "" || strings.TrimSpace(artifact.Digest) == "" || artifact.SizeBytes < 0 {
			return fmt.Errorf("%w: invalid released remote artifact", ErrWireContract)
		}
	}
	for _, artifact := range result.RejectedArtifacts {
		if strings.TrimSpace(artifact.Name) == "" || strings.TrimSpace(artifact.Reason) == "" {
			return fmt.Errorf("%w: invalid rejected remote artifact", ErrWireContract)
		}
	}
	if result.Checkpoint != nil && (!validProtocolIdentifier(result.Checkpoint.ID) || strings.TrimSpace(result.Checkpoint.Digest) == "") {
		return fmt.Errorf("%w: invalid remote checkpoint", ErrWireContract)
	}
	return nil
}

func cloneRemoteRunState(state RemoteRunState) RemoteRunState {
	copy := state
	copy.Input = cloneRemoteRunInput(state.Input)
	if state.Result != nil {
		result := cloneRemoteRunResult(*state.Result)
		copy.Result = &result
	}
	return copy
}

func cloneRemoteRunInput(input RemoteRunInput) RemoteRunInput {
	copy := input
	copy.Policy.Egress.AllowHosts = append([]string(nil), input.Policy.Egress.AllowHosts...)
	copy.Workspace = cloneWorkspaceBundle(input.Workspace)
	copy.Request = append(json.RawMessage(nil), input.Request...)
	copy.RedactValues = nil
	return copy
}

func cloneWorkspaceBundle(bundle WorkspaceBundle) WorkspaceBundle {
	copy := bundle
	copy.Manifest.Entries = append([]WorkspaceManifestEntry(nil), bundle.Manifest.Entries...)
	copy.Manifest.ExcludedPaths = append([]string(nil), bundle.Manifest.ExcludedPaths...)
	copy.Files = make([]WorkspaceFile, len(bundle.Files))
	for index, file := range bundle.Files {
		copy.Files[index] = file
		copy.Files[index].Data = append([]byte(nil), file.Data...)
	}
	return copy
}

func cloneRemoteRunResult(result RemoteRunResult) RemoteRunResult {
	copy := result
	copy.Payload = append(json.RawMessage(nil), result.Payload...)
	copy.Artifacts = append([]ReleasedArtifact(nil), result.Artifacts...)
	copy.RejectedArtifacts = append([]RejectedArtifact(nil), result.RejectedArtifacts...)
	copy.Checkpoint = cloneCheckpointPayload(result.Checkpoint)
	return copy
}

func remoteRunInputEqual(left, right RemoteRunInput) bool {
	leftRaw, leftErr := json.Marshal(left)
	rightRaw, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftRaw) == string(rightRaw)
}

func remoteRunResultEqual(left, right RemoteRunResult) bool {
	leftRaw, leftErr := json.Marshal(left)
	rightRaw, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftRaw) == string(rightRaw)
}

var _ RemoteRunStore = (*FileRemoteRunStore)(nil)
var _ RemoteResultRecorder = (*FileRemoteRunStore)(nil)
