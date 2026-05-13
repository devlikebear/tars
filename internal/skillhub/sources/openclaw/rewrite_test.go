package openclaw

import (
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/skill"
	"github.com/devlikebear/tars/internal/skillhub"
)

func TestRewriteSkillMD_GithubRoundtrip(t *testing.T) {
	entry := &skillhub.RegistryEntry{Name: "github", Path: "skills/github"}
	res, err := RewriteSkillMD(RewriteInput{
		Raw:        []byte(sampleGithubSkill),
		Entry:      entry,
		OriginURL:  "https://github.com/steipete/openclaw/blob/abc/skills/github/SKILL.md",
		CommitSHA:  "abc1234",
		ImportedAt: time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RewriteSkillMD: %v", err)
	}
	if len(res.Warnings) != 2 {
		t.Errorf("warnings = %d, want 2", len(res.Warnings))
	}

	fm, body, err := skill.ParseFrontmatter(string(res.Converted))
	if err != nil {
		t.Fatalf("skill.ParseFrontmatter on converted output: %v", err)
	}
	if fm.Name != "github" {
		t.Errorf("converted Name = %q, want github", fm.Name)
	}
	if fm.UserInvocable == nil || !*fm.UserInvocable {
		t.Errorf("converted user_invocable should be true, got %v", fm.UserInvocable)
	}
	if len(fm.RequiresBins) != 1 || fm.RequiresBins[0] != "gh" {
		t.Errorf("converted requires_bins = %v, want [gh]", fm.RequiresBins)
	}
	if !strings.Contains(body, "# GitHub Skill") {
		t.Errorf("body missing header: %q", body)
	}
}

func TestRewriteSkillMD_RejectsMissingFields(t *testing.T) {
	missingName := `---
description: foo
---
body
`
	_, err := RewriteSkillMD(RewriteInput{
		Raw:   []byte(missingName),
		Entry: &skillhub.RegistryEntry{Name: "x"},
	})
	if err == nil {
		t.Fatalf("expected error for missing name")
	}
}

func TestRewriteSkillMD_PreservesOrigin(t *testing.T) {
	res, err := RewriteSkillMD(RewriteInput{
		Raw:        []byte(sampleSimpleSkill),
		Entry:      &skillhub.RegistryEntry{Name: "simple-skill", Path: "skills/simple-skill"},
		OriginURL:  "https://example/foo",
		CommitSHA:  "deadbeef",
		ImportedAt: time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RewriteSkillMD: %v", err)
	}
	if !strings.Contains(string(res.Converted), "adapter_origin") {
		t.Errorf("converted output missing adapter_origin: %s", res.Converted)
	}
	if !strings.Contains(string(res.Converted), "deadbeef") {
		t.Errorf("converted output missing commit_sha: %s", res.Converted)
	}
}
