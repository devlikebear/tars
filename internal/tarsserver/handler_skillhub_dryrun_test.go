package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/rs/zerolog"
)

// hubStubSource implements just enough of HubSource for the install
// handler's external-hub dry-run path: a single entry "foo" plus the
// converter / license / companion-lister capabilities.
type hubStubSource struct{}

func (hubStubSource) ID() string { return "demo" }

func (hubStubSource) SearchSkills(_ context.Context, _ string) ([]skillhub.RegistryEntry, error) {
	return []skillhub.RegistryEntry{{Name: "foo", Path: "skills/foo"}}, nil
}

func (hubStubSource) FindSkillByName(_ context.Context, name string) (*skillhub.RegistryEntry, error) {
	if name == "foo" {
		e := skillhub.RegistryEntry{Name: "foo", Path: "skills/foo"}
		return &e, nil
	}
	return nil, errors.New("not found")
}

func (hubStubSource) FetchSkillContent(_ context.Context, _ *skillhub.RegistryEntry) ([]byte, error) {
	return []byte("---\nname: foo\ndescription: hi\n---\nraw"), nil
}

func (hubStubSource) FetchSkillFile(_ context.Context, _ *skillhub.RegistryEntry, _ string) ([]byte, error) {
	return nil, errors.New("not used")
}

func (hubStubSource) ConvertSkillContent(_ *skillhub.RegistryEntry, _ []byte) ([]byte, []string, error) {
	return []byte("---\nname: foo\ndescription: hi\nuser_invocable: true\n---\nconverted"), []string{"demo warning"}, nil
}

func (hubStubSource) FetchLicense(_ context.Context, _ *skillhub.RegistryEntry) ([]byte, string, error) {
	return []byte("MIT License\n\nCopyright (c) 2025 Demo\n\nPermission is hereby granted, free of charge"), skillhub.LicenseMIT, nil
}

func (hubStubSource) ListCompanionFiles(_ context.Context, _ *skillhub.RegistryEntry) ([]string, error) {
	return nil, nil
}

// TestSkillhubInstall_DryRunReturnsPreview drives the handler end-to-end
// with `dry_run: true` and verifies the JSON response carries the preview
// without materializing files on disk.
func TestSkillhubInstall_DryRunReturnsPreview(t *testing.T) {
	workspace := t.TempDir()
	inst := skillhub.NewInstaller(workspace)
	if err := inst.Sources.Register(hubStubSource{}); err != nil {
		t.Fatalf("register stub: %v", err)
	}

	handler := newSkillhubAPIHandler(inst, nil, zerolog.Nop())

	body, _ := json.Marshal(map[string]any{
		"type":    "skill",
		"name":    "foo",
		"source":  "demo",
		"dry_run": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/hub/install", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["dry_run"] != true {
		t.Errorf("response missing dry_run=true: %v", resp)
	}
	if _, ok := resp["preview"]; !ok {
		t.Errorf("response missing preview field: %v", resp)
	}
}

func TestSkillhubInstall_RejectsEmptyName(t *testing.T) {
	workspace := t.TempDir()
	inst := skillhub.NewInstaller(workspace)
	handler := newSkillhubAPIHandler(inst, nil, zerolog.Nop())

	req := httptest.NewRequest(http.MethodPost, "/v1/hub/install", strings.NewReader(`{"type":"skill","name":""}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestSkillhubSources_ReturnsRegisteredAdapters drives the new
// /v1/hub/sources endpoint and verifies the response shape the console
// consumes.
func TestSkillhubSources_ReturnsRegisteredAdapters(t *testing.T) {
	workspace := t.TempDir()
	inst := skillhub.NewInstaller(workspace)
	if err := inst.Sources.Register(hubStubSource{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	handler := newSkillhubAPIHandler(inst, nil, zerolog.Nop())

	req := httptest.NewRequest(http.MethodGet, "/v1/hub/sources", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Sources []map[string]any `json:"sources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Sources) != 2 { // tars-hub (built-in) + demo
		t.Fatalf("expected 2 sources, got %d: %v", len(resp.Sources), resp.Sources)
	}
	for _, s := range resp.Sources {
		switch s["id"] {
		case "tars-hub":
			if s["default"] != true {
				t.Errorf("tars-hub should be default=true: %v", s)
			}
		case "demo":
			if s["external"] != true {
				t.Errorf("demo should be external=true: %v", s)
			}
		}
	}
}

// TestSkillhubSkills_FederatedSearch verifies that /v1/hub/skills returns
// SkillSearchResult entries and respects the optional source filter.
func TestSkillhubSkills_FederatedSearch(t *testing.T) {
	workspace := t.TempDir()
	inst := skillhub.NewInstaller(workspace)
	if err := inst.Sources.Register(hubStubSource{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	handler := newSkillhubAPIHandler(inst, nil, zerolog.Nop())

	req := httptest.NewRequest(http.MethodGet, "/v1/hub/skills", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"SourceID":"demo"`) {
		t.Errorf("response missing demo source entry: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/hub/skills?source=demo&q=foo", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"SourceID":"demo"`) {
		t.Errorf("filtered response missing demo entry: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/hub/skills?source=nope", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown source, got %d", rec.Code)
	}
}

func TestSkillhubSources_NilInstaller(t *testing.T) {
	handler := newSkillhubAPIHandler(nil, nil, zerolog.Nop())
	req := httptest.NewRequest(http.MethodGet, "/v1/hub/sources", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"sources":[]`) {
		t.Errorf("expected empty sources list, got %s", rec.Body.String())
	}
}

func TestSkillhubSkills_NilInstaller(t *testing.T) {
	handler := newSkillhubAPIHandler(nil, nil, zerolog.Nop())
	req := httptest.NewRequest(http.MethodGet, "/v1/hub/skills", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestSkillhubSources_RejectsNonGET(t *testing.T) {
	inst := skillhub.NewInstaller(t.TempDir())
	handler := newSkillhubAPIHandler(inst, nil, zerolog.Nop())
	req := httptest.NewRequest(http.MethodPost, "/v1/hub/sources", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Errorf("POST should not return 200")
	}
}

func TestSkillhubSkills_RejectsNonGET(t *testing.T) {
	inst := skillhub.NewInstaller(t.TempDir())
	handler := newSkillhubAPIHandler(inst, nil, zerolog.Nop())
	req := httptest.NewRequest(http.MethodDelete, "/v1/hub/skills", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Errorf("DELETE should not return 200")
	}
}

func TestHubSourceLabel(t *testing.T) {
	cases := map[string]string{
		"tars-hub":  "TARS Hub",
		"openclaw":  "openclaw",
		"hermes":    "hermes-agent",
		"anthropic": "Anthropic skills",
		"custom":    "custom",
	}
	for id, want := range cases {
		if got := hubSourceLabel(id); got != want {
			t.Errorf("hubSourceLabel(%q) = %q, want %q", id, got, want)
		}
	}
}
