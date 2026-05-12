package tarsserver

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestLoopbackCookieBoundary_GateIsOnlyOpenForLoopback is the security pin for
// CodeQL alerts #100 and #101 (go/cookie-secure-not-set).
//
// writeSessionCookie and writeClearSessionCookie each have a Secure: false
// branch. That branch is reachable ONLY when evaluateLoginRequest returns
// CookieSecure: false, which it does only for `isLocalLoginHost(r.Host)` —
// i.e. Host header == 127.0.0.1 / localhost / ::1 (with or without a port).
// If a request reaches an HTTP listener bound to a non-loopback interface,
// evaluateLoginRequest sets Allowed: false with "secure connection required"
// and the handler never invokes writeSessionCookie at all.
//
// This test pins that invariant: any Host that is NOT a loopback literal
// MUST either be rejected outright or be granted Secure: true. There is no
// third option that would let a non-Secure cookie escape to a public origin.
func TestLoopbackCookieBoundary_GateIsOnlyOpenForLoopback(t *testing.T) {
	loopbackHosts := []string{
		"127.0.0.1",
		"127.0.0.1:43180",
		"localhost",
		"localhost:43180",
		"[::1]",
		"[::1]:43180",
		"LOCALHOST", // case-insensitive
	}
	for _, host := range loopbackHosts {
		host := host
		t.Run("loopback/"+host, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
			req.Host = host
			req.RemoteAddr = "127.0.0.1:55555"
			gate := evaluateLoginRequest(req)
			if !gate.Allowed {
				t.Fatalf("loopback host %q should be allowed, got reason=%q", host, gate.Reason)
			}
			if gate.CookieSecure {
				t.Fatalf("loopback host %q should set CookieSecure=false, got true", host)
			}
		})
	}

	// Remote hosts: every non-loopback Host header must either be rejected
	// (Allowed=false) OR upgraded to Secure cookies (CookieSecure=true).
	// The non-Secure cookie branch must be unreachable in all of these.
	remoteCases := []struct {
		name string
		host string
		tls  bool
	}{
		{name: "public ip plain http", host: "203.0.113.10", tls: false},
		{name: "public ip with port", host: "203.0.113.10:8080", tls: false},
		{name: "public dns plain http", host: "example.com", tls: false},
		{name: "tailscale tagged plain http", host: "tars.tailnet.ts.net", tls: false},
		{name: "private rfc1918 plain http", host: "10.0.0.5", tls: false},
		{name: "public ip https", host: "203.0.113.10", tls: true},
		{name: "public dns https", host: "example.com", tls: true},
	}
	for _, tc := range remoteCases {
		tc := tc
		t.Run("remote/"+tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
			req.Host = tc.host
			req.RemoteAddr = "203.0.113.10:55555"
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			gate := evaluateLoginRequest(req)
			if gate.Allowed && !gate.CookieSecure {
				t.Fatalf("non-loopback host %q allowed with CookieSecure=false — non-Secure cookie could escape loopback", tc.host)
			}
		})
	}

	// Nil request must NOT be allowed — defense in depth against a future
	// caller that forgets to pass r.
	if gate := evaluateLoginRequest(nil); gate.Allowed {
		t.Fatalf("nil request should not be allowed")
	}
}

// TestLoopbackCookieBoundary_ClearMirrorsLogin pins that
// shouldClearSecureCookie's Secure decision matches evaluateLoginRequest, so
// the logout cookie cannot end up Secure: false on a host where the login
// cookie was Secure: true (which would prevent browser deletion and strand
// a Secure session cookie on the user). The reverse — Secure: true on a
// loopback host — is harmless because browsers honor the bit on first-party
// HTTP only via the Secure-context relaxation that loopback gets.
func TestLoopbackCookieBoundary_ClearMirrorsLogin(t *testing.T) {
	cases := []struct {
		name string
		host string
		want bool
	}{
		{name: "loopback ipv4", host: "127.0.0.1", want: false},
		{name: "loopback localhost", host: "localhost:43180", want: false},
		{name: "loopback ipv6", host: "[::1]:43180", want: false},
		{name: "public over https", host: "example.com", want: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
			req.Host = tc.host
			req.RemoteAddr = "127.0.0.1:55555"
			if tc.want {
				req.TLS = &tls.ConnectionState{}
			}
			if got := shouldClearSecureCookie(req); got != tc.want {
				t.Fatalf("shouldClearSecureCookie(%q tls=%v) = %v, want %v", tc.host, tc.want, got, tc.want)
			}
		})
	}
}
