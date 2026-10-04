//go:build !windows

package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExecShellDetachedPipeTimeout(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 required to create a detached writer")
	}
	for _, ending := range []string{"wait", ":"} {
		t.Run(ending, func(t *testing.T) {
			root := t.TempDir()
			args, err := json.Marshal(map[string]any{
				"command":    `python3 -c 'import os,time; os.setsid(); open("child.pid","w").write(str(os.getpid())); print("ready",flush=True); time.sleep(3)' & ` + ending,
				"timeout_ms": 300,
			})
			if err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			result, err := NewExecTool(root).Execute(context.Background(), args)
			elapsed := time.Since(start)
			pidText, readErr := os.ReadFile(filepath.Join(root, "child.pid"))
			if readErr != nil {
				t.Fatalf("detached child did not start: %v; %s", readErr, result.Text())
			}
			pid, errPID := strconv.Atoi(strings.TrimSpace(string(pidText)))
			if errPID != nil {
				t.Fatal(errPID)
			}
			if elapsed < 3*time.Second {
				child, findErr := os.FindProcess(pid)
				if findErr == nil {
					_ = child.Kill()
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			var response execResponse
			if err := json.Unmarshal([]byte(result.Text()), &response); err != nil {
				t.Fatal(err)
			}
			if !response.TimedOut || !result.IsError {
				t.Fatalf("expected timeout: %s", result.Text())
			}
			if elapsed > time.Second {
				t.Fatalf("pipe drain exceeded timeout bound: %s", elapsed)
			}
		})
	}
}

func TestExecShellBackgroundParentExitCleansChildren(t *testing.T) {
	root := t.TempDir()
	manager := NewProcessManager()
	snap, err := manager.Start(context.Background(), root, `sh -c 'printf ready > ready.txt; sleep 0.7; printf survived > survived.txt' >/dev/null 2>&1 & while [ ! -f ready.txt ]; do sleep 0.01; done`, 2000)
	if err != nil {
		t.Fatal(err)
	}
	ended, timedOut, err := manager.Wait(context.Background(), snap.SessionID, 2000)
	if err != nil || timedOut || !ended.Done {
		t.Fatalf("session failed to settle: %+v %v", ended, err)
	}
	time.Sleep(800 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(root, "survived.txt")); !os.IsNotExist(err) {
		t.Fatalf("child outlived completed session: %v", err)
	}
}
