package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/devlikebear/tars/internal/computeruse"
)

// TestMain keeps cmd/tars tests off the network. Commands here talk to
// external hubs (GitHub for openclaw/hermes/anthropic skills) through
// http.DefaultTransport, so a test that forgets to point a source at an
// httptest server would depend on upstream content and fail CI for
// unrelated PRs. The guard refuses every non-loopback dial and fails the
// package even when the code under test swallowed the error.
//
// It also keeps doctor/service-install paths from spawning the developer's
// real cua-driver binary. checkDoctorComputerUse's default probe resolves
// the binary through configured path -> CUA_DRIVER_PATH -> PATH -> known
// install locations and, if found, pings it — starting a real subprocess
// that asynchronously writes $HOME/.cua-driver/.telemetry_id. Tests that set
// HOME to a t.TempDir() race that write against RemoveAll cleanup
// (`TempDir RemoveAll cleanup: directory not empty`, flaky under -count>1).
// Swapping in a probe that never execs anything closes every lookup step at
// once, regardless of what env vars or PATH an individual test sets.
func TestMain(m *testing.M) {
	guard := &loopbackOnlyDialer{}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // dial targets directly so the guard sees them
	transport.DialContext = guard.DialContext
	http.DefaultTransport = transport

	// os.Exit below means this never needs to be restored.
	doctorComputerUseProbeFor = hermeticDoctorComputerUseProbe

	code := m.Run()
	if blocked := guard.blockedHosts(); len(blocked) > 0 {
		fmt.Fprintf(os.Stderr, "cmd/tars tests attempted network access to %s; serve it from httptest instead\n",
			strings.Join(blocked, ", "))
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// hermeticDoctorComputerUseProbe replaces defaultDoctorComputerUseProbe for
// the whole cmd/tars test binary (installed in TestMain). findDriver always
// reports the driver unavailable without touching the configured path, env,
// PATH, or well-known install locations, so no test can resolve — let alone
// exec — a host cua-driver. pingDriver only runs after a successful find, so
// it is unreachable through checkDoctorComputerUse with this probe; it still
// errors instead of pinging anything, in case a future caller reaches it
// some other way.
func hermeticDoctorComputerUseProbe() doctorComputerUseProbe {
	return doctorComputerUseProbe{
		findDriver: func(string) (string, error) {
			return "", fmt.Errorf("%w: cua-driver exec is disabled in cmd/tars tests (see TestMain)", computeruse.ErrDriverUnavailable)
		},
		pingDriver: func(context.Context, string) error {
			return fmt.Errorf("%w: hermeticDoctorComputerUseProbe.pingDriver must never be called in cmd/tars tests", computeruse.ErrDriverUnavailable)
		},
	}
}

type loopbackOnlyDialer struct {
	dialer  net.Dialer
	mu      sync.Mutex
	blocked map[string]struct{}
}

func (d *loopbackOnlyDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if !isLoopbackHost(host) {
		d.mu.Lock()
		if d.blocked == nil {
			d.blocked = map[string]struct{}{}
		}
		d.blocked[addr] = struct{}{}
		d.mu.Unlock()
		return nil, fmt.Errorf("network access to %s is blocked in cmd/tars tests", addr)
	}
	return d.dialer.DialContext(ctx, network, addr)
}

func (d *loopbackOnlyDialer) blockedHosts() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, 0, len(d.blocked))
	for addr := range d.blocked {
		out = append(out, addr)
	}
	sort.Strings(out)
	return out
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func TestLoopbackOnlyDialer_RefusesRemoteHosts(t *testing.T) {
	d := &loopbackOnlyDialer{}
	if _, err := d.DialContext(context.Background(), "tcp", "api.github.com:443"); err == nil {
		t.Fatal("expected remote dial to be refused")
	}
	if got := d.blockedHosts(); len(got) != 1 || got[0] != "api.github.com:443" {
		t.Errorf("blockedHosts = %v, want [api.github.com:443]", got)
	}
	for _, host := range []string{"127.0.0.1", "::1", "localhost"} {
		if !isLoopbackHost(host) {
			t.Errorf("isLoopbackHost(%q) = false", host)
		}
	}
	if isLoopbackHost("10.0.0.1") {
		t.Error("isLoopbackHost(10.0.0.1) = true")
	}
}
