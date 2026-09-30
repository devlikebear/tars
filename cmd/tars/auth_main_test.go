package main

import (
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/consoleauth"
)

func TestRootCommand_IncludesAuthSubcommand(t *testing.T) {
	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	if subcmd, _, err := cmd.Find([]string{"auth"}); err != nil || subcmd == nil || subcmd.Name() != "auth" {
		t.Fatalf("expected auth subcommand, got subcmd=%v err=%v", subcmd, err)
	}
}

func TestAuthInitCreatesAdminPasswordFromFlag(t *testing.T) {
	workspace := t.TempDir()
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"auth", "init", "--workspace-dir", workspace, "--password", "admin secret"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth init: %v", err)
	}
	if !strings.Contains(stdout.String(), "admin account initialized") {
		t.Fatalf("expected init success output, got %q", stdout.String())
	}
	ok, err := consoleauth.NewStore(workspace).VerifyPassword(consoleauth.RoleAdmin, "admin secret")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatalf("expected stored admin password to verify")
	}
}

func TestAuthInitUsesInitialAdminPasswordEnv(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("TARS_INITIAL_ADMIN_PASSWORD", "env admin secret")
	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"auth", "init", "--workspace-dir", workspace})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth init: %v", err)
	}
	ok, err := consoleauth.NewStore(workspace).VerifyPassword(consoleauth.RoleAdmin, "env admin secret")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatalf("expected env admin password to verify")
	}
}

func TestAuthPasswdChangesUserPassword(t *testing.T) {
	workspace := t.TempDir()
	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"auth", "passwd", "user", "--workspace-dir", workspace, "--password", "user secret"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth passwd: %v", err)
	}
	ok, err := consoleauth.NewStore(workspace).VerifyPassword(consoleauth.RoleUser, "user secret")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatalf("expected stored user password to verify")
	}
}

func TestAuthPairingCodeCreatesOneTimeUserCode(t *testing.T) {
	workspace := t.TempDir()
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"auth", "pairing-code", "--workspace-dir", workspace, "--role", "user", "--ttl", "5m"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth pairing-code: %v", err)
	}
	code := regexp.MustCompile(`\b\d{6}\b`).FindString(stdout.String())
	if code == "" {
		t.Fatalf("expected 6-digit code in output, got %q", stdout.String())
	}
	used, ok, err := consoleauth.NewStore(workspace, consoleauth.WithNow(func() time.Time {
		return time.Now().UTC()
	})).ConsumePairingCode(code)
	if err != nil {
		t.Fatalf("ConsumePairingCode: %v", err)
	}
	if !ok || used.Role != consoleauth.RoleUser {
		t.Fatalf("expected user pairing code, got=%+v ok=%v", used, ok)
	}
}

func TestAuthPasswdReadsPasswordFromPipedStdin(t *testing.T) {
	workspace := t.TempDir()
	var stderr strings.Builder
	cmd := newRootCommand(strings.NewReader("piped secret\r\n"), io.Discard, &stderr)
	cmd.SetArgs([]string{"auth", "passwd", "admin", "--workspace-dir", workspace})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth passwd: %v", err)
	}
	if stderr.String() != "Password: " {
		t.Fatalf("expected a single prompt without confirmation, got %q", stderr.String())
	}
	ok, err := consoleauth.NewStore(workspace).VerifyPassword(consoleauth.RoleAdmin, "piped secret")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatalf("expected piped password to verify")
	}
}

func TestAuthInitReadsPasswordFromStdinWithoutTrailingNewline(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("TARS_INITIAL_ADMIN_PASSWORD", "")
	cmd := newRootCommand(strings.NewReader("no newline secret"), io.Discard, io.Discard)
	cmd.SetArgs([]string{"auth", "init", "--workspace-dir", workspace})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth init: %v", err)
	}
	ok, err := consoleauth.NewStore(workspace).VerifyPassword(consoleauth.RoleAdmin, "no newline secret")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatalf("expected stdin password to verify")
	}
}

func TestAuthPasswdRejectsEmptyStdinPassword(t *testing.T) {
	for _, input := range []string{"", "\n", "   \r\n"} {
		workspace := t.TempDir()
		cmd := newRootCommand(strings.NewReader(input), io.Discard, io.Discard)
		cmd.SetArgs([]string{"auth", "passwd", "user", "--workspace-dir", workspace})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "password is required") {
			t.Fatalf("input %q: expected password is required, got %v", input, err)
		}
		if has, _ := consoleauth.NewStore(workspace).HasPassword(consoleauth.RoleUser); has {
			t.Fatalf("input %q: expected no password to be stored", input)
		}
	}
}

func TestStdinTerminalFdTreatsFilesAndReadersAsNonTerminal(t *testing.T) {
	if _, ok := stdinTerminalFd(strings.NewReader("x")); ok {
		t.Fatalf("expected in-memory reader to be non-terminal")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	if _, ok := stdinTerminalFd(r); ok {
		t.Fatalf("expected pipe to be non-terminal")
	}
}

// fakeTerminal drives the terminal branch of resolveAuthPassword without a
// real TTY: every read returns the next queued line.
func fakeTerminal(t *testing.T, lines ...string) *int {
	t.Helper()
	origFd, origRead := stdinTerminalFd, readTerminalPassword
	t.Cleanup(func() { stdinTerminalFd, readTerminalPassword = origFd, origRead })
	reads := 0
	stdinTerminalFd = func(io.Reader) (int, bool) { return 42, true }
	readTerminalPassword = func(fd int) ([]byte, error) {
		if fd != 42 {
			t.Fatalf("unexpected fd %d", fd)
		}
		if reads >= len(lines) {
			return nil, io.EOF
		}
		line := lines[reads]
		reads++
		return []byte(line), nil
	}
	return &reads
}

func TestResolveAuthPasswordTerminalAsksTwiceWithoutEcho(t *testing.T) {
	reads := fakeTerminal(t, "tty secret", "tty secret")
	var stderr strings.Builder

	got, err := resolveAuthPassword(strings.NewReader("must not be read\n"), &stderr, "", "")
	if err != nil {
		t.Fatalf("resolveAuthPassword: %v", err)
	}
	if got != "tty secret" {
		t.Fatalf("expected tty secret, got %q", got)
	}
	if *reads != 2 {
		t.Fatalf("expected two no-echo reads, got %d", *reads)
	}
	if want := "Password: \nConfirm password: \n"; stderr.String() != want {
		t.Fatalf("expected prompts followed by newlines %q, got %q", want, stderr.String())
	}
}

func TestResolveAuthPasswordTerminalRejectsMismatch(t *testing.T) {
	fakeTerminal(t, "first", "second")

	_, err := resolveAuthPassword(nil, io.Discard, "", "")
	if err == nil || !strings.Contains(err.Error(), "passwords do not match") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}

func TestResolveAuthPasswordTerminalRejectsEmptyWithoutConfirming(t *testing.T) {
	reads := fakeTerminal(t, "  ", "  ")

	_, err := resolveAuthPassword(nil, io.Discard, "", "")
	if err == nil || !strings.Contains(err.Error(), "password is required") {
		t.Fatalf("expected password is required, got %v", err)
	}
	if *reads != 1 {
		t.Fatalf("expected no confirmation prompt after an empty password, got %d reads", *reads)
	}
}

func TestResolveAuthPasswordTerminalReportsReadError(t *testing.T) {
	fakeTerminal(t)
	var stderr strings.Builder

	_, err := resolveAuthPassword(nil, &stderr, "", "")
	if err == nil || !strings.Contains(err.Error(), "read password") {
		t.Fatalf("expected read error, got %v", err)
	}
	if stderr.String() != "Password: \n" {
		t.Fatalf("expected the newline to be written even on error, got %q", stderr.String())
	}
}

func TestResolveAuthPasswordPrefersFlagOverTerminal(t *testing.T) {
	reads := fakeTerminal(t, "unused", "unused")

	got, err := resolveAuthPassword(nil, io.Discard, "flag secret", "")
	if err != nil || got != "flag secret" {
		t.Fatalf("expected flag secret, got %q err=%v", got, err)
	}
	if *reads != 0 {
		t.Fatalf("expected no terminal reads when --password is set, got %d", *reads)
	}
}

func TestResolveAuthPasswordTerminalReportsConfirmReadError(t *testing.T) {
	fakeTerminal(t, "only once")

	_, err := resolveAuthPassword(nil, nil, "", "")
	if err == nil || !strings.Contains(err.Error(), "read password") {
		t.Fatalf("expected confirmation read error, got %v", err)
	}
}

// failAfterWriter accepts n writes and fails every later one.
type failAfterWriter struct{ n int }

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.n <= 0 {
		return 0, errors.New("stderr closed")
	}
	w.n--
	return len(p), nil
}

func TestResolveAuthPasswordReportsPromptWriteErrors(t *testing.T) {
	if _, err := resolveAuthPassword(strings.NewReader("secret\n"), &failAfterWriter{}, "", ""); err == nil || !strings.Contains(err.Error(), "stderr closed") {
		t.Fatalf("expected piped prompt write error, got %v", err)
	}

	fakeTerminal(t, "secret", "secret")
	if _, err := resolveAuthPassword(nil, &failAfterWriter{}, "", ""); err == nil || !strings.Contains(err.Error(), "stderr closed") {
		t.Fatalf("expected terminal prompt write error, got %v", err)
	}
	if _, err := resolveAuthPassword(nil, &failAfterWriter{n: 1}, "", ""); err == nil || !strings.Contains(err.Error(), "stderr closed") {
		t.Fatalf("expected newline write error, got %v", err)
	}
}

func TestReadPasswordNoEchoRejectsNonTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	if _, err := readPasswordNoEcho(int(r.Fd())); err == nil {
		t.Fatalf("expected an error for a pipe descriptor")
	}
}

func TestAuthPasswdRejectsUnknownRole(t *testing.T) {
	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"auth", "passwd", "root", "--workspace-dir", t.TempDir(), "--password", "x"})

	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unsupported role") {
		t.Fatalf("expected unsupported role error, got %v", err)
	}
}

func TestOnInterruptStopDoesNotRestore(t *testing.T) {
	restored := make(chan struct{}, 1)
	stop := onInterrupt(func() { restored <- struct{}{} })
	stop()
	select {
	case <-restored:
		t.Fatalf("expected no restore without an interrupt")
	case <-time.After(50 * time.Millisecond):
	}
}
