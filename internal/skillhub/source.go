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

// SkillContentConverter is implemented by external hub adapters whose
// SKILL.md frontmatter must be rewritten before TARS can load the skill.
// Built-in tars-hub does not implement this — the Installer treats a
// missing implementation as identity (raw == converted, no warnings).
type SkillContentConverter interface {
	// ConvertSkillContent takes the raw SKILL.md bytes for an entry and
	// returns the converted bytes (in TARS frontmatter form) plus any
	// human-readable adapter warnings to surface to the user.
	ConvertSkillContent(entry *RegistryEntry, raw []byte) (converted []byte, warnings []string, err error)
}

// LicenseFetcher is implemented by hubs that need to emit an ATTRIBUTION.md
// alongside the skill (i.e. external MIT/Apache-2.0 hubs). It returns the
// canonical license body for the imported skill plus a SPDX-like label
// ("MIT", "Apache-2.0", "Proprietary", "Unknown"). Built-in tars-hub does
// not implement this — TARS owns those skills.
type LicenseFetcher interface {
	// FetchLicense returns the license body and label for the given entry.
	// Implementations may return a Proprietary label to make the installer
	// refuse the import (see the federation mapping doc).
	FetchLicense(ctx context.Context, entry *RegistryEntry) (body []byte, label string, err error)
}

// CompanionFileLister is implemented by hubs whose registry entries do not
// pre-declare every companion file. The adapter discovers them at install
// time (directory listing) and returns relative paths the installer should
// download via FetchSkillFile. Built-in tars-hub relies on RegistryEntry.Files
// instead and does not implement this.
type CompanionFileLister interface {
	// ListCompanionFiles returns the relative paths (relative to the
	// skill's directory in the source repo) of every non-SKILL.md file
	// that should be materialized alongside the skill.
	ListCompanionFiles(ctx context.Context, entry *RegistryEntry) ([]string, error)
}
