//go:build !windows

package focusprobe

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/execsandbox"
)

// shRunner runs a check with sh, unsandboxed: the replay's own logic under
// test, not the sandbox.
func shRunner(ctx context.Context, dir, command string) (ReplayRun, error) {
	return RunReplayCommand(ctx, dir, "/bin/sh", []string{"-c", command})
}

// state writes the given files, as a recorded work tree would.
func state(files map[string]string) ReplayState {
	return func(_ context.Context, dest string) error {
		for name, content := range files {
			path := filepath.Join(dest, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return err
			}
		}
		return nil
	}
}

func TestReplay(t *testing.T) {
	repo, base := focusReviewRepo(t)
	defect := state(map[string]string{"app/x.go": "err := f() // ignored\n"})
	fix := state(map[string]string{"app/x.go": "if err := f(); err != nil { return err }\n"})
	check := []ReplayFile{{Path: "scripts/no-ignored.sh", Content: "#!/bin/sh\n! grep -rq '// ignored' app\n"}}
	req := ReplayRequest{Repo: repo, Base: base, Defect: defect, Fix: fix, Files: check, Command: "./scripts/no-ignored.sh", Run: shRunner}

	res := Replay(context.Background(), req)
	if res.Reason != ReplayVerified || res.Fix.ExitCode != 0 || res.Defect.ExitCode != 1 {
		t.Fatalf("verified: %+v fix=%+v defect=%+v", res, res.Fix, res.Defect)
	}
	// The clone sees the pipeline's change as a diff from the base, and
	// the repository itself is untouched.
	req.Command = "git diff --quiet HEAD -- b.go && test ! -e b.go && git rev-parse HEAD"
	req.Files = nil
	if res := Replay(context.Background(), req); res.Reason != ReplayFailsOnFix {
		// b.go is in the base but not in the snapshot: deleted.
		t.Fatalf("git-aware: %+v", res)
	}
	if out := gitRun(t, repo, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		t.Fatalf("the repository changed: %q", out)
	}

	for _, c := range []struct {
		name, command, want string
	}{
		{"fails on fix", "exit 2", ReplayFailsOnFix},
		{"passes on defect", "true", ReplayPassesOnDefect},
		{"unreproducible", "no-such-program-tars", ReplayUnreproducible},
		{"unreproducible on defect", "test -e .fixed || no-such-program-tars", ReplayUnreproducible},
	} {
		r := req
		r.Command = c.command
		r.Fix = state(map[string]string{".fixed": "1"})
		if res := Replay(context.Background(), r); res.Reason != c.want {
			t.Errorf("%s: %+v", c.name, res)
		}
	}

	prev := ReplayRunTimeout
	ReplayRunTimeout = 200 * time.Millisecond
	t.Cleanup(func() { ReplayRunTimeout = prev })
	for _, c := range []struct {
		name string
		fix  ReplayState
	}{
		{"fix", fix},
		{"defect", state(map[string]string{".fixed": "1"})},
	} {
		r := req
		r.Fix = c.fix
		r.Command = "test -e .fixed || sleep 5"
		if c.name == "fix" {
			r.Command = "sleep 5"
		}
		if res := Replay(context.Background(), r); res.Reason != ReplayTimeout {
			t.Errorf("timeout on %s: %+v", c.name, res)
		}
	}
	ReplayRunTimeout = prev

	broken := func(context.Context, string) error { return errors.New("gone") }
	for name, r := range map[string]ReplayRequest{
		"no repo":       {Repo: filepath.Join(t.TempDir(), "none"), Fix: fix, Defect: defect, Command: "true", Run: shRunner},
		"bad base":      {Repo: repo, Base: "0123456789abcdef", Fix: fix, Defect: defect, Command: "true", Run: shRunner},
		"fix snapshot":  {Repo: repo, Fix: broken, Defect: defect, Command: "true", Run: shRunner},
		"defect state":  {Repo: repo, Fix: fix, Defect: broken, Command: "true", Run: shRunner},
		"no runner":     {Repo: repo, Fix: fix, Defect: defect, Command: "true"},
		"escaping file": {Repo: repo, Fix: fix, Defect: defect, Command: "true", Run: shRunner, Files: []ReplayFile{{Path: "../x", Content: "x"}}},
	} {
		if res := Replay(context.Background(), r); res.Reason != ReplaySetup {
			t.Errorf("%s: %+v", name, res)
		}
	}
}

func TestRunReplayCommand(t *testing.T) {
	dir := t.TempDir()
	prev := ReplayOutputLimit
	ReplayOutputLimit = 8
	t.Cleanup(func() { ReplayOutputLimit = prev })
	t.Setenv("GIT_DIR", "/nowhere")
	run, err := RunReplayCommand(context.Background(), dir, "/bin/sh", []string{"-c", `printf 0123456789abcdef; test -z "$GIT_DIR" && test "$CI" = 1 && exit 3`})
	if err != nil || run.ExitCode != 3 || run.Output != "89abcdef" {
		t.Fatalf("%+v %v", run, err)
	}
	if _, err := RunReplayCommand(context.Background(), dir, filepath.Join(dir, "missing"), nil); err == nil {
		t.Fatal("a program that cannot start is an error")
	}
}

func TestRemoveTreeReadOnly(t *testing.T) {
	dir := t.TempDir()
	ro := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(ro, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ro, "f"), []byte("x"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := removeTree(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("left: %v", err)
	}
}

// The real sandbox, where this host has one: the clone is writable, the
// person's home is not.
func TestSandboxRunner(t *testing.T) {
	backend, err := execsandbox.Detect()
	if err == nil {
		err = backend.Probe(context.Background())
	}
	if err != nil {
		t.Skipf("no OS sandbox: %v", err)
	}
	run := SandboxRunner(backend)
	dir := t.TempDir()
	res, err := run(context.Background(), dir, "echo ok > inside && test -f inside")
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("inside: %+v %v", res, err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home")
	}
	outside := filepath.Join(home, ".tars-replay-sandbox-test")
	t.Cleanup(func() { _ = os.Remove(outside) })
	if res, err := run(context.Background(), dir, "echo x > "+outside); err != nil || res.ExitCode == 0 {
		t.Fatalf("wrote outside the sandbox: %+v %v", res, err)
	}
}
