package hermes

import (
	"strings"
	"testing"
)

func TestParseFrontmatter_FullSample(t *testing.T) {
	fm, err := ParseFrontmatter([]byte(sampleAuthSkill))
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if fm.Name != "github-auth" {
		t.Errorf("Name = %q", fm.Name)
	}
	if fm.Version != "1.1.0" || fm.Author != "Hermes Agent" || fm.License != "MIT" {
		t.Errorf("version/author/license mismatch: %+v", fm)
	}
	if len(fm.Tags) != 3 {
		t.Errorf("expected 3 tags, got %v", fm.Tags)
	}
	if len(fm.RelatedSkills) != 2 {
		t.Errorf("expected 2 related_skills, got %v", fm.RelatedSkills)
	}
}

func TestParseFrontmatter_TrailingDashes(t *testing.T) {
	in := "---\nname: foo\ndescription: bar\n---"
	fm, err := ParseFrontmatter([]byte(in))
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if fm.Name != "foo" {
		t.Errorf("Name = %q", fm.Name)
	}
}

func TestParseFrontmatter_UnterminatedBlock(t *testing.T) {
	if _, err := ParseFrontmatter([]byte("---\nname: foo\nbody without closing")); err == nil {
		t.Errorf("expected error")
	}
}

func TestHermesMeta_NilAndMissing(t *testing.T) {
	if hermesMeta(nil) != nil {
		t.Errorf("nil input should return nil")
	}
	if hermesMeta(map[string]any{}) != nil {
		t.Errorf("missing 'hermes' key should return nil")
	}
}

func TestHermesMeta_MapAnyAnyShape(t *testing.T) {
	in := map[string]any{
		"hermes": map[any]any{
			"tags":  []any{"a"},
			77:      "skipped non-string key",
			"":      "skipped empty key",
		},
	}
	got := hermesMeta(in)
	if got == nil {
		t.Fatalf("expected normalized map, got nil")
	}
	if _, has := got["tags"]; !has {
		t.Errorf("tags key dropped during normalization: %v", got)
	}
}

func TestHermesMeta_NonMapValue(t *testing.T) {
	in := map[string]any{"hermes": "not-a-map"}
	if hermesMeta(in) != nil {
		t.Errorf("non-map value should yield nil")
	}
}

func TestStringSlice_Defensive(t *testing.T) {
	if stringSlice(nil) != nil {
		t.Errorf("nil should return nil")
	}
	if stringSlice("not-a-slice") != nil {
		t.Errorf("non-slice should return nil")
	}
	got := stringSlice([]any{"a", 42, " b ", ""})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("stringSlice = %v, want [a b]", got)
	}
}

func TestCloneHermesMeta_PassesThroughExtras(t *testing.T) {
	fm := Frontmatter{
		Tags:          []string{"x"},
		RelatedSkills: []string{"y"},
		RawMetadata: map[string]any{
			"hermes": map[string]any{
				"tags":           []any{"x"},
				"related_skills": []any{"y"},
				"custom_field":   "kept",
			},
		},
	}
	got := cloneHermesMeta(fm)
	if got["custom_field"] != "kept" {
		t.Errorf("expected custom_field passthrough, got %v", got)
	}
}

func TestRewriteSkillMD_HermesRoundtrip(t *testing.T) {
	res, err := RewriteSkillMD(RewriteInput{Raw: []byte(sampleAuthSkill)})
	if err != nil {
		t.Fatalf("RewriteSkillMD: %v", err)
	}
	if !strings.Contains(string(res.Converted), "name: github-auth") {
		t.Errorf("missing name field: %s", res.Converted)
	}
	if !strings.Contains(string(res.Converted), "adapter_origin") {
		t.Errorf("missing adapter_origin metadata")
	}
}

func TestRewriteSkillMD_RejectsMissingFields(t *testing.T) {
	missingName := "---\ndescription: hi\n---\nbody"
	if _, err := RewriteSkillMD(RewriteInput{Raw: []byte(missingName)}); err == nil {
		t.Errorf("expected missing-name error")
	}
	missingDesc := "---\nname: foo\n---\nbody"
	if _, err := RewriteSkillMD(RewriteInput{Raw: []byte(missingDesc)}); err == nil {
		t.Errorf("expected missing-description error")
	}
}
