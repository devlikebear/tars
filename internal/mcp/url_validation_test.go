package mcp

import (
	"net/url"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestValidateRemoteMCPURL_AcceptsExpectedSchemes(t *testing.T) {
	cases := []struct {
		name      string
		transport string
		raw       string
	}{
		{"http", config.MCPTransportStreamableHTTP, "http://example.com/mcp"},
		{"https", config.MCPTransportStreamableHTTP, "https://example.com/mcp"},
		{"sse-http", config.MCPTransportSSE, "http://example.com/mcp/sse"},
		{"ws", config.MCPTransportWebSocket, "ws://example.com:9000/mcp"},
		{"wss", config.MCPTransportWebSocket, "wss://example.com/mcp"},
		{"loopback-http", config.MCPTransportStreamableHTTP, "http://127.0.0.1:43180/mcp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateRemoteMCPURL(tc.transport, tc.raw); err != nil {
				t.Fatalf("expected %s to be accepted, got %v", tc.raw, err)
			}
		})
	}
}

func TestValidateRemoteMCPURL_RejectsBadInputs(t *testing.T) {
	cases := []struct {
		name      string
		transport string
		raw       string
		expectMsg string
	}{
		{"empty", config.MCPTransportStreamableHTTP, "   ", "required"},
		{"file scheme on http transport", config.MCPTransportStreamableHTTP, "file:///etc/passwd", "scheme"},
		{"gopher scheme", config.MCPTransportStreamableHTTP, "gopher://example.com/foo", "scheme"},
		{"http scheme on websocket transport", config.MCPTransportWebSocket, "http://example.com/mcp", "scheme"},
		{"ws scheme on http transport", config.MCPTransportStreamableHTTP, "ws://example.com/mcp", "scheme"},
		{"no host", config.MCPTransportStreamableHTTP, "http:///path/only", "host"},
		{"stdio transport", config.MCPTransportStdio, "http://example.com/mcp", "does not accept a remote url"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateRemoteMCPURL(tc.transport, tc.raw)
			if err == nil {
				t.Fatalf("expected error for %s", tc.raw)
			}
			if !strings.Contains(err.Error(), tc.expectMsg) {
				t.Fatalf("expected error containing %q, got %q", tc.expectMsg, err.Error())
			}
		})
	}
}

func TestResolveLegacySSEPostURL_AllowsSameOrigin(t *testing.T) {
	base, err := url.Parse("https://mcp.example.com/v1/sse")
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"absolute same origin", "https://mcp.example.com/v1/messages", "https://mcp.example.com/v1/messages"},
		{"relative path", "/v1/messages?session=abc", "https://mcp.example.com/v1/messages?session=abc"},
		{"relative ref", "messages", "https://mcp.example.com/v1/messages"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveLegacySSEPostURL(base, tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

func TestResolveLegacySSEPostURL_RejectsCrossOrigin(t *testing.T) {
	base, err := url.Parse("https://mcp.example.com/v1/sse")
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}
	cases := []struct {
		name string
		raw  string
	}{
		{"different host", "https://attacker.example.net/v1/messages"},
		{"different scheme", "http://mcp.example.com/v1/messages"},
		{"empty", "  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := resolveLegacySSEPostURL(base, tc.raw); err == nil {
				t.Fatalf("expected rejection for %s", tc.raw)
			}
		})
	}
}

func TestAssertSameOriginAsServerURL(t *testing.T) {
	parsed, err := url.Parse("https://mcp.example.com/v1/sse")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ps := &pooledSession{serverURL: parsed}
	if err := assertSameOriginAsServerURL(ps, "https://mcp.example.com/v1/messages"); err != nil {
		t.Fatalf("same origin should pass: %v", err)
	}
	if err := assertSameOriginAsServerURL(ps, "https://attacker.example.net/path"); err == nil {
		t.Fatalf("expected rejection for different host")
	}
	if err := assertSameOriginAsServerURL(&pooledSession{}, "https://mcp.example.com/x"); err == nil {
		t.Fatalf("expected error when session has no validated url")
	}
}

func TestGetOrStartSession_BlocksUnknownStdioCommand(t *testing.T) {
	client := NewClient(nil)
	client.SetServers([]ServerConfig{{
		Name:    "blocked",
		Command: "definitely-not-on-this-path-hopefully",
	}})
	client.SetCommandAllowlist(nil)
	_, err := client.getOrStartSession(t.Context(), ServerConfig{
		Name:      "blocked",
		Command:   "definitely-not-on-this-path-hopefully",
		Transport: config.MCPTransportStdio,
	})
	if err == nil {
		t.Fatalf("expected blocked command error")
	}
	if !strings.Contains(err.Error(), "mcp_command_allowlist_json") {
		t.Fatalf("expected allowlist error, got %v", err)
	}
}

func TestGetOrStartSession_RejectsUnresolvableAllowlistedCommand(t *testing.T) {
	client := NewClient(nil)
	client.SetCommandAllowlist([]string{"definitely-not-on-this-path-hopefully"})
	_, err := client.getOrStartSession(t.Context(), ServerConfig{
		Name:      "broken",
		Command:   "definitely-not-on-this-path-hopefully",
		Transport: config.MCPTransportStdio,
	})
	if err == nil {
		t.Fatalf("expected lookup error")
	}
	if !strings.Contains(err.Error(), "resolve mcp server command") {
		t.Fatalf("expected lookup error, got %v", err)
	}
}

func TestGetOrStartSession_RejectsInvalidRemoteURL(t *testing.T) {
	client := NewClient(nil)
	cases := []struct {
		name      string
		transport string
	}{
		{"http", config.MCPTransportStreamableHTTP},
		{"sse", config.MCPTransportSSE},
		{"ws", config.MCPTransportWebSocket},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.getOrStartSession(t.Context(), ServerConfig{
				Name:      "bad",
				URL:       "file:///etc/passwd",
				Transport: tc.transport,
			})
			if err == nil {
				t.Fatalf("expected url validation error")
			}
			if !strings.Contains(err.Error(), "scheme") {
				t.Fatalf("expected scheme error, got %v", err)
			}
		})
	}
}

func TestDialWebSocket_RequiresParsedURL(t *testing.T) {
	client := NewClient(nil)
	_, err := client.dialWebSocket(t.Context(), ServerConfig{Name: "x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing validated url") {
		t.Fatalf("expected missing-url error, got %v", err)
	}
}

func TestEnsureLegacySSEStream_RequiresValidatedURL(t *testing.T) {
	client := NewClient(nil)
	err := client.ensureLegacySSEStream(t.Context(), &pooledSession{})
	if err == nil || !strings.Contains(err.Error(), "validated server url") {
		t.Fatalf("expected validated-url error, got %v", err)
	}
}

func TestDoHTTPRPC_RejectsCrossOriginEndpoint(t *testing.T) {
	parsed, err := url.Parse("https://mcp.example.com/v1/sse")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	client := NewClient(nil)
	ps := &pooledSession{serverURL: parsed}
	_, err = client.doHTTPRPC(t.Context(), ps, "https://attacker.example.net/path", rpcRequest{ID: 1}, false)
	if err == nil || !strings.Contains(err.Error(), "outside the configured origin") {
		t.Fatalf("expected cross-origin rejection, got %v", err)
	}
}
