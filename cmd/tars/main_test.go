package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/buildinfo"
	"github.com/devlikebear/tars/internal/tarsserver"
	"github.com/devlikebear/tars/pkg/tarsclient"
)

func TestRootCommand_DoesNotAcceptWorkspaceIDFlag(t *testing.T) {
	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"--workspace-id", "ws-main"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown flag error for --workspace-id")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown flag") {
		t.Fatalf("expected unknown flag error, got %v", err)
	}
}

func TestRootCommand_ServeSubcommandInvokesRunner(t *testing.T) {
	original := serveRunner
	defer func() { serveRunner = original }()

	var got serveOptions
	serveRunner = func(_ context.Context, opts serveOptions, _ io.Writer, _ io.Writer) error {
		got = opts
		return nil
	}

	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{
		"serve",
		"--config", "config/standalone.yaml",
		"--workspace-dir", "./workspace",
		"--api-addr", tarsserver.DefaultAPIAddr,
		"--verbose",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("serve command: %v", err)
	}

	if !got.serveAPI {
		t.Fatalf("expected serveAPI=true, got %#v", got)
	}
	if got.configPath != "config/standalone.yaml" {
		t.Fatalf("unexpected configPath: %#v", got)
	}
	if got.workspaceDir != "./workspace" {
		t.Fatalf("unexpected workspaceDir: %#v", got)
	}
	if got.apiAddr != tarsserver.DefaultAPIAddr {
		t.Fatalf("unexpected apiAddr: %#v", got)
	}
	if got.logFile != ".logs/tars-debug.log" {
		t.Fatalf("unexpected logFile default: %#v", got)
	}
}

func TestRootCommand_ServeDeprecatedFlagsDoNotDisableAPI(t *testing.T) {
	original := serveRunner
	defer func() { serveRunner = original }()

	cases := []struct {
		name string
		args []string
	}{
		{"run-once", []string{"serve", "--run-once"}},
		{"run-loop", []string{"serve", "--run-loop"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got serveOptions
			serveRunner = func(_ context.Context, opts serveOptions, _ io.Writer, _ io.Writer) error {
				got = opts
				return nil
			}

			var stderr strings.Builder
			cmd := newRootCommand(strings.NewReader(""), io.Discard, &stderr)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("serve command: %v", err)
			}
			if !got.serveAPI {
				t.Fatalf("deprecated flag must not disable serveAPI, got serveAPI=%v", got.serveAPI)
			}
			if !strings.Contains(stderr.String(), "deprecated") {
				t.Fatalf("expected deprecation warning on stderr, got %q", stderr.String())
			}
		})
	}
}

func TestRootCommand_AssistantSubcommandInvokesRunner(t *testing.T) {
	original := assistantRunner
	defer func() { assistantRunner = original }()

	var got assistantOptions
	assistantRunner = func(_ context.Context, opts assistantOptions, _ io.Writer, _ io.Writer) error {
		got = opts
		return nil
	}

	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{
		"assistant",
		"start",
		"--server-url", "http://127.0.0.1:43180",
		"--session", "sess_main",
		"--hotkey", "Ctrl+Option+Space",
		"--audio-input", "default",
		"--whisper-bin", "whisper-cli",
		"--whisper-model", "/tmp/ggml-base.bin",
		"--whisper-language", "ko",
		"--ffmpeg-bin", "ffmpeg",
		"--tts-bin", "say",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("assistant start: %v", err)
	}
	if got.serverURL != "http://127.0.0.1:43180" {
		t.Fatalf("unexpected serverURL: %#v", got)
	}
	if got.sessionID != "sess_main" {
		t.Fatalf("unexpected sessionID: %#v", got)
	}
	if got.hotkey != "Ctrl+Option+Space" {
		t.Fatalf("unexpected hotkey: %#v", got)
	}
	if got.audioInput != "default" {
		t.Fatalf("unexpected audioInput: %#v", got)
	}
	if got.whisperModel != "/tmp/ggml-base.bin" {
		t.Fatalf("unexpected whisperModel: %#v", got)
	}
	if got.whisperLanguage != "ko" {
		t.Fatalf("unexpected whisperLanguage: %#v", got)
	}
	if got.whisperBin != "whisper-cli" || got.ffmpegBin != "ffmpeg" || got.ttsBin != "say" {
		t.Fatalf("unexpected binary options: %#v", got)
	}
}

func TestDefaultClientOptions_UsesPkgDefaultWhenEnvMissing(t *testing.T) {
	prevServerURL, hadServerURL := os.LookupEnv("TARS_SERVER_URL")
	defer func() {
		if hadServerURL {
			_ = os.Setenv("TARS_SERVER_URL", prevServerURL)
			return
		}
		_ = os.Unsetenv("TARS_SERVER_URL")
	}()
	_ = os.Unsetenv("TARS_SERVER_URL")

	opts := defaultClientOptions()
	if strings.TrimSpace(opts.serverURL) != tarsclient.DefaultServerURL {
		t.Fatalf("unexpected serverURL default: %q", opts.serverURL)
	}
}

func TestDefaultServeOptions_UsesDefaultLogFile(t *testing.T) {
	opts := defaultServeOptions()
	if opts.logFile != ".logs/tars-debug.log" {
		t.Fatalf("unexpected default log file: %q", opts.logFile)
	}
}

func TestRootCommand_VersionSubcommandPrintsBuildInfo(t *testing.T) {
	prevVersion, prevCommit, prevDate := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	buildinfo.Version = "0.1.0"
	buildinfo.Commit = "abc1234"
	buildinfo.Date = "2026-03-08T00:00:00Z"
	defer func() {
		buildinfo.Version = prevVersion
		buildinfo.Commit = prevCommit
		buildinfo.Date = prevDate
	}()

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	if got != "tars 0.1.0 (abc1234, 2026-03-08T00:00:00Z)" {
		t.Fatalf("unexpected version output: %q", got)
	}
}

func TestRootCommand_VersionFlagPrintsBuildInfo(t *testing.T) {
	prevVersion, prevCommit, prevDate := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	buildinfo.Version = "0.1.0"
	buildinfo.Commit = "abc1234"
	buildinfo.Date = "2026-03-08T00:00:00Z"
	defer func() {
		buildinfo.Version = prevVersion
		buildinfo.Commit = prevCommit
		buildinfo.Date = prevDate
	}()

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version flag: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	if got != "tars 0.1.0 (abc1234, 2026-03-08T00:00:00Z)" {
		t.Fatalf("unexpected version output: %q", got)
	}
}

func TestRootCommand_IncludesMCPSubcommand(t *testing.T) {
	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	if subcmd, _, err := cmd.Find([]string{"mcp"}); err != nil || subcmd == nil || subcmd.Name() != "mcp" {
		t.Fatalf("expected mcp subcommand, got subcmd=%v err=%v", subcmd, err)
	}
}

func TestRootCommand_NoArgsOpensConsole(t *testing.T) {
	originalConsoleRunner := consoleCommandRunner
	originalClientRunner := clientCommandRunner
	defer func() {
		consoleCommandRunner = originalConsoleRunner
		clientCommandRunner = originalClientRunner
	}()

	var got clientOptions
	consoleCalled := false
	clientCalled := false
	consoleCommandRunner = func(_ context.Context, _ io.Writer, _ io.Writer, opts clientOptions) error {
		consoleCalled = true
		got = opts
		return nil
	}
	clientCommandRunner = func(context.Context, io.Reader, io.Writer, io.Writer, clientOptions) error {
		clientCalled = true
		return nil
	}

	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("root command: %v", err)
	}
	if !consoleCalled {
		t.Fatal("expected root command to open console")
	}
	if clientCalled {
		t.Fatal("did not expect root command to launch legacy TUI client")
	}
	if strings.TrimSpace(got.serverURL) != tarsclient.DefaultServerURL {
		t.Fatalf("unexpected serverURL: %q", got.serverURL)
	}
}

func TestRootCommand_MessageFlagUsesClientRunner(t *testing.T) {
	originalConsoleRunner := consoleCommandRunner
	originalClientRunner := clientCommandRunner
	defer func() {
		consoleCommandRunner = originalConsoleRunner
		clientCommandRunner = originalClientRunner
	}()

	consoleCalled := false
	var got clientOptions
	consoleCommandRunner = func(context.Context, io.Writer, io.Writer, clientOptions) error {
		consoleCalled = true
		return nil
	}
	clientCommandRunner = func(_ context.Context, _ io.Reader, _ io.Writer, _ io.Writer, opts clientOptions) error {
		got = opts
		return nil
	}

	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"--message", "hello"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("root command with message: %v", err)
	}
	if consoleCalled {
		t.Fatal("did not expect message mode to open console")
	}
	if got.message != "hello" {
		t.Fatalf("unexpected message: %#v", got)
	}
}

func TestRootCommand_StatusSubcommandUsesRunner(t *testing.T) {
	original := statusCommandRunner
	defer func() { statusCommandRunner = original }()

	var got clientOptions
	statusCommandRunner = func(_ context.Context, _ io.Writer, _ io.Writer, opts clientOptions) error {
		got = opts
		return nil
	}

	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"status", "--server-url", "http://127.0.0.1:43180", "--api-token", "token"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status command: %v", err)
	}
	if got.serverURL != "http://127.0.0.1:43180" || got.apiToken != "token" {
		t.Fatalf("unexpected options: %#v", got)
	}
}

func TestRootCommand_HealthSubcommandUsesRunner(t *testing.T) {
	original := healthCommandRunner
	defer func() { healthCommandRunner = original }()

	var got clientOptions
	healthCommandRunner = func(_ context.Context, _ io.Writer, _ io.Writer, opts clientOptions) error {
		got = opts
		return nil
	}

	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"health", "--server-url", "http://127.0.0.1:43180", "--api-token", "token"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("health command: %v", err)
	}
	if got.serverURL != "http://127.0.0.1:43180" || got.apiToken != "token" {
		t.Fatalf("unexpected options: %#v", got)
	}
}

func TestRootCommand_CronListSubcommandUsesRunner(t *testing.T) {
	original := cronCommandRunner
	defer func() { cronCommandRunner = original }()

	var got cronCommandOptions
	cronCommandRunner = func(_ context.Context, _ io.Writer, _ io.Writer, opts cronCommandOptions) error {
		got = opts
		return nil
	}

	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"cron", "list", "--server-url", "http://127.0.0.1:43180", "--api-token", "token"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cron list command: %v", err)
	}
	if got.action != "list" || got.client.serverURL != "http://127.0.0.1:43180" || got.client.apiToken != "token" {
		t.Fatalf("unexpected options: %#v", got)
	}
}

func TestRootCommand_CronRunSubcommandUsesRunner(t *testing.T) {
	original := cronCommandRunner
	defer func() { cronCommandRunner = original }()

	var got cronCommandOptions
	cronCommandRunner = func(_ context.Context, _ io.Writer, _ io.Writer, opts cronCommandOptions) error {
		got = opts
		return nil
	}

	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"cron", "run", "job_123"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cron run command: %v", err)
	}
	if got.action != "run" || got.jobID != "job_123" {
		t.Fatalf("unexpected options: %#v", got)
	}
}

func TestRootCommand_ApproveRunSubcommandUsesRunner(t *testing.T) {
	original := approvalCommandRunner
	defer func() { approvalCommandRunner = original }()

	var got approvalCommandOptions
	approvalCommandRunner = func(_ context.Context, _ io.Writer, _ io.Writer, opts approvalCommandOptions) error {
		got = opts
		return nil
	}

	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"approve", "run", "approval_123", "--server-url", "http://127.0.0.1:43180"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("approve run command: %v", err)
	}
	if got.action != "run" || got.approvalID != "approval_123" || got.client.serverURL != "http://127.0.0.1:43180" {
		t.Fatalf("unexpected options: %#v", got)
	}
}
