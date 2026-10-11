//go:build windows

package onboarding

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The detached server runs console programs of its own (the claude CLI, the
// shell behind the exec tool, git, gh). Windows gives a console program that
// has no console to inherit a new one, in a new window, so a server started
// without a console flashes a terminal window for every one of them. These
// tests start a stand-in for the server the way SpawnDetached does and ask a
// child of it whose console it ended up on.

const (
	consoleHelperEnv = "TARS_ONBOARDING_CONSOLE_HELPER"
	consoleOutEnv    = "TARS_ONBOARDING_CONSOLE_OUT"
)

var procGetConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

// consoleProcessIDs lists the processes attached to this process's console.
func consoleProcessIDs() []uint32 {
	ids := make([]uint32, 64)
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	if n == 0 || int(n) > len(ids) {
		return nil
	}
	return ids[:n]
}

// TestConsoleHelper is not a test: it is the body of the two helper processes.
// "server" stands in for the detached server and starts "child" the way the
// server starts its own programs, with no creation flags; "child" reports
// whether it shares a console with the process that started it.
func TestConsoleHelper(t *testing.T) {
	switch os.Getenv(consoleHelperEnv) {
	case "server":
		cmd := exec.Command(os.Args[0], "-test.run=^TestConsoleHelper$")
		cmd.Env = append(os.Environ(), consoleHelperEnv+"=child")
		if err := cmd.Run(); err != nil {
			t.Fatalf("run child: %v", err)
		}
	case "child":
		shared := false
		parent := uint32(os.Getppid())
		for _, id := range consoleProcessIDs() {
			if id == parent {
				shared = true
			}
		}
		out := fmt.Sprintf("shared=%t", shared)
		if err := os.WriteFile(os.Getenv(consoleOutEnv), []byte(out), 0o600); err != nil {
			t.Fatalf("write result: %v", err)
		}
	default:
		t.Skip("helper process only")
	}
}

// childSharesServerConsole starts the stand-in server with configure applied
// and returns what its child reported.
func childSharesServerConsole(t *testing.T, configure func(*exec.Cmd)) bool {
	t.Helper()
	out := t.TempDir() + `\result.txt`
	cmd := exec.Command(os.Args[0], "-test.run=^TestConsoleHelper$")
	cmd.Env = append(os.Environ(), consoleHelperEnv+"=server", consoleOutEnv+"="+out)
	configure(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server stand-in: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server stand-in: %v", err)
		}
	case <-time.After(2 * time.Minute):
		_ = cmd.Process.Kill()
		t.Fatal("server stand-in did not finish")
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	return strings.TrimSpace(string(data)) == "shared=true"
}

func TestConfigureDetachedProcessGivesChildrenAConsoleToInherit(t *testing.T) {
	// The control shows the check can tell the two cases apart: a server
	// started with DETACHED_PROCESS has no console, so its child gets its own.
	detachedOnly := func(cmd *exec.Cmd) {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
		}
	}
	if childSharesServerConsole(t, detachedOnly) {
		t.Fatal("control: a child of a DETACHED_PROCESS server reported a shared console")
	}

	if !childSharesServerConsole(t, configureDetachedProcess) {
		t.Fatal("a child of the detached server got a console of its own, which Windows shows as a new terminal window")
	}
}

func TestConfigureDetachedProcessFlags(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	configureDetachedProcess(cmd)
	flags := cmd.SysProcAttr.CreationFlags
	if flags&windows.CREATE_NO_WINDOW == 0 {
		t.Errorf("CreationFlags %#x lack CREATE_NO_WINDOW", flags)
	}
	// Windows ignores CREATE_NO_WINDOW when DETACHED_PROCESS is also set.
	if flags&windows.DETACHED_PROCESS != 0 {
		t.Errorf("CreationFlags %#x include DETACHED_PROCESS, which leaves the server without a console", flags)
	}
	if flags&windows.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Errorf("CreationFlags %#x lack CREATE_NEW_PROCESS_GROUP", flags)
	}
}
