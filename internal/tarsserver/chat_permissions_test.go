package tarsserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestChatPermissionSessionRule(t *testing.T) {
	for _, tc := range []struct {
		name, tool, input string
		display           string
		content           string
	}{
		{"bash first word", "Bash", `{"command":"touch hello.txt"}`, "Bash(touch:*)", "touch:*"},
		{"bash multiplexer keeps its subcommand", "Bash", `{"command":"npm run test -- --watch"}`, "Bash(npm run:*)", "npm run:*"},
		{"bash git subcommand", "Bash", `{"command":"git status --short"}`, "Bash(git status:*)", "git status:*"},
		{"bash flag is not a subcommand", "Bash", `{"command":"go -C sub test"}`, "Bash(go:*)", "go:*"},
		{"compound command gets no rule", "Bash", `{"command":"make build && rm -rf dist"}`, "", ""},
		{"pipe gets no rule", "Bash", `{"command":"cat x | sh"}`, "", ""},
		{"redirect gets no rule", "Bash", `{"command":"echo hi > /etc/hosts"}`, "", ""},
		{"substitution gets no rule", "Bash", `{"command":"echo $(whoami)"}`, "", ""},
		{"rm is never allowed wholesale", "Bash", `{"command":"rm old.txt"}`, "", ""},
		{"sudo is never allowed wholesale", "Bash", `{"command":"sudo ls"}`, "", ""},
		{"empty command", "Bash", `{"command":"  "}`, "", ""},
		{"edit tool", "Edit", `{"file_path":"/repo/a.go"}`, "Edit", ""},
		{"web fetch is per domain", "WebFetch", `{"url":"https://docs.example.com/a?b=1"}`, "WebFetch(domain:docs.example.com)", "domain:docs.example.com"},
		{"web fetch without a host", "WebFetch", `{"url":"not a url"}`, "", ""},
		{"mcp tool", "mcp__github__create_issue", `{}`, "mcp__github__create_issue", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			display, update := chatPermissionSessionRule(llm.ClaudeCodePermissionRequest{ToolName: tc.tool, Input: json.RawMessage(tc.input)})
			if display != tc.display {
				t.Fatalf("display = %q, want %q", display, tc.display)
			}
			if tc.display == "" {
				if update != nil {
					t.Fatalf("no rule expected, got %s", update)
				}
				return
			}
			var decoded struct {
				Type        string `json:"type"`
				Behavior    string `json:"behavior"`
				Destination string `json:"destination"`
				Rules       []struct {
					ToolName    string `json:"toolName"`
					RuleContent string `json:"ruleContent"`
				} `json:"rules"`
			}
			if err := json.Unmarshal(update, &decoded); err != nil {
				t.Fatalf("decode update %s: %v", update, err)
			}
			// Session scope only: any other destination writes a settings
			// file in the user's project.
			if decoded.Type != "addRules" || decoded.Behavior != "allow" || decoded.Destination != "session" {
				t.Fatalf("update = %s", update)
			}
			if len(decoded.Rules) != 1 || decoded.Rules[0].ToolName != tc.tool || decoded.Rules[0].RuleContent != tc.content {
				t.Fatalf("rules = %+v", decoded.Rules)
			}
		})
	}
}

func TestChatPermissionBroker(t *testing.T) {
	broker := newChatPermissionBroker()
	id, answers, done := broker.open("s1")
	if id == "" {
		t.Fatal("empty request id")
	}
	if err := broker.answer(id, "other-session", chatPermissionAnswer{Decision: "allow_once"}); !errors.Is(err, errChatPermissionNotFound) {
		t.Fatalf("answer from another session: err = %v", err)
	}
	if err := broker.answer("nope", "s1", chatPermissionAnswer{Decision: "allow_once"}); !errors.Is(err, errChatPermissionNotFound) {
		t.Fatalf("answer to unknown id: err = %v", err)
	}
	if err := broker.answer(id, "s1", chatPermissionAnswer{Decision: "deny"}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if got := <-answers; got.Decision != "deny" {
		t.Fatalf("answer = %+v", got)
	}
	// A prompt takes one answer.
	if err := broker.answer(id, "s1", chatPermissionAnswer{Decision: "allow_once"}); !errors.Is(err, errChatPermissionNotFound) {
		t.Fatalf("second answer: err = %v", err)
	}
	done()

	id2, _, done2 := broker.open("s1")
	done2()
	if err := broker.answer(id2, "s1", chatPermissionAnswer{Decision: "allow_once"}); !errors.Is(err, errChatPermissionNotFound) {
		t.Fatalf("answer after close: err = %v", err)
	}
}

// permissionAskingClient stands in for claude-code-cli: during the turn it
// asks the chat server's handler about one Bash call.
type permissionAskingClient struct {
	mu         sync.Mutex
	sawHandler bool
	decision   llm.ClaudeCodePermissionDecision
	handlerErr error
}

func (c *permissionAskingClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *permissionAskingClient) Chat(ctx context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	c.sawHandler = opts.ClaudeCodePermissionHandler != nil
	c.mu.Unlock()
	if opts.ClaudeCodePermissionHandler != nil {
		decision, err := opts.ClaudeCodePermissionHandler(ctx, llm.ClaudeCodePermissionRequest{
			ToolName:       "Bash",
			ToolUseID:      "toolu_1",
			Input:          json.RawMessage(`{"command":"touch hello.txt","description":"Create hello.txt"}`),
			DecisionReason: "writes a file",
			AgentID:        "agent-9",
		})
		c.mu.Lock()
		c.decision, c.handlerErr = decision, err
		c.mu.Unlock()
		if err != nil {
			return llm.ChatResponse{}, err
		}
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "done"}}, nil
}

type permissionTestServer struct {
	server  *httptest.Server
	store   *session.Store
	session string
}

func newPermissionTestServer(t *testing.T, client llm.Client) permissionTestServer {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("approvals")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	server := httptest.NewServer(newChatAPIHandler(root, store, client, zerolog.New(io.Discard)))
	t.Cleanup(server.Close)
	return permissionTestServer{server: server, store: store, session: sess.ID}
}

// startChat posts a turn and returns a channel of its SSE events.
func (s permissionTestServer) startChat(t *testing.T, interactive bool) <-chan map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"session_id": s.session, "message": "make hello.txt", "interactive_permissions": interactive})
	resp, err := http.Post(s.server.URL+"/v1/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post chat: %v", err)
	}
	events := make(chan map[string]any, 64)
	go func() {
		defer close(events)
		defer func() { _ = resp.Body.Close() }()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line, ok := strings.CutPrefix(scanner.Text(), "data: ")
			if !ok {
				continue
			}
			var event map[string]any
			if json.Unmarshal([]byte(line), &event) == nil {
				events <- event
			}
		}
	}()
	return events
}

func (s permissionTestServer) answer(t *testing.T, requestID, decision string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"session_id": s.session, "decision": decision})
	resp, err := http.Post(s.server.URL+"/v1/chat/permissions/"+requestID, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post answer: %v", err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func waitForEvent(t *testing.T, events <-chan map[string]any, eventType string) map[string]any {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatalf("stream ended before a %q event", eventType)
			}
			if event["type"] == eventType {
				return event
			}
		case <-timeout:
			t.Fatalf("no %q event", eventType)
		}
	}
}

func TestChatAPI_InteractivePermissionRoundTrip(t *testing.T) {
	client := &permissionAskingClient{}
	srv := newPermissionTestServer(t, client)
	events := srv.startChat(t, true)

	prompt := waitForEvent(t, events, "permission_request")
	requestID, _ := prompt["request_id"].(string)
	if requestID == "" || prompt["session_id"] != srv.session || prompt["tool_name"] != "Bash" ||
		prompt["tool_use_id"] != "toolu_1" || prompt["reason"] != "writes a file" || prompt["agent_id"] != "agent-9" {
		t.Fatalf("permission_request = %v", prompt)
	}
	if input, _ := prompt["input"].(map[string]any); input["command"] != "touch hello.txt" {
		t.Fatalf("input = %v", prompt["input"])
	}
	if prompt["session_rule"] != "Bash(touch:*)" {
		t.Fatalf("session_rule = %v", prompt["session_rule"])
	}

	if code := srv.answer(t, requestID, "sideways"); code != http.StatusBadRequest {
		t.Fatalf("unknown decision: status %d", code)
	}
	if code := srv.answer(t, requestID, "allow_session"); code != http.StatusOK {
		t.Fatalf("answer: status %d", code)
	}
	resolved := waitForEvent(t, events, "permission_resolved")
	if resolved["request_id"] != requestID || resolved["outcome"] != "allowed_session" {
		t.Fatalf("permission_resolved = %v", resolved)
	}
	waitForEvent(t, events, "done")

	client.mu.Lock()
	defer client.mu.Unlock()
	if client.handlerErr != nil || !client.decision.Allow {
		t.Fatalf("decision = %+v, err = %v", client.decision, client.handlerErr)
	}
	if len(client.decision.UpdatedPermissions) != 1 || !strings.Contains(string(client.decision.UpdatedPermissions[0]), `"ruleContent":"touch:*"`) {
		t.Fatalf("updated permissions = %s", client.decision.UpdatedPermissions)
	}
	if code := srv.answer(t, requestID, "allow_once"); code != http.StatusNotFound {
		t.Fatalf("answering a closed prompt: status %d", code)
	}
}

func TestChatAPI_PermissionDenyReachesTheProvider(t *testing.T) {
	client := &permissionAskingClient{}
	srv := newPermissionTestServer(t, client)
	events := srv.startChat(t, true)

	requestID := waitForEvent(t, events, "permission_request")["request_id"].(string)
	if code := srv.answer(t, requestID, "deny"); code != http.StatusOK {
		t.Fatalf("answer: status %d", code)
	}
	if outcome := waitForEvent(t, events, "permission_resolved")["outcome"]; outcome != "denied" {
		t.Fatalf("outcome = %v", outcome)
	}
	waitForEvent(t, events, "done")
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.decision.Allow || strings.TrimSpace(client.decision.Message) == "" {
		t.Fatalf("decision = %+v", client.decision)
	}
}

// Without the console's opt-in nobody is there to answer, so the provider
// keeps its one-shot path instead of blocking the turn on a prompt.
func TestChatAPI_PermissionHandlerNeedsInteractiveOptIn(t *testing.T) {
	client := &permissionAskingClient{}
	srv := newPermissionTestServer(t, client)
	waitForEvent(t, srv.startChat(t, false), "done")
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.sawHandler {
		t.Fatal("a turn without interactive_permissions must not get a permission handler")
	}
}

func TestChatAPI_CancelWithdrawsPendingPermission(t *testing.T) {
	client := &permissionAskingClient{}
	srv := newPermissionTestServer(t, client)
	events := srv.startChat(t, true)

	requestID := waitForEvent(t, events, "permission_request")["request_id"].(string)
	resp, err := http.Post(srv.server.URL+"/v1/chat/cancel?session_id="+srv.session, "application/json", nil)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	_ = resp.Body.Close()
	if outcome := waitForEvent(t, events, "permission_resolved")["outcome"]; outcome != "withdrawn" {
		t.Fatalf("outcome = %v", outcome)
	}
	waitForEvent(t, events, "cancelled")
	if code := srv.answer(t, requestID, "allow_once"); code != http.StatusNotFound {
		t.Fatalf("answering a withdrawn prompt: status %d", code)
	}
}

func TestChatPermissionEndpointRejectsOtherMethods(t *testing.T) {
	srv := newPermissionTestServer(t, &permissionAskingClient{})
	resp, err := http.Get(srv.server.URL + "/v1/chat/permissions/abc")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
