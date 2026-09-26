package checkpoint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// gitRunner runs git for checkpoints. It never inherits the caller's GIT_*
// variables, so a GIT_DIR or GIT_INDEX_FILE left in the server's environment
// cannot point a snapshot at the user's repository. System config is skipped
// and the risky global keys are pinned in each shadow's local config (see
// shadow.go); global config still applies, which keeps core.excludesFile.
type gitRunner struct {
	exe     string
	timeout time.Duration
}

type gitCall struct {
	// gitDir, when set, targets a shadow repository whose work tree is
	// workTree. When empty the call runs against whatever repository dir is
	// in (only read-only commands are issued that way).
	gitDir   string
	workTree string
	dir      string
	stdin    []byte
	args     []string
	// okExit lists non-zero exit codes that are an answer rather than a
	// failure, such as 1 from check-ignore ("not ignored").
	okExit []int
	// patternPaths drops GIT_LITERAL_PATHSPECS for commands that reject it.
	// check-ignore does; it reads its arguments as paths anyway.
	patternPaths bool
}

// gitError keeps the exit code and git's own message.
type gitError struct {
	args   []string
	code   int
	stderr string
	err    error
}

func (e *gitError) Error() string {
	name := "git"
	if len(e.args) > 0 {
		name = "git " + e.args[0]
	}
	if e.stderr != "" {
		return fmt.Sprintf("checkpoint: %s: %s", name, e.stderr)
	}
	return fmt.Sprintf("checkpoint: %s: %v", name, e.err)
}

func (e *gitError) Unwrap() error { return e.err }

const checkpointIdentity = "TARS checkpoint"
const checkpointEmail = "checkpoint@tars.invalid"

func gitEnv(c gitCall) []string {
	env := make([]string, 0, len(os.Environ())+12)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(kv), "GIT_") {
			continue
		}
		env = append(env, kv)
	}
	if !c.patternPaths {
		env = append(env, "GIT_LITERAL_PATHSPECS=1")
	}
	env = append(env,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME="+checkpointIdentity,
		"GIT_AUTHOR_EMAIL="+checkpointEmail,
		"GIT_COMMITTER_NAME="+checkpointIdentity,
		"GIT_COMMITTER_EMAIL="+checkpointEmail,
	)
	if c.gitDir != "" {
		env = append(env, "GIT_DIR="+c.gitDir)
		if c.workTree != "" {
			env = append(env, "GIT_WORK_TREE="+c.workTree)
		}
	}
	return env
}

// run returns stdout and the exit code. A non-zero exit listed in okExit is
// not an error; any other failure comes back as *gitError with git's stderr.
func (r gitRunner) run(ctx context.Context, c gitCall) ([]byte, int, error) {
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, r.exe, c.args...)
	cmd.Dir = c.dir
	cmd.Env = gitEnv(c)
	if c.stdin != nil {
		cmd.Stdin = bytes.NewReader(c.stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err == nil {
		return out, 0, nil
	}
	code := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
		if slices.Contains(c.okExit, code) {
			return out, code, nil
		}
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return out, code, &gitError{args: c.args, code: code, stderr: trimStderr(stderr.String()), err: err}
}

func trimStderr(s string) string {
	s = strings.TrimSpace(s)
	const max = 2000
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// splitNUL splits -z output into its fields, dropping the trailing empty one.
func splitNUL(out []byte) []string {
	if len(out) == 0 {
		return nil
	}
	parts := strings.Split(string(out), "\x00")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
