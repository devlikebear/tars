package sessionoverride

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/devlikebear/tars/internal/session"
)

// Resolution is the cached output of one Service.Resolve call.
type Resolution struct {
	SessionID   string            `json:"session_id"`
	Cwd         string            `json:"cwd"`
	Effective   EffectiveConfig   `json:"effective"`
	Sources     map[string]Source `json:"sources"`
	Diagnostics []Diagnostic      `json:"diagnostics,omitempty"`
}

// Service resolves a session's EffectiveConfig from its base configuration
// (sessions.json) plus any `.tars/` overrides at the session's active cwd.
// Results are cached per-session keyed by (cwd, settings.json mtime,
// settings.local.json mtime); a Resolve call detects file mutations and
// reloads automatically. Cache entries are also dropped explicitly via
// Invalidate when the active cwd transitions.
//
// The zero value is not usable; callers must construct via NewService.
type Service struct {
	store *session.Store
	mu    sync.RWMutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	cwd         string
	sharedMtime int64 // unix nanos; 0 = file absent
	localMtime  int64
	resolution  Resolution
}

// NewService constructs a Service backed by the supplied session store.
// store may be nil for tests that want to manually inject behavior — calls
// to Resolve will then surface the missing-store error.
func NewService(store *session.Store) *Service {
	return &Service{
		store: store,
		cache: map[string]cacheEntry{},
	}
}

// Resolve returns the up-to-date Resolution for sessionID. Cached values
// are returned when the active cwd has not changed AND neither
// settings.json nor settings.local.json has been modified.
func (s *Service) Resolve(sessionID string) (Resolution, bool, error) {
	if s == nil || s.store == nil {
		return Resolution{}, false, errors.New("sessionoverride.Service: store not configured")
	}
	cwd, err := s.store.GetCurrentDir(sessionID)
	if err != nil {
		return Resolution{}, false, err
	}
	sharedMtime, localMtime := overrideFileMtimes(cwd)

	s.mu.RLock()
	if entry, ok := s.cache[sessionID]; ok &&
		entry.cwd == cwd &&
		entry.sharedMtime == sharedMtime &&
		entry.localMtime == localMtime {
		s.mu.RUnlock()
		return entry.resolution, false, nil
	}
	s.mu.RUnlock()

	sess, err := s.store.Get(sessionID)
	if err != nil {
		return Resolution{}, false, err
	}
	baseToolConfig := session.SessionToolConfig{}
	if sess.ToolConfig != nil {
		baseToolConfig = *sess.ToolConfig
	}

	shared, local, diags, err := Load(cwd)
	if err != nil {
		// Hard error from the loader (parse failure / IO). Surface it; do
		// not cache the failure — operator-fixable, retry-friendly.
		return Resolution{}, false, err
	}

	effective, sources := Merge(baseToolConfig, sess.PromptOverride, shared, local)
	resolution := Resolution{
		SessionID:   sessionID,
		Cwd:         cwd,
		Effective:   effective,
		Sources:     sources,
		Diagnostics: diags,
	}

	s.mu.Lock()
	prior, hadPrior := s.cache[sessionID]
	s.cache[sessionID] = cacheEntry{
		cwd:         cwd,
		sharedMtime: sharedMtime,
		localMtime:  localMtime,
		resolution:  resolution,
	}
	s.mu.Unlock()

	changed := !hadPrior || prior.cwd != cwd || prior.sharedMtime != sharedMtime || prior.localMtime != localMtime
	return resolution, changed, nil
}

// Invalidate drops any cached resolution for sessionID. Safe to call for
// a session ID that was never resolved.
func (s *Service) Invalidate(sessionID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.cache, sessionID)
	s.mu.Unlock()
}

// overrideFileMtimes returns the mtime (unix nanos) of settings.json and
// settings.local.json under cwd/.tars. A missing file maps to 0 so two
// "missing" snapshots compare equal.
func overrideFileMtimes(cwd string) (int64, int64) {
	if cwd == "" {
		return 0, 0
	}
	return mtime(filepath.Join(cwd, settingsDirName, sharedSettingsName)),
		mtime(filepath.Join(cwd, settingsDirName, localSettingsName))
}

func mtime(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

