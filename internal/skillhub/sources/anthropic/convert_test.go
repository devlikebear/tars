package anthropic

import (
	"strings"
	"testing"
)

func TestAnthropicParseFrontmatter_Sample(t *testing.T) {
	fm, err := ParseFrontmatter([]byte(sampleCreatorSkill))
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if fm.Name != "skill-creator" {
		t.Errorf("Name = %q", fm.Name)
	}
	if fm.LicenseHint != "" {
		t.Errorf("LicenseHint should be empty for the sample, got %q", fm.LicenseHint)
	}
}

func TestAnthropicParseFrontmatter_LicenseHint(t *testing.T) {
	fm, err := ParseFrontmatter([]byte(sampleDocxSkill))
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if !strings.Contains(fm.LicenseHint, "Proprietary") {
		t.Errorf("LicenseHint should preserve hint string, got %q", fm.LicenseHint)
	}
}

func TestAnthropicParseFrontmatter_TrailingDashes(t *testing.T) {
	in := "---\nname: foo\ndescription: bar\n---"
	fm, err := ParseFrontmatter([]byte(in))
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if fm.Name != "foo" {
		t.Errorf("Name = %q", fm.Name)
	}
}

func TestAnthropicParseFrontmatter_Unterminated(t *testing.T) {
	if _, err := ParseFrontmatter([]byte("---\nname: foo\nbody without closing")); err == nil {
		t.Errorf("expected error")
	}
}

func TestAnthropicSplitBody(t *testing.T) {
	body, err := SplitBody([]byte(sampleCreatorSkill))
	if err != nil {
		t.Fatalf("SplitBody: %v", err)
	}
	if !strings.Contains(body, "# Skill Creator body") {
		t.Errorf("body missing header: %q", body)
	}
}

func TestAnthropicRewriteSkillMD_Roundtrip(t *testing.T) {
	res, err := RewriteSkillMD(RewriteInput{Raw: []byte(sampleCreatorSkill)})
	if err != nil {
		t.Fatalf("RewriteSkillMD: %v", err)
	}
	out := string(res.Converted)
	if !strings.Contains(out, "name: skill-creator") {
		t.Errorf("missing name field: %s", out)
	}
	if !strings.Contains(out, "adapter_origin") {
		t.Errorf("missing adapter_origin metadata")
	}
}

func TestAnthropicRewriteSkillMD_PreservesLicenseHint(t *testing.T) {
	res, err := RewriteSkillMD(RewriteInput{Raw: []byte(sampleDocxSkill)})
	if err != nil {
		t.Fatalf("RewriteSkillMD: %v", err)
	}
	if !strings.Contains(string(res.Converted), "license_hint") {
		t.Errorf("converted output should preserve license_hint in adapter_origin: %s", res.Converted)
	}
}

func TestAnthropicRewriteSkillMD_RejectsMissingFields(t *testing.T) {
	missing := "---\ndescription: hi\n---\nbody"
	if _, err := RewriteSkillMD(RewriteInput{Raw: []byte(missing)}); err == nil {
		t.Errorf("expected missing-name error")
	}
	missingDesc := "---\nname: x\n---\nbody"
	if _, err := RewriteSkillMD(RewriteInput{Raw: []byte(missingDesc)}); err == nil {
		t.Errorf("expected missing-description error")
	}
}
