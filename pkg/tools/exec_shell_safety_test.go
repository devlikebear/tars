package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecShellBlocked(t *testing.T) {
	for _, command := range []string{"echo ok; rm unused", "echo ok | sudo true", "'rm' unused", "$(echo rm) unused", "sh -c 'rm unused'"} {
		t.Run(command, func(t *testing.T) {
			args, err := json.Marshal(map[string]any{"command": command})
			if err != nil {
				t.Fatal(err)
			}
			r, err := NewExecTool(t.TempDir()).Execute(context.Background(), args)
			if err != nil || !r.IsError {
				t.Fatalf("expected rejection: %s %v", r.Text(), err)
			}
		})
	}
}

func TestExecShellCancellation(t *testing.T) {
	requirePOSIXShell(t)
	for _, background := range []bool{false, true} {
		for _, cancelNow := range []bool{false, true} {
			t.Run(fmt.Sprintf("background=%t/cancel=%t", background, cancelNow), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				timeoutMS := 100
				if cancelNow {
					timeoutMS = 5000
					timer := time.AfterFunc(100*time.Millisecond, cancel)
					defer timer.Stop()
				}
				root := t.TempDir()
				manager := NewProcessManager()
				args, err := json.Marshal(map[string]any{"command": "sh -c 'sleep 0.6; printf survived > survived.txt' & wait", "timeout_ms": timeoutMS, "background": background})
				if err != nil {
					t.Fatal(err)
				}
				start := time.Now()
				r, err := NewExecToolWithManager(root, manager).Execute(ctx, args)
				if err != nil {
					t.Fatal(err)
				}
				var body execResponse
				if err := json.Unmarshal([]byte(r.Text()), &body); err != nil {
					t.Fatal(err)
				}
				if background {
					snap, timeout, err := manager.Wait(context.Background(), body.SessionID, 2000)
					if err != nil || timeout || !snap.Done || snap.ExitCode == 0 {
						t.Fatalf("wait: %+v %v", snap, err)
					}
				} else if !r.IsError || body.TimedOut == cancelNow {
					t.Fatalf("wrong cancellation result: %s", r.Text())
				}
				if time.Since(start) > time.Second {
					t.Fatal("pipeline exceeded cancellation bound")
				}
				time.Sleep(650 * time.Millisecond)
				if _, err := os.Stat(filepath.Join(root, "survived.txt")); !os.IsNotExist(err) {
					t.Fatalf("descendant survived cancellation: %v", err)
				}
			})
		}
	}
}

func TestExecShellBackgroundKill(t *testing.T) {
	requirePOSIXShell(t)
	root := t.TempDir()
	manager := NewProcessManager()
	snap, err := manager.Start(context.Background(), root, "sh -c 'printf ready > ready.txt; sleep 0.6; printf survived > survived.txt' & wait", 5000)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "ready.txt")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := manager.Kill(snap.SessionID); err != nil {
		t.Fatal(err)
	}
	ended, timedOut, err := manager.Wait(context.Background(), snap.SessionID, 2000)
	if err != nil || timedOut || !ended.Done || ended.ExitCode == 0 {
		t.Fatalf("kill did not settle process: %+v %v", ended, err)
	}
	time.Sleep(650 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(root, "survived.txt")); !os.IsNotExist(err) {
		t.Fatalf("descendant survived kill: %v", err)
	}
}
