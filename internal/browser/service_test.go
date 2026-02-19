package browser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeSecretReader struct {
	values map[string]map[string]string
}

func (f fakeSecretReader) ReadKV(_ context.Context, secretPath string) (map[string]string, error) {
	if v, ok := f.values[secretPath]; ok {
		return v, nil
	}
	return nil, os.ErrNotExist
}

func TestServiceProfilesAndBasicActions(t *testing.T) {
	svc := NewService(Config{WorkspaceDir: t.TempDir(), DefaultProfile: "managed"})
	profiles := svc.Profiles()
	if len(profiles) < 2 {
		t.Fatalf("expected at least two profiles, got %d", len(profiles))
	}
	state := svc.Start("managed")
	if !state.Running {
		t.Fatalf("expected browser running")
	}
	if state.Profile != "managed" {
		t.Fatalf("expected managed profile, got %q", state.Profile)
	}
	if _, err := svc.Open("https://example.com"); err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := svc.Snapshot(); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := svc.Act("click", "#submit", ""); err != nil {
		t.Fatalf("act: %v", err)
	}
	if _, err := svc.Screenshot("shot.txt"); err != nil {
		t.Fatalf("screenshot: %v", err)
	}
}

func TestServiceLoginManualDefault(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "example.yaml")
	content := strings.Join([]string{
		"id: example",
		"enabled: true",
		"profile: managed",
		"allowed_hosts: [\"example.com\"]",
		"login:",
		"  success_selector: '#ok'",
	}, "\n")
	if err := os.WriteFile(flowPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	svc := NewService(Config{WorkspaceDir: t.TempDir(), SiteFlowsDir: dir, DefaultProfile: "managed"})
	svc.Start("managed")
	res, err := svc.Login(context.Background(), "example", "")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.Mode != "manual" {
		t.Fatalf("expected manual mode, got %q", res.Mode)
	}
	if !strings.Contains(strings.ToLower(res.Message), "manual") {
		t.Fatalf("expected manual login message, got %q", res.Message)
	}
}

func TestServiceLoginVaultFormRequiresAllowlist(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "grafana.yaml")
	content := strings.Join([]string{
		"id: grafana",
		"enabled: true",
		"profile: managed",
		"login:",
		"  mode: vault_form",
		"  vault_path: sites/grafana",
		"  username_selector: '#user'",
		"  password_selector: '#pass'",
		"  submit_selector: 'button[type=submit]'",
	}, "\n")
	if err := os.WriteFile(flowPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	svc := NewService(Config{
		WorkspaceDir:   t.TempDir(),
		SiteFlowsDir:   dir,
		DefaultProfile: "managed",
		Vault: fakeSecretReader{values: map[string]map[string]string{
			"sites/grafana": {"username": "alice", "password": "secret"},
		}},
		AutoLoginSiteAllowlist: []string{},
	})
	svc.Start("managed")
	if _, err := svc.Login(context.Background(), "grafana", "managed"); err == nil {
		t.Fatalf("expected allowlist policy error")
	}

	svc2 := NewService(Config{
		WorkspaceDir:   t.TempDir(),
		SiteFlowsDir:   dir,
		DefaultProfile: "managed",
		Vault: fakeSecretReader{values: map[string]map[string]string{
			"sites/grafana": {"username": "alice", "password": "secret"},
		}},
		AutoLoginSiteAllowlist: []string{"grafana"},
	})
	svc2.Start("managed")
	res, err := svc2.Login(context.Background(), "grafana", "managed")
	if err != nil {
		t.Fatalf("login with allowlist: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected successful auto login result")
	}
	if strings.Contains(strings.ToLower(res.Message), "secret") {
		t.Fatalf("expected secret redaction in message, got %q", res.Message)
	}
}

func TestServiceRunAndCheckFlow(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "portal.yaml")
	content := strings.Join([]string{
		"id: portal",
		"enabled: true",
		"profile: managed",
		"checks:",
		"  - selector: '#welcome'",
		"    contains: 'hello'",
		"actions:",
		"  export:",
		"    steps:",
		"      - open: 'https://portal.example.com'",
		"      - click: '#export'",
	}, "\n")
	if err := os.WriteFile(flowPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	svc := NewService(Config{WorkspaceDir: t.TempDir(), SiteFlowsDir: dir, DefaultProfile: "managed"})
	svc.Start("managed")
	checkRes, err := svc.Check(context.Background(), "portal", "managed")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if checkRes.CheckCount != 1 {
		t.Fatalf("expected check count 1, got %d", checkRes.CheckCount)
	}
	runRes, err := svc.Run(context.Background(), "portal", "export", "managed")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if runRes.StepCount != 2 {
		t.Fatalf("expected step count 2, got %d", runRes.StepCount)
	}
}

func TestServiceRunRejectsDisallowedOpenHost(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "portal.yaml")
	content := strings.Join([]string{
		"id: portal",
		"enabled: true",
		"profile: managed",
		"allowed_hosts: [\"portal.example.com\"]",
		"actions:",
		"  export:",
		"    steps:",
		"      - open: 'https://evil.example.com/export'",
	}, "\n")
	if err := os.WriteFile(flowPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	svc := NewService(Config{WorkspaceDir: t.TempDir(), SiteFlowsDir: dir, DefaultProfile: "managed"})
	svc.Start("managed")
	_, err := svc.Run(context.Background(), "portal", "export", "managed")
	if err == nil {
		t.Fatalf("expected allowed_hosts policy error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "allowed_hosts") {
		t.Fatalf("expected allowed_hosts error message, got %v", err)
	}
}

func TestServiceRunAppliesFlowProfileByDefault(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "portal.yaml")
	content := strings.Join([]string{
		"id: portal",
		"enabled: true",
		"profile: chrome",
		"actions:",
		"  export:",
		"    steps:",
		"      - open: 'https://portal.example.com/export'",
	}, "\n")
	if err := os.WriteFile(flowPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	svc := NewService(Config{WorkspaceDir: t.TempDir(), SiteFlowsDir: dir, DefaultProfile: "managed"})
	svc.Start("managed")

	result, err := svc.Run(context.Background(), "portal", "export", "")
	if err != nil {
		t.Fatalf("run without explicit profile: %v", err)
	}
	if result.Profile != "chrome" {
		t.Fatalf("expected flow profile chrome, got %q", result.Profile)
	}

	if _, err := svc.Run(context.Background(), "portal", "export", "managed"); err == nil {
		t.Fatalf("expected profile mismatch error")
	}
}

func TestServiceRunAllowsWildcardHostPolicy(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "portal.yaml")
	content := strings.Join([]string{
		"id: portal",
		"enabled: true",
		"allowed_hosts: [\"*.example.com\"]",
		"actions:",
		"  export:",
		"    steps:",
		"      - open: 'https://app.example.com/export'",
	}, "\n")
	if err := os.WriteFile(flowPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	svc := NewService(Config{WorkspaceDir: t.TempDir(), SiteFlowsDir: dir, DefaultProfile: "managed"})
	svc.Start("managed")
	res, err := svc.Run(context.Background(), "portal", "export", "")
	if err != nil {
		t.Fatalf("expected wildcard host policy to pass, got %v", err)
	}
	if !res.Success {
		t.Fatalf("expected successful run result")
	}
}
