package skillhub

import (
	"fmt"
	"strings"
	"sync"
)

// DefaultSourceID is the source identifier assigned to legacy InstalledSkill
// rows with an empty Source field and to entries from the built-in hub.
const DefaultSourceID = "tars-hub"

// SourceRegistry holds the set of registered HubSources. It is safe for
// concurrent use.
type SourceRegistry struct {
	mu      sync.RWMutex
	sources map[string]HubSource
	order   []string
}

// NewSourceRegistry returns an empty registry.
func NewSourceRegistry() *SourceRegistry {
	return &SourceRegistry{sources: map[string]HubSource{}}
}

// Register adds a source. Returns an error if the source has an empty ID or
// if another source with the same canonical ID is already registered.
func (r *SourceRegistry) Register(src HubSource) error {
	if src == nil {
		return fmt.Errorf("skillhub: cannot register nil source")
	}
	id := canonicalSourceID(src.ID())
	if id == "" {
		return fmt.Errorf("skillhub: source has empty ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sources[id]; ok {
		return fmt.Errorf("skillhub: source %q already registered", id)
	}
	r.sources[id] = src
	r.order = append(r.order, id)
	return nil
}

// Get returns the source registered under id (case-insensitive).
func (r *SourceRegistry) Get(id string) (HubSource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	src, ok := r.sources[canonicalSourceID(id)]
	return src, ok
}

// List returns all registered sources in registration order.
func (r *SourceRegistry) List() []HubSource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]HubSource, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.sources[id])
	}
	return out
}

// IDs returns the registered source IDs in registration order.
func (r *SourceRegistry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Len returns the number of registered sources.
func (r *SourceRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.order)
}

// ResolveSkillRef parses a user-facing reference like "openclaw:foo" and
// returns the source ID plus the bare skill name. An empty sourceID means
// "search every registered source" — the install path is expected to handle
// that ambiguity.
//
// Whitespace around either component is trimmed. A trailing colon with no
// name (e.g. "openclaw:") yields the source plus an empty name. A leading
// colon (":foo") is treated as no source plus "foo".
func ResolveSkillRef(ref string) (sourceID, name string) {
	trimmed := strings.TrimSpace(ref)
	idx := strings.Index(trimmed, ":")
	if idx < 0 {
		return "", trimmed
	}
	sourceID = canonicalSourceID(trimmed[:idx])
	name = strings.TrimSpace(trimmed[idx+1:])
	return sourceID, name
}

func canonicalSourceID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
