package tarsserver

import (
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
)

const defaultWorkspaceID = gateway.DefaultWorkspaceID

func normalizeWorkspaceID(raw string) string {
	return gateway.NormalizeWorkspaceID(raw)
}

func workspaceIDFromRequest(_ *http.Request) string {
	return defaultWorkspaceID
}

func resolveWorkspaceDir(baseWorkspaceDir, _ string) string {
	base := strings.TrimSpace(baseWorkspaceDir)
	if base == "" {
		return "."
	}
	return base
}

func ensureWorkspaceDir(baseWorkspaceDir string) (string, error) {
	workspaceDir := resolveWorkspaceDir(baseWorkspaceDir, defaultWorkspaceID)
	if err := memory.EnsureWorkspace(workspaceDir); err != nil {
		return "", err
	}
	return workspaceDir, nil
}

func resolveSessionStoreForRequest(baseWorkspaceDir string, baseStore *session.Store, _ *http.Request) (*session.Store, string, string, error) {
	workspaceDir, err := ensureWorkspaceDir(baseWorkspaceDir)
	if err != nil {
		return nil, "", "", err
	}
	if baseStore != nil {
		return baseStore, workspaceDir, defaultWorkspaceID, nil
	}
	return session.NewStore(workspaceDir), workspaceDir, defaultWorkspaceID, nil
}

func newWorkspaceSessionStoreResolver(baseWorkspaceDir string, defaultStore *session.Store) func(workspaceID string) *session.Store {
	return func(workspaceID string) *session.Store {
		if defaultStore != nil {
			return defaultStore
		}
		workspaceDir, err := ensureWorkspaceDir(baseWorkspaceDir)
		if err != nil {
			return nil
		}
		_ = workspaceID
		return session.NewStore(workspaceDir)
	}
}

func resolveCronStoreForRequest(baseWorkspaceDir string, runHistoryLimit int, _ *http.Request) (*cron.Store, string, string, error) {
	workspaceDir, err := ensureWorkspaceDir(baseWorkspaceDir)
	if err != nil {
		return nil, "", "", err
	}
	store := cron.NewStoreWithOptions(workspaceDir, cron.StoreOptions{RunHistoryLimit: runHistoryLimit})
	return store, workspaceDir, defaultWorkspaceID, nil
}

type workspaceCronStoreResolver struct {
	baseWorkspaceDir string
	runHistoryLimit  int
	defaultStore     *cron.Store
}

func newWorkspaceCronStoreResolver(baseWorkspaceDir string, runHistoryLimit int, defaultStore *cron.Store) *workspaceCronStoreResolver {
	return &workspaceCronStoreResolver{
		baseWorkspaceDir: strings.TrimSpace(baseWorkspaceDir),
		runHistoryLimit:  runHistoryLimit,
		defaultStore:     defaultStore,
	}
}

func (r *workspaceCronStoreResolver) Resolve(workspaceID string) (*cron.Store, error) {
	if r == nil {
		return nil, nil
	}
	if r.defaultStore != nil {
		return r.defaultStore, nil
	}
	workspaceDir, err := ensureWorkspaceDir(r.baseWorkspaceDir)
	if err != nil {
		return nil, err
	}
	_ = workspaceID
	return cron.NewStoreWithOptions(workspaceDir, cron.StoreOptions{RunHistoryLimit: r.runHistoryLimit}), nil
}

func (r *workspaceCronStoreResolver) ResolveFromRequest(_ *http.Request) (*cron.Store, string, error) {
	store, err := r.Resolve(defaultWorkspaceID)
	if err != nil {
		return nil, "", err
	}
	return store, defaultWorkspaceID, nil
}

func (r *workspaceCronStoreResolver) WorkspaceIDs() ([]string, error) {
	_ = r
	return []string{defaultWorkspaceID}, nil
}
