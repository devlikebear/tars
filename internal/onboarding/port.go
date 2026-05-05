// Package onboarding holds helpers used by `tars init` (and its alias
// `tars onboard`) to bring a fresh install all the way from "no config"
// to a running server with the setup wizard open in a browser.
package onboarding

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// DefaultPortRangeStart is the first port the onboarding picker tries
// when the user has not pinned an address. It matches
// tarsserver.DefaultAPIAddr so existing service plists keep working.
const DefaultPortRangeStart = 43180

// DefaultPortRangeEnd bounds the auto-pick scan. 20 ports is enough for
// a developer machine that already runs a few local servers; beyond
// that the user should pass --port explicitly.
const DefaultPortRangeEnd = 43199

// PickFreePort returns the first candidate that can be bound on the
// loopback interface. A candidate of 0 asks the kernel to assign any
// free port (useful in tests). The returned port is the actual port,
// not the candidate value. Returns an error if every candidate is
// busy.
func PickFreePort(candidates []int) (int, error) {
	if len(candidates) == 0 {
		return 0, fmt.Errorf("no port candidates supplied")
	}
	var lastErr error
	for _, candidate := range candidates {
		if candidate < 0 || candidate > 65535 {
			lastErr = fmt.Errorf("port %d out of range", candidate)
			continue
		}
		addr := "127.0.0.1:" + strconv.Itoa(candidate)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			lastErr = err
			continue
		}
		port := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
		return port, nil
	}
	return 0, fmt.Errorf("no free port among %d candidate(s): %w", len(candidates), lastErr)
}

// PortRange returns an inclusive list of ports between start and end.
func PortRange(start, end int) []int {
	if end < start {
		return nil
	}
	out := make([]int, 0, end-start+1)
	for p := start; p <= end; p++ {
		out = append(out, p)
	}
	return out
}

// ParseAPIAddrPort extracts the port number from a host:port string.
// The host portion is ignored. Returns an error if the address is
// malformed or the port is not a valid integer.
func ParseAPIAddrPort(addr string) (int, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return 0, fmt.Errorf("address is empty")
	}
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, fmt.Errorf("split host:port %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("port %q: %w", portStr, err)
	}
	return port, nil
}

// FormatLoopbackAddr returns the canonical "127.0.0.1:<port>" string
// the rest of TARS uses for the API listener.
func FormatLoopbackAddr(port int) string {
	return "127.0.0.1:" + strconv.Itoa(port)
}
