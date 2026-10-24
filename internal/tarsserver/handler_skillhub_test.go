package tarsserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/rs/zerolog"
)

const hubTestTemplate = `id: hotfix
name: Hotfix
stages:
  - id: plan
  - id: build
  - id: pr
  - id: merge
`

// newHubTestInstaller serves a registry with one focus template and a pack
// that installs it, and returns an installer pointed at it.
func newHubTestInstaller(t *testing.T) *skillhub.Installer {
	t.Helper()
	sum := sha256.Sum256([]byte(hubTestTemplate))
	index := skillhub.RegistryIndex{
		Version: 3,
		FocusTemplates: []skillhub.FocusTemplateEntry{{
			Name: "hotfix", Version: "0.1.0", Path: "focus-templates/hotfix",
			Files: skillhub.RegistryFiles{{Path: "template.yaml", SHA256: hex.EncodeToString(sum[:])}},
		}},
		Packs: []skillhub.PackEntry{{Name: "fast", Version: "0.1.0", FocusTemplates: []string{"hotfix"}}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/registry.json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(index)
	})
	mux.HandleFunc("/focus-templates/hotfix/template.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(hubTestTemplate))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	inst := skillhub.NewInstaller(t.TempDir())
	inst.Registry.RegistryURL = srv.URL + "/registry.json"
	inst.Registry.SkillBaseURL = srv.URL
	inst.Registry.HTTPClient = srv.Client()
	return inst
}

// recordingExtensions is an extensionsProvider that records SetDisabled.
type recordingExtensions struct {
	reloads  int
	disabled []string
}

func (r *recordingExtensions) Snapshot() extensions.Snapshot       { return extensions.Snapshot{} }
func (r *recordingExtensions) DisabledSet() extensions.DisabledSet { return extensions.DisabledSet{} }
func (r *recordingExtensions) Reload(context.Context) error        { r.reloads++; return nil }
func (r *recordingExtensions) SetDisabled(_ context.Context, kind, name string, disabled bool) error {
	if !disabled {
		r.disabled = append(r.disabled, kind+":"+name)
	}
	return nil
}

func serveHub(t *testing.T, handler http.Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(method, path, reader))
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("%s %s: decode %q: %v", method, path, rec.Body.String(), err)
	}
	return rec.Code, payload
}

func TestSkillhubAPI_FocusTemplateInstallListUninstall(t *testing.T) {
	inst := newHubTestInstaller(t)
	ext := &recordingExtensions{}
	handler := newSkillhubAPIHandler(inst, ext, zerolog.Nop())

	code, payload := serveHub(t, handler, http.MethodPost, "/v1/hub/install", map[string]any{"type": "focus_template", "name": "hotfix"})
	if code != http.StatusOK {
		t.Fatalf("install: %d %v", code, payload)
	}
	if ext.reloads != 0 {
		t.Fatalf("a focus template install must not reload extensions, got %d reloads", ext.reloads)
	}
	code, payload = serveHub(t, handler, http.MethodGet, "/v1/hub/installed", nil)
	templates, _ := payload["focus_templates"].([]any)
	if code != http.StatusOK || len(templates) != 1 {
		t.Fatalf("installed: %d %v", code, payload)
	}

	code, payload = serveHub(t, handler, http.MethodPost, "/v1/hub/uninstall", map[string]any{"type": "focus_template", "name": "hotfix"})
	if code != http.StatusOK {
		t.Fatalf("uninstall: %d %v", code, payload)
	}
	if _, err := os.Stat(filepath.Join(inst.WorkspaceDir, "focus-templates", "hotfix.yaml")); !os.IsNotExist(err) {
		t.Fatalf("template file still present: %v", err)
	}
}

func TestSkillhubAPI_PackDryRunReturnsPlanThenInstalls(t *testing.T) {
	inst := newHubTestInstaller(t)
	handler := newSkillhubAPIHandler(inst, nil, zerolog.Nop())

	code, payload := serveHub(t, handler, http.MethodPost, "/v1/hub/install", map[string]any{"type": "pack", "name": "fast", "dry_run": true})
	plan, _ := payload["plan"].(map[string]any)
	items, _ := plan["items"].([]any)
	if code != http.StatusOK || len(items) != 1 {
		t.Fatalf("pack plan: %d %v", code, payload)
	}
	if _, err := os.Stat(filepath.Join(inst.WorkspaceDir, "focus-templates")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote files: %v", err)
	}

	code, payload = serveHub(t, handler, http.MethodPost, "/v1/hub/install", map[string]any{"type": "pack", "name": "fast"})
	if code != http.StatusOK {
		t.Fatalf("pack install: %d %v", code, payload)
	}
	if _, err := os.Stat(filepath.Join(inst.WorkspaceDir, "focus-templates", "hotfix.yaml")); err != nil {
		t.Fatalf("pack did not install its template: %v", err)
	}
}

func TestSkillhubAPI_UpdateCoversEveryPackageKind(t *testing.T) {
	inst := newHubTestInstaller(t)
	handler := newSkillhubAPIHandler(inst, nil, zerolog.Nop())

	code, payload := serveHub(t, handler, http.MethodPost, "/v1/hub/update", nil)
	if code != http.StatusOK {
		t.Fatalf("update: %d %v", code, payload)
	}
	for _, key := range []string{"skill_update_result", "plugin_update_result", "mcp_update_result", "focus_template_update_result"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("update response is missing %s: %v", key, payload)
		}
	}
}

func TestSkillhubAPI_InstalledReportsBrokenDB(t *testing.T) {
	inst := newHubTestInstaller(t)
	if err := os.WriteFile(filepath.Join(inst.WorkspaceDir, "skillhub.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := newSkillhubAPIHandler(inst, nil, zerolog.Nop())

	code, payload := serveHub(t, handler, http.MethodGet, "/v1/hub/installed", nil)
	if code != http.StatusInternalServerError || !strings.Contains(payload["error"].(string), "skillhub.json") {
		t.Fatalf("expected the broken DB to be reported, got %d %v", code, payload)
	}
}

func TestSkillhubAPI_UninstallClearsDisabledState(t *testing.T) {
	inst := newHubTestInstaller(t)
	dir := filepath.Join(inst.WorkspaceDir, "mcp-servers", "echo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	db := `{"skills":[],"mcps":[{"name":"echo","version":"0.1.0","source":"tars-hub","dir":` + strconvQuote(dir) + `,"manifest":"tars.mcp.json"}]}`
	if err := os.WriteFile(filepath.Join(inst.WorkspaceDir, "skillhub.json"), []byte(db), 0o600); err != nil {
		t.Fatal(err)
	}
	ext := &recordingExtensions{}
	handler := newSkillhubAPIHandler(inst, ext, zerolog.Nop())

	code, payload := serveHub(t, handler, http.MethodPost, "/v1/hub/uninstall", map[string]any{"type": "mcp", "name": "echo"})
	if code != http.StatusOK {
		t.Fatalf("uninstall: %d %v", code, payload)
	}
	if len(ext.disabled) != 1 || ext.disabled[0] != "mcp:echo" {
		t.Fatalf("expected the disabled state to be cleared, got %v", ext.disabled)
	}
}

func strconvQuote(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}
