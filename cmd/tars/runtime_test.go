package main

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
			_ = json.NewEncoder(w).Encode(map[string]any{"enabled": true, "version": 1})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/gateway/reload":
			adminAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{"enabled": true, "version": 2})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := runtimeClient{
		serverURL:     server.URL,
		apiToken:      "user-token",
		adminAPIToken: "admin-token",
		workspaceID:   "ws-a",
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
	_, err = client.spawnRun(ctx, spawnRequest{Message: "hello"})
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
