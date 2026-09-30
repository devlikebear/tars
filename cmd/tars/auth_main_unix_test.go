//go:build unix

package main

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestOnInterruptRestoresTerminalBeforeExit(t *testing.T) {
	var order []string
	exited := make(chan struct{})
	origExit := exitInterrupted
	t.Cleanup(func() { exitInterrupted = origExit })
	exitInterrupted = func() {
		order = append(order, "exit")
		close(exited)
	}

	stop := onInterrupt(func() { order = append(order, "restore") })
	defer stop()
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatalf("interrupt was not handled")
	}
	if len(order) != 2 || order[0] != "restore" || order[1] != "exit" {
		t.Fatalf("expected restore then exit, got %v", order)
	}
}
