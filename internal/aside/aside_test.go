package aside

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type fakeRun struct {
	stdout, stderr string
	err            error
	gotBin         string
	gotArgs        []string
}

func (f *fakeRun) run(_ context.Context, bin string, args []string) ([]byte, []byte, error) {
	f.gotBin, f.gotArgs = bin, args
	return []byte(f.stdout), []byte(f.stderr), f.err
}

func fakeBinary(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "aside")
	if err := os.WriteFile(p, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveOrder(t *testing.T) {
	configured := fakeBinary(t)
	fromEnv := fakeBinary(t)
	t.Setenv(PathEnv, fromEnv)

	if got, err := (Client{Path: configured}).Resolve(); err != nil || got != configured {
		t.Fatalf("configured path: got %q, %v", got, err)
	}
	if got, err := (Client{}).Resolve(); err != nil || got != fromEnv {
		t.Fatalf("env path: got %q, %v", got, err)
	}
	if _, err := (Client{Path: filepath.Join(t.TempDir(), "missing")}).Resolve(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing configured path: want ErrNotFound, got %v", err)
	}
}

func TestResolveExpandsHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME is not how Windows finds the user's home directory")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	p := filepath.Join(home, "bin", "aside")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := (Client{Path: "~/bin/aside"}).Resolve(); err != nil || got != p {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestReplAndExecArgsAndColors(t *testing.T) {
	bin := fakeBinary(t)
	f := &fakeRun{stdout: "\x1b[32mhello\x1b[0m\n"}
	c := Client{Path: bin, run: f.run}

	out, err := c.Repl(context.Background(), "console.log(1)")
	if err != nil || out != "hello\n" {
		t.Fatalf("repl: %q, %v", out, err)
	}
	if f.gotBin != bin || strings.Join(f.gotArgs, " ") != "repl console.log(1)" {
		t.Fatalf("repl args: %s %v", f.gotBin, f.gotArgs)
	}

	if _, err := c.Exec(context.Background(), "do it"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.gotArgs, "|") != "exec|--permission|guard|do it" {
		t.Fatalf("exec args: %v", f.gotArgs)
	}
}

func TestCallErrors(t *testing.T) {
	bin := fakeBinary(t)
	f := &fakeRun{stdout: "partial", stderr: "\x1b[31mnot signed in\x1b[0m", err: errors.New("exit status 1")}
	_, err := Client{Path: bin, run: f.run}.Repl(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Fatalf("want stderr in error, got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Client{Path: bin, run: f.run}.Repl(ctx, "x")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context error, got %v", err)
	}

	_, err = Client{Path: filepath.Join(t.TempDir(), "none")}.Repl(context.Background(), "x")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestRunCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell stand-in")
	}
	p := filepath.Join(t.TempDir(), "aside")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho \"$1\"\necho oops >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := runCommand(context.Background(), p, []string{"repl"})
	if err != nil || string(stdout) != "repl\n" || string(stderr) != "oops\n" {
		t.Fatalf("got %q %q %v", stdout, stderr, err)
	}
}

func TestLastLines(t *testing.T) {
	if got := lastLines("a\nb\nc\nd\n", 2); got != "c\nd" {
		t.Fatalf("got %q", got)
	}
}
