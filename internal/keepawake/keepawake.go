// Package keepawake keeps the computer from sleeping while it is plugged
// in, so a paired phone can still reach this desktop
// (docs/decisions/mobile-remote.md, "A device that sleeps is unreachable").
//
// On macOS it runs the system's own caffeinate with -s (prevent system
// sleep, honored only on AC power) and -w <this process>, so the assertion
// ends with the server even if the server dies without releasing it. The
// display still sleeps. Other systems are not supported yet.
package keepawake

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// ErrUnsupported is returned by Hold on a system with no way to keep awake.
var ErrUnsupported = errors.New("keeping the computer awake is not supported on this system")

const caffeinatePath = "/usr/bin/caffeinate"

// releaseWait bounds how long Release waits for the helper to exit.
const releaseWait = 3 * time.Second

// Holder holds or releases one keep-awake assertion. Safe for concurrent use.
type Holder struct {
	// command builds the helper process; nil means unsupported.
	command func() *exec.Cmd

	mu   sync.Mutex
	cmd  *exec.Cmd
	done chan struct{}
}

// New returns the holder for this system.
func New() *Holder {
	if runtime.GOOS != "darwin" {
		return &Holder{}
	}
	if _, err := os.Stat(caffeinatePath); err != nil {
		return &Holder{}
	}
	pid := strconv.Itoa(os.Getpid())
	return &Holder{command: func() *exec.Cmd {
		return exec.Command(caffeinatePath, "-s", "-w", pid)
	}}
}

// newWithCommand is New with the helper replaced (tests).
func newWithCommand(command func() *exec.Cmd) *Holder {
	return &Holder{command: command}
}

// Supported reports whether this system can be kept awake.
func (h *Holder) Supported() bool {
	return h != nil && h.command != nil
}

// Held reports whether the assertion is held now: the helper runs.
func (h *Holder) Held() bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.runningLocked()
}

func (h *Holder) runningLocked() bool {
	if h.cmd == nil {
		return false
	}
	select {
	case <-h.done:
		return false
	default:
		return true
	}
}

// Hold takes the assertion; holding it already is not an error. A helper
// that exited on its own (killed from outside) is started again.
func (h *Holder) Hold() error {
	if !h.Supported() {
		return ErrUnsupported
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.runningLocked() {
		return nil
	}
	cmd := h.command()
	if err := cmd.Start(); err != nil {
		h.cmd, h.done = nil, nil
		return err
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	h.cmd, h.done = cmd, done
	return nil
}

// Release lets the computer sleep again; not holding is not an error.
func (h *Holder) Release() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.runningLocked() {
		h.cmd, h.done = nil, nil
		return
	}
	_ = h.cmd.Process.Kill()
	select {
	case <-h.done:
	case <-time.After(releaseWait):
	}
	h.cmd, h.done = nil, nil
}
