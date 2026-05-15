package tarsserver

import (
	"testing"

	"github.com/devlikebear/tars/internal/skill"
)

func TestToClaudeCodeSkills_FiltersAndMaps(t *testing.T) {
	got := toClaudeCodeSkills([]skill.Definition{
		{Name: "github-flow", Description: "release flow", Content: "Do the flow."},
		{Name: "  ", Description: "blank name", Content: "x"},     // dropped: empty name
		{Name: "no-body", Description: "has no content"},          // dropped: empty content
		{Name: "  trimmed  ", Description: "  desc  ", Content: "body"},
	})
	if len(got) != 2 {
		names := make([]string, 0, len(got))
		for _, s := range got {
			names = append(names, s.Name)
		}
		t.Fatalf("expected 2 skills, got %d: %v", len(got), names)
	}
	if got[0].Name != "github-flow" || got[0].Description != "release flow" || got[0].Content != "Do the flow." {
		t.Fatalf("unexpected first skill: %+v", got[0])
	}
	if got[1].Name != "trimmed" || got[1].Description != "desc" {
		t.Fatalf("name/description should be trimmed: %+v", got[1])
	}
}

func TestToClaudeCodeSkills_NilWhenEmptyOrAllFiltered(t *testing.T) {
	if got := toClaudeCodeSkills(nil); got != nil {
		t.Fatalf("nil input → nil, got %+v", got)
	}
	if got := toClaudeCodeSkills([]skill.Definition{}); got != nil {
		t.Fatalf("empty input → nil, got %+v", got)
	}
	if got := toClaudeCodeSkills([]skill.Definition{
		{Name: "", Content: "x"},
		{Name: "y", Content: ""},
	}); got != nil {
		t.Fatalf("all-filtered → nil, got %+v", got)
	}
}
