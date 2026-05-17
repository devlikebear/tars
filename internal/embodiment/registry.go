package embodiment

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Registry struct {
	mu          sync.RWMutex
	descriptors map[string]ProviderDescriptor
}

func NewRegistry() *Registry {
	return &Registry{
		descriptors: map[string]ProviderDescriptor{},
	}
}

func (r *Registry) Register(desc ProviderDescriptor) error {
	if r == nil {
		return fmt.Errorf("embodiment registry is nil")
	}
	normalized := normalizeDescriptor(desc)
	if normalized.Name == "" {
		return fmt.Errorf("embodiment provider name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.descriptors[normalized.Name]; exists {
		return fmt.Errorf("embodiment provider %q already registered", normalized.Name)
	}
	r.descriptors[normalized.Name] = normalized
	return nil
}

func (r *Registry) Get(name string) (ProviderDescriptor, bool) {
	if r == nil {
		return ProviderDescriptor{}, false
	}
	key := normalizeName(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	desc, ok := r.descriptors[key]
	if !ok {
		return ProviderDescriptor{}, false
	}
	return cloneDescriptor(desc), true
}

func (r *Registry) Enabled() []ProviderDescriptor {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ProviderDescriptor, 0, len(r.descriptors))
	for _, desc := range r.descriptors {
		if desc.Enabled {
			out = append(out, cloneDescriptor(desc))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func (r *Registry) CapabilitiesOf(name string) []Capability {
	desc, ok := r.Get(name)
	if !ok {
		return nil
	}
	return append([]Capability(nil), desc.Capabilities...)
}

func (r *Registry) Empty() bool {
	return len(r.Enabled()) == 0
}

func normalizeDescriptor(desc ProviderDescriptor) ProviderDescriptor {
	desc.Name = normalizeName(desc.Name)
	desc.Transport = Transport(strings.TrimSpace(strings.ToLower(string(desc.Transport))))
	desc.Endpoint = strings.TrimSpace(desc.Endpoint)
	desc.Capabilities = normalizeCapabilities(desc.Capabilities)
	return desc
}

func normalizeName(name string) string {
	return strings.TrimSpace(strings.ToLower(name))
}

func normalizeCapabilities(values []Capability) []Capability {
	out := make([]Capability, 0, len(values))
	seen := map[Capability]struct{}{}
	for _, value := range values {
		normalized := Capability(strings.TrimSpace(strings.ToLower(string(value))))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func cloneDescriptor(desc ProviderDescriptor) ProviderDescriptor {
	desc.Capabilities = append([]Capability(nil), desc.Capabilities...)
	return desc
}
