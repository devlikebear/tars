package focusprobe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/execsandbox"
	"github.com/devlikebear/tars/internal/shellexec"
)

// The verify promotion's replay (focuspipeline/promotion.go): the server's
// own proof that a proposed check catches a recurring defect. The check
// runs twice, each time in a fresh temporary clone of the repository whose
// work tree is replaced by a recorded snapshot — the work tree right after
// the turn that fixed the defect (the check must pass) and right before it
// (the check must fail). The clone shares the repository's objects
// (`git clone --shared`) but has its own .git, so nothing the check does
// can reach the repository's refs, index or work tree; under the OS sandbox
// (SandboxRunner) only the clone and build caches are writable, credentials
// are hidden and the network is off.
//
// This is the one place in focusprobe that runs a command other than git
// and gh, and it never runs it in the person's checkout.

// Replay outcomes (ReplayResult.Reason).
const (
	ReplayVerified = "verified"
	// ReplaySetup: the clone or a snapshot could not be prepared.
	ReplaySetup = "setup"
	// ReplayFailsOnFix: the check fails on the fixed work tree too — it
	// would fail every pipeline (or needs something the clone lacks).
	ReplayFailsOnFix = "fails_on_fix"
	// ReplayUnreproducible: the shell could not run the check's program
	// (exit 126 or 127), most often a dependency the clone lacks.
	ReplayUnreproducible = "unreproducible"
	// ReplayPassesOnDefect: the check passes on the defective work tree —
	// it does not catch the defect.
	ReplayPassesOnDefect = "passes_on_defect"
	// ReplayTimeout: a run did not finish in time.
	ReplayTimeout = "timeout"
)

// ReplayRunTimeout bounds one run of a check; ReplayOutputLimit caps the
// output kept from it (its tail).
var (
	ReplayRunTimeout  = 5 * time.Minute
	ReplayOutputLimit = 16 << 10
)

// ReplayRun is one run of a check.
type ReplayRun struct {
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out,omitempty"`
	Output   string `json:"-"`
}

// ReplayRunner runs command, a shell line, with dir as its working folder.
type ReplayRunner func(ctx context.Context, dir, command string) (ReplayRun, error)

// ReplayState writes one recorded work tree into dest (an empty folder
// inside the clone's work tree).
type ReplayState func(ctx context.Context, dest string) error

// ReplayFile is a file the check adds, relative to the top level.
type ReplayFile struct {
	Path    string
	Content string
}

// ReplayRequest is one check to confirm.
type ReplayRequest struct {
	// Repo is the repository's main checkout, Base a commit in it the
	// clone's HEAD is set to ("" keeps the checkout's HEAD) so git-aware
	// checks see the pipeline's change as a diff.
	Repo string
	Base string
	// Defect and Fix write the work trees before and after the fix.
	Defect, Fix ReplayState
	Files       []ReplayFile
	Command     string
	Run         ReplayRunner
}

// ReplayResult is how a replay ended; Fix and Defect are the runs made.
type ReplayResult struct {
	Reason string     `json:"reason"`
	Fix    *ReplayRun `json:"fix,omitempty"`
	Defect *ReplayRun `json:"defect,omitempty"`
}

// Replay confirms a check: it must pass on the fixed work tree and fail on
// the defective one. The fixed side runs first: a check that fails there is
// rejected without a second run.
func Replay(ctx context.Context, req ReplayRequest) ReplayResult {
	var res ReplayResult
	fix, err := replayOnce(ctx, req, req.Fix)
	if err != nil {
		res.Reason = ReplaySetup
		return res
	}
	res.Fix = &fix
	switch {
	case fix.TimedOut:
		res.Reason = ReplayTimeout
		return res
	case fix.ExitCode == 126 || fix.ExitCode == 127:
		res.Reason = ReplayUnreproducible
		return res
	case fix.ExitCode != 0:
		res.Reason = ReplayFailsOnFix
		return res
	}
	defect, err := replayOnce(ctx, req, req.Defect)
	if err != nil {
		res.Reason = ReplaySetup
		return res
	}
	res.Defect = &defect
	switch {
	case defect.TimedOut:
		res.Reason = ReplayTimeout
	case defect.ExitCode == 126 || defect.ExitCode == 127:
		res.Reason = ReplayUnreproducible
	case defect.ExitCode == 0:
		res.Reason = ReplayPassesOnDefect
	default:
		res.Reason = ReplayVerified
	}
	return res
}

// replayCloneTimeout bounds the clone and the reset.
const replayCloneTimeout = 2 * time.Minute

func replayOnce(ctx context.Context, req ReplayRequest, state ReplayState) (ReplayRun, error) {
	if state == nil || req.Run == nil {
		return ReplayRun{}, errors.New("replay: nothing to run")
	}
	tmp, err := os.MkdirTemp("", "tars-verify-replay-")
	if err != nil {
		return ReplayRun{}, err
	}
	defer func() { _ = removeTree(tmp) }()
	dir := filepath.Join(tmp, "tree")
	if _, err := RunGit(ctx, replayCloneTimeout, tmp, nil, "clone", "--quiet", "--shared", "--no-checkout", "--", req.Repo, dir); err != nil {
		return ReplayRun{}, fmt.Errorf("replay: clone: %w", err)
	}
	if base := strings.TrimSpace(req.Base); base != "" && IsGitObjectID(base) {
		// Index at the base, work tree left empty for the snapshot.
		if _, err := RunGit(ctx, replayCloneTimeout, dir, nil, "reset", "--quiet", base); err != nil {
			return ReplayRun{}, fmt.Errorf("replay: reset: %w", err)
		}
	}
	if err := state(ctx, dir); err != nil {
		return ReplayRun{}, fmt.Errorf("replay: snapshot: %w", err)
	}
	for _, f := range req.Files {
		target := filepath.Join(dir, filepath.FromSlash(f.Path))
		if rel, err := filepath.Rel(dir, target); err != nil || strings.HasPrefix(rel, "..") {
			return ReplayRun{}, fmt.Errorf("replay: file %q leaves the tree", f.Path)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return ReplayRun{}, err
		}
		mode := os.FileMode(0o644)
		if strings.HasPrefix(f.Content, "#!") {
			mode = 0o755
		}
		// Replace, not write through: the snapshot may hold a link there.
		_ = os.Remove(target)
		if err := os.WriteFile(target, []byte(f.Content), mode); err != nil {
			return ReplayRun{}, err
		}
	}
	return req.Run(ctx, dir, req.Command)
}

// removeTree removes a replay's folder, making read-only folders a check
// left behind (a Go module download, say) writable first.
func removeTree(dir string) error {
	if err := os.RemoveAll(dir); err == nil {
		return nil
	}
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() {
			_ = os.Chmod(p, 0o755)
		}
		return nil
	})
	return os.RemoveAll(dir)
}

// SandboxRunner runs a check under the OS sandbox: the clone (and the build
// caches execsandbox allows) writable, credentials hidden, no network.
func SandboxRunner(backend execsandbox.Backend) ReplayRunner {
	return func(ctx context.Context, dir, command string) (ReplayRun, error) {
		shell, err := shellexec.Executable()
		if err != nil {
			return ReplayRun{}, err
		}
		policy := execsandbox.Resolve(execsandbox.Options{Dirs: []string{dir}})
		name, args := backend.Command(policy, dir, []string{shell, "-c", command})
		return RunReplayCommand(ctx, dir, name, args)
	}
}

// RunReplayCommand runs name with args in dir within ReplayRunTimeout,
// keeping the tail of its output. A non-zero exit is a result, not an
// error; only a program that cannot start is.
func RunReplayCommand(ctx context.Context, dir, name string, args []string) (ReplayRun, error) {
	ctx, cancel := context.WithTimeout(ctx, ReplayRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...) // NOSONAR: the check's command was validated, replayed in a throwaway clone under the OS sandbox.
	cmd.Dir = dir
	cmd.Env = replayEnv()
	out := &tailWriter{limit: ReplayOutputLimit}
	cmd.Stdout, cmd.Stderr = out, out
	cmd.WaitDelay = KillGrace
	KillProcessGroupOnCancel(cmd, KillGrace)
	err := cmd.Run()
	run := ReplayRun{Output: out.String()}
	if ctx.Err() != nil {
		run.TimedOut, run.ExitCode = true, -1
		return run, nil
	}
	if err == nil {
		return run, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		run.ExitCode = exitErr.ExitCode()
		return run, nil
	}
	return run, err
}

// replayEnv is the server's environment without GIT_* (a GIT_DIR left in it
// would point git at another repository), never prompting.
func replayEnv() []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(kv), "GIT_") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GIT_TERMINAL_PROMPT=0", "CI=1")
}

// tailWriter keeps the last limit bytes written to it.
type tailWriter struct {
	limit int
	buf   []byte
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	if over := len(w.buf) - w.limit; over > 0 {
		w.buf = append([]byte(nil), w.buf[over:]...)
	}
	return len(p), nil
}

func (w *tailWriter) String() string { return string(w.buf) }
