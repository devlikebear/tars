package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	svc.managed = &fakeManagedRuntime{}
	profiles := svc.Profiles()
	if len(profiles) != 1 {
		t.Fatalf("expected one managed profile, got %d", len(profiles))
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
	status := svc.Status()
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(status.LastScreenshot)), ".png") {
		t.Fatalf("expected png screenshot path, got %q", status.LastScreenshot)
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
	svc2.runner = fakeFlowRunner{response: flowRunResponse{Message: "vault login ok"}}
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
	svc.runner = fakeFlowRunner{response: flowRunResponse{Passed: true, Message: "ok"}}
	checkRes, err := svc.Check(context.Background(), "portal", "managed")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if checkRes.CheckCount != 1 {
		t.Fatalf("expected check count 1, got %d", checkRes.CheckCount)
	}
	svc.runner = fakeFlowRunner{response: flowRunResponse{Message: "run ok"}}
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
		"profile: managed",
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
	svc.runner = fakeFlowRunner{response: flowRunResponse{Message: "run ok"}}

	result, err := svc.Run(context.Background(), "portal", "export", "")
	if err != nil {
		t.Fatalf("run without explicit profile: %v", err)
	}
	if result.Profile != "managed" {
		t.Fatalf("expected flow profile managed, got %q", result.Profile)
	}

	if _, err := svc.Run(context.Background(), "portal", "export", "chrome"); err != nil {
		t.Fatalf("expected unsupported profile alias to resolve to managed, got %v", err)
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
	svc.runner = fakeFlowRunner{response: flowRunResponse{Message: "run ok"}}
	res, err := svc.Run(context.Background(), "portal", "export", "")
	if err != nil {
		t.Fatalf("expected wildcard host policy to pass, got %v", err)
	}
	if !res.Success {
		t.Fatalf("expected successful run result")
	}
}

func TestServiceScreenshot_DefaultNameUsesPNG(t *testing.T) {
	workspace := t.TempDir()
	runtime := &fakeManagedRuntime{}
	svc := NewService(Config{WorkspaceDir: workspace, DefaultProfile: "managed"})
	svc.managed = runtime
	svc.Start("managed")
	if _, err := svc.Open("https://example.com"); err != nil {
		t.Fatalf("open: %v", err)
	}
	state, err := svc.Screenshot("")
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(state.LastScreenshot)), ".png") {
		t.Fatalf("expected .png screenshot, got %q", state.LastScreenshot)
	}
}

func TestServiceScreenshot_RewritesNonPNGExtension(t *testing.T) {
	workspace := t.TempDir()
	runtime := &fakeManagedRuntime{}
	svc := NewService(Config{WorkspaceDir: workspace, DefaultProfile: "managed"})
	svc.managed = runtime
	svc.Start("managed")
	if _, err := svc.Open("https://example.com"); err != nil {
		t.Fatalf("open: %v", err)
	}
	state, err := svc.Screenshot("capture.txt")
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(state.LastScreenshot)), ".png") {
		t.Fatalf("expected .png screenshot, got %q", state.LastScreenshot)
	}
	if len(runtime.screenshots) == 0 || !strings.HasSuffix(strings.ToLower(runtime.screenshots[len(runtime.screenshots)-1]), ".png") {
		t.Fatalf("expected runtime screenshot path to be png, got %+v", runtime.screenshots)
	}
}

func TestServiceProfilesUsePlaywrightDriver(t *testing.T) {
	workspace := t.TempDir()
	managed := &fakeManagedRuntime{}
	svc := NewService(Config{WorkspaceDir: workspace, DefaultProfile: "managed"})
	svc.managed = managed

	profiles := svc.Profiles()
	if len(profiles) != 1 {
		t.Fatalf("expected one managed profile, got %+v", profiles)
	}
	if profiles[0].Driver != "playwright" {
		t.Fatalf("expected playwright driver, got %+v", profiles[0])
	}
}

func TestServiceStart_SameProfileRestartsManagedRuntime(t *testing.T) {
	workspace := t.TempDir()
	managed := &fakeManagedRuntime{}
	svc := NewService(Config{WorkspaceDir: workspace, DefaultProfile: "managed"})
	svc.managed = managed

	state := svc.Start("managed")
	if !state.Running {
		t.Fatalf("expected managed profile running")
	}
	if managed.stopCalls != 0 {
		t.Fatalf("unexpected stop calls before restart managed=%d", managed.stopCalls)
	}

	state = svc.Start("managed")
	if !state.Running {
		t.Fatalf("expected managed profile running after restart")
	}
	if managed.stopCalls != 1 {
		t.Fatalf("expected managed stop once, got %d", managed.stopCalls)
	}
}

func TestServiceOpen_RetriesOnceOnManagedContextCanceled(t *testing.T) {
	workspace := t.TempDir()
	managed := &flakyManagedRuntime{failOpenOnce: true}
	svc := NewService(Config{WorkspaceDir: workspace, DefaultProfile: "managed"})
	svc.managed = managed

	state := svc.Start("managed")
	if !state.Running {
		t.Fatalf("expected managed profile running, got error=%q", state.LastError)
	}

	state, err := svc.Open("https://example.com")
	if err != nil {
		t.Fatalf("expected open recovery success, got %v", err)
	}
	if !state.Running {
		t.Fatalf("expected running=true after recovery open")
	}
	if managed.openCalls != 2 {
		t.Fatalf("expected two open attempts (initial + retry), got %d", managed.openCalls)
	}
	if managed.stopCalls != 1 {
		t.Fatalf("expected one runtime stop for recovery, got %d", managed.stopCalls)
	}
	if managed.startCalls != 2 {
		t.Fatalf("expected second start for recovery, got %d", managed.startCalls)
	}
}

type fakeManagedRuntime struct {
	started     bool
	currentURL  string
	screenshots []string
	startCalls  int
	stopCalls   int
}

type fakeFlowRunner struct {
	response flowRunResponse
	err      error
	lastReq  flowRunRequest
}

func (f fakeFlowRunner) Execute(_ context.Context, req flowRunRequest) (flowRunResponse, error) {
	if f.err != nil {
		return flowRunResponse{}, f.err
	}
	f.lastReq = req
	return f.response, nil
}

func (f *fakeManagedRuntime) Start(context.Context) error {
	f.startCalls++
	f.started = true
	return nil
}

func (f *fakeManagedRuntime) Stop(context.Context) error {
	f.stopCalls++
	f.started = false
	return nil
}

func (f *fakeManagedRuntime) Open(_ context.Context, rawURL string) error {
	if !f.started {
		return fmt.Errorf("browser is not running")
	}
	f.currentURL = strings.TrimSpace(rawURL)
	return nil
}

func (f *fakeManagedRuntime) Snapshot(context.Context) (string, error) {
	if !f.started {
		return "", fmt.Errorf("browser is not running")
	}
	if f.currentURL == "" {
		return "no page opened", nil
	}
	return "snapshot captured", nil
}

func (f *fakeManagedRuntime) Act(_ context.Context, action string, target string, value string) (string, error) {
	if !f.started {
		return "", fmt.Errorf("browser is not running")
	}
	return fmt.Sprintf("%s target=%s value=%s", strings.TrimSpace(action), strings.TrimSpace(target), strings.TrimSpace(value)), nil
}

func (f *fakeManagedRuntime) Screenshot(_ context.Context, path string) error {
	if !f.started {
		return fmt.Errorf("browser is not running")
	}
	targetPath := strings.TrimSpace(path)
	f.screenshots = append(f.screenshots, targetPath)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(targetPath, []byte("PNG"), 0o644)
}

func (f *fakeManagedRuntime) Close() error {
	f.started = false
	return nil
}

func (f *fakeManagedRuntime) Timeout() time.Duration {
	return 0
}

type flakyManagedRuntime struct {
	started      bool
	startCalls   int
	stopCalls    int
	openCalls    int
	failOpenOnce bool
}

func (f *flakyManagedRuntime) Start(context.Context) error {
	f.started = true
	f.startCalls++
	return nil
}

func (f *flakyManagedRuntime) Stop(context.Context) error {
	f.started = false
	f.stopCalls++
	return nil
}

func (f *flakyManagedRuntime) Open(_ context.Context, _ string) error {
	if !f.started {
		return fmt.Errorf("browser is not running")
	}
	f.openCalls++
	if f.failOpenOnce {
		f.failOpenOnce = false
		return fmt.Errorf("context canceled")
	}
	return nil
}

func (f *flakyManagedRuntime) Snapshot(context.Context) (string, error) {
	if !f.started {
		return "", fmt.Errorf("browser is not running")
	}
	return "snapshot captured", nil
}

func (f *flakyManagedRuntime) Act(context.Context, string, string, string) (string, error) {
	if !f.started {
		return "", fmt.Errorf("browser is not running")
	}
	return "ok", nil
}

func (f *flakyManagedRuntime) Screenshot(context.Context, string) error {
	if !f.started {
		return fmt.Errorf("browser is not running")
	}
	return nil
}
