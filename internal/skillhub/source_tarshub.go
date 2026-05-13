package skillhub

import "context"

// TarsHubSource adapts the built-in tars-skills registry to the HubSource
// interface. It is a thin wrapper around *Registry — preserving the existing
// fetch and search behaviour while letting the installer route skill
// operations through the same code path as future external hubs.
type TarsHubSource struct {
	Registry *Registry
}

// NewTarsHubSource returns a TarsHubSource backed by a default Registry.
func NewTarsHubSource() *TarsHubSource {
	return &TarsHubSource{Registry: NewRegistry()}
}

// ID returns the canonical built-in hub identifier.
func (s *TarsHubSource) ID() string { return DefaultSourceID }

// SearchSkills delegates to the underlying Registry.
func (s *TarsHubSource) SearchSkills(ctx context.Context, query string) ([]RegistryEntry, error) {
	return s.Registry.Search(ctx, query)
}

// FindSkillByName delegates to the underlying Registry.
func (s *TarsHubSource) FindSkillByName(ctx context.Context, name string) (*RegistryEntry, error) {
	return s.Registry.FindByName(ctx, name)
}

// FetchSkillContent returns the SKILL.md for the entry.
func (s *TarsHubSource) FetchSkillContent(ctx context.Context, entry *RegistryEntry) ([]byte, error) {
	return s.Registry.FetchSkillContent(ctx, entry)
}

// FetchSkillFile returns a companion file relative to the entry path.
func (s *TarsHubSource) FetchSkillFile(ctx context.Context, entry *RegistryEntry, relPath string) ([]byte, error) {
	return s.Registry.FetchFile(ctx, entry, relPath)
}

// Compile-time assertion that TarsHubSource satisfies HubSource.
var _ HubSource = (*TarsHubSource)(nil)
