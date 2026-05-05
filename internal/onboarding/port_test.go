package onboarding

import (
	"net"
	"strconv"
	"strings"
	"testing"
)

func TestPickFreePort_PrefersFirstFreeCandidate(t *testing.T) {
	port, err := PickFreePort([]int{0, 0, 0})
	if err != nil {
		t.Fatalf("pick free port: %v", err)
	}
	if port <= 0 {
		t.Fatalf("expected positive port, got %d", port)
	}
}

func TestPickFreePort_SkipsBoundPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	bound := listener.Addr().(*net.TCPAddr).Port

	// Find another free port to use as the second candidate so the picker
	// has something to fall back to.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	fallback := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()

	port, err := PickFreePort([]int{bound, fallback})
	if err != nil {
		t.Fatalf("pick free port: %v", err)
	}
	if port == bound {
		t.Fatalf("expected picker to skip bound port %d", bound)
	}
}

func TestPickFreePort_NoFreeReturnsError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	bound := listener.Addr().(*net.TCPAddr).Port

	if _, err := PickFreePort([]int{bound}); err == nil {
		t.Fatal("expected error when no candidate is free")
	}
}

func TestPortRange_InclusiveBounds(t *testing.T) {
	candidates := PortRange(43180, 43182)
	want := []int{43180, 43181, 43182}
	if len(candidates) != len(want) {
		t.Fatalf("expected %d ports, got %d", len(want), len(candidates))
	}
	for i, p := range candidates {
		if p != want[i] {
			t.Fatalf("index %d: expected %d, got %d", i, want[i], p)
		}
	}
}

func TestParseAPIAddrPort_Valid(t *testing.T) {
	port, err := ParseAPIAddrPort("127.0.0.1:43180")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if port != 43180 {
		t.Fatalf("expected 43180, got %d", port)
	}
}

func TestParseAPIAddrPort_Invalid(t *testing.T) {
	cases := []string{"", "not-an-addr", "127.0.0.1", "127.0.0.1:abc"}
	for _, c := range cases {
		if _, err := ParseAPIAddrPort(c); err == nil {
			t.Fatalf("expected error for %q", c)
		}
	}
}

func TestFormatLoopbackAddr(t *testing.T) {
	got := FormatLoopbackAddr(43181)
	if !strings.HasSuffix(got, ":"+strconv.Itoa(43181)) {
		t.Fatalf("expected suffix :43181, got %q", got)
	}
	if !strings.HasPrefix(got, "127.0.0.1") {
		t.Fatalf("expected loopback prefix, got %q", got)
	}
}
