package executionplane

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

var safeStateID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type FileStateStore struct {
	rootDir string
	mu      sync.Mutex
}

func NewFileStateStore(rootDir string) (*FileStateStore, error) {
	root := strings.TrimSpace(rootDir)
	if root == "" {
		return nil, fmt.Errorf("executionplane: state root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("executionplane: resolve state root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("executionplane: create state root: %w", err)
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("executionplane: secure state root: %w", err)
	}
	return &FileStateStore{rootDir: absolute}, nil
}

func (store *FileStateStore) Save(ctx context.Context, state LifecycleState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if store == nil || !safeStateID.MatchString(strings.TrimSpace(state.AttemptID)) || state.SchemaVersion != lifecycleSchemaVersion {
		return fmt.Errorf("executionplane: invalid lifecycle state")
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("executionplane: encode lifecycle state: %w", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	path := store.statePath(state.AttemptID)
	if err := atomicwrite.Write(path, append(payload, '\n')); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("executionplane: secure lifecycle state: %w", err)
	}
	return nil
}

func (store *FileStateStore) Load(ctx context.Context, attemptID string) (LifecycleState, bool, error) {
	if err := ctx.Err(); err != nil {
		return LifecycleState{}, false, err
	}
	if store == nil || !safeStateID.MatchString(strings.TrimSpace(attemptID)) {
		return LifecycleState{}, false, fmt.Errorf("executionplane: invalid attempt id")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	raw, err := os.ReadFile(store.statePath(attemptID))
	if os.IsNotExist(err) {
		return LifecycleState{}, false, nil
	}
	if err != nil {
		return LifecycleState{}, false, fmt.Errorf("executionplane: read lifecycle state: %w", err)
	}
	var state LifecycleState
	if err := json.Unmarshal(raw, &state); err != nil {
		return LifecycleState{}, false, fmt.Errorf("executionplane: decode lifecycle state: %w", err)
	}
	if state.SchemaVersion != lifecycleSchemaVersion || state.AttemptID != attemptID {
		return LifecycleState{}, false, fmt.Errorf("executionplane: incompatible lifecycle state")
	}
	return state, true, nil
}

func (store *FileStateStore) Delete(ctx context.Context, attemptID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if store == nil || !safeStateID.MatchString(strings.TrimSpace(attemptID)) {
		return fmt.Errorf("executionplane: invalid attempt id")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	err := os.Remove(store.statePath(attemptID))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("executionplane: delete lifecycle state: %w", err)
	}
	return nil
}

func (store *FileStateStore) statePath(attemptID string) string {
	return filepath.Join(store.rootDir, attemptID+".json")
}

var _ StateStore = (*FileStateStore)(nil)
