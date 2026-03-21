package skillhub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testIndex() RegistryIndex {
	return RegistryIndex{
		Version: 1,
		Skills: []RegistryEntry{
			{
				Name:          "project-start",
				Description:   "Kick off a software project",
				Version:       "0.6.0",
				Author:        "devlikebear",
				Tags:          []string{"project", "kickoff"},
				Path:          "skills/project-start",
				UserInvocable: true,
			},
			{
				Name:          "novelist",
				Description:   "Creative writing guide",
				Version:       "0.6.0",
				Author:        "devlikebear",
				Tags:          []string{"creative", "writing"},
				Path:          "skills/novelist",
				UserInvocable: true,
			},
		},
	}
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/registry.json", func(w http.ResponseWriter, _ *http.Request) {
		data, _ := json.Marshal(testIndex())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/skills/project-start/SKILL.md", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("---\nname: project-start\n---\n# Project Start\n"))
	})
	return httptest.NewServer(mux)
}

func TestFetchIndex(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	reg := &Registry{
		RegistryURL: srv.URL + "/registry.json",
		HTTPClient:  srv.Client(),
	}
	index, err := reg.FetchIndex(context.Background())
	if err != nil {
		t.Fatalf("FetchIndex: %v", err)
	}
	if len(index.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(index.Skills))
	}
}

func TestSearchByName(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	reg := &Registry{
		RegistryURL: srv.URL + "/registry.json",
		HTTPClient:  srv.Client(),
	}
	results, err := reg.Search(context.Background(), "novelist")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Name != "novelist" {
		t.Fatalf("expected novelist, got %v", results)
	}
}

func TestSearchByTag(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	reg := &Registry{
		RegistryURL: srv.URL + "/registry.json",
		HTTPClient:  srv.Client(),
	}
	results, err := reg.Search(context.Background(), "project")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for tag search, got %d", len(results))
	}
}

func TestSearchEmpty(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	reg := &Registry{
		RegistryURL: srv.URL + "/registry.json",
		HTTPClient:  srv.Client(),
	}
	results, err := reg.Search(context.Background(), "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected all 2 skills, got %d", len(results))
	}
}

func TestFindByName(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	reg := &Registry{
		RegistryURL: srv.URL + "/registry.json",
		HTTPClient:  srv.Client(),
	}
	entry, err := reg.FindByName(context.Background(), "project-start")
	if err != nil {
		t.Fatalf("FindByName: %v", err)
	}
	if entry.Name != "project-start" {
		t.Fatalf("expected project-start, got %s", entry.Name)
	}
}

func TestFindByNameNotFound(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	reg := &Registry{
		RegistryURL: srv.URL + "/registry.json",
		HTTPClient:  srv.Client(),
	}
	_, err := reg.FindByName(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent skill")
	}
}

func TestFetchSkillContent(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	reg := &Registry{
		RegistryURL:  srv.URL + "/registry.json",
		SkillBaseURL: srv.URL,
		HTTPClient:   srv.Client(),
	}
	entry := &RegistryEntry{Path: "skills/project-start"}
	content, err := reg.FetchSkillContent(context.Background(), entry)
	if err != nil {
		t.Fatalf("FetchSkillContent: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("expected non-empty content")
	}
}
