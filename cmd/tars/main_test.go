package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecuteCommand_NewAndStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "s-new", "title": "nightly"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"workspace_dir": "/tmp/ws", "session_count": 3})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runtime := runtimeClient{serverURL: server.URL}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	handled, session, err := executeCommand(context.Background(), runtime, "/new nightly", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/new: %v", err)
	}
	if !handled || session != "s-new" {
		t.Fatalf("expected handled with new session, handled=%t session=%q", handled, session)
	}
	if !strings.Contains(stderr.String(), "session=s-new") {
		t.Fatalf("expected session output in stderr, got %q", stderr.String())
	}

	stdout.Reset()
	_, _, err = executeCommand(context.Background(), runtime, "/status", session, stdout, stderr)
	if err != nil {
		t.Fatalf("/status: %v", err)
	}
	if !strings.Contains(stdout.String(), "workspace=/tmp/ws") {
		t.Fatalf("expected workspace status output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "scope=default") {
		t.Fatalf("expected default scope output, got %q", stdout.String())
	}
}

func TestExecuteCommand_CompactRequiresSession(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := runtimeClient{serverURL: "http://127.0.0.1:43180"}
	_, _, err := executeCommand(context.Background(), runtime, "/compact", "", stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "active session") {
		t.Fatalf("expected active session error, got %v", err)
	}
}

func TestExecuteCommand_CronAndChannels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/cron/jobs":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "job_1", "name": "daily", "prompt": "check", "schedule": "every:1h", "enabled": true},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/cron/jobs/job_1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job_1", "name": "daily", "prompt": "check", "schedule": "every:1h", "enabled": true})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/cron/jobs/job_1/runs":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"job_id": "job_1", "ran_at": "2026-02-18T09:00:00Z", "response": "ok"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/gateway/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":                   true,
				"version":                   9,
				"channels_local_enabled":    true,
				"channels_webhook_enabled":  false,
				"channels_telegram_enabled": true,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runtime := runtimeClient{serverURL: server.URL}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	_, _, err := executeCommand(context.Background(), runtime, "/cron list", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/cron list: %v", err)
	}
	if !strings.Contains(stdout.String(), "job_1") {
		t.Fatalf("expected cron list output, got %q", stdout.String())
	}

	stdout.Reset()
	_, _, err = executeCommand(context.Background(), runtime, "/cron get job_1", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/cron get: %v", err)
	}
	if !strings.Contains(stdout.String(), "schedule=every:1h") {
		t.Fatalf("expected cron get output, got %q", stdout.String())
	}

	stdout.Reset()
	_, _, err = executeCommand(context.Background(), runtime, "/cron runs job_1 1", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/cron runs: %v", err)
	}
	if !strings.Contains(stdout.String(), "2026-02-18T09:00:00Z") {
		t.Fatalf("expected cron runs output, got %q", stdout.String())
	}

	stdout.Reset()
	_, _, err = executeCommand(context.Background(), runtime, "/channels", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/channels: %v", err)
	}
	if !strings.Contains(stdout.String(), "channels_local=true") {
		t.Fatalf("expected channels output, got %q", stdout.String())
	}
}

func TestExecuteCommand_ResumeAndAgentsDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agent/agents":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"agents": []map[string]any{
					{
						"name":              "researcher",
						"kind":              "prompt",
						"source":            "workspace",
						"entry":             "workspace/agents/researcher/AGENT.md",
						"policy_mode":       "allowlist",
						"tools_allow_count": 3,
						"tools_deny_count":  1,
						"tools_risk_max":    "medium",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runtime := runtimeClient{serverURL: server.URL}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	_, session, err := executeCommand(context.Background(), runtime, "/resume s-9", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/resume: %v", err)
	}
	if session != "s-9" {
		t.Fatalf("expected resumed session, got %q", session)
	}

	stdout.Reset()
	_, _, err = executeCommand(context.Background(), runtime, "/agents --detail", session, stdout, stderr)
	if err != nil {
		t.Fatalf("/agents --detail: %v", err)
	}
	if !strings.Contains(stdout.String(), "entry=workspace/agents/researcher/AGENT.md") {
		t.Fatalf("expected detailed agent output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "deny=1") || !strings.Contains(stdout.String(), "risk_max=medium") {
		t.Fatalf("expected deny/risk detail output, got %q", stdout.String())
	}

	stdout.Reset()
	_, _, err = executeCommand(context.Background(), runtime, "/agents -d", session, stdout, stderr)
	if err != nil {
		t.Fatalf("/agents -d: %v", err)
	}
	if !strings.Contains(stdout.String(), "entry=workspace/agents/researcher/AGENT.md") {
		t.Fatalf("expected detailed agent output for -d, got %q", stdout.String())
	}
}

func TestExecuteCommand_ResumeWithoutIDUsesLatestSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "s-latest", "title": "latest"},
				{"id": "s-old", "title": "old"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runtime := runtimeClient{serverURL: server.URL}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	_, session, err := executeCommand(context.Background(), runtime, "/resume", "s-prev", stdout, stderr)
	if err != nil {
		t.Fatalf("/resume: %v", err)
	}
	if session != "s-latest" {
		t.Fatalf("expected latest session, got %q", session)
	}
	if !strings.Contains(stdout.String(), "resumed session=s-latest") {
		t.Fatalf("expected resume output, got %q", stdout.String())
	}
}

func TestExecuteCommand_NotifyCommands(t *testing.T) {
	center := newNotificationCenter(10)
	center.add(notificationMessage{
		Category:  "cron",
		Severity:  "info",
		Title:     "cron completed",
		Message:   "nightly summary done",
		Timestamp: "2026-02-18T12:00:00Z",
	})
	center.add(notificationMessage{
		Category:  "error",
		Severity:  "error",
		Title:     "run failed",
		Message:   "agent run failed",
		Timestamp: "2026-02-18T12:01:00Z",
	})
	state := &localRuntimeState{notifications: center}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := runtimeClient{}

	if _, _, err := executeCommandWithState(context.Background(), runtime, "/notify list", "", stdout, stderr, state); err != nil {
		t.Fatalf("/notify list: %v", err)
	}
	if !strings.Contains(stdout.String(), "cron completed") {
		t.Fatalf("expected notify list output, got %q", stdout.String())
	}

	stdout.Reset()
	if _, _, err := executeCommandWithState(context.Background(), runtime, "/notify filter error", "", stdout, stderr, state); err != nil {
		t.Fatalf("/notify filter: %v", err)
	}
	if !strings.Contains(stdout.String(), "notification filter: error") {
		t.Fatalf("expected notify filter output, got %q", stdout.String())
	}

	stdout.Reset()
	if _, _, err := executeCommandWithState(context.Background(), runtime, "/notify open 1", "", stdout, stderr, state); err != nil {
		t.Fatalf("/notify open: %v", err)
	}
	if !strings.Contains(stdout.String(), "run failed") {
		t.Fatalf("expected notify open output, got %q", stdout.String())
	}

	stdout.Reset()
	if _, _, err := executeCommandWithState(context.Background(), runtime, "/notify clear", "", stdout, stderr, state); err != nil {
		t.Fatalf("/notify clear: %v", err)
	}
	if !strings.Contains(stdout.String(), "notifications cleared") {
		t.Fatalf("expected notify clear output, got %q", stdout.String())
	}
}

func TestExecuteCommand_GatewayReports(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/gateway/reports/summary":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"generated_at":       "2026-02-19T00:00:00Z",
				"summary_enabled":    true,
				"archive_enabled":    false,
				"runs_total":         3,
				"runs_active":        1,
				"runs_by_status":     map[string]any{"running": 1, "completed": 2},
				"channels_total":     1,
				"messages_total":     4,
				"messages_by_source": map[string]any{"webhook": 4},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/gateway/reports/runs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"generated_at":    "2026-02-19T00:00:01Z",
				"archive_enabled": false,
				"count":           1,
				"runs": []map[string]any{
					{"run_id": "run-1", "status": "completed", "agent": "default"},
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
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := runtimeClient{serverURL: server.URL}

	if _, _, err := executeCommand(context.Background(), runtime, "/gateway summary", "", stdout, stderr); err != nil {
		t.Fatalf("/gateway summary: %v", err)
	}
	if !strings.Contains(stdout.String(), "runs_total=3") {
		t.Fatalf("expected summary output, got %q", stdout.String())
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/gateway runs 5", "", stdout, stderr); err != nil {
		t.Fatalf("/gateway runs: %v", err)
	}
	if !strings.Contains(stdout.String(), "run-1") {
		t.Fatalf("expected runs output, got %q", stdout.String())
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/gateway channels 5", "", stdout, stderr); err != nil {
		t.Fatalf("/gateway channels: %v", err)
	}
	if !strings.Contains(stdout.String(), "general messages=1") {
		t.Fatalf("expected channels output, got %q", stdout.String())
	}
}

func TestExecuteCommand_GatewayStatusTelemetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/gateway/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":                      true,
				"version":                      11,
				"runs_total":                   7,
				"runs_active":                  2,
				"agents_count":                 4,
				"agents_watch_enabled":         true,
				"persistence_enabled":          true,
				"runs_persistence_enabled":     true,
				"channels_persistence_enabled": false,
				"restore_on_startup":           true,
				"runs_restored":                3,
				"channels_restored":            9,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := runtimeClient{serverURL: server.URL}
	runtime.workspaceID = "ws-main"

	if _, _, err := executeCommand(context.Background(), runtime, "/gateway status", "", stdout, stderr); err != nil {
		t.Fatalf("/gateway status: %v", err)
	}

	out := stdout.String()
	containsAll := strings.Contains(out, "runs_total=7") &&
		strings.Contains(out, "runs_active=2") &&
		strings.Contains(out, "agents=4") &&
		strings.Contains(out, "watch=true") &&
		strings.Contains(out, "persistence=true") &&
		strings.Contains(out, "runs_store=true") &&
		strings.Contains(out, "channels_store=false") &&
		strings.Contains(out, "scope=ws-main") &&
		strings.Contains(out, "restored_runs=3") &&
		strings.Contains(out, "restored_channels=9")
	if !containsAll {
		t.Fatalf("expected gateway telemetry output, got %q", out)
	}
}

func TestExecuteCommand_RunShowsPolicyDiagnosticDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agent/runs/run_1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id":               "run_1",
				"status":               "failed",
				"agent":                "researcher",
				"session_id":           "s-1",
				"workspace_id":         "team-a",
				"error":                "tool not injected for this request: exec",
				"diagnostic_code":      "policy_tool_blocked",
				"diagnostic_reason":    "tool not injected for this request: exec",
				"policy_blocked_tool":  "exec",
				"policy_allowed_tools": []string{"read_file", "list_dir"},
				"policy_denied_tools":  []string{"exec"},
				"policy_risk_max":      "medium",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := runtimeClient{serverURL: server.URL}

	if _, _, err := executeCommand(context.Background(), runtime, "/run run_1", "", stdout, stderr); err != nil {
		t.Fatalf("/run: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "diagnostic: policy_tool_blocked") {
		t.Fatalf("expected diagnostic output, got %q", out)
	}
	if !strings.Contains(out, "policy_blocked_tool=exec") {
		t.Fatalf("expected blocked tool output, got %q", out)
	}
	if !strings.Contains(out, "policy_allowed=read_file,list_dir") {
		t.Fatalf("expected allowed tools output, got %q", out)
	}
	if !strings.Contains(out, "policy_denied=exec") {
		t.Fatalf("expected denied tools output, got %q", out)
	}
	if !strings.Contains(out, "policy_risk_max=medium") {
		t.Fatalf("expected risk max output, got %q", out)
	}
}

func TestExecuteCommand_RunsShowsPolicyDiagnosticSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agent/runs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runs": []map[string]any{
					{
						"run_id":              "run_2",
						"status":              "failed",
						"agent":               "researcher",
						"session_id":          "s-2",
						"workspace_id":        "team-b",
						"diagnostic_code":     "policy_tool_blocked",
						"policy_blocked_tool": "exec",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := runtimeClient{serverURL: server.URL}

	if _, _, err := executeCommand(context.Background(), runtime, "/runs", "", stdout, stderr); err != nil {
		t.Fatalf("/runs: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "diag=policy_tool_blocked") {
		t.Fatalf("expected run diagnostic summary, got %q", out)
	}
	if !strings.Contains(out, "blocked=exec") {
		t.Fatalf("expected blocked tool summary, got %q", out)
	}
}

func TestExecuteCommand_GatewayStatusShowsReloadAndRestoreDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/gateway/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":                      true,
				"version":                      15,
				"runs_total":                   10,
				"runs_active":                  1,
				"agents_count":                 5,
				"agents_watch_enabled":         true,
				"agents_reload_version":        8,
				"persistence_enabled":          true,
				"runs_persistence_enabled":     true,
				"channels_persistence_enabled": true,
				"runs_restored":                7,
				"channels_restored":            19,
				"last_restore_error":           "decode snapshot failed",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := runtimeClient{serverURL: server.URL}

	if _, _, err := executeCommand(context.Background(), runtime, "/gateway status", "", stdout, stderr); err != nil {
		t.Fatalf("/gateway status: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "reload_version=8") {
		t.Fatalf("expected reload version in status output, got %q", out)
	}
	if !strings.Contains(out, "restore_error=decode snapshot failed") {
		t.Fatalf("expected restore error in status output, got %q", out)
	}
}

func TestExecuteCommand_Health(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/healthz":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "component": "tarsd", "time": "2026-02-19T00:00:00Z"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := runtimeClient{serverURL: server.URL}

	if _, _, err := executeCommand(context.Background(), runtime, "/health", "", stdout, stderr); err != nil {
		t.Fatalf("/health: %v", err)
	}
	if !strings.Contains(stdout.String(), "ok=true") {
		t.Fatalf("expected health output, got %q", stdout.String())
	}
}

func TestFormatRuntimeError_ProvidesAuthHintForUnauthorized(t *testing.T) {
	err := &apiHTTPError{
		Method:   http.MethodGet,
		Endpoint: "http://127.0.0.1:43180/v1/status",
		Status:   http.StatusUnauthorized,
		Code:     "unauthorized",
		Message:  "unauthorized",
	}
	msg := formatRuntimeError(err, runtimeClient{})
	if !strings.Contains(msg, "hint:") {
		t.Fatalf("expected hint in message, got %q", msg)
	}
	if !strings.Contains(msg, "--api-token") {
		t.Fatalf("expected api token hint, got %q", msg)
	}
}

func TestFormatRuntimeError_ProvidesWorkspaceHint(t *testing.T) {
	err := &apiHTTPError{
		Method:   http.MethodGet,
		Endpoint: "http://127.0.0.1:43180/v1/status",
		Status:   http.StatusBadRequest,
		Code:     "workspace_id_required",
		Message:  "workspace id is required",
	}
	msg := formatRuntimeError(err, runtimeClient{})
	if !strings.Contains(msg, "--workspace-id") {
		t.Fatalf("expected workspace hint, got %q", msg)
	}
}

func TestFormatRuntimeError_ProvidesAdminHintOnAdminEndpoint(t *testing.T) {
	err := &apiHTTPError{
		Method:   http.MethodPost,
		Endpoint: "http://127.0.0.1:43180/v1/gateway/reload",
		Status:   http.StatusForbidden,
		Code:     "forbidden",
		Message:  "forbidden",
	}
	msg := formatRuntimeError(err, runtimeClient{})
	if !strings.Contains(msg, "--admin-api-token") {
		t.Fatalf("expected admin token hint, got %q", msg)
	}
}
