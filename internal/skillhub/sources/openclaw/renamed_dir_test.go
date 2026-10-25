package openclaw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const renamedDirSkill = `---
name: gh-review
description: Review pull requests with gh.
---

# gh-review
`

// newRenamedDirSource serves one skill whose directory ("github-review")
// is not its frontmatter name ("gh-review"), next to simple-skill.
func newRenamedDirSource(t *testing.T) *Source {
	t.Helper()
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any
		switch r.URL.Path {
		case "/repos/openclaw/openclaw/contents/skills":
			body = []map[string]any{
				{"name": "github-review", "path": "skills/github-review", "type": "dir"},
				{"name": "simple-skill", "path": "skills/simple-skill", "type": "dir"},
			}
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(apiSrv.Close)
	rawSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openclaw/openclaw/main/skills/github-review/SKILL.md":
			_, _ = w.Write([]byte(renamedDirSkill))
		case "/openclaw/openclaw/main/skills/simple-skill/SKILL.md":
			_, _ = w.Write([]byte(sampleSimpleSkill))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(rawSrv.Close)

	src := New()
	src.APIBaseURL = apiSrv.URL
	src.RawBaseURL = rawSrv.URL
	src.TokenEnv = "OPENCLAW_TEST_TOKEN_THAT_IS_UNSET"
	return src
}

func TestSource_FindSkillByName_DirectoryDiffersFromName(t *testing.T) {
	src := newRenamedDirSource(t)

	results, err := src.SearchSkills(context.Background(), "github-review")
	if err != nil || len(results) != 1 || results[0].Name != "gh-review" {
		t.Fatalf("search = %+v, %v; want the gh-review entry", results, err)
	}

	entry, err := src.FindSkillByName(context.Background(), "gh-review")
	if err != nil {
		t.Fatalf("FindSkillByName(gh-review): %v", err)
	}
	if entry.Name != "gh-review" || entry.Path != "skills/github-review" {
		t.Fatalf("entry = %+v, want gh-review at skills/github-review", entry)
	}
	raw, err := src.FetchSkillContent(context.Background(), entry)
	if err != nil || !strings.Contains(string(raw), "name: gh-review") {
		t.Fatalf("FetchSkillContent = %q, %v", raw, err)
	}

	// The directory name is not the skill's name.
	if _, err := src.FindSkillByName(context.Background(), "github-review"); err == nil {
		t.Fatal("FindSkillByName(github-review) should not match the directory of gh-review")
	}
	if _, err := src.FindSkillByName(context.Background(), "missing"); err == nil {
		t.Fatal("FindSkillByName(missing) should fail")
	}
}

func TestSource_FindSkillByPath(t *testing.T) {
	src := newRenamedDirSource(t)

	entry, err := src.FindSkillByPath(context.Background(), "skills/github-review")
	if err != nil {
		t.Fatalf("FindSkillByPath: %v", err)
	}
	if entry.Name != "gh-review" || entry.Path != "skills/github-review" {
		t.Fatalf("entry = %+v", entry)
	}
	for _, bad := range []string{"", "skills", "skills/", "other/github-review", "skills/a/b", "skills/..", `skills/a\b`} {
		if _, err := src.FindSkillByPath(context.Background(), bad); err == nil {
			t.Errorf("FindSkillByPath(%q) should be rejected", bad)
		}
	}
}
