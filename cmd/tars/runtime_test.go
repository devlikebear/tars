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
