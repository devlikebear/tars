// Package autofix holds the whitelist of corrective actions pulse may
// invoke without further human approval. Every autofix must be:
//
//   - idempotent: running it twice in a row produces the same end state;
//   - scoped: touches only files pulse understands to be its own;
//   - bounded: completes in under a minute on typical workspaces;
//   - safe-on-failure: partial failures leave the workspace in a
//     consistent state (no half-deleted, half-renamed, half-compressed
//     files).
//
// New autofixes are added by implementing the Fixer interface and
// registering a constructor in the DefaultRegistry initializer below.
// The decider cannot invoke an autofix whose name is not in both the
// code registry AND the operator's config allow-list — this double gate
// is intentional.
package autofix

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Result captures what an autofix did. Fields are optional; an empty
// Result is valid for autofixes that have nothing meaningful to report
// (e.g. "there was nothing to clean up").
type Result struct {
	Name    string         `json:"name"`
	Summary string         `json:"summary,omitempty"`
	Details map[string]any `json:"details,omitempty"`
	Changed bool           `json:"changed"`
}

// Fixer is the single-method interface each autofix implementation must
// satisfy. Fixers are expected to be pure functions of their constructor
// arguments (workspace dir, clock, etc.) — no package-level state.
type Fixer interface {
	Name() string
	Run(ctx context.Context) (Result, error)
}

// Registry is a concurrent-safe name → Fixer map. Its primary use is the
// pulse runtime looking up a named autofix from a Decision.
type Registry struct {
	mu     sync.RWMutex
	fixers map[string]Fixer
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{fixers: map[string]Fixer{}}
}

// Register adds a fixer to the registry. Re-registering a name replaces
// the previous fixer — callers that care should check Has first.
func (r *Registry) Register(f Fixer) {
	if f == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fixers[f.Name()] = f
}

// Has reports whether a fixer with the given name is registered.
func (r *Registry) Has(name string) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.fixers[name]
	return ok
}

// Names returns all registered fixer names in sorted order. Used by
// pulse startup logging and by HTTP handlers that want to report which
// autofixes are known.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.fixers))
	for n := range r.fixers {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Run executes the named fixer. It returns ErrUnknown when the name is
// not registered, letting callers distinguish that from legitimate fixer
// errors.
func (r *Registry) Run(ctx context.Context, name string) (Result, error) {
	if r == nil {
		return Result{}, ErrUnknown{Name: name}
	}
	r.mu.RLock()
	fixer, ok := r.fixers[name]
	r.mu.RUnlock()
	if !ok {
		return Result{}, ErrUnknown{Name: name}
	}
	return fixer.Run(ctx)
}

// ErrUnknown is returned when a requested autofix is not registered.
type ErrUnknown struct{ Name string }

func (e ErrUnknown) Error() string {
	return fmt.Sprintf("autofix %q is not registered", e.Name)
}

// AllowedIntersection returns the intersection of the registry's names
// and the operator-configured allow-list. This is the exact list that
// should be handed to the decider as the autofix policy — anything
// outside this set cannot safely run.
func (r *Registry) AllowedIntersection(configured []string) []string {
	if r == nil || len(configured) == 0 {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(configured))
	seen := map[string]struct{}{}
	for _, n := range configured {
		if _, ok := r.fixers[n]; !ok {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
