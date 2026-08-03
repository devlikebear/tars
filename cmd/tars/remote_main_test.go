package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/consoleauth"
)

func TestRootCommand_IncludesRemoteSubcommand(t *testing.T) {
	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	if subcmd, _, err := cmd.Find([]string{"remote"}); err != nil || subcmd == nil || subcmd.Name() != "remote" {
		t.Fatalf("expected remote subcommand, got subcmd=%v err=%v", subcmd, err)
	}
}

func TestRemoteStatusSubcommandInvokesRunner(t *testing.T) {
	original := remoteStatusRunner
	defer func() { remoteStatusRunner = original }()
	var got remoteStatusOptions
	remoteStatusRunner = func(_ context.Context, opts remoteStatusOptions, stdout, _ io.Writer) error {
		got = opts
		_, err := fmt.Fprintln(stdout, "remote ok")
		return err
	}

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"remote", "status", "--json", "--port", "9443"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote status: %v", err)
	}
	if !got.json || got.httpsPort != 9443 {
		t.Fatalf("unexpected remote status options: %+v", got)
	}
	if strings.TrimSpace(stdout.String()) != "remote ok" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRemoteEnableDisableURLSubcommandsInvokeRunners(t *testing.T) {
	origEnable := remoteEnableRunner
	origDisable := remoteDisableRunner
	origURL := remoteURLRunner
	defer func() {
		remoteEnableRunner = origEnable
		remoteDisableRunner = origDisable
		remoteURLRunner = origURL
	}()

	var enabled remoteAccessOptions
	remoteEnableRunner = func(_ context.Context, opts remoteAccessOptions, stdout, _ io.Writer) error {
		enabled = opts
		_, err := fmt.Fprintln(stdout, "enabled")
		return err
	}
	var disabled remoteAccessOptions
	remoteDisableRunner = func(_ context.Context, opts remoteAccessOptions, stdout, _ io.Writer) error {
		disabled = opts
		_, err := fmt.Fprintln(stdout, "disabled")
		return err
	}
	var urlOpts remoteAccessOptions
	remoteURLRunner = func(_ context.Context, opts remoteAccessOptions, stdout, _ io.Writer) error {
		urlOpts = opts
		_, err := fmt.Fprintln(stdout, "https://mac.tail.ts.net:9443")
		return err
	}

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"remote", "enable", "--port", "9443"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote enable: %v", err)
	}
	if enabled.httpsPort != 9443 || !strings.Contains(stdout.String(), "enabled") {
		t.Fatalf("unexpected enable result opts=%+v stdout=%q", enabled, stdout.String())
	}

	stdout.Reset()
	cmd = newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"remote", "disable", "--port", "9443"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote disable: %v", err)
	}
	if disabled.httpsPort != 9443 || !strings.Contains(stdout.String(), "disabled") {
		t.Fatalf("unexpected disable result opts=%+v stdout=%q", disabled, stdout.String())
	}

	stdout.Reset()
	cmd = newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"remote", "url", "--port", "9443"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote url: %v", err)
	}
	if urlOpts.httpsPort != 9443 || strings.TrimSpace(stdout.String()) != "https://mac.tail.ts.net:9443" {
		t.Fatalf("unexpected url result opts=%+v stdout=%q", urlOpts, stdout.String())
	}
}

func TestRemoteAccessCLIConfigRequiresBothBrowserRolesAndPersistsOwnership(t *testing.T) {
	t.Setenv("API_AUTH_MODE", "")
	t.Setenv("TARS_API_AUTH_MODE", "")
	t.Setenv("TARS_WORKSPACE_DIR", "")

	workspaceDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "tars.config.yaml")
	writeConfig := func(mode string) {
		t.Helper()
		raw := "workspace_dir: " + workspaceDir + "\napi_auth_mode: " + mode + "\n"
		if err := os.WriteFile(configPath, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	opts := remoteAccessOptions{httpsPort: 9443, configPath: configPath}
	writeConfig("off")
	if err := validateRemoteAccessCLIAuth(opts); err == nil || !strings.Contains(err.Error(), "api_auth_mode") {
		t.Fatalf("disabled API auth error=%v", err)
	}

	writeConfig("required")
	if err := validateRemoteAccessCLIAuth(opts); err == nil || !strings.Contains(err.Error(), "admin") {
		t.Fatalf("missing admin password error=%v", err)
	}
	authStore := consoleauth.NewStore(workspaceDir)
	if err := authStore.SetPassword(consoleauth.RoleAdmin, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteAccessCLIAuth(opts); err == nil || !strings.Contains(err.Error(), "user") {
		t.Fatalf("missing user password error=%v", err)
	}
	if err := authStore.SetPassword(consoleauth.RoleUser, "user correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteAccessCLIAuth(opts); err != nil {
		t.Fatalf("fully authenticated remote config: %v", err)
	}

	if err := patchRemoteAccessCLIState(opts, true); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RemoteAccessTailscaleServeEnabled || cfg.RemoteAccessTailscaleServeHTTPSPort != 9443 {
		t.Fatalf("patched remote config enabled=%v port=%d", cfg.RemoteAccessTailscaleServeEnabled, cfg.RemoteAccessTailscaleServeHTTPSPort)
	}
	if err := patchRemoteAccessCLIState(opts, false); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.LoadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RemoteAccessTailscaleServeEnabled || cfg.RemoteAccessTailscaleServeHTTPSPort != 9443 {
		t.Fatalf("disabled remote config enabled=%v port=%d", cfg.RemoteAccessTailscaleServeEnabled, cfg.RemoteAccessTailscaleServeHTTPSPort)
	}
}

func TestRemoteAccessCLIConfigRejectsUnreadableFilesAndResolvesWorkspaceOverride(t *testing.T) {
	t.Setenv("TARS_WORKSPACE_DIR", "")
	invalidPath := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(invalidPath, []byte("runtime: [broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRemoteAccessCLIConfig(remoteAccessOptions{configPath: invalidPath}); err == nil {
		t.Fatal("invalid remote config loaded")
	}
	if err := patchRemoteAccessCLIState(remoteAccessOptions{configPath: t.TempDir()}, true); err == nil {
		t.Fatal("directory remote config path patched")
	}

	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	configPath := filepath.Join(t.TempDir(), "valid.yaml")
	if err := os.WriteFile(configPath, []byte("api_auth_mode: required\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadRemoteAccessCLIConfig(remoteAccessOptions{configPath: configPath, workspaceDir: workspaceDir})
	if err != nil {
		t.Fatal(err)
	}
	wantWorkspace, err := filepath.Abs(workspaceDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkspaceDir != wantWorkspace {
		t.Fatalf("workspace override=%q want=%q", cfg.WorkspaceDir, wantWorkspace)
	}
}

func TestRemoteAccessCLIRendersLiveStatusURLAndOwnedMutations(t *testing.T) {
	binDir := t.TempDir()
	serveOwned := `{"Web":{"mac.tail.ts.net:9443":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:43180"}}}}}`
	writeFakeTailscale(t, binDir,
		`{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
		serveOwned,
	)
	t.Setenv("PATH", binDir)

	var stdout strings.Builder
	if err := runRemoteStatus(context.Background(), remoteStatusOptions{httpsPort: 9443}, &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	if output := stdout.String(); !strings.Contains(output, "installed, logged in") ||
		!strings.Contains(output, "mac.tail.ts.net") || !strings.Contains(output, "serving on https:9443") {
		t.Fatalf("plain remote status=%q", output)
	}
	stdout.Reset()
	if err := runRemoteStatus(context.Background(), remoteStatusOptions{json: true, httpsPort: 9443}, &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	if output := stdout.String(); !strings.Contains(output, `"installed": true`) || !strings.Contains(output, `"owned_by_tars": true`) {
		t.Fatalf("JSON remote status=%q", output)
	}
	stdout.Reset()
	if err := runRemoteURL(context.Background(), remoteAccessOptions{httpsPort: 9443}, &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != "https://mac.tail.ts.net:9443" {
		t.Fatalf("remote URL=%q", stdout.String())
	}

	workspaceDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "tars.config.yaml")
	if err := os.WriteFile(configPath, []byte("workspace_dir: "+workspaceDir+"\napi_auth_mode: required\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	authStore := consoleauth.NewStore(workspaceDir)
	if err := authStore.SetPassword(consoleauth.RoleAdmin, "admin correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err := authStore.SetPassword(consoleauth.RoleUser, "user correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	opts := remoteAccessOptions{httpsPort: 9443, configPath: configPath}
	stdout.Reset()
	if err := runRemoteEnable(context.Background(), opts, &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "enabled on https:9443") {
		t.Fatalf("enable output=%q", stdout.String())
	}
	stdout.Reset()
	if err := runRemoteDisable(context.Background(), opts, &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "disabled for https:9443") {
		t.Fatalf("disable output=%q", stdout.String())
	}

	missingBinDir := t.TempDir()
	t.Setenv("PATH", missingBinDir)
	stdout.Reset()
	if err := runRemoteStatus(context.Background(), remoteStatusOptions{httpsPort: 9443}, &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "not installed, logged out") || !strings.Contains(stdout.String(), "serve: idle") {
		t.Fatalf("missing tailscale status=%q", stdout.String())
	}
	if err := runRemoteURL(context.Background(), remoteAccessOptions{httpsPort: 9443}, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("missing tailscale URL error=%v", err)
	}

	t.Setenv("PATH", binDir)
	writeFakeTailscale(t, binDir, `{"BackendState":"Stopped"}`, `{}`)
	if err := runRemoteURL(context.Background(), remoteAccessOptions{httpsPort: 9443}, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("logged-out URL error=%v", err)
	}
	writeFakeTailscale(t, binDir, `{"BackendState":"Running","Self":{"HostName":"mac"}}`, `{}`)
	if err := runRemoteURL(context.Background(), remoteAccessOptions{httpsPort: 9443}, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "DNS name") {
		t.Fatalf("missing DNS URL error=%v", err)
	}
	writeFakeTailscale(t, binDir,
		`{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
		`{"Web":{"mac.tail.ts.net:9443":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:9999"}}}}}`,
	)
	stdout.Reset()
	if err := runRemoteStatus(context.Background(), remoteStatusOptions{httpsPort: 9443}, &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "used by another Serve target") {
		t.Fatalf("foreign Serve status=%q", stdout.String())
	}
}

func writeFakeTailscale(t *testing.T, binDir, statusJSON, serveJSON string) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"status\" ]; then\n  printf '%s' '" + statusJSON + "'\n  exit 0\nfi\n" +
		"if [ \"$1\" = \"serve\" ] && [ \"$2\" = \"status\" ]; then\n  printf '%s' '" + serveJSON + "'\n  exit 0\nfi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "tailscale"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}
