package skillhub

import (
	"context"
	"strings"
	"testing"
)

func TestSearchAllSkills_AllSources(t *testing.T) {
	inst := &Installer{
		WorkspaceDir: t.TempDir(),
		Sources: func() *SourceRegistry {
			r := NewSourceRegistry()
			_ = r.Register(&stubSource{id: "a", entries: map[string]RegistryEntry{
				"foo": {Name: "foo", Path: "skills/foo"},
			}})
			_ = r.Register(&stubSource{id: "b", entries: map[string]RegistryEntry{
				"bar": {Name: "bar", Path: "skills/bar"},
			}})
			return r
		}(),
	}

	results, err := inst.SearchAllSkills(context.Background(), "", "")
	if err != nil {
		t.Fatalf("SearchAllSkills: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].SourceID != "a" || results[1].SourceID != "b" {
		t.Errorf("expected sources [a, b], got [%s, %s]", results[0].SourceID, results[1].SourceID)
	}
}

func TestSearchAllSkills_FromFilter(t *testing.T) {
	inst := &Installer{
		WorkspaceDir: t.TempDir(),
		Sources: func() *SourceRegistry {
			r := NewSourceRegistry()
			_ = r.Register(&stubSource{id: "a", entries: map[string]RegistryEntry{
				"foo": {Name: "foo"},
			}})
			_ = r.Register(&stubSource{id: "b", entries: map[string]RegistryEntry{
				"bar": {Name: "bar"},
			}})
			return r
		}(),
	}

	results, err := inst.SearchAllSkills(context.Background(), "", "b")
	if err != nil {
		t.Fatalf("SearchAllSkills: %v", err)
	}
	if len(results) != 1 || results[0].Entry.Name != "bar" {
		t.Fatalf("expected only bar, got %+v", results)
	}
}

func TestSearchAllSkills_UnknownFrom(t *testing.T) {
	inst := &Installer{
		WorkspaceDir: t.TempDir(),
		Sources:      NewSourceRegistry(),
	}
	_ = inst.Sources.Register(&stubSource{id: "a"})

	_, err := inst.SearchAllSkills(context.Background(), "", "nonexistent")
	if err == nil {
		t.Fatalf("expected error for unknown source")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error %q missing source name", err)
	}
}

func TestSearchAllSkills_DowngradesSourceError(t *testing.T) {
	// A source that returns an error from SearchSkills must not blank the
	// overall result: the next source's entries still show up.
	good := &stubSource{id: "good", entries: map[string]RegistryEntry{
		"foo": {Name: "foo"},
	}}
	bad := &errorSearchStub{stubSource: stubSource{id: "bad"}}

	inst := &Installer{
		WorkspaceDir: t.TempDir(),
		Sources: func() *SourceRegistry {
			r := NewSourceRegistry()
			_ = r.Register(bad)
			_ = r.Register(good)
			return r
		}(),
	}

	results, err := inst.SearchAllSkills(context.Background(), "", "")
	if err != nil {
		t.Fatalf("SearchAllSkills: %v", err)
	}
	if len(results) != 1 || results[0].SourceID != "good" {
		t.Fatalf("expected only good's result, got %+v", results)
	}
}

func TestLookupSkill(t *testing.T) {
	inst := &Installer{
		WorkspaceDir: t.TempDir(),
		Sources: func() *SourceRegistry {
			r := NewSourceRegistry()
			_ = r.Register(&stubSource{id: "a", entries: map[string]RegistryEntry{
				"foo": {Name: "foo", Version: "1.0.0"},
			}})
			return r
		}(),
	}

	entry, sourceID, err := inst.LookupSkill(context.Background(), "a:foo")
	if err != nil {
		t.Fatalf("LookupSkill: %v", err)
	}
	if sourceID != "a" {
		t.Errorf("sourceID = %q, want a", sourceID)
	}
	if entry.Name != "foo" {
		t.Errorf("entry.Name = %q", entry.Name)
	}
}

func TestLookupSkill_NotFound(t *testing.T) {
	inst := &Installer{
		WorkspaceDir: t.TempDir(),
		Sources: func() *SourceRegistry {
			r := NewSourceRegistry()
			_ = r.Register(&stubSource{id: "a"})
			return r
		}(),
	}

	if _, _, err := inst.LookupSkill(context.Background(), "a:missing"); err == nil {
		t.Fatalf("expected error for missing skill")
	}
}

// errorSearchStub forces SearchSkills to fail so we can exercise the
// downgrade-to-empty branch of collectFromSource.
type errorSearchStub struct {
	stubSource
}

func (e *errorSearchStub) SearchSkills(_ context.Context, _ string) ([]RegistryEntry, error) {
	return nil, ErrInstallAborted // any error will do
}
