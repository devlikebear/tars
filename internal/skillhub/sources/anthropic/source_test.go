package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/skillhub"
)

const sampleCreatorSkill = `---
name: skill-creator
description: Create new skills and iteratively improve them.
---

# Skill Creator body
`

const sampleDocxSkill = `---
name: docx
description: Use this skill whenever the user wants to create, read, edit, or manipulate Word documents.
license: Proprietary. LICENSE.txt has complete terms
---

# DOCX body
`

func newTestSource(t *testing.T) (*Source, *httptest.Server, *httptest.Server) {
	t.Helper()

	skillsRoot := []map[string]any{
		{"name": "skill-creator", "path": "skills/skill-creator", "type": "dir"},
		{"name": "docx", "path": "skills/docx", "type": "dir"},
	}
	creatorDir := []map[string]any{
		{"name": "SKILL.md", "path": "skills/skill-creator/SKILL.md", "type": "file", "size": int64(len(sampleCreatorSkill))},
		{"name": "LICENSE.txt", "path": "skills/skill-creator/LICENSE.txt", "type": "file", "size": 100},
		{"name": "references", "path": "skills/skill-creator/references", "type": "dir"},
	}
	creatorReferences := []map[string]any{
		{"name": "guide.md", "path": "skills/skill-creator/references/guide.md", "type": "file", "size": 25},
	}
	docxDir := []map[string]any{
		{"name": "SKILL.md", "path": "skills/docx/SKILL.md", "type": "file", "size": int64(len(sampleDocxSkill))},
		{"name": "LICENSE.txt", "path": "skills/docx/LICENSE.txt", "type": "file", "size": 200},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any
		switch r.URL.Path {
		case "/repos/anthropics/skills/contents/skills":
			body = skillsRoot
		case "/repos/anthropics/skills/contents/skills/skill-creator":
			body = creatorDir
		case "/repos/anthropics/skills/contents/skills/skill-creator/references":
			body = creatorReferences
		case "/repos/anthropics/skills/contents/skills/docx":
			body = docxDir
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}))

	rawSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/anthropics/skills/main/skills/skill-creator/SKILL.md":
			_, _ = w.Write([]byte(sampleCreatorSkill))
		case "/anthropics/skills/main/skills/skill-creator/LICENSE.txt":
			_, _ = w.Write([]byte("                                 Apache License\n                           Version 2.0, January 2004\n..."))
		case "/anthropics/skills/main/skills/skill-creator/references/guide.md":
			_, _ = w.Write([]byte("guide"))
		case "/anthropics/skills/main/skills/docx/SKILL.md":
			_, _ = w.Write([]byte(sampleDocxSkill))
		case "/anthropics/skills/main/skills/docx/LICENSE.txt":
			_, _ = w.Write([]byte("© 2025 Anthropic, PBC. All rights reserved.\nLICENSE: Use of these materials..."))
		default:
			http.NotFound(w, r)
		}
	}))

	src := New()
	src.APIBaseURL = apiSrv.URL
	src.RawBaseURL = rawSrv.URL
	src.TokenEnv = "ANTHROPIC_TEST_UNSET"
	return src, apiSrv, rawSrv
}

func TestAnthropicSource_SearchSkills(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()
	results, err := src.SearchSkills(context.Background(), "")
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestAnthropicSource_FindSkillByName(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()
	entry, err := src.FindSkillByName(context.Background(), "skill-creator")
	if err != nil {
		t.Fatalf("FindSkillByName: %v", err)
	}
	if entry.Author != "Anthropic, PBC" {
		t.Errorf("Author = %q", entry.Author)
	}
}

func TestAnthropicSource_FetchLicense_Apache(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()
	entry := &skillhub.RegistryEntry{Name: "skill-creator", Path: "skills/skill-creator"}
	body, label, err := src.FetchLicense(context.Background(), entry)
	if err != nil {
		t.Fatalf("FetchLicense: %v", err)
	}
	if label != skillhub.LicenseApache2 {
		t.Errorf("label = %q, want Apache-2.0", label)
	}
	if !strings.Contains(string(body), "Apache License") {
		t.Errorf("license body missing Apache marker")
	}
}

func TestAnthropicSource_FetchLicense_Proprietary(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()
	entry := &skillhub.RegistryEntry{Name: "docx", Path: "skills/docx"}
	_, label, err := src.FetchLicense(context.Background(), entry)
	if err != nil {
		t.Fatalf("FetchLicense: %v", err)
	}
	if label != skillhub.LicenseProprietary {
		t.Errorf("label = %q, want Proprietary", label)
	}
}

func TestAnthropicSource_ListCompanionFiles_ExcludesLicense(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()
	entry := &skillhub.RegistryEntry{Name: "skill-creator", Path: "skills/skill-creator"}
	files, err := src.ListCompanionFiles(context.Background(), entry)
	if err != nil {
		t.Fatalf("ListCompanionFiles: %v", err)
	}
	for _, f := range files {
		if f == "LICENSE.txt" {
			t.Errorf("LICENSE.txt must be excluded from companion files (handled by FetchLicense)")
		}
		if f == "SKILL.md" {
			t.Errorf("SKILL.md must not appear in companion files")
		}
	}
	if len(files) != 1 || files[0] != "references/guide.md" {
		t.Errorf("companion files = %v, want [references/guide.md]", files)
	}
}

func TestAnthropicSource_ConvertSkillContent(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()
	entry := &skillhub.RegistryEntry{Name: "skill-creator", Path: "skills/skill-creator"}
	raw1, err := src.FetchSkillContent(context.Background(), entry)
	if err != nil {
		t.Fatalf("FetchSkillContent: %v", err)
	}
	converted, _, err := src.ConvertSkillContent(entry, raw1)
	if err != nil {
		t.Fatalf("ConvertSkillContent: %v", err)
	}
	if !strings.Contains(string(converted), "name: skill-creator") {
		t.Errorf("converted missing name: %s", converted)
	}
}

func TestAnthropicSource_RejectsTraversal(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()
	entry := &skillhub.RegistryEntry{Path: "skills/skill-creator"}
	if _, err := src.FetchSkillFile(context.Background(), entry, "../../etc/passwd"); err == nil {
		t.Errorf("expected traversal rejection")
	}
}

func TestAnthropicSource_ID_AndDefaults(t *testing.T) {
	s := New()
	if s.ID() != SourceID {
		t.Errorf("ID mismatch")
	}
	if s.Owner != DefaultRepoOwner {
		t.Errorf("Owner default broken")
	}
	_ = s.token()
}

func TestAnthropicParseFrontmatter_NoDelimiter(t *testing.T) {
	if _, err := ParseFrontmatter([]byte("no fm")); err == nil {
		t.Errorf("expected error")
	}
}

func TestAnthropicSource_ConvertSkillContent_NilEntry(t *testing.T) {
	if _, _, err := New().ConvertSkillContent(nil, []byte("x")); err == nil {
		t.Errorf("expected error")
	}
}

func TestAnthropicSource_FetchLicense_NilEntry(t *testing.T) {
	if _, _, err := New().FetchLicense(context.Background(), nil); err == nil {
		t.Errorf("expected error")
	}
}

func TestAnthropicSource_SearchSkills_FilterMisses(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()

	results, err := src.SearchSkills(context.Background(), "no-such-skill")
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected zero results for unmatched query, got %v", results)
	}
}

func TestAnthropicSource_SearchSkills_IndexError(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer apiSrv.Close()
	src := New()
	src.APIBaseURL = apiSrv.URL
	src.TokenEnv = "ANTHROPIC_TEST_UNSET"

	if _, err := src.SearchSkills(context.Background(), ""); err == nil {
		t.Errorf("expected error when Contents API fails")
	}
}

func TestAnthropicSource_FindSkillByName_NotFound(t *testing.T) {
	src, api, raw := newTestSource(t)
	defer api.Close()
	defer raw.Close()

	if _, err := src.FindSkillByName(context.Background(), "missing-skill"); err == nil {
		t.Errorf("expected not-found error")
	}
}

func TestAnthropicSource_FetchSkillFile_NilEntry(t *testing.T) {
	if _, err := New().FetchSkillFile(context.Background(), nil, "foo"); err == nil {
		t.Errorf("expected error")
	}
}

func TestAnthropicSource_ListCompanionFiles_NilEntry(t *testing.T) {
	if _, err := New().ListCompanionFiles(context.Background(), nil); err == nil {
		t.Errorf("expected error")
	}
}

func TestAnthropicSource_FetchLicense_404(t *testing.T) {
	rawSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer rawSrv.Close()
	src := New()
	src.RawBaseURL = rawSrv.URL
	src.TokenEnv = "ANTHROPIC_TEST_UNSET"

	entry := &skillhub.RegistryEntry{Name: "missing", Path: "skills/missing"}
	if _, _, err := src.FetchLicense(context.Background(), entry); err == nil {
		t.Errorf("expected error on LICENSE 404")
	}
}
