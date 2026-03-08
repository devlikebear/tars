package tarsclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSpawnCommand(t *testing.T) {
	cmd, err := parseSpawnCommand("--agent researcher --title nightly --session s-1 --wait investigate latency")
	if err != nil {
		t.Fatalf("parseSpawnCommand: %v", err)
	}
	if cmd.Agent != "researcher" || cmd.Title != "nightly" || cmd.SessionID != "s-1" || !cmd.Wait {
		t.Fatalf("unexpected parsed command: %+v", cmd)
	}
	if cmd.Message != "investigate latency" {
		t.Fatalf("expected message parsed, got %q", cmd.Message)
	}
}

func TestParseProfileFlag(t *testing.T) {
	profile, err := parseProfileFlag([]string{"--profile", "chrome"})
	if err != nil {
		t.Fatalf("parseProfileFlag: %v", err)
	}
	if profile != "chrome" {
		t.Fatalf("expected chrome profile, got %q", profile)
	}
	profile, err = parseProfileFlag([]string{"--profile=managed"})
	if err != nil {
		t.Fatalf("parseProfileFlag with equals: %v", err)
	}
	if profile != "managed" {
		t.Fatalf("expected managed profile, got %q", profile)
	}
	if _, err := parseProfileFlag([]string{"--unknown"}); err == nil {
		t.Fatalf("expected unknown option error")
	}
}

func TestRuntimeClientEndpoints(t *testing.T) {
	adminAuth := ""
	normalAuth := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions":
			normalAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "s1", "title": "chat"}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s2", "title": "new-chat"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/s1/history":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"role": "user", "content": "hello"}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions/s1/export":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("# Session export"))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/sessions/search"):
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "s1", "title": "chat"}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"workspace_dir": "/tmp/ws", "session_count": 2})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/healthz":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "component": "tars", "time": "2026-02-19T00:00:00Z"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/compact":
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "compaction complete"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/heartbeat/run-once":
			_ = json.NewEncoder(w).Encode(map[string]any{"response": "ok", "skipped": false})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/skills":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"name": "coder", "user_invocable": true}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/plugins":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "p1", "source": "workspace"}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/mcp/servers":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"name": "fs", "connected": true, "tool_count": 2}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/mcp/tools":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"server": "fs", "name": "read"}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runtime/extensions/reload":
			adminAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{"reloaded": true, "version": 2, "skills": 1, "plugins": 1, "mcp_count": 1, "gateway_refreshed": true, "gateway_agents": 3})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agent/agents":
			normalAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 1, "agents": []map[string]any{{"name": "default"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/agent/runs":
			_ = json.NewEncoder(w).Encode(map[string]any{"run_id": "r1", "status": "accepted", "accepted": true})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/agent/runs"):
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 1, "runs": []map[string]any{{"run_id": "r1", "status": "running", "accepted": true}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/gateway/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":                   true,
				"version":                   1,
				"channels_local_enabled":    true,
				"channels_webhook_enabled":  false,
				"channels_telegram_enabled": true,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/browser/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"running":     true,
				"profile":     "managed",
				"driver":      "chromedp",
				"last_action": "snapshot",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/browser/profiles":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"count": 1,
				"profiles": []map[string]any{
					{"name": "managed", "driver": "playwright", "default": true, "running": true},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/browser/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"site_id": "portal", "profile": "managed", "mode": "manual", "success": true, "message": "manual login required"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/browser/check":
			_ = json.NewEncoder(w).Encode(map[string]any{"site_id": "portal", "profile": "managed", "check_count": 1, "passed": true, "message": "ok"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/browser/run":
			_ = json.NewEncoder(w).Encode(map[string]any{"site_id": "portal", "profile": "managed", "action": "ping", "step_count": 2, "success": true, "message": "ok"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/vault/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"enabled": true, "ready": true, "auth_mode": "token", "addr": "http://127.0.0.1:8200", "allowlist_count": 2})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/gateway/reload":
			adminAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{"enabled": true, "version": 2})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/gateway/reports/summary":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"generated_at":       "2026-02-19T00:00:00Z",
				"summary_enabled":    true,
				"archive_enabled":    false,
				"runs_total":         4,
				"runs_active":        1,
				"runs_by_status":     map[string]any{"running": 1, "completed": 3},
				"channels_total":     2,
				"messages_total":     7,
				"messages_by_source": map[string]any{"webhook": 5, "telegram": 2},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/gateway/reports/runs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"generated_at":    "2026-02-19T00:00:01Z",
				"archive_enabled": false,
				"count":           1,
				"runs": []map[string]any{
					{"run_id": "r1", "status": "completed"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/gateway/reports/channels":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"generated_at":    "2026-02-19T00:00:02Z",
				"archive_enabled": false,
				"count":           1,
				"messages": map[string]any{
					"general": []map[string]any{
						{"id": "m1", "channel_id": "general", "source": "webhook", "direction": "inbound", "text": "hello", "timestamp": "2026-02-19T00:00:02Z"},
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/cron/jobs":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "job_1", "name": "daily", "prompt": "check", "schedule": "every:1h", "enabled": true}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/cron/jobs":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job_2", "name": "", "prompt": "check logs", "schedule": "every:1h", "enabled": true})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/cron/jobs/job_2":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job_2", "name": "", "prompt": "check logs", "schedule": "every:1h", "enabled": true})
		case r.Method == http.MethodPut && r.URL.Path == "/v1/cron/jobs/job_2":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job_2", "name": "", "prompt": "check logs", "schedule": "every:1h", "enabled": false})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/cron/jobs/job_2/run":
			_ = json.NewEncoder(w).Encode(map[string]any{"response": "cron ok"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/cron/jobs/job_2/runs":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"job_id": "job_2", "ran_at": "2026-02-18T10:00:00Z", "response": "cron ok"}})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/cron/jobs/job_2":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/events/history":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": 1, "type": "notification", "category": "cron", "severity": "info", "title": "cron-1", "message": "done", "timestamp": "2026-02-19T01:00:00Z"},
				},
				"unread_count": 1,
				"read_cursor":  0,
				"last_id":      1,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/events/read":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"acknowledged": true,
				"read_cursor":  1,
				"unread_count": 0,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := runtimeClient{
		serverURL:     server.URL,
		apiToken:      "user-token",
		adminAPIToken: "admin-token",
	}
	ctx := context.Background()
	sessions, err := client.listSessions(ctx)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("listSessions: sessions=%+v err=%v", sessions, err)
	}
	created, err := client.createSession(ctx, "new-chat")
	if err != nil || created.ID == "" {
		t.Fatalf("createSession: created=%+v err=%v", created, err)
	}
	history, err := client.getHistory(ctx, "s1")
	if err != nil || len(history) != 1 {
		t.Fatalf("getHistory: history=%+v err=%v", history, err)
	}
	exported, err := client.exportSession(ctx, "s1")
	if err != nil || !strings.Contains(exported, "Session export") {
		t.Fatalf("exportSession: exported=%q err=%v", exported, err)
	}
	found, err := client.searchSessions(ctx, "chat")
	if err != nil || len(found) != 1 {
		t.Fatalf("searchSessions: found=%+v err=%v", found, err)
	}
	status, err := client.status(ctx)
	if err != nil || status.WorkspaceDir == "" {
		t.Fatalf("status: status=%+v err=%v", status, err)
	}
	if health, err := client.healthz(ctx); err != nil || !health.OK {
		t.Fatalf("healthz: health=%+v err=%v", health, err)
	}
	if _, err := client.compact(ctx, "s1"); err != nil {
		t.Fatalf("compact: %v", err)
	}
	if _, err := client.heartbeatRunOnce(ctx); err != nil {
		t.Fatalf("heartbeatRunOnce: %v", err)
	}
	if skills, err := client.listSkills(ctx); err != nil || len(skills) != 1 {
		t.Fatalf("listSkills: skills=%+v err=%v", skills, err)
	}
	if plugins, err := client.listPlugins(ctx); err != nil || len(plugins) != 1 {
		t.Fatalf("listPlugins: plugins=%+v err=%v", plugins, err)
	}
	if servers, err := client.listMCPServers(ctx); err != nil || len(servers) != 1 {
		t.Fatalf("listMCPServers: servers=%+v err=%v", servers, err)
	}
	if tools, err := client.listMCPTools(ctx); err != nil || len(tools) != 1 {
		t.Fatalf("listMCPTools: tools=%+v err=%v", tools, err)
	}
	if _, err := client.reloadExtensions(ctx); err != nil {
		t.Fatalf("reloadExtensions: %v", err)
	}
	agents, err := client.listAgents(ctx)
	if err != nil {
		t.Fatalf("listAgents: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "default" {
		t.Fatalf("unexpected agents: %+v", agents)
	}
	_, err = client.spawnRun(ctx, agentSpawnRequest{Message: "hello"})
	if err != nil {
		t.Fatalf("spawnRun: %v", err)
	}
	_, err = client.listRuns(ctx, 10)
	if err != nil {
		t.Fatalf("listRuns: %v", err)
	}
	_, err = client.gatewayStatus(ctx)
	if err != nil {
		t.Fatalf("gatewayStatus: %v", err)
	}
	if browserStatus, err := client.browserStatus(ctx); err != nil || !browserStatus.Running {
		t.Fatalf("browserStatus: status=%+v err=%v", browserStatus, err)
	}
	if profiles, err := client.browserProfiles(ctx); err != nil || len(profiles) != 1 {
		t.Fatalf("browserProfiles: profiles=%+v err=%v", profiles, err)
	}
	if _, err := client.browserLogin(ctx, "portal", "managed"); err != nil {
		t.Fatalf("browserLogin: %v", err)
	}
	if _, err := client.browserCheck(ctx, "portal", "managed"); err != nil {
		t.Fatalf("browserCheck: %v", err)
	}
	if _, err := client.browserRun(ctx, "portal", "ping", "managed"); err != nil {
		t.Fatalf("browserRun: %v", err)
	}
	if vault, err := client.vaultStatus(ctx); err != nil || !vault.Enabled {
		t.Fatalf("vaultStatus: vault=%+v err=%v", vault, err)
	}
	if summary, err := client.gatewayReportSummary(ctx); err != nil || summary.RunsTotal != 4 {
		t.Fatalf("gatewayReportSummary: summary=%+v err=%v", summary, err)
	}
	if runs, err := client.gatewayReportRuns(ctx, 20); err != nil || runs.Count != 1 {
		t.Fatalf("gatewayReportRuns: runs=%+v err=%v", runs, err)
	}
	if channels, err := client.gatewayReportChannels(ctx, 20); err != nil || channels.Count != 1 {
		t.Fatalf("gatewayReportChannels: channels=%+v err=%v", channels, err)
	}
	if jobs, err := client.listCronJobs(ctx); err != nil || len(jobs) != 1 {
		t.Fatalf("listCronJobs: jobs=%+v err=%v", jobs, err)
	}
	if _, err := client.createCronJob(ctx, "every:1h", "check logs"); err != nil {
		t.Fatalf("createCronJob: %v", err)
	}
	if _, err := client.getCronJob(ctx, "job_2"); err != nil {
		t.Fatalf("getCronJob: %v", err)
	}
	if _, err := client.updateCronJobEnabled(ctx, "job_2", false); err != nil {
		t.Fatalf("updateCronJobEnabled: %v", err)
	}
	if response, err := client.runCronJob(ctx, "job_2"); err != nil || response != "cron ok" {
		t.Fatalf("runCronJob: response=%q err=%v", response, err)
	}
	if runs, err := client.listCronRuns(ctx, "job_2", 10); err != nil || len(runs) != 1 {
		t.Fatalf("listCronRuns: runs=%+v err=%v", runs, err)
	}
	if err := client.deleteCronJob(ctx, "job_2"); err != nil {
		t.Fatalf("deleteCronJob: %v", err)
	}
	if history, err := client.eventHistory(ctx, 20); err != nil || history.LastID != 1 || len(history.Items) != 1 {
		t.Fatalf("eventHistory: history=%+v err=%v", history, err)
	}
	if readInfo, err := client.markEventsRead(ctx, 1); err != nil || !readInfo.Acknowledged || readInfo.UnreadCount != 0 {
		t.Fatalf("markEventsRead: readInfo=%+v err=%v", readInfo, err)
	}
	_, err = client.gatewayReload(ctx)
	if err != nil {
		t.Fatalf("gatewayReload: %v", err)
	}
	if normalAuth != "Bearer user-token" {
		t.Fatalf("expected user auth header, got %q", normalAuth)
	}
	if adminAuth != "Bearer admin-token" {
		t.Fatalf("expected admin auth header, got %q", adminAuth)
	}
}

func TestRuntimeClientRequestText_UsesJSONErrorPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "forbidden",
			"code":  "forbidden",
		})
	}))
	defer server.Close()

	client := runtimeClient{serverURL: server.URL}
	_, err := client.status(context.Background())
	if err == nil {
		t.Fatalf("expected status request error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "forbidden") {
		t.Fatalf("expected error to include code/message, got %q", msg)
	}
	if strings.Contains(msg, "{") || strings.Contains(msg, "}") {
		t.Fatalf("expected parsed error message, got raw json: %q", msg)
	}
}

func TestRuntimeClientRequestText_FallbacksToPlainTextOnNonJSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "broken upstream", http.StatusBadGateway)
	}))
	defer server.Close()

	client := runtimeClient{serverURL: server.URL}
	_, err := client.status(context.Background())
	if err == nil {
		t.Fatalf("expected status request error")
	}
	if !strings.Contains(err.Error(), "broken upstream") {
		t.Fatalf("expected plain error body in message, got %q", err.Error())
	}
}

func TestRuntimeClientWhoami(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/auth/whoami":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authenticated": true,
				"auth_role":     "user",
				"is_admin":      false,
				"auth_mode":     "required",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := runtimeClient{serverURL: server.URL}
	out, err := client.whoami(context.Background())
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if !out.Authenticated || out.AuthRole != "user" || out.AuthMode != "required" {
		t.Fatalf("unexpected whoami payload: %+v", out)
	}
}
