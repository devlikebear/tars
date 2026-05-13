package skillhub

import "context"

// HubSource is one place TARS can fetch skill packages from.
// The built-in tars-hub implementation is TarsHubSource. External
// hubs (openclaw, hermes, anthropic) live in internal/skillhub/sources/.
type HubSource interface {
	// ID returns the stable identifier used in `--from` flags and the
	// InstalledSkill.Source field (e.g. "tars-hub", "openclaw").
	ID() string

	// SearchSkills returns skill entries matching the query. Empty query
	// returns every available skill from this source.
	SearchSkills(ctx context.Context, query string) ([]RegistryEntry, error)

	// FindSkillByName returns an exact-match entry, or an error if the
	// source does not advertise a skill with the given name.
	FindSkillByName(ctx context.Context, name string) (*RegistryEntry, error)

	// FetchSkillContent returns the raw SKILL.md bytes for the entry.
	FetchSkillContent(ctx context.Context, entry *RegistryEntry) ([]byte, error)

	// FetchSkillFile returns a companion file relative to the entry's path.
	FetchSkillFile(ctx context.Context, entry *RegistryEntry, relPath string) ([]byte, error)
}
