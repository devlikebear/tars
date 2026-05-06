package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/remoteaccess"
	"github.com/rs/zerolog"
)

func TestRemoteAccessAPI_EnablePreflightRequiresAuthAndPasswords(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	runner := newRemoteAccessTestRunner(map[string]string{
		"tailscale status --json":       `{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
		"tailscale serve status --json": `{}`,
	})
	handler := newRemoteAccessAPIHandler(remoteAccessHandlerOptions{
		Config: config.Config{
			RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspace},
			APIConfig:     config.APIConfig{APIAuthMode: "off"},
			RemoteAccessConfig: config.RemoteAccessConfig{
				RemoteAccessTailscaleServeHTTPSPort: remoteaccess.DefaultHTTPSPort,
			},
		},
		ConfigPath: configPath,
		Logger:     zerolog.Nop(),
		Runner:     runner,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/remote-access/enable", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%q", rec.Code, rec.Body.String())
	}
	if len(runner.commands) != 2 {
		t.Fatalf("preflight failure must not run serve enable, commands=%v", runner.commands)
	}
	var body struct {
		Checks []remoteAccessPreflightCheck `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !hasFailedRemoteAccessCheck(body.Checks, "api_auth_required") || !hasFailedRemoteAccessCheck(body.Checks, "admin_password_configured") {
		t.Fatalf("expected auth/password failures, got %+v", body.Checks)
	}
}

func TestRemoteAccessAPI_EnableStartsServeAndPersistsDesiredState(t *testing.T) {
	workspace := t.TempDir()
	store := consoleauth.NewStore(workspace)
	if err := store.SetPassword(consoleauth.RoleAdmin, "admin secret"); err != nil {
		t.Fatalf("set admin password: %v", err)
	}
	if err := store.SetPassword(consoleauth.RoleUser, "user secret"); err != nil {
		t.Fatalf("set user password: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("api_auth_mode: required\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	runner := newRemoteAccessTestRunner(map[string]string{
		"tailscale status --json":                                 `{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
		"tailscale serve status --json":                           `{}`,
		"tailscale serve --https=443 --bg http://127.0.0.1:43180": ``,
	})
	handler := newRemoteAccessAPIHandler(remoteAccessHandlerOptions{
		Config: config.Config{
			RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspace},
			APIConfig:     config.APIConfig{APIAuthMode: "required"},
			RemoteAccessConfig: config.RemoteAccessConfig{
				RemoteAccessTailscaleServeHTTPSPort: remoteaccess.DefaultHTTPSPort,
			},
		},
		ConfigPath: configPath,
		Logger:     zerolog.Nop(),
		Runner:     runner,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/remote-access/enable", strings.NewReader(`{"https_port":443}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q commands=%v", rec.Code, rec.Body.String(), runner.commands)
	}
	if !containsCommand(runner.commands, "tailscale serve --https=443 --bg http://127.0.0.1:43180") {
		t.Fatalf("expected serve enable command, got %v", runner.commands)
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !loaded.RemoteAccessTailscaleServeEnabled || loaded.RemoteAccessTailscaleServeHTTPSPort != 443 {
		t.Fatalf("expected desired state persisted, got enabled=%v port=%d", loaded.RemoteAccessTailscaleServeEnabled, loaded.RemoteAccessTailscaleServeHTTPSPort)
	}
}

func TestRemoteAccessAPI_DisableClearsDesiredStateWithoutTouchingNonOwnedTarget(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("remote_access:\n  tailscale_serve:\n    enabled: true\n    https_port: 443\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	runner := newRemoteAccessTestRunner(map[string]string{
		"tailscale status --json": `{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
		"tailscale serve status --json": `{
			"Web": {"mac.tail.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:3000"}}}}
		}`,
	})
	handler := newRemoteAccessAPIHandler(remoteAccessHandlerOptions{
		Config: config.Config{
			RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspace},
			APIConfig:     config.APIConfig{APIAuthMode: "required"},
			RemoteAccessConfig: config.RemoteAccessConfig{
				RemoteAccessTailscaleServeEnabled:   true,
				RemoteAccessTailscaleServeHTTPSPort: remoteaccess.DefaultHTTPSPort,
			},
		},
		ConfigPath: configPath,
		Logger:     zerolog.Nop(),
		Runner:     runner,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/remote-access/disable", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if containsCommand(runner.commands, "tailscale serve --https=443 off") {
		t.Fatalf("disable must not remove non-owned Serve target, commands=%v", runner.commands)
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.RemoteAccessTailscaleServeEnabled {
		t.Fatalf("expected desired state disabled")
	}
}

func TestRemoteAccessReconcileOnStartUsesDesiredConfig(t *testing.T) {
	runner := newRemoteAccessTestRunner(map[string]string{
		"tailscale status --json":                                 `{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
		"tailscale serve status --json":                           `{}`,
		"tailscale serve --https=443 --bg http://127.0.0.1:43180": ``,
	})
	runtime := &serveAPIRuntime{
		cfg: config.Config{
			RemoteAccessConfig: config.RemoteAccessConfig{
				RemoteAccessTailscaleServeEnabled:   true,
				RemoteAccessTailscaleServeHTTPSPort: 443,
			},
		},
		remoteAccessRunner:    runner,
		remoteAccessTargetURL: remoteaccess.DefaultTargetURL,
	}

	reconcileRemoteAccessOnStart(context.Background(), runtime, zerolog.Nop())

	if !containsCommand(runner.commands, "tailscale serve --https=443 --bg http://127.0.0.1:43180") {
		t.Fatalf("expected reconcile to enable Serve, commands=%v", runner.commands)
	}
}

func hasFailedRemoteAccessCheck(checks []remoteAccessPreflightCheck, key string) bool {
	for _, check := range checks {
		if check.Key == key && !check.OK {
			return true
		}
	}
	return false
}

func containsCommand(commands []string, want string) bool {
	for _, command := range commands {
		if command == want {
			return true
		}
	}
	return false
}

type remoteAccessTestRunner struct {
	outputs  map[string]string
	commands []string
}

func newRemoteAccessTestRunner(outputs map[string]string) *remoteAccessTestRunner {
	return &remoteAccessTestRunner{outputs: outputs}
}

func (r *remoteAccessTestRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.commands = append(r.commands, key)
	out, ok := r.outputs[key]
	if !ok {
		return "", "", errors.New("unexpected command: " + key)
	}
	return out, "", nil
}
