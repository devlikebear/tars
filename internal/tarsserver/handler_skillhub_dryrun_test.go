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
