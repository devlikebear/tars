package sentinel

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type processExit struct {
	pid      int
	exitCode int
	err      error
}

func startTargetProcess(opts Options) (*exec.Cmd, int, <-chan processExit, error) {
	command := strings.TrimSpace(opts.TargetCommand)
	if command == "" {
		return nil, 0, nil, fmt.Errorf("target command is required")
	}
	cmd := exec.Command(command, opts.TargetArgs...)
	if wd := strings.TrimSpace(opts.TargetWorkingDir); wd != "" {
		cmd.Dir = wd
	}
	env := append([]string{}, os.Environ()...)
	for key, value := range opts.TargetEnv {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		env = append(env, k+"="+value)
	}
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return nil, 0, nil, err
	}
	exitCh := make(chan processExit, 1)
	go func() {
		err := cmd.Wait()
		exitCode := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok && ee.ProcessState != nil {
				exitCode = ee.ProcessState.ExitCode()
			}
		} else if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		exitCh <- processExit{pid: cmd.Process.Pid, exitCode: exitCode, err: err}
		close(exitCh)
	}()
	return cmd, cmd.Process.Pid, exitCh, nil
}

func stopTargetProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
	}
	return nil
}

func waitProcessExit(ctx context.Context, exitCh <-chan processExit) (processExit, bool) {
	if exitCh == nil {
		return processExit{}, false
	}
	select {
	case <-ctx.Done():
		return processExit{}, false
	case ex, ok := <-exitCh:
		if !ok {
			return processExit{}, false
		}
		return ex, true
	}
}
