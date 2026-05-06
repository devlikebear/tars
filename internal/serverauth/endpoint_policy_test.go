package serverauth

import (
	"net/http"
	"testing"
)

func TestEndpointAllowsRole_UserAllowlistIsFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{name: "chat post is allowed", method: http.MethodPost, path: "/v1/chat", want: true},
		{name: "chat helper route is allowed", method: http.MethodGet, path: "/v1/chat/mentions/files", want: true},
		{name: "logout is allowed", method: http.MethodPost, path: "/v1/auth/logout", want: true},
		{name: "user password self-change is allowed", method: http.MethodPatch, path: "/v1/auth/users/user/password", want: true},
		{name: "admin password change is admin only", method: http.MethodPatch, path: "/v1/auth/users/admin/password", want: false},
		{name: "session admin-prefix exception is allowed", method: http.MethodGet, path: "/v1/admin/sessions/main/history", want: true},
		{name: "session config read is allowed", method: http.MethodGet, path: "/v1/admin/sessions/main/config", want: true},
		{name: "session config mutation is admin only", method: http.MethodPatch, path: "/v1/admin/sessions/main/config", want: false},
		{name: "session local config write is admin only", method: http.MethodPatch, path: "/v1/admin/sessions/main/local-config", want: false},
		{name: "session workdir write is admin only", method: http.MethodPut, path: "/v1/admin/sessions/main/workdirs", want: false},
		{name: "global tasks admin-prefix exception is allowed", method: http.MethodGet, path: "/v1/admin/tasks", want: true},
		{name: "config remains admin only", method: http.MethodGet, path: "/v1/admin/config", want: false},
		{name: "config schema remains admin only", method: http.MethodGet, path: "/v1/admin/config/schema", want: false},
		{name: "remote access status remains admin only", method: http.MethodGet, path: "/v1/admin/remote-access/status", want: false},
		{name: "remote access enable remains admin only", method: http.MethodPost, path: "/v1/admin/remote-access/enable", want: false},
		{name: "terminal remains admin only", method: http.MethodPost, path: "/v1/terminal/open", want: false},
		{name: "git inspector remains admin only", method: http.MethodGet, path: "/v1/git/status", want: false},
		{name: "absolute filesystem browse remains admin only", method: http.MethodGet, path: "/v1/filesystem/browse", want: false},
		{name: "workspace-scoped files are allowed", method: http.MethodGet, path: "/v1/workspace/files", want: true},
		{name: "hub install remains admin only", method: http.MethodPost, path: "/v1/hub/install", want: false},
		{name: "hub registry read is allowed", method: http.MethodGet, path: "/v1/hub/registry", want: true},
		{name: "unknown endpoint is closed to user", method: http.MethodGet, path: "/v1/new/adminish", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EndpointAllowsRole(tc.method, tc.path, RoleUser); got != tc.want {
				t.Fatalf("expected user allowed=%v for %s %s, got %v", tc.want, tc.method, tc.path, got)
			}
		})
	}
}

func TestEndpointAllowsRole_MethodSensitiveMutations(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{name: "subagent detail read is allowed", method: http.MethodGet, path: "/v1/agentruntime/subagents/researcher", want: true},
		{name: "subagent tier mutation is not user allowed", method: http.MethodPatch, path: "/v1/agentruntime/subagents/researcher", want: false},
		{name: "subagent builder draft is not user allowed", method: http.MethodPost, path: "/v1/agentruntime/subagents/builder/draft", want: false},
		{name: "agent run event stream is allowed", method: http.MethodGet, path: "/v1/agentruntime/runs/run-1/events", want: true},
		{name: "agent run restart is not user allowed", method: http.MethodPost, path: "/v1/agentruntime/runs/run-1/restart", want: false},
		{name: "extension health stays admin only despite read shape", method: http.MethodGet, path: "/v1/runtime/extensions/health", want: false},
		{name: "event read acknowledgement is allowed", method: http.MethodPost, path: "/v1/events/read", want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EndpointAllowsRole(tc.method, tc.path, RoleUser); got != tc.want {
				t.Fatalf("expected user allowed=%v for %s %s, got %v", tc.want, tc.method, tc.path, got)
			}
		})
	}
}

func TestEndpointAllowsRole_AdminMayAccessEveryEndpoint(t *testing.T) {
	for _, path := range []string{
		"/v1/chat",
		"/v1/admin/config",
		"/v1/terminal/open",
		"/v1/new/adminish",
	} {
		t.Run(path, func(t *testing.T) {
			if !EndpointAllowsRole(http.MethodPost, path, RoleAdmin) {
				t.Fatalf("expected admin to be allowed for %s", path)
			}
		})
	}
}

func TestResolveEndpointAccessClassifiesPolicy(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   EndpointAccess
	}{
		{method: http.MethodGet, path: "/v1/healthz", want: EndpointAccessPublic},
		{method: http.MethodGet, path: "/v1/auth/whoami", want: EndpointAccessUser},
		{method: http.MethodPost, path: "/v1/hub/install", want: EndpointAccessAdmin},
		{method: http.MethodGet, path: "/v1/unknown", want: EndpointAccessAdmin},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			if got := ResolveEndpointAccess(tc.method, tc.path); got != tc.want {
				t.Fatalf("expected access %q, got %q", tc.want, got)
			}
		})
	}
}
