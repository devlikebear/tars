package tarsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tarsncase/internal/secrets"
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

func TestExecuteCommand_Providers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/providers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"current_provider": "openai-codex",
				"current_model":    "gpt-5.3-codex",
				"auth_mode":        "oauth",
				"providers": []map[string]any{
					{"id": "openai-codex", "supports_live_models": true},
					{"id": "openai", "supports_live_models": true},
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

	if _, _, err := executeCommand(context.Background(), runtime, "/providers", "", stdout, stderr); err != nil {
		t.Fatalf("/providers: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "provider=openai-codex") || !strings.Contains(out, "- openai-codex live_models=true") {
		t.Fatalf("unexpected /providers output: %q", out)
	}
}

func TestExecuteCommand_Models(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"provider":      "openai-codex",
				"current_model": "gpt-5.3-codex",
				"source":        "live",
				"stale":         false,
				"models":        []string{"gpt-5.3-codex", "gpt-4.1-codex"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runtime := runtimeClient{serverURL: server.URL}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	if _, _, err := executeCommand(context.Background(), runtime, "/models", "", stdout, stderr); err != nil {
		t.Fatalf("/models: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "models provider=openai-codex") || !strings.Contains(out, "- gpt-5.3-codex") {
		t.Fatalf("unexpected /models output: %q", out)
	}
}

func TestExecuteCommand_ModelListAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"provider":      "openai",
				"current_model": "gpt-4o-mini",
				"source":        "cache",
				"stale":         true,
				"warning":       "live provider unavailable",
				"models":        []string{"gpt-4o-mini"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runtime := runtimeClient{serverURL: server.URL}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	if _, _, err := executeCommand(context.Background(), runtime, "/model list", "", stdout, stderr); err != nil {
		t.Fatalf("/model list: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "models provider=openai") || !strings.Contains(out, "warning=live provider unavailable") {
		t.Fatalf("unexpected /model list output: %q", out)
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
				{"job_id": "job_1", "ran_at": "2026-02-18T08:00:00Z", "error": "timeout"},
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
	if !strings.Contains(stdout.String(), "prompt:") || !strings.Contains(stdout.String(), "check") {
		t.Fatalf("expected cron prompt output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "cron run logs") || !strings.Contains(stdout.String(), "error=timeout") {
		t.Fatalf("expected cron run logs output, got %q", stdout.String())
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

func TestExecuteCommand_TelegramPairingsAndApprove(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/channels/telegram/pairings":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"dm_policy":       "pairing",
				"polling_enabled": true,
				"pending": []map[string]any{
					{"code": "ABCD1234", "user_id": 11, "chat_id": "101", "username": "alice", "expires_at": "2026-02-21T01:00:00Z"},
				},
				"allowed": []map[string]any{
					{"user_id": 22, "chat_id": "202", "username": "bob", "approved_at": "2026-02-21T00:20:00Z"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/channels/telegram/pairings/approve":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"approved": map[string]any{
					"user_id":     11,
					"chat_id":     "101",
					"username":    "alice",
					"approved_at": "2026-02-21T00:30:00Z",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runtime := runtimeClient{
		serverURL:     server.URL,
		apiToken:      "user-token",
		adminAPIToken: "admin-token",
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	if _, _, err := executeCommand(context.Background(), runtime, "/telegram pairings", "", stdout, stderr); err != nil {
		t.Fatalf("/telegram pairings: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "dm_policy=pairing") || !strings.Contains(out, "pending code=ABCD1234") {
		t.Fatalf("unexpected telegram pairings output: %q", out)
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/telegram pairing approve ABCD1234", "", stdout, stderr); err != nil {
		t.Fatalf("/telegram pairing approve: %v", err)
	}
	if !strings.Contains(stdout.String(), "approved telegram pairing user_id=11") {
		t.Fatalf("unexpected approve output: %q", stdout.String())
	}
}

func TestExecuteCommand_BrowserAndVault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/browser/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"running":             true,
				"profile":             "managed",
				"driver":              "chromedp",
				"extension_connected": false,
				"attached_tabs":       0,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/browser/profiles":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"count": 2,
				"profiles": []map[string]any{
					{"name": "managed", "driver": "chromedp", "default": true, "running": true},
					{"name": "chrome", "driver": "relay", "default": false, "running": false},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/browser/relay":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":             true,
				"running":             true,
				"addr":                "127.0.0.1:43182",
				"relay_token":         "relay-token",
				"extension_connected": true,
				"extension_ws_url":    "ws://127.0.0.1:43182/extension",
				"cdp_ws_url":          "ws://127.0.0.1:43182/cdp?token=relay-token",
				"origin_allowlist":    []string{"chrome-extension://*"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/browser/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"site_id": "portal", "profile": "managed", "mode": "manual", "success": true, "message": "manual login required"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/browser/check":
			_ = json.NewEncoder(w).Encode(map[string]any{"site_id": "portal", "profile": "managed", "check_count": 1, "passed": true, "message": "ok"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/browser/run":
			_ = json.NewEncoder(w).Encode(map[string]any{"site_id": "portal", "profile": "managed", "action": "ping", "step_count": 2, "success": true, "message": "ok"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/vault/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"enabled": true, "ready": true, "auth_mode": "token", "addr": "http://127.0.0.1:8200", "allowlist_count": 2})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runtime := runtimeClient{serverURL: server.URL}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	_, _, err := executeCommand(context.Background(), runtime, "/browser status", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/browser status: %v", err)
	}
	if !strings.Contains(stdout.String(), "browser running=true") {
		t.Fatalf("expected browser status output, got %q", stdout.String())
	}

	stdout.Reset()
	_, _, err = executeCommand(context.Background(), runtime, "/browser profiles", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/browser profiles: %v", err)
	}
	if !strings.Contains(stdout.String(), "managed") {
		t.Fatalf("expected browser profiles output, got %q", stdout.String())
	}

	stdout.Reset()
	_, _, err = executeCommand(context.Background(), runtime, "/browser relay", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/browser relay: %v", err)
	}
	if !strings.Contains(stdout.String(), "extension_ws=ws://127.0.0.1:43182/extension") || !strings.Contains(stdout.String(), "cdp_ws=ws://127.0.0.1:43182/cdp?token=relay-token") {
		t.Fatalf("expected browser relay output, got %q", stdout.String())
	}

	stdout.Reset()
	_, _, err = executeCommand(context.Background(), runtime, "/browser login portal --profile managed", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/browser login: %v", err)
	}
	if !strings.Contains(stdout.String(), "success=true") {
		t.Fatalf("expected browser login output, got %q", stdout.String())
	}

	stdout.Reset()
	_, _, err = executeCommand(context.Background(), runtime, "/browser check portal --profile managed", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/browser check: %v", err)
	}
	if !strings.Contains(stdout.String(), "checks=1") {
		t.Fatalf("expected browser check output, got %q", stdout.String())
	}

	stdout.Reset()
	_, _, err = executeCommand(context.Background(), runtime, "/browser run portal ping --profile managed", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/browser run: %v", err)
	}
	if !strings.Contains(stdout.String(), "steps=2") {
		t.Fatalf("expected browser run output, got %q", stdout.String())
	}

	stdout.Reset()
	_, _, err = executeCommand(context.Background(), runtime, "/vault status", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/vault status: %v", err)
	}
	if !strings.Contains(stdout.String(), "vault enabled=true") {
		t.Fatalf("expected vault status output, got %q", stdout.String())
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

func TestExecuteCommand_ResumeMainAliasUsesStatusMainSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"workspace_dir":   "/tmp/ws",
				"session_count":   2,
				"main_session_id": "s-main",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runtime := runtimeClient{serverURL: server.URL}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	_, session, err := executeCommand(context.Background(), runtime, "/resume main", "s-prev", stdout, stderr)
	if err != nil {
		t.Fatalf("/resume main: %v", err)
	}
	if session != "s-main" {
		t.Fatalf("expected main session alias to resolve to s-main, got %q", session)
	}
	if !strings.Contains(stdout.String(), "resumed session=s-main") {
		t.Fatalf("expected resume output, got %q", stdout.String())
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
	if session != "s-prev" {
		t.Fatalf("expected unchanged session without selection, got %q", session)
	}
	if !strings.Contains(stdout.String(), "resume targets") {
		t.Fatalf("expected resume target listing output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "1. s-latest latest") {
		t.Fatalf("expected numbered session list, got %q", stdout.String())
	}
}

func TestExecuteCommand_ResumeByNumber(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "s-latest", "title": "latest"},
				{"id": "s-2", "title": "daily"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runtime := runtimeClient{serverURL: server.URL}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	_, session, err := executeCommand(context.Background(), runtime, "/resume 2", "s-prev", stdout, stderr)
	if err != nil {
		t.Fatalf("/resume 2: %v", err)
	}
	if session != "s-2" {
		t.Fatalf("expected selected session s-2, got %q", session)
	}
	if !strings.Contains(stdout.String(), "resumed session=s-2") {
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

func TestNotifySync_NotifyListMarksRead(t *testing.T) {
	var readBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/events/read":
			if err := json.NewDecoder(r.Body).Decode(&readBody); err != nil {
				t.Fatalf("decode read body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"acknowledged": true,
				"read_cursor":  3,
				"unread_count": 0,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	center := newNotificationCenter(10)
	center.add(notificationMessage{ID: 2, Type: "notification", Category: "cron", Severity: "info", Title: "cron", Message: "done"})
	center.add(notificationMessage{ID: 3, Type: "notification", Category: "error", Severity: "error", Title: "error", Message: "failed"})
	state := &localRuntimeState{notifications: center}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := runtimeClient{serverURL: server.URL}

	if _, _, err := executeCommandWithState(context.Background(), runtime, "/notify list", "", stdout, stderr, state); err != nil {
		t.Fatalf("/notify list: %v", err)
	}
	if readBody == nil {
		t.Fatalf("expected read ack request")
	}
	if got := int(readBody["last_id"].(float64)); got != 3 {
		t.Fatalf("expected last_id=3, got %+v", readBody)
	}
}

func TestExecuteCommand_TraceToggle(t *testing.T) {
	state := &localRuntimeState{
		notifications: newNotificationCenter(10),
		chatTrace:     true,
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := runtimeClient{}

	if _, _, err := executeCommandWithState(context.Background(), runtime, "/trace", "", stdout, stderr, state); err != nil {
		t.Fatalf("/trace: %v", err)
	}
	if !strings.Contains(stdout.String(), "trace=on") {
		t.Fatalf("expected trace on output, got %q", stdout.String())
	}

	stdout.Reset()
	if _, _, err := executeCommandWithState(context.Background(), runtime, "/trace off", "", stdout, stderr, state); err != nil {
		t.Fatalf("/trace off: %v", err)
	}
	if state.chatTrace {
		t.Fatal("expected trace state off")
	}

	stdout.Reset()
	if _, _, err := executeCommandWithState(context.Background(), runtime, "/trace on", "", stdout, stderr, state); err != nil {
		t.Fatalf("/trace on: %v", err)
	}
	if !state.chatTrace {
		t.Fatal("expected trace state on")
	}
}

func TestExecuteCommand_HelpStructured(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := runtimeClient{}

	_, _, err := executeCommand(context.Background(), runtime, "/help", "", stdout, stderr)
	if err != nil {
		t.Fatalf("/help: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "SYSTEM > commands") {
		t.Fatalf("expected help header, got %q", out)
	}
	if !strings.Contains(out, "Session:") {
		t.Fatalf("expected Session section, got %q", out)
	}
	if !strings.Contains(out, "Runtime:") {
		t.Fatalf("expected Runtime section, got %q", out)
	}
	if !strings.Contains(out, "Chat:") {
		t.Fatalf("expected Chat section, got %q", out)
	}
}

func TestSendMessage_PrintsToolStatusFeedback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"status\",\"phase\":\"before_tool_call\",\"message\":\"executing tool\",\"tool_name\":\"read_file\",\"tool_call_id\":\"call_1\",\"tool_args_preview\":\"{\\\"path\\\":\\\"README.md\\\"}\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"done\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"s-1\"}\n\n"))
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	client := chatClient{serverURL: server.URL}

	res, err := sendMessage(context.Background(), client, "", "hello", true, false, stdout, stderr)
	if err != nil {
		t.Fatalf("sendMessage: %v", err)
	}
	if strings.TrimSpace(res.SessionID) != "s-1" {
		t.Fatalf("expected session id s-1, got %q", res.SessionID)
	}
	if !strings.Contains(stderr.String(), "executing tool (read_file)") {
		t.Fatalf("expected tool status feedback, got %q", stderr.String())
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
	if strings.Contains(out, "DIAG") || strings.Contains(out, "BLOCKED") {
		t.Fatalf("expected no DIAG/BLOCKED positional columns, got %q", out)
	}
	if strings.Count(out, "policy_tool_blocked") != 1 {
		t.Fatalf("expected diagnostic value once, got %q", out)
	}
	if strings.Count(out, "exec") != 1 {
		t.Fatalf("expected blocked tool value once, got %q", out)
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
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "component": "tars", "time": "2026-02-19T00:00:00Z"})
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

func TestExecuteCommand_Whoami(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/auth/whoami":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authenticated": true,
				"auth_role":     "admin",
				"is_admin":      true,
				"auth_mode":     "required",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := runtimeClient{serverURL: server.URL}

	if _, _, err := executeCommand(context.Background(), runtime, "/whoami", "", stdout, stderr); err != nil {
		t.Fatalf("/whoami: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "authenticated=true") || !strings.Contains(out, "role=admin") {
		t.Fatalf("unexpected whoami output: %q", out)
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
	msg := formatRuntimeError(err)
	if !strings.Contains(msg, "hint:") {
		t.Fatalf("expected hint in message, got %q", msg)
	}
	if !strings.Contains(msg, "--api-token") {
		t.Fatalf("expected api token hint, got %q", msg)
	}
}

func TestFormatRuntimeError_DoesNotIncludeRemovedWorkspaceHint(t *testing.T) {
	err := &apiHTTPError{
		Method:   http.MethodGet,
		Endpoint: "http://127.0.0.1:43180/v1/status",
		Status:   http.StatusBadRequest,
		Code:     "bad_request",
		Message:  "bad request",
	}
	msg := formatRuntimeError(err)
	if strings.Contains(strings.ToLower(msg), "workspace") {
		t.Fatalf("workspace hint must be removed, got %q", msg)
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
	msg := formatRuntimeError(err)
	if !strings.Contains(msg, "--admin-api-token") {
		t.Fatalf("expected admin token hint, got %q", msg)
	}
}

func TestFormatRuntimeError_RedactsSensitiveValues(t *testing.T) {
	secrets.ResetForTests()
	secret := "super_secret_token_value_1234567890"
	secrets.RegisterNamed("API_TOKEN", secret)

	msg := formatRuntimeError(fmt.Errorf("failed token=%s", secret))
	if strings.Contains(msg, secret) {
		t.Fatalf("expected redacted runtime error, got %q", msg)
	}
}
