package hermes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/skillhub"
)

const sampleAuthSkill = `---
name: github-auth
description: Set up GitHub authentication.
version: 1.1.0
author: Hermes Agent
license: MIT
metadata:
  hermes:
    tags: [GitHub, Authentication, Git]
    related_skills: [github-pr-workflow, github-code-review]
---

# GitHub Authentication body
`

const samplePRSkill = `---
name: github-pr-workflow
description: Full pull request lifecycle.
version: 1.1.0
author: Hermes Agent
license: MIT
metadata:
  hermes:
    tags: [GitHub, Pull-Requests, CI/CD]
    related_skills: [github-auth]
---

# PR workflow body
`

func newTestSource(t *testing.T) (*Source, *httptest.Server, *httptest.Server) {
	t.Helper()
	tree := treesResponse{
		Tree: []treeNode{
			{Path: "skills/github/github-auth/SKILL.md", Type: "blob", Size: int64(len(sampleAuthSkill))},
			{Path: "skills/github/github-auth/references/notes.md", Type: "blob", Size: 12},
			{Path: "skills/github/github-pr-workflow/SKILL.md", Type: "blob", Size: int64(len(samplePRSkill))},
			{Path: "skills/github/github-pr-workflow/templates/pr-body.md", Type: "blob", Size: 20},
			{Path: "skills/github/DESCRIPTION.md", Type: "blob", Size: 10}, // not a skill
		},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/NousResearch/hermes-agent/git/trees/main") {
			_ = json.NewEncoder(w).Encode(tree)
			return
		}
		http.NotFound(w, r)
	}))

	rawSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/NousResearch/hermes-agent/main/skills/github/github-auth/SKILL.md":
			_, _ = w.Write([]byte(sampleAuthSkill))
		case "/NousResearch/hermes-agent/main/skills/github/github-pr-workflow/SKILL.md":
			_, _ = w.Write([]byte(samplePRSkill))
		case "/NousResearch/hermes-agent/main/skills/github/github-auth/references/notes.md":
			_, _ = w.Write([]byte("notes"))
		case "/NousResearch/hermes-agent/main/skills/github/github-pr-workflow/templates/pr-body.md":
			_, _ = w.Write([]byte("template"))
		case "/NousResearch/hermes-agent/main/LICENSE":
			_, _ = w.Write([]byte("MIT License\n\nCopyright (c) 2025 NousResearch\n\nPermission is hereby granted, free of charge"))
		default:
			http.NotFound(w, r)
		}
	}))

	src := New()
	src.APIBaseURL = apiSrv.URL
	src.RawBaseURL = rawSrv.URL
	src.TokenEnv = "HERMES_TEST_UNSET"
	return src, apiSrv, rawSrv
}

func TestHermesSource_SearchSkills(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()

	results, err := src.SearchSkills(context.Background(), "")
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 skills, got %d (%v)", len(results), results)
	}
}

func TestHermesSource_FindSkillByName(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()

	entry, err := src.FindSkillByName(context.Background(), "github-auth")
	if err != nil {
		t.Fatalf("FindSkillByName: %v", err)
	}
	if entry.Path != "skills/github/github-auth" {
		t.Errorf("Path = %q", entry.Path)
	}
	if entry.Version != "1.1.0" {
		t.Errorf("Version = %q", entry.Version)
	}
	if entry.Author != "Hermes Agent" {
		t.Errorf("Author = %q", entry.Author)
	}
	if len(entry.Tags) == 0 {
		t.Errorf("Tags should be populated from metadata.hermes.tags")
	}
}

func TestHermesSource_FindSkillByName_NotFound(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()

	if _, err := src.FindSkillByName(context.Background(), "missing"); err == nil {
		t.Fatalf("expected not-found error")
	}
}

func TestHermesSource_ListCompanionFiles(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()

	entry := &skillhub.RegistryEntry{Name: "github-pr-workflow", Path: "skills/github/github-pr-workflow"}
	files, err := src.ListCompanionFiles(context.Background(), entry)
	if err != nil {
		t.Fatalf("ListCompanionFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "templates/pr-body.md" {
		t.Errorf("files = %v, want [templates/pr-body.md]", files)
	}
}

func TestHermesSource_FetchLicense(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()

	body, label, err := src.FetchLicense(context.Background(), nil)
	if err != nil {
		t.Fatalf("FetchLicense: %v", err)
	}
	if label != skillhub.LicenseMIT {
		t.Errorf("label = %q, want MIT", label)
	}
	if !strings.Contains(string(body), "MIT License") {
		t.Errorf("license body missing MIT marker")
	}
}

func TestHermesSource_ConvertSkillContent(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()

	entry := &skillhub.RegistryEntry{Name: "github-auth", Path: "skills/github/github-auth"}
	raw1, err := src.FetchSkillContent(context.Background(), entry)
	if err != nil {
		t.Fatalf("FetchSkillContent: %v", err)
	}
	converted, warnings, err := src.ConvertSkillContent(entry, raw1)
	if err != nil {
		t.Fatalf("ConvertSkillContent: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("hermes should not emit install warnings, got %v", warnings)
	}
	if !strings.Contains(string(converted), "name: github-auth") {
		t.Errorf("converted missing name field: %s", converted)
	}
	if !strings.Contains(string(converted), "adapter_origin") {
		t.Errorf("converted missing adapter_origin metadata")
	}
}

func TestHermesSource_ConvertSkillContent_NilEntry(t *testing.T) {
	src := New()
	if _, _, err := src.ConvertSkillContent(nil, []byte("x")); err == nil {
		t.Errorf("expected error for nil entry")
	}
}

func TestHermesSource_FetchSkillFile_RejectsTraversal(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()
	entry := &skillhub.RegistryEntry{Path: "skills/github/github-auth"}
	if _, err := src.FetchSkillFile(context.Background(), entry, "../../etc/passwd"); err == nil {
		t.Errorf("expected traversal rejection")
	}
}

func TestHermesSource_ID_AndDefaults(t *testing.T) {
	s := New()
	if s.ID() != SourceID {
		t.Errorf("ID = %q", s.ID())
	}
	if s.Owner != DefaultRepoOwner {
		t.Errorf("Owner = %q", s.Owner)
	}
	if s.apiBase() == "" || s.rawBase() == "" {
		t.Errorf("apiBase/rawBase should have defaults")
	}
	_ = s.token()
}

func TestHermesParseFrontmatter_NoDelimiter(t *testing.T) {
	if _, err := ParseFrontmatter([]byte("no frontmatter here")); err == nil {
		t.Errorf("expected error for missing delimiter")
	}
}

func TestHermesSource_SearchSkills_FilterMisses(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()

	results, err := src.SearchSkills(context.Background(), "no-such-thing-anywhere")
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected zero results for unmatched query, got %v", results)
	}
}

func TestHermesSource_FindSkillByName_AmbiguousAcrossCategories(t *testing.T) {
	tree := treesResponse{
		Tree: []treeNode{
			{Path: "skills/devops/foo/SKILL.md", Type: "blob", Size: 50},
			{Path: "skills/data-science/foo/SKILL.md", Type: "blob", Size: 50},
		},
	}
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/NousResearch/hermes-agent/git/trees/main") {
			_ = json.NewEncoder(w).Encode(tree)
			return
		}
		http.NotFound(w, r)
	}))
	defer apiSrv.Close()
	src := New()
	src.APIBaseURL = apiSrv.URL
	src.TokenEnv = "HERMES_TEST_UNSET"

	_, err := src.FindSkillByName(context.Background(), "foo")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous error, got %v", err)
	}
}

func TestHermesSource_SearchSkills_IndexError(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer apiSrv.Close()
	src := New()
	src.APIBaseURL = apiSrv.URL
	src.TokenEnv = "HERMES_TEST_UNSET"

	if _, err := src.SearchSkills(context.Background(), ""); err == nil {
		t.Errorf("expected error when Trees API fails")
	}
}

func TestHermesSource_FetchLicense_NotFound(t *testing.T) {
	rawSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer rawSrv.Close()
	src := New()
	src.RawBaseURL = rawSrv.URL
	src.TokenEnv = "HERMES_TEST_UNSET"

	if _, _, err := src.FetchLicense(context.Background(), nil); err == nil {
		t.Errorf("expected error on LICENSE 404")
	}
}

func TestHermesSource_FetchSkillFile_NilEntry(t *testing.T) {
	if _, err := New().FetchSkillFile(context.Background(), nil, "foo"); err == nil {
		t.Errorf("expected error for nil entry")
	}
}

func TestHermesSource_ListCompanionFiles_NilEntry(t *testing.T) {
	if _, err := New().ListCompanionFiles(context.Background(), nil); err == nil {
		t.Errorf("expected error for nil entry")
	}
}
