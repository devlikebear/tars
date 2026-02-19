package main

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/serverauth"
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
	return normalizeWorkspaceID(serverauth.WorkspaceIDFromRequest(r))
}

func resolveWorkspaceDir(baseWorkspaceDir, workspaceID string) string {
	base := strings.TrimSpace(baseWorkspaceDir)
	if base == "" {
		base = "."
	}
	normalized := normalizeWorkspaceID(workspaceID)
	if normalized == defaultWorkspaceID {
		return base
	}
	return filepath.Join(base, "_workspaces", normalized)
}

func resolveSessionStoreForRequest(baseWorkspaceDir string, baseStore *session.Store, r *http.Request) (*session.Store, string, string, error) {
	workspaceID := workspaceIDFromRequest(r)
	workspaceDir := resolveWorkspaceDir(baseWorkspaceDir, workspaceID)
	if err := memory.EnsureWorkspace(workspaceDir); err != nil {
		return nil, "", "", err
	}
	if workspaceID == defaultWorkspaceID && baseStore != nil {
		return baseStore, workspaceDir, workspaceID, nil
	}
	return session.NewStore(workspaceDir), workspaceDir, workspaceID, nil
}

func newWorkspaceSessionStoreResolver(baseWorkspaceDir string, defaultStore *session.Store) func(workspaceID string) *session.Store {
	base := strings.TrimSpace(baseWorkspaceDir)
	var cache sync.Map
	if defaultStore != nil {
		cache.Store(defaultWorkspaceID, defaultStore)
	}
	return func(workspaceID string) *session.Store {
		normalizedWorkspaceID := normalizeWorkspaceID(workspaceID)
		if value, ok := cache.Load(normalizedWorkspaceID); ok {
			if resolved, ok := value.(*session.Store); ok && resolved != nil {
				return resolved
			}
		}
		workspaceDir := resolveWorkspaceDir(base, normalizedWorkspaceID)
		if err := memory.EnsureWorkspace(workspaceDir); err != nil {
			return defaultStore
		}
		resolved := session.NewStore(workspaceDir)
		cache.Store(normalizedWorkspaceID, resolved)
		return resolved
	}
}

func resolveCronStoreForRequest(baseWorkspaceDir string, runHistoryLimit int, r *http.Request) (*cron.Store, string, string, error) {
	workspaceID := workspaceIDFromRequest(r)
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
	cache            sync.Map
}

func newWorkspaceCronStoreResolver(baseWorkspaceDir string, runHistoryLimit int, defaultStore *cron.Store) *workspaceCronStoreResolver {
	resolver := &workspaceCronStoreResolver{
		baseWorkspaceDir: strings.TrimSpace(baseWorkspaceDir),
		runHistoryLimit:  runHistoryLimit,
		defaultStore:     defaultStore,
	}
	if defaultStore != nil {
		resolver.cache.Store(defaultWorkspaceID, defaultStore)
	}
	return resolver
}

func (r *workspaceCronStoreResolver) Resolve(workspaceID string) (*cron.Store, error) {
	if r == nil {
		return nil, nil
	}
	normalizedWorkspaceID := normalizeWorkspaceID(workspaceID)
	if value, ok := r.cache.Load(normalizedWorkspaceID); ok {
		if resolved, ok := value.(*cron.Store); ok && resolved != nil {
			return resolved, nil
		}
	}
	baseWorkspaceDir := strings.TrimSpace(r.baseWorkspaceDir)
	if baseWorkspaceDir == "" && r.defaultStore != nil {
		return r.defaultStore, nil
	}
	workspaceDir := resolveWorkspaceDir(baseWorkspaceDir, normalizedWorkspaceID)
	if err := memory.EnsureWorkspace(workspaceDir); err != nil {
		return nil, err
	}
	store := cron.NewStoreWithOptions(workspaceDir, cron.StoreOptions{RunHistoryLimit: r.runHistoryLimit})
	if normalizedWorkspaceID == defaultWorkspaceID && r.defaultStore != nil {
		store = r.defaultStore
	}
	r.cache.Store(normalizedWorkspaceID, store)
	return store, nil
}

func (r *workspaceCronStoreResolver) ResolveFromRequest(req *http.Request) (*cron.Store, string, error) {
	workspaceID := workspaceIDFromRequest(req)
	store, err := r.Resolve(workspaceID)
	if err != nil {
		return nil, "", err
	}
	return store, workspaceID, nil
}

func (r *workspaceCronStoreResolver) WorkspaceIDs() ([]string, error) {
	if r == nil {
		return []string{defaultWorkspaceID}, nil
	}
	ids := map[string]struct{}{defaultWorkspaceID: {}}
	r.cache.Range(func(key, _ any) bool {
		value, ok := key.(string)
		if !ok {
			return true
		}
		normalized := normalizeWorkspaceID(value)
		if normalized != "" {
			ids[normalized] = struct{}{}
		}
		return true
	})
	baseWorkspaceDir := strings.TrimSpace(r.baseWorkspaceDir)
	if baseWorkspaceDir != "" {
		workspaceRoots := filepath.Join(baseWorkspaceDir, "_workspaces")
		entries, err := os.ReadDir(workspaceRoots)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := normalizeWorkspaceID(entry.Name())
			if strings.TrimSpace(name) == "" {
				continue
			}
			ids[name] = struct{}{}
		}
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}
