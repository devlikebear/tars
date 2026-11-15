package tarsserver

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/apihandlers"
	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/rs/zerolog"
)

type fakeKeepAwakeHolder struct {
	mu        sync.Mutex
	supported bool
	held      bool
	holdErr   error
	holds     int
	releases  int
}

func (f *fakeKeepAwakeHolder) Supported() bool { return f.supported }
func (f *fakeKeepAwakeHolder) Held() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.held
}
func (f *fakeKeepAwakeHolder) Hold() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.holds++
	if f.holdErr != nil {
		return f.holdErr
	}
	f.held = true
	return nil
}
func (f *fakeKeepAwakeHolder) Release() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releases++
	f.held = false
}

func newKeepAwakeFixture(t *testing.T, supported bool) (*deskKeepAwake, *fakeKeepAwakeHolder, *bool) {
	t.Helper()
	paired := false
	holder := &fakeKeepAwakeHolder{supported: supported}
	k := &deskKeepAwake{
		store:  consoleauth.NewStore(t.TempDir()),
		holder: holder,
		paired: func() (bool, error) { return paired, nil },
		logger: zerolog.New(io.Discard),
	}
	return k, holder, &paired
}

func TestKeepAwakeHoldsOnlyWhenOnAndAPhoneIsPaired(t *testing.T) {
	k, holder, paired := newKeepAwakeFixture(t, true)
	status := func() apihandlers.KeepAwakeStatus {
		t.Helper()
		s, err := k.KeepAwakeStatus()
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	k.ApplyKeepAwake()
	if holder.Held() || status() != (apihandlers.KeepAwakeStatus{Supported: true}) {
		t.Fatalf("off by default: held=%t %+v", holder.Held(), status())
	}

	if err := k.store.SetKeepAwake(true); err != nil {
		t.Fatal(err)
	}
	k.ApplyKeepAwake()
	if holder.Held() {
		t.Fatal("held with no phone paired")
	}
	if got := status(); got.Reason != "no_phone" || !got.Enabled || got.Active {
		t.Fatalf("on, no phone: %+v", got)
	}

	*paired = true
	k.ApplyKeepAwake()
	k.ApplyKeepAwake() // holding already
	if !holder.Held() || holder.holds != 1 {
		t.Fatalf("on with a phone: held=%t holds=%d", holder.Held(), holder.holds)
	}
	if got := status(); got != (apihandlers.KeepAwakeStatus{Enabled: true, Supported: true, Active: true}) {
		t.Fatalf("holding: %+v", got)
	}

	// The phone is unpaired: let the desk sleep.
	*paired = false
	k.ApplyKeepAwake()
	if holder.Held() || holder.releases != 1 {
		t.Fatalf("no phone again: held=%t releases=%d", holder.Held(), holder.releases)
	}

	// Turned off with a phone paired.
	*paired = true
	k.ApplyKeepAwake()
	if err := k.store.SetKeepAwake(false); err != nil {
		t.Fatal(err)
	}
	k.ApplyKeepAwake()
	if holder.Held() {
		t.Fatal("held after it was turned off")
	}
}

func TestKeepAwakeUnsupportedAndFailedHold(t *testing.T) {
	k, holder, paired := newKeepAwakeFixture(t, false)
	*paired = true
	if err := k.store.SetKeepAwake(true); err != nil {
		t.Fatal(err)
	}
	k.ApplyKeepAwake()
	if holder.holds != 0 {
		t.Fatal("an unsupported system must not try")
	}
	if s, _ := k.KeepAwakeStatus(); s.Reason != "unsupported" || s.Supported {
		t.Fatalf("unsupported: %+v", s)
	}

	k, holder, paired = newKeepAwakeFixture(t, true)
	*paired = true
	holder.holdErr = errors.New("no caffeinate")
	if err := k.store.SetKeepAwake(true); err != nil {
		t.Fatal(err)
	}
	k.ApplyKeepAwake()
	if s, _ := k.KeepAwakeStatus(); s.Reason != "failed" || s.Active {
		t.Fatalf("failed hold: %+v", s)
	}
	// It tries again on the next tick, and the reason clears once it holds.
	holder.holdErr = nil
	k.ApplyKeepAwake()
	if s, _ := k.KeepAwakeStatus(); s.Reason != "" || !s.Active {
		t.Fatalf("held on retry: %+v", s)
	}
}

func TestKeepAwakeReadErrorsChangeNothing(t *testing.T) {
	k, holder, _ := newKeepAwakeFixture(t, true)
	k.paired = func() (bool, error) { return false, errors.New("devices unreadable") }
	if err := k.store.SetKeepAwake(true); err != nil {
		t.Fatal(err)
	}
	k.ApplyKeepAwake()
	if holder.holds != 0 || holder.releases != 0 {
		t.Fatal("an unreadable device list must change nothing")
	}
	if _, err := k.KeepAwakeStatus(); err == nil {
		t.Fatal("status must report the read error")
	}
	var none *deskKeepAwake
	none.ApplyKeepAwake()
	none.start(context.Background())
}

func TestKeepAwakeReleasesOnShutdown(t *testing.T) {
	k, holder, paired := newKeepAwakeFixture(t, true)
	*paired = true
	if err := k.store.SetKeepAwake(true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	k.start(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for !holder.Held() {
		if time.Now().After(deadline) {
			t.Fatal("start did not apply at once")
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	for holder.Held() {
		if time.Now().After(deadline.Add(2 * time.Second)) {
			t.Fatal("shutdown did not release")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestKeepAwakeWanted(t *testing.T) {
	for _, tc := range []struct{ enabled, paired, want bool }{
		{false, false, false}, {true, false, false}, {false, true, false}, {true, true, true},
	} {
		if got := keepAwakeWanted(tc.enabled, tc.paired); got != tc.want {
			t.Errorf("enabled=%t paired=%t: %t", tc.enabled, tc.paired, got)
		}
	}
}
