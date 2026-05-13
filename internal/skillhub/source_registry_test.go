package skillhub

import (
	"context"
	"errors"
	"testing"
)

type stubSource struct {
	id      string
	entries map[string]RegistryEntry
}

func (s *stubSource) ID() string { return s.id }
func (s *stubSource) SearchSkills(_ context.Context, _ string) ([]RegistryEntry, error) {
	out := make([]RegistryEntry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e)
	}
	return out, nil
}
func (s *stubSource) FindSkillByName(_ context.Context, name string) (*RegistryEntry, error) {
	if e, ok := s.entries[name]; ok {
		return &e, nil
	}
	return nil, errors.New("not found")
}
func (s *stubSource) FetchSkillContent(_ context.Context, _ *RegistryEntry) ([]byte, error) {
	return nil, errors.New("stub")
}
func (s *stubSource) FetchSkillFile(_ context.Context, _ *RegistryEntry, _ string) ([]byte, error) {
	return nil, errors.New("stub")
}

func TestSourceRegistry_RegisterAndGet(t *testing.T) {
	reg := NewSourceRegistry()
	a := &stubSource{id: "tars-hub"}
	b := &stubSource{id: "openclaw"}

	if err := reg.Register(a); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if err := reg.Register(b); err != nil {
		t.Fatalf("register b: %v", err)
	}

	got, ok := reg.Get("openclaw")
	if !ok || got != b {
		t.Fatalf("Get(openclaw) returned (%v, %v), want stub b", got, ok)
	}

	// Case-insensitive lookup.
	if _, ok := reg.Get("TARS-HUB"); !ok {
		t.Fatalf("Get is not case-insensitive")
	}

	// Insertion order preserved.
	if ids := reg.IDs(); len(ids) != 2 || ids[0] != "tars-hub" || ids[1] != "openclaw" {
		t.Fatalf("IDs order = %v, want [tars-hub openclaw]", ids)
	}

	if reg.Len() != 2 {
		t.Fatalf("Len = %d, want 2", reg.Len())
	}
}

func TestSourceRegistry_RegisterRejectsDuplicates(t *testing.T) {
	reg := NewSourceRegistry()
	if err := reg.Register(&stubSource{id: "tars-hub"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := reg.Register(&stubSource{id: "TARS-HUB"}); err == nil {
		t.Fatalf("expected duplicate register to fail")
	}
}

func TestSourceRegistry_RegisterRejectsEmpty(t *testing.T) {
	reg := NewSourceRegistry()
	if err := reg.Register(&stubSource{id: ""}); err == nil {
		t.Fatalf("expected empty ID register to fail")
	}
	if err := reg.Register(nil); err == nil {
		t.Fatalf("expected nil source register to fail")
	}
}

func TestResolveSkillRef(t *testing.T) {
	tests := []struct {
		ref      string
		wantSrc  string
		wantName string
	}{
		{"foo", "", "foo"},
		{"openclaw:foo", "openclaw", "foo"},
		{"  openclaw:foo  ", "openclaw", "foo"},
		{"  openclaw  :  foo  ", "openclaw", "foo"},
		{"openclaw:", "openclaw", ""},
		{":foo", "", "foo"},
		{"", "", ""},
		{"OPENCLAW:foo", "openclaw", "foo"},
	}
	for _, tt := range tests {
		gotSrc, gotName := ResolveSkillRef(tt.ref)
		if gotSrc != tt.wantSrc || gotName != tt.wantName {
			t.Errorf("ResolveSkillRef(%q) = (%q, %q), want (%q, %q)",
				tt.ref, gotSrc, gotName, tt.wantSrc, tt.wantName)
		}
	}
}

func TestTarsHubSourceSatisfiesHubSource(t *testing.T) {
	// Compile-time assertion is in source_tarshub.go; this test is a
	// runtime smoke that NewTarsHubSource returns a usable value.
	src := NewTarsHubSource()
	if src.ID() != DefaultSourceID {
		t.Fatalf("ID = %q, want %q", src.ID(), DefaultSourceID)
	}
	if src.Registry == nil {
		t.Fatalf("Registry is nil")
	}
}
