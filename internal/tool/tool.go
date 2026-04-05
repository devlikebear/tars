package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/llm"
)

// RegistryScope identifies the surface a tool Registry belongs to.
//
// The TARS runtime is split into two surfaces:
//
//   - User surface  — chat sessions, agents. Registries use RegistryScopeUser.
//   - System surface — pulse (watchdog) and reflection (nightly batch).
//     Pulse uses RegistryScopePulse, reflection uses RegistryScopeReflection.
//
// Each non-Any scope declares prefixes that must NOT be registered in it.
// This prevents accidental cross-surface tool leakage (e.g. a pulse-only
// tool ending up in a user chat registry). Violations panic at Register()
// time — these are programmer errors, not runtime conditions.
type RegistryScope int

const (
	// RegistryScopeAny is the default scope with no restrictions. It exists
	// for backwards compatibility with NewRegistry() callers and tests that
	// don't care about surface isolation.
	RegistryScopeAny RegistryScope = iota
	// RegistryScopeUser is the user-facing surface (chat, agent).
	RegistryScopeUser
	// RegistryScopePulse is the pulse runtime surface.
	RegistryScopePulse
	// RegistryScopeReflection is the reflection runtime surface.
	RegistryScopeReflection
)

// String returns a human-readable scope name used in panic messages.
func (s RegistryScope) String() string {
	switch s {
	case RegistryScopeAny:
		return "any"
	case RegistryScopeUser:
		return "user"
	case RegistryScopePulse:
		return "pulse"
	case RegistryScopeReflection:
		return "reflection"
	default:
		return fmt.Sprintf("scope(%d)", int(s))
	}
}

// registryForbiddenPrefixes lists tool-name prefixes that are disallowed in
// each scope. Any tool whose Name starts with a forbidden prefix triggers a
// panic on Register(). An empty or missing entry means no restrictions.
//
// The "ops_" prefix is omitted from the user scope intentionally during the
// heartbeat→pulse migration: ops_* tool wrappers still exist in this commit
// and will be removed in a later commit of the same PR. Once removed, the
// "ops_" prefix will be added here to prevent regressions.
var registryForbiddenPrefixes = map[RegistryScope][]string{
	RegistryScopeUser:       {"pulse_", "reflection_"},
	RegistryScopePulse:      {"ops_", "reflection_"},
	RegistryScopeReflection: {"ops_", "pulse_"},
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Result struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"is_error,omitempty"`
}

func (r Result) Text() string {
	if len(r.Content) == 0 {
		return ""
	}
	out := ""
	for i, block := range r.Content {
		if block.Text == "" {
			continue
		}
		if out != "" && i > 0 {
			out += "\n"
		}
		out += block.Text
	}
	return out
}

type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	Execute     func(ctx context.Context, params json.RawMessage) (Result, error)
}

type Registry struct {
	mu    sync.RWMutex
	scope RegistryScope
	tools map[string]Tool
}

// NewRegistry creates a registry with RegistryScopeAny (no isolation).
// Prefer NewRegistryWithScope for user/pulse/reflection surfaces.
func NewRegistry() *Registry {
	return NewRegistryWithScope(RegistryScopeAny)
}

// NewRegistryWithScope creates a registry bound to the given scope. Any
// Register() call whose tool name starts with a forbidden prefix for that
// scope will panic — this is a compile-time-style guarantee enforced at
// wiring time to prevent cross-surface tool leakage.
func NewRegistryWithScope(scope RegistryScope) *Registry {
	return &Registry{
		scope: scope,
		tools: map[string]Tool{},
	}
}

// Scope returns the scope this registry was created with.
func (r *Registry) Scope() RegistryScope {
	return r.scope
}

func (r *Registry) Register(t Tool) {
	for _, p := range registryForbiddenPrefixes[r.scope] {
		if strings.HasPrefix(t.Name, p) {
			panic(fmt.Sprintf(
				"tool %q cannot be registered in %s scope (forbidden prefix %q)",
				t.Name, r.scope, p,
			))
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}

func (r *Registry) Schemas() []llm.ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.tools) == 0 {
		return nil
	}

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	schemas := make([]llm.ToolSchema, 0, len(names))
	for _, name := range names {
		t := r.tools[name]
		schemas = append(schemas, llm.ToolSchema{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return schemas
}

func (r *Registry) SchemasForNames(names []string) []llm.ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.tools) == 0 || len(names) == 0 {
		return nil
	}

	schemas := make([]llm.ToolSchema, 0, len(names))
	seen := map[string]struct{}{}
	for _, name := range names {
		key := strings.TrimSpace(name)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		t, ok := r.tools[key]
		if !ok {
			continue
		}
		schemas = append(schemas, llm.ToolSchema{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return schemas
}
