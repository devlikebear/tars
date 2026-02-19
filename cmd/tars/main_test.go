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
