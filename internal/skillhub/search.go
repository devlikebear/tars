package skillhub

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// SkillSearchResult is one item produced by Installer.SearchAllSkills. The
// SourceID field lets the CLI render the hub a result came from.
type SkillSearchResult struct {
	SourceID string
	Entry    RegistryEntry
}

// SearchAllSkills queries every registered HubSource (or a single one, if
// fromID is non-empty) and returns the combined result set. The CLI uses
// this for `tars skill search [--from <hub>] [query]`.
//
// A source-level error is downgraded to a per-source skip so a transient
// network failure on one hub does not blank the whole listing.
func (inst *Installer) SearchAllSkills(ctx context.Context, query, fromID string) ([]SkillSearchResult, error) {
	sources := inst.ensureSources()
	if id := strings.TrimSpace(fromID); id != "" {
		src, ok := sources.Get(id)
		if !ok {
			return nil, fmt.Errorf("hub source %q is not registered (known: %s)", id, strings.Join(sources.IDs(), ", "))
		}
		return collectFromSource(ctx, src, query), nil
	}
	var out []SkillSearchResult
	for _, src := range sources.List() {
		out = append(out, collectFromSource(ctx, src, query)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SourceID != out[j].SourceID {
			return out[i].SourceID < out[j].SourceID
		}
		return strings.ToLower(out[i].Entry.Name) < strings.ToLower(out[j].Entry.Name)
	})
	return out, nil
}

func collectFromSource(ctx context.Context, src HubSource, query string) []SkillSearchResult {
	entries, err := src.SearchSkills(ctx, query)
	if err != nil {
		return nil
	}
	out := make([]SkillSearchResult, 0, len(entries))
	for _, e := range entries {
		out = append(out, SkillSearchResult{SourceID: src.ID(), Entry: e})
	}
	return out
}

// LookupSkill resolves a "<source>:<name>" or bare-name ref through the
// SourceRegistry and returns the entry plus the source it came from. Used
// by `tars skill info` so the output can show the hub of origin.
func (inst *Installer) LookupSkill(ctx context.Context, ref string) (*RegistryEntry, string, error) {
	sourceID, bareName := ResolveSkillRef(ref)
	src, entry, err := inst.resolveSkillSource(ctx, sourceID, bareName)
	if err != nil {
		return nil, "", err
	}
	return entry, src.ID(), nil
}
