package tarsserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterBrowserRoutes_RegistersBrowserRoutes(t *testing.T) {
	mux := http.NewServeMux()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	registerBrowserRoutes(mux, handler)

	paths := []string{
		"/v1/browser/status",
		"/v1/browser/profiles",
		"/v1/browser/login",
		"/v1/browser/check",
		"/v1/browser/run",
		"/v1/vault/status",
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

func TestRegisterAPIRoutes_RegistersCoreRoutes(t *testing.T) {
	mux := http.NewServeMux()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	registerAPIRoutes(mux, apiRouteHandlers{
		heartbeat:       handler,
		chat:            handler,
		sessions:        handler,
		projects:        handler,
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
		browser:         handler,
		channels:        handler,
		events:          handler,
	})

	paths := []string{
		"/v1/heartbeat/ws-main",
		"/v1/chat",
		"/v1/sessions",
		"/v1/sessions/main",
		"/v1/projects",
		"/v1/projects/demo",
		"/v1/projects/demo/state",
		"/v1/project-briefs/demo",
		"/v1/project-briefs/demo/finalize",
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
		"/v1/agent/agents",
		"/v1/agent/runs",
		"/v1/agent/runs/run-1",
		"/v1/gateway/status",
		"/v1/gateway/reload",
		"/v1/gateway/restart",
		"/v1/gateway/reports/summary",
		"/v1/gateway/reports/runs",
		"/v1/gateway/reports/channels",
		"/v1/browser/status",
		"/v1/browser/profiles",
		"/v1/browser/login",
		"/v1/browser/check",
		"/v1/browser/run",
		"/v1/vault/status",
		"/v1/channels/webhook/inbound/general",
		"/v1/channels/telegram/webhook/bot-1",
		"/v1/channels/telegram/send",
		"/v1/channels/telegram/pairings",
		"/v1/channels/telegram/pairings/default",
		"/v1/events/stream",
		"/v1/events/history",
		"/v1/events/read",
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
