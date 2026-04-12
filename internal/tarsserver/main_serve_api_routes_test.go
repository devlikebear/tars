package tarsserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterAPIRoutes_RegistersCoreRoutes(t *testing.T) {
	mux := http.NewServeMux()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	consoleHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	registerAPIRoutes(mux, apiRouteHandlers{
		pulse:           handler,
		reflection:      handler,
		chat:            handler,
		sessions:        handler,
		memory:          handler,
		console:         consoleHandler,
		usage:           handler,
		ops:             handler,
		status:          handler,
		auth:            handler,
		healthz:         handler,
		providersModels: handler,
		compact:         handler,
		cron:            handler,
		schedules:       handler,
		mcp:             handler,
		extensions:      handler,
		agentRuns:       handler,
		gateway:         handler,
		channels:        handler,
		events:          handler,
		config:          handler,
		skillhub:        handler,
		filesystem:      handler,
		workspaceFiles:  handler,
	})

	paths := []string{
		"/v1/pulse/status",
		"/v1/pulse/run-once",
		"/v1/pulse/config",
		"/v1/reflection/status",
		"/v1/reflection/run-once",
		"/v1/reflection/config",
		"/v1/chat",
		"/v1/sessions",
		"/v1/sessions/main",
		"/v1/admin/sessions",
		"/v1/admin/sessions/main",
		"/v1/memory/kb/notes",
		"/v1/memory/kb/notes/coffee-preference",
		"/v1/memory/kb/graph",
		"/v1/memory/assets",
		"/v1/memory/file",
		"/v1/memory/search",
		"/v1/workspace/sysprompt/files",
		"/v1/workspace/sysprompt/file",
		"/console",
		"/console/",
		"/v1/usage/summary",
		"/v1/usage/limits",
		"/v1/ops/status",
		"/v1/ops/cleanup/plan",
		"/v1/ops/cleanup/apply",
		"/v1/ops/approvals",
		"/v1/ops/approvals/123",
		"/v1/status",
		"/v1/auth/whoami",
		"/v1/healthz",
		"/v1/providers",
		"/v1/models",
		"/v1/compact",
		"/v1/cron/jobs",
		"/v1/cron/jobs/job-1",
		"/v1/schedules",
		"/v1/schedules/main",
		"/v1/mcp/servers",
		"/v1/mcp/tools",
		"/v1/skills",
		"/v1/skills/default",
		"/v1/plugins",
		"/v1/runtime/extensions/reload",
		"/v1/runtime/extensions/disabled",
		"/v1/agent/agents",
		"/v1/agent/runs",
		"/v1/agent/runs/run-1",
		"/v1/gateway/status",
		"/v1/gateway/reload",
		"/v1/gateway/restart",
		"/v1/gateway/reports/summary",
		"/v1/gateway/reports/runs",
		"/v1/gateway/reports/channels",
		"/v1/channels/webhook/inbound/general",
		"/v1/channels/telegram/webhook/bot-1",
		"/v1/channels/telegram/send",
		"/v1/channels/telegram/pairings",
		"/v1/channels/telegram/pairings/default",
		"/v1/events/stream",
		"/v1/events/history",
		"/v1/events/read",
		"/v1/admin/config",
		"/v1/admin/config/values",
		"/v1/admin/config/schema",
		"/v1/admin/reset/workspace",
		"/v1/admin/restart",
		"/v1/hub/registry",
		"/v1/hub/installed",
		"/v1/hub/install",
		"/v1/hub/uninstall",
		"/v1/hub/update",
		"/v1/hub/skill-content",
		"/v1/filesystem/browse",
		"/v1/workspace/files",
	}
	for _, path := range paths {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		mux.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("expected registered route, got 404 for %s", path)
		}
	}
}
