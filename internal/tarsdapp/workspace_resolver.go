package tarsdapp

import (
	"net/http"
	"strings"
	"unicode"

	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/session"
)

const defaultWorkspaceID = "default"

func normalizeWorkspaceID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultWorkspaceID
	}
	var b strings.Builder
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	sanitized := strings.TrimSpace(b.String())
	if sanitized == "" {
		return defaultWorkspaceID
	}
	return sanitized
}

func workspaceIDFromRequest(r *http.Request) string {
	return defaultWorkspaceID
}

func resolveWorkspaceDir(baseWorkspaceDir, workspaceID string) string {
	base := strings.TrimSpace(baseWorkspaceDir)
	if base == "" {
		base = "."
	}
	return base
}

func resolveSessionStoreForRequest(baseWorkspaceDir string, baseStore *session.Store, r *http.Request) (*session.Store, string, string, error) {
	workspaceID := defaultWorkspaceID
	workspaceDir := resolveWorkspaceDir(baseWorkspaceDir, workspaceID)
	if err := memory.EnsureWorkspace(workspaceDir); err != nil {
		return nil, "", "", err
	}
	if baseStore != nil {
		return baseStore, workspaceDir, workspaceID, nil
	}
	return session.NewStore(workspaceDir), workspaceDir, workspaceID, nil
}

func newWorkspaceSessionStoreResolver(baseWorkspaceDir string, defaultStore *session.Store) func(workspaceID string) *session.Store {
	return func(workspaceID string) *session.Store {
		if defaultStore != nil {
			return defaultStore
		}
		workspaceDir := resolveWorkspaceDir(baseWorkspaceDir, defaultWorkspaceID)
		if err := memory.EnsureWorkspace(workspaceDir); err != nil {
			return nil
		}
		return session.NewStore(workspaceDir)
	}
}

func resolveCronStoreForRequest(baseWorkspaceDir string, runHistoryLimit int, r *http.Request) (*cron.Store, string, string, error) {
	workspaceID := defaultWorkspaceID
	workspaceDir := resolveWorkspaceDir(baseWorkspaceDir, workspaceID)
	if err := memory.EnsureWorkspace(workspaceDir); err != nil {
		return nil, "", "", err
	}
	store := cron.NewStoreWithOptions(workspaceDir, cron.StoreOptions{RunHistoryLimit: runHistoryLimit})
	return store, workspaceDir, workspaceID, nil
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
	baseWorkspaceDir := strings.TrimSpace(r.baseWorkspaceDir)
	if baseWorkspaceDir == "" && r.defaultStore != nil {
		return r.defaultStore, nil
	}
	workspaceDir := resolveWorkspaceDir(baseWorkspaceDir, defaultWorkspaceID)
	if err := memory.EnsureWorkspace(workspaceDir); err != nil {
		return nil, err
	}
	store := cron.NewStoreWithOptions(workspaceDir, cron.StoreOptions{RunHistoryLimit: r.runHistoryLimit})
	if r.defaultStore != nil {
		store = r.defaultStore
	}
	return store, nil
}

func (r *workspaceCronStoreResolver) ResolveFromRequest(req *http.Request) (*cron.Store, string, error) {
	store, err := r.Resolve(defaultWorkspaceID)
	if err != nil {
		return nil, "", err
	}
	return store, defaultWorkspaceID, nil
}

func (r *workspaceCronStoreResolver) WorkspaceIDs() ([]string, error) {
	return []string{defaultWorkspaceID}, nil
}
