package keepawake

import (
	"errors"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// sleeper stands in for caffeinate: a process that runs until killed.
func sleeper(t *testing.T) *Holder {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no sleep command to stand in for the helper")
	}
	h := newWithCommand(func() *exec.Cmd { return exec.Command("sleep", "60") })
	t.Cleanup(h.Release)
	return h
}

func TestHoldAndRelease(t *testing.T) {
	h := sleeper(t)
	if !h.Supported() || h.Held() {
		t.Fatalf("supported=%t held=%t before Hold", h.Supported(), h.Held())
	}
	if err := h.Hold(); err != nil {
		t.Fatal(err)
	}
	first := h.cmd.Process.Pid
	if !h.Held() {
		t.Fatal("not held after Hold")
	}
	// Holding again keeps the same helper.
	if err := h.Hold(); err != nil || h.cmd.Process.Pid != first {
		t.Fatalf("second Hold: err=%v pid %d → %d", err, first, h.cmd.Process.Pid)
	}
	h.Release()
	if h.Held() {
		t.Fatal("held after Release")
	}
	h.Release() // twice is fine
}

func TestHoldAgainAfterTheHelperDies(t *testing.T) {
	h := sleeper(t)
	if err := h.Hold(); err != nil {
		t.Fatal(err)
	}
	_ = h.cmd.Process.Kill()
	deadline := time.Now().Add(3 * time.Second)
	for h.Held() {
		if time.Now().After(deadline) {
			t.Fatal("a killed helper still counts as held")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := h.Hold(); err != nil || !h.Held() {
		t.Fatalf("Hold after the helper died: err=%v held=%t", err, h.Held())
	}
}

func TestUnsupportedAndFailedStart(t *testing.T) {
	var none *Holder
	if none.Supported() || none.Held() {
		t.Fatal("a nil holder supports nothing")
	}
	none.Release()
	if err := (&Holder{}).Hold(); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Hold without a helper = %v", err)
	}
	broken := newWithCommand(func() *exec.Cmd { return exec.Command("/nonexistent/caffeinate") })
	if err := broken.Hold(); err == nil || broken.Held() {
		t.Fatalf("a helper that cannot start: err=%v held=%t", err, broken.Held())
	}
}

func TestNewMatchesTheSystem(t *testing.T) {
	h := New()
	if runtime.GOOS != "darwin" && h.Supported() {
		t.Fatal("only macOS is supported")
	}
	if runtime.GOOS == "darwin" && h.Supported() {
		// The real helper takes the assertion and gives it back.
		if err := h.Hold(); err != nil {
			t.Fatal(err)
		}
		defer h.Release()
		if !h.Held() {
			t.Fatal("caffeinate did not start")
		}
	}
}
