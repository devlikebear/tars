package openclaw

import (
	"strings"
	"testing"
)

const sampleGithubSkill = `---
name: github
description: "Use gh for GitHub issues, PR status, CI/logs, comments, reviews, releases, and API queries."
metadata:
  {
    "openclaw":
      {
        "emoji": "🐙",
        "requires": { "bins": ["gh"] },
        "install":
          [
            {
              "id": "brew",
              "kind": "brew",
              "formula": "gh",
              "bins": ["gh"],
              "label": "Install GitHub CLI (brew)",
            },
            {
              "id": "apt",
              "kind": "apt",
              "package": "gh",
              "bins": ["gh"],
              "label": "Install GitHub CLI (apt)",
            },
          ],
      },
  }
---

# GitHub Skill

Body content.
`

const sampleSimpleSkill = `---
name: simple-skill
description: A skill with no metadata.
---

# Simple skill body
`

func TestParseFrontmatter_GithubSample(t *testing.T) {
	fm, err := ParseFrontmatter([]byte(sampleGithubSkill))
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if fm.Name != "github" {
		t.Errorf("Name = %q, want %q", fm.Name, "github")
	}
	if !strings.Contains(fm.Description, "Use gh") {
		t.Errorf("Description = %q", fm.Description)
	}
	if len(fm.RequiresBins) != 1 || fm.RequiresBins[0] != "gh" {
		t.Errorf("RequiresBins = %v, want [gh]", fm.RequiresBins)
	}
	if len(fm.InstallBlocks) != 2 {
		t.Fatalf("InstallBlocks = %d, want 2", len(fm.InstallBlocks))
	}
	if fm.InstallBlocks[0]["kind"] != "brew" {
		t.Errorf("first install kind = %v, want brew", fm.InstallBlocks[0]["kind"])
	}
}

func TestParseFrontmatter_Simple(t *testing.T) {
	fm, err := ParseFrontmatter([]byte(sampleSimpleSkill))
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if fm.Name != "simple-skill" {
		t.Errorf("Name = %q", fm.Name)
	}
	if len(fm.RequiresBins) != 0 {
		t.Errorf("RequiresBins should be empty, got %v", fm.RequiresBins)
	}
	if len(fm.InstallBlocks) != 0 {
		t.Errorf("InstallBlocks should be empty, got %v", fm.InstallBlocks)
	}
}

func TestParseFrontmatter_MissingDelimiter(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"no opening", "name: foo\n---\nbody\n"},
		{"no closing", "---\nname: foo\nbody without closing\n"},
		{"empty", ""},
	}
	for _, tt := range cases {
		_, err := ParseFrontmatter([]byte(tt.raw))
		if err == nil {
			t.Errorf("%s: expected error", tt.name)
		}
	}
}

func TestSummarizeInstallBlocks(t *testing.T) {
	fm, err := ParseFrontmatter([]byte(sampleGithubSkill))
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	warnings := SummarizeInstallBlocks(fm.InstallBlocks)
	if len(warnings) != 2 {
		t.Fatalf("warnings = %d, want 2", len(warnings))
	}
	if !strings.Contains(warnings[0], "brew install gh") {
		t.Errorf("brew warning = %q", warnings[0])
	}
	if !strings.Contains(warnings[1], "apt install gh") {
		t.Errorf("apt warning = %q", warnings[1])
	}
}

func TestSplitBody(t *testing.T) {
	body, err := SplitBody([]byte(sampleSimpleSkill))
	if err != nil {
		t.Fatalf("SplitBody: %v", err)
	}
	if !strings.Contains(body, "# Simple skill body") {
		t.Errorf("body missing expected content: %q", body)
	}
}
