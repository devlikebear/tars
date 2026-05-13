package openclaw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/skillhub"
)

// newTestSource spins up httptest servers that mimic GitHub Contents API
// and raw.githubusercontent.com for openclaw/openclaw. It serves:
//   - GET /repos/{owner}/{repo}/contents/skills          → two skill dirs
//   - GET /repos/{owner}/{repo}/contents/skills/<name>   → SKILL.md + scripts/
//   - raw /<owner>/<repo>/<branch>/skills/<name>/SKILL.md → frontmatter sample
//   - raw /<owner>/<repo>/<branch>/LICENSE                → MIT text
func newTestSource(t *testing.T) (*Source, *httptest.Server, *httptest.Server) {
	t.Helper()

	skillsRoot := []map[string]any{
		{"name": "github", "path": "skills/github", "type": "dir"},
		{"name": "simple-skill", "path": "skills/simple-skill", "type": "dir"},
	}
	githubDir := []map[string]any{
		{"name": "SKILL.md", "path": "skills/github/SKILL.md", "type": "file", "size": int64(len(sampleGithubSkill))},
		{"name": "scripts", "path": "skills/github/scripts", "type": "dir"},
	}
	githubScripts := []map[string]any{
		{"name": "install.sh", "path": "skills/github/scripts/install.sh", "type": "file", "size": int64(12)},
	}
	simpleDir := []map[string]any{
		{"name": "SKILL.md", "path": "skills/simple-skill/SKILL.md", "type": "file", "size": int64(len(sampleSimpleSkill))},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any
		switch r.URL.Path {
		case "/repos/openclaw/openclaw/contents/skills":
			body = skillsRoot
		case "/repos/openclaw/openclaw/contents/skills/github":
			body = githubDir
		case "/repos/openclaw/openclaw/contents/skills/github/scripts":
			body = githubScripts
		case "/repos/openclaw/openclaw/contents/skills/simple-skill":
			body = simpleDir
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}))

	rawSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openclaw/openclaw/main/skills/github/SKILL.md":
			_, _ = w.Write([]byte(sampleGithubSkill))
		case "/openclaw/openclaw/main/skills/github/scripts/install.sh":
			_, _ = w.Write([]byte("#!/bin/sh\n"))
		case "/openclaw/openclaw/main/skills/simple-skill/SKILL.md":
			_, _ = w.Write([]byte(sampleSimpleSkill))
		case "/openclaw/openclaw/main/LICENSE":
			_, _ = w.Write([]byte("MIT License\n\nCopyright (c) 2025 Peter Steinberger\n\nPermission is hereby granted, free of charge"))
		default:
			http.NotFound(w, r)
		}
	}))

	src := New()
	src.APIBaseURL = apiSrv.URL
	src.RawBaseURL = rawSrv.URL
	src.TokenEnv = "OPENCLAW_TEST_TOKEN_THAT_IS_UNSET"
	return src, apiSrv, rawSrv
}

func TestSource_SearchSkills_All(t *testing.T) {
	src, apiSrv, rawSrv := newTestSource(t)
	defer apiSrv.Close()
	defer rawSrv.Close()

	results, err := src.SearchSkills(context.Background(), "")
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d (%v)", len(results), results)
	}
	if results[0].Name != "github" {
		t.Errorf("first result = %q, want github", results[0].Name)
	}
}

func TestSource_SearchSkills_FilteredByQuery(t *testing.T) {
	src, apiSrv, rawSrv := newTestSource(t)
	defer apiSrv.Close()
	defer rawSrv.Close()

	results, err := src.SearchSkills(context.Background(), "github")
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(results) != 1 || results[0].Name != "github" {
		t.Fatalf("expected only github, got %+v", results)
	}
}

func TestSource_FindSkillByName(t *testing.T) {
	src, apiSrv, rawSrv := newTestSource(t)
	defer apiSrv.Close()
	defer rawSrv.Close()

	entry, err := src.FindSkillByName(context.Background(), "github")
	if err != nil {
		t.Fatalf("FindSkillByName: %v", err)
	}
	if entry.Name != "github" {
		t.Errorf("Name = %q", entry.Name)
	}
	if entry.Path != "skills/github" {
		t.Errorf("Path = %q", entry.Path)
	}
}

func TestSource_FetchSkillContent_And_ConvertSkillContent(t *testing.T) {
	src, apiSrv, rawSrv := newTestSource(t)
	defer apiSrv.Close()
	defer rawSrv.Close()

	entry := &skillhub.RegistryEntry{Name: "github", Path: "skills/github"}
	raw, err := src.FetchSkillContent(context.Background(), entry)
	if err != nil {
		t.Fatalf("FetchSkillContent: %v", err)
	}
	if !strings.HasPrefix(string(raw), "---") {
		t.Errorf("expected frontmatter, got %q...", string(raw[:20]))
	}

	converted, warnings, err := src.ConvertSkillContent(entry, raw)
	if err != nil {
		t.Fatalf("ConvertSkillContent: %v", err)
	}
	if len(warnings) != 2 {
		t.Errorf("expected 2 warnings, got %v", warnings)
	}
	if !strings.Contains(string(converted), "name: github") {
		t.Errorf("converted SKILL.md missing name: %s", converted)
	}
}

func TestSource_ListCompanionFiles(t *testing.T) {
	src, apiSrv, rawSrv := newTestSource(t)
	defer apiSrv.Close()
	defer rawSrv.Close()

	entry := &skillhub.RegistryEntry{Name: "github", Path: "skills/github"}
	files, err := src.ListCompanionFiles(context.Background(), entry)
	if err != nil {
		t.Fatalf("ListCompanionFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "scripts/install.sh" {
		t.Errorf("companion files = %v, want [scripts/install.sh]", files)
	}
}

func TestSource_FetchLicense(t *testing.T) {
	src, apiSrv, rawSrv := newTestSource(t)
	defer apiSrv.Close()
	defer rawSrv.Close()

	body, label, err := src.FetchLicense(context.Background(), nil)
	if err != nil {
		t.Fatalf("FetchLicense: %v", err)
	}
	if label != skillhub.LicenseMIT {
		t.Errorf("label = %q, want MIT", label)
	}
	if !strings.Contains(string(body), "MIT License") {
		t.Errorf("license body missing MIT marker: %s", body)
	}
}

func TestSource_FetchSkillFile_RejectsTraversal(t *testing.T) {
	src, apiSrv, rawSrv := newTestSource(t)
	defer apiSrv.Close()
	defer rawSrv.Close()

	entry := &skillhub.RegistryEntry{Name: "github", Path: "skills/github"}
	if _, err := src.FetchSkillFile(context.Background(), entry, "../../etc/passwd"); err == nil {
		t.Fatalf("expected traversal rejection")
	}
}

func TestSource_ID_AndDefaults(t *testing.T) {
	src := New()
	if src.ID() != SourceID {
		t.Errorf("ID = %q, want %q", src.ID(), SourceID)
	}
	if src.Owner != DefaultRepoOwner {
		t.Errorf("Owner = %q, want %q", src.Owner, DefaultRepoOwner)
	}
	if src.apiBase() != "https://api.github.com" {
		t.Errorf("apiBase = %q", src.apiBase())
	}
	if src.rawBase() != "https://raw.githubusercontent.com" {
		t.Errorf("rawBase = %q", src.rawBase())
	}
	// Empty TokenEnv should fall back to DefaultTokenEnv.
	src.TokenEnv = ""
	_ = src.token() // exercises the empty-name branch
}

func TestSource_FindSkillByName_EmptyName(t *testing.T) {
	src := New()
	if _, err := src.FindSkillByName(context.Background(), "   "); err == nil {
		t.Errorf("expected error for blank name")
	}
}

func TestSource_FetchSkillFile_EmptyEntry(t *testing.T) {
	src := New()
	if _, err := src.FetchSkillFile(context.Background(), nil, "foo"); err == nil {
		t.Errorf("expected error for nil entry")
	}
}

func TestSource_ListCompanionFiles_EmptyEntry(t *testing.T) {
	src := New()
	if _, err := src.ListCompanionFiles(context.Background(), nil); err == nil {
		t.Errorf("expected error for nil entry")
	}
}

func TestSource_ConvertSkillContent_EmptyEntry(t *testing.T) {
	src := New()
	if _, _, err := src.ConvertSkillContent(nil, []byte("---\nname: x\ndescription: y\n---\n")); err == nil {
		t.Errorf("expected error for nil entry")
	}
}

func TestSource_FindSkillByName_NotFound(t *testing.T) {
	src, apiSrv, rawSrv := newTestSource(t)
	defer apiSrv.Close()
	defer rawSrv.Close()

	if _, err := src.FindSkillByName(context.Background(), "does-not-exist"); err == nil {
		t.Fatalf("expected error for missing skill")
	}
}
