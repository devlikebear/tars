package skillhub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/devlikebear/tars/internal/focuspipeline"
)

const hotfixTemplateYAML = `id: hotfix
name: Hotfix
description: Small fixes, fast.
stages:
  - id: plan
    label: Scope
  - id: build
    label: Fix
  - id: pr
  - id: merge
`

// focusTemplateHub is a registry whose focus template can be republished
// mid-test, so update paths see a new version.
type focusTemplateHub struct {
	mu      sync.Mutex
	version string
	content []byte
	srv     *httptest.Server
}

func newFocusTemplateHub(t *testing.T, content string) *focusTemplateHub {
	t.Helper()
	hub := &focusTemplateHub{version: "0.1.0", content: []byte(content)}
	mux := http.NewServeMux()
	mux.HandleFunc("/registry.json", func(w http.ResponseWriter, _ *http.Request) {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		index := RegistryIndex{
			Version: 3,
			FocusTemplates: []FocusTemplateEntry{{
				Name:    "hotfix",
				Version: hub.version,
				Path:    "focus-templates/hotfix",
				Files:   RegistryFiles{{Path: "template.yaml", SHA256: sha256Hex(hub.content)}},
			}},
			Packs: []PackEntry{{Name: "fast-pack", Version: "0.1.0", FocusTemplates: []string{"hotfix"}}},
		}
		_ = json.NewEncoder(w).Encode(index)
	})
	mux.HandleFunc("/focus-templates/hotfix/template.yaml", func(w http.ResponseWriter, _ *http.Request) {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		_, _ = w.Write(hub.content)
	})
	hub.srv = httptest.NewServer(mux)
	t.Cleanup(hub.srv.Close)
	return hub
}

func (h *focusTemplateHub) publish(version, content string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.version, h.content = version, []byte(content)
}

func (h *focusTemplateHub) installer(workspace string) *Installer {
	return &Installer{
		WorkspaceDir: workspace,
		Registry: &Registry{
			RegistryURL:  h.srv.URL + "/registry.json",
			SkillBaseURL: h.srv.URL,
			HTTPClient:   h.srv.Client(),
		},
	}
}

func loadedTemplate(t *testing.T, workspace, id string) focuspipeline.Template {
	t.Helper()
	tmpl, ok := focuspipeline.FindTemplate(filepath.Join(workspace, focuspipeline.TemplateDirName), id)
	if !ok {
		t.Fatalf("template %q is not loaded from the workspace", id)
	}
	return tmpl
}

func TestInstallFocusTemplateWritesLoadableTemplate(t *testing.T) {
	hub := newFocusTemplateHub(t, hotfixTemplateYAML)
	workspace := t.TempDir()
	inst := hub.installer(workspace)

	result, err := inst.InstallFocusTemplate(context.Background(), "hotfix")
	if err != nil {
		t.Fatalf("InstallFocusTemplate: %v", err)
	}
	if !result.Sandbox.Passed || result.Sandbox.PackageType != "focus_template" || len(result.Sandbox.Checks) != 1 {
		t.Fatalf("unexpected sandbox report: %+v", result.Sandbox)
	}
	if got := loadedTemplate(t, workspace, "hotfix"); got.Name != "Hotfix" || len(got.Stages) != 4 {
		t.Fatalf("unexpected loaded template: %+v", got)
	}
	rows, err := inst.ListFocusTemplates()
	if err != nil {
		t.Fatalf("ListFocusTemplates: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "hotfix" || rows[0].Version != "0.1.0" || rows[0].File != "hotfix.yaml" || rows[0].SHA256 == "" {
		t.Fatalf("unexpected installed rows: %+v", rows)
	}
}

func TestInstallFocusTemplateRefusesLocalTemplateOfSameID(t *testing.T) {
	hub := newFocusTemplateHub(t, hotfixTemplateYAML)
	workspace := t.TempDir()
	dir := filepath.Join(workspace, focuspipeline.TemplateDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	local := strings.Replace(hotfixTemplateYAML, "name: Hotfix", "name: Mine", 1)
	if err := os.WriteFile(filepath.Join(dir, "mine.yaml"), []byte(local), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := hub.installer(workspace).InstallFocusTemplate(context.Background(), "hotfix")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected a local-template conflict, got %v", err)
	}
	if got := loadedTemplate(t, workspace, "hotfix"); got.Name != "Mine" {
		t.Fatalf("local template was replaced: %+v", got)
	}
}

func TestInstallFocusTemplateRejectsInvalidTemplate(t *testing.T) {
	hub := newFocusTemplateHub(t, strings.Replace(hotfixTemplateYAML, "  - id: plan\n    label: Scope\n", "", 1))
	workspace := t.TempDir()

	_, err := hub.installer(workspace).InstallFocusTemplate(context.Background(), "hotfix")
	var sandboxErr *SandboxError
	if !errors.As(err, &sandboxErr) || sandboxErr.Report.Passed {
		t.Fatalf("expected a failed sandbox report, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, focuspipeline.TemplateDirName, "hotfix.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("invalid template was written: %v", statErr)
	}
}

func TestInstallFocusTemplateRejectsMismatchedID(t *testing.T) {
	hub := newFocusTemplateHub(t, strings.Replace(hotfixTemplateYAML, "id: hotfix", "id: other", 1))

	_, err := hub.installer(t.TempDir()).InstallFocusTemplate(context.Background(), "hotfix")
	if err == nil || !strings.Contains(err.Error(), "does not match registry name") {
		t.Fatalf("expected an id mismatch, got %v", err)
	}
}

func TestUpdateFocusTemplatesReplacesUntouchedAndSkipsEdited(t *testing.T) {
	hub := newFocusTemplateHub(t, hotfixTemplateYAML)
	workspace := t.TempDir()
	inst := hub.installer(workspace)
	ctx := context.Background()
	if _, err := inst.InstallFocusTemplate(ctx, "hotfix"); err != nil {
		t.Fatalf("InstallFocusTemplate: %v", err)
	}

	hub.publish("0.2.0", strings.Replace(hotfixTemplateYAML, "name: Hotfix", "name: Hotfix v2", 1))
	result, err := inst.UpdateFocusTemplates(ctx)
	if err != nil {
		t.Fatalf("UpdateFocusTemplates: %v", err)
	}
	if len(result.Updated) != 1 || loadedTemplate(t, workspace, "hotfix").Name != "Hotfix v2" {
		t.Fatalf("expected the untouched template to update, got %+v", result)
	}

	// The user edits the installed template; the next release must not
	// overwrite it.
	path := filepath.Join(workspace, focuspipeline.TemplateDirName, "hotfix.yaml")
	if _, err := focuspipeline.SaveTemplate(filepath.Dir(path), "hotfix", focuspipeline.Template{
		ID: "hotfix", Name: "My hotfix", Stages: []focuspipeline.TemplateStage{{ID: "plan"}, {ID: "build"}},
	}); err != nil {
		t.Fatal(err)
	}
	hub.publish("0.3.0", strings.Replace(hotfixTemplateYAML, "name: Hotfix", "name: Hotfix v3", 1))
	result, err = inst.UpdateFocusTemplates(ctx)
	if err != nil {
		t.Fatalf("UpdateFocusTemplates: %v", err)
	}
	if len(result.Updated) != 0 || len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0].Reason, "edited locally") {
		t.Fatalf("expected the edited template to be skipped, got %+v", result)
	}
	if got := loadedTemplate(t, workspace, "hotfix"); got.Name != "My hotfix" {
		t.Fatalf("edited template was overwritten: %+v", got)
	}
}

func TestUninstallFocusTemplateRemovesFileAndRow(t *testing.T) {
	hub := newFocusTemplateHub(t, hotfixTemplateYAML)
	workspace := t.TempDir()
	inst := hub.installer(workspace)
	if _, err := inst.InstallFocusTemplate(context.Background(), "hotfix"); err != nil {
		t.Fatalf("InstallFocusTemplate: %v", err)
	}

	if err := inst.UninstallFocusTemplate("hotfix"); err != nil {
		t.Fatalf("UninstallFocusTemplate: %v", err)
	}
	if _, ok := focuspipeline.FindTemplate(filepath.Join(workspace, focuspipeline.TemplateDirName), "hotfix"); ok {
		t.Fatal("template is still loaded after uninstall")
	}
	if rows, _ := inst.ListFocusTemplates(); len(rows) != 0 {
		t.Fatalf("expected no rows, got %+v", rows)
	}
	if err := inst.UninstallFocusTemplate("hotfix"); err == nil {
		t.Fatal("expected an error uninstalling a template that is not installed")
	}
}

func TestListFocusTemplatesHidesTemplatesDeletedOutsideTheHub(t *testing.T) {
	hub := newFocusTemplateHub(t, hotfixTemplateYAML)
	workspace := t.TempDir()
	inst := hub.installer(workspace)
	if _, err := inst.InstallFocusTemplate(context.Background(), "hotfix"); err != nil {
		t.Fatalf("InstallFocusTemplate: %v", err)
	}
	if err := focuspipeline.DeleteTemplate(filepath.Join(workspace, focuspipeline.TemplateDirName), "hotfix"); err != nil {
		t.Fatal(err)
	}

	if rows, _ := inst.ListFocusTemplates(); len(rows) != 0 {
		t.Fatalf("expected the deleted template to drop out, got %+v", rows)
	}
	// Uninstalling the stale row still succeeds and clears it.
	if err := inst.UninstallFocusTemplate("hotfix"); err != nil {
		t.Fatalf("UninstallFocusTemplate on a stale row: %v", err)
	}
}

func TestInstallPackInstallsFocusTemplates(t *testing.T) {
	hub := newFocusTemplateHub(t, hotfixTemplateYAML)
	workspace := t.TempDir()
	inst := hub.installer(workspace)

	result, err := inst.InstallPack(context.Background(), "fast-pack")
	if err != nil {
		t.Fatalf("InstallPack: %v", err)
	}
	if len(result.Installed) != 1 || result.Installed[0].Type != "focus_template" {
		t.Fatalf("unexpected pack result: %+v", result)
	}
	loadedTemplate(t, workspace, "hotfix")
}
