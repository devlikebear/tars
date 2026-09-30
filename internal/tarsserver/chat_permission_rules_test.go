package tarsserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/pkg/agentloop"
	"github.com/rs/zerolog"
)

func TestChatAlwaysRuleStorePersistsPerFolder(t *testing.T) {
	workspace := t.TempDir()
	project := t.TempDir()
	other := t.TempDir()
	store := newChatAlwaysRuleStore(workspace)

	bash := chatAlwaysRule{Provider: chatRuleProviderClaudeCode, Tool: "Bash", Content: "npm test:*"}
	edit := chatAlwaysRule{Provider: chatRuleProviderTARS, Tool: "write_file"}
	for _, rule := range []chatAlwaysRule{bash, edit, bash} {
		if err := store.add(project, rule); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	got := store.list(project)
	if len(got) != 2 {
		t.Fatalf("rules = %+v, want two (duplicates dropped)", got)
	}
	if got[0].Display() != "Bash(npm test:*)" || got[1].Display() != "write_file" || got[0].CreatedAt.IsZero() {
		t.Fatalf("rules = %+v", got)
	}
	if len(store.list(other)) != 0 {
		t.Fatal("a rule leaked into another folder")
	}

	// A fresh store (a restarted server) reads the same rules.
	if again := newChatAlwaysRuleStore(workspace).list(project); len(again) != 2 {
		t.Fatalf("after restart: %+v", again)
	}
	// Stored outside the project, where only TARS writes.
	if _, err := os.Stat(filepath.Join(workspace, "_shared", "permissions", "always-allow.json")); err != nil {
		t.Fatalf("store file: %v", err)
	}
	entries, _ := os.ReadDir(project)
	if len(entries) != 0 {
		t.Fatalf("the store wrote into the project: %v", entries)
	}

	removed, err := store.remove(project, chatRuleProviderClaudeCode, "Bash(npm test:*)")
	if err != nil || !removed {
		t.Fatalf("remove: %v %v", removed, err)
	}
	if removed, _ := store.remove(project, chatRuleProviderClaudeCode, "Bash(npm test:*)"); removed {
		t.Fatal("removed twice")
	}
	if left := store.list(project); len(left) != 1 || left[0].Tool != "write_file" {
		t.Fatalf("left = %+v", left)
	}
}

func TestChatAlwaysRuleStoreResolvesSymlinks(t *testing.T) {
	workspace := t.TempDir()
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	store := newChatAlwaysRuleStore(workspace)
	if err := store.add(link, chatAlwaysRule{Provider: chatRuleProviderTARS, Tool: "exec", Content: "ls:*"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if got := store.list(target); len(got) != 1 {
		t.Fatalf("the same folder through a symlink has no rules: %+v", got)
	}
	if got := store.list(filepath.Join(target, ".")); len(got) != 1 {
		t.Fatalf("an unclean path of the same folder has no rules: %+v", got)
	}
}

func TestChatAlwaysRuleStoreIgnoresBlankFolders(t *testing.T) {
	store := newChatAlwaysRuleStore(t.TempDir())
	if err := store.add("  ", chatAlwaysRule{Provider: chatRuleProviderTARS, Tool: "exec"}); err == nil {
		t.Fatal("a rule without a folder must be refused")
	}
	if got := store.list(""); len(got) != 0 {
		t.Fatalf("list(\"\") = %+v", got)
	}
}

func newAlwaysBroker(t *testing.T) (*chatPermissionBroker, *chatAlwaysRuleStore) {
	t.Helper()
	store := newChatAlwaysRuleStore(t.TempDir())
	broker := newChatPermissionBroker()
	broker.always = store
	return broker, store
}

func TestChatPermissionHandlerAllowAlwaysPersistsTheRule(t *testing.T) {
	broker, store := newAlwaysBroker(t)
	project := t.TempDir()
	sink := newEventSink()
	handler := newChatPermissionHandler(broker, "s1", project, newChatStreamWriter(sink, "s1", zerolog.New(io.Discard)))

	type result struct {
		decision llm.ClaudeCodePermissionDecision
		err      error
	}
	done := make(chan result, 1)
	go func() {
		d, err := handler(context.Background(), llm.ClaudeCodePermissionRequest{ToolName: "Bash", Input: json.RawMessage(`{"command":"npm test -- --watch"}`)})
		done <- result{d, err}
	}()
	prompt := sink.next(t)
	if prompt["always_dir"] != chatAlwaysRuleDir(project) || prompt["session_rule"] != "Bash(npm test:*)" {
		t.Fatalf("prompt = %v", prompt)
	}
	if err := broker.answer(prompt["request_id"].(string), "s1", chatPermissionAnswer{Decision: "allow_always"}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if outcome := sink.next(t)["outcome"]; outcome != "allowed_always" {
		t.Fatalf("outcome = %v", outcome)
	}
	r := <-done
	// The running CLI learns the rule for this session too.
	if r.err != nil || !r.decision.Allow || len(r.decision.UpdatedPermissions) != 1 {
		t.Fatalf("decision = %+v err %v", r.decision, r.err)
	}
	rules := store.list(project)
	if len(rules) != 1 || rules[0].Provider != chatRuleProviderClaudeCode || rules[0].Display() != "Bash(npm test:*)" {
		t.Fatalf("stored = %+v", rules)
	}
}

func TestChatPermissionAlwaysNeedsARuleAndAFolder(t *testing.T) {
	broker, store := newAlwaysBroker(t)
	project := t.TempDir()
	for name, tc := range map[string]struct {
		cwd, command string
	}{
		"compound command": {project, "make && rm -rf dist"},
		"no folder":        {"", "npm test"},
	} {
		t.Run(name, func(t *testing.T) {
			sink := newEventSink()
			handler := newChatPermissionHandler(broker, "s1", tc.cwd, newChatStreamWriter(sink, "s1", zerolog.New(io.Discard)))
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = handler(context.Background(), llm.ClaudeCodePermissionRequest{ToolName: "Bash", Input: json.RawMessage(`{"command":"` + tc.command + `"}`)})
			}()
			prompt := sink.next(t)
			if prompt["always_dir"] != "" {
				t.Fatalf("always offered: %v", prompt)
			}
			_ = broker.answer(prompt["request_id"].(string), "s1", chatPermissionAnswer{Decision: "allow_always"})
			if outcome := sink.next(t)["outcome"]; outcome == "allowed_always" {
				t.Fatalf("outcome = %v", outcome)
			}
			<-done
		})
	}
	if rules := store.list(project); len(rules) != 0 {
		t.Fatalf("stored = %+v", rules)
	}
}

// A native tool's always rule outlives the server: a new broker over the same
// store lets the call through without asking.
func TestChatToolGateAllowAlwaysSurvivesARestart(t *testing.T) {
	broker, store := newAlwaysBroker(t)
	project := t.TempDir()
	sink := newEventSink()
	gate := newChatToolGate(broker, "s1", project, newChatStreamWriter(sink, "s1", zerolog.New(io.Discard)))
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = gate.Authorize(context.Background(), agentloop.ToolCallRequest{ToolName: "exec", ToolArgs: `{"command":"go test ./..."}`})
	}()
	prompt := sink.next(t)
	if prompt["always_dir"] != chatAlwaysRuleDir(project) {
		t.Fatalf("prompt = %v", prompt)
	}
	_ = broker.answer(prompt["request_id"].(string), "s1", chatPermissionAnswer{Decision: "allow_always"})
	if outcome := sink.next(t)["outcome"]; outcome != "allowed_always" {
		t.Fatalf("outcome = %v", outcome)
	}
	<-done
	if rules := store.list(project); len(rules) != 1 || rules[0].Provider != chatRuleProviderTARS || rules[0].Display() != "exec(go test:*)" {
		t.Fatalf("stored = %+v", rules)
	}

	restarted := newChatPermissionBroker()
	restarted.always = store
	fresh := newEventSink()
	gate = newChatToolGate(restarted, "s9", project, newChatStreamWriter(fresh, "s9", zerolog.New(io.Discard)))
	d, err := gate.Authorize(context.Background(), agentloop.ToolCallRequest{ToolName: "exec", ToolArgs: `{"command":"go test ./pkg/..."}`})
	if err != nil || !d.Allow {
		t.Fatalf("after restart: %+v %v", d, err)
	}
	fresh.none(t)

	// Another folder is not covered.
	other := newChatToolGate(restarted, "s9", t.TempDir(), newChatStreamWriter(fresh, "s9", zerolog.New(io.Discard)))
	asked := make(chan struct{})
	go func() {
		defer close(asked)
		_, _ = other.Authorize(context.Background(), agentloop.ToolCallRequest{ToolName: "exec", ToolArgs: `{"command":"go test ./..."}`})
	}()
	p := fresh.next(t)
	_ = restarted.answer(p["request_id"].(string), "s9", chatPermissionAnswer{Decision: "deny"})
	<-asked
}

func TestChatAPI_PassesAlwaysAllowRulesToClaudeCode(t *testing.T) {
	client := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}}}
	srv := newPermissionTestServer(t, client)
	cwd := filepath.Join(srv.root, "artifacts", srv.session)
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	store := newChatAlwaysRuleStore(srv.root)
	_ = store.add(cwd, chatAlwaysRule{Provider: chatRuleProviderClaudeCode, Tool: "Bash", Content: "npm test:*"})
	_ = store.add(cwd, chatAlwaysRule{Provider: chatRuleProviderTARS, Tool: "exec", Content: "ls:*"})

	waitForEvent(t, srv.startChat(t, false), "done")
	if len(client.seenAllow) == 0 {
		t.Fatal("no llm call")
	}
	if got := client.seenAllow[0]; len(got) != 1 || got[0] != "Bash(npm test:*)" {
		t.Fatalf("allow rules passed = %v, want only the Claude Code rule", got)
	}
}

func TestChatPermissionRulesAPI(t *testing.T) {
	srv := newPermissionTestServer(t, &permissionAskingClient{})
	project := t.TempDir()
	store := newChatAlwaysRuleStore(srv.root)
	_ = store.add(project, chatAlwaysRule{Provider: chatRuleProviderClaudeCode, Tool: "Bash", Content: "npm test:*"})
	_ = store.add(project, chatAlwaysRule{Provider: chatRuleProviderTARS, Tool: "write_file"})
	base := srv.server.URL + "/v1/chat/permission-rules?dir=" + url.QueryEscape(project)

	resp, err := http.Get(base)
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Dir   string `json:"dir"`
		Rules []struct {
			Provider string `json:"provider"`
			Rule     string `json:"rule"`
		} `json:"rules"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&listed)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(listed.Rules) != 2 || listed.Rules[0].Rule != "Bash(npm test:*)" || listed.Rules[1].Provider != "tars" {
		t.Fatalf("GET %d %+v", resp.StatusCode, listed)
	}

	del := func(provider, rule string) int {
		req, _ := http.NewRequest(http.MethodDelete, base+"&provider="+provider+"&rule="+url.QueryEscape(rule), nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if code := del("claude-code", "Bash(npm test:*)"); code != http.StatusOK {
		t.Fatalf("DELETE status %d", code)
	}
	if code := del("claude-code", "Bash(npm test:*)"); code != http.StatusNotFound {
		t.Fatalf("second DELETE status %d", code)
	}
	if left := store.list(project); len(left) != 1 || left[0].Tool != "write_file" {
		t.Fatalf("left = %+v", left)
	}

	resp, err = http.Get(srv.server.URL + "/v1/chat/permission-rules")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET without dir: %d", resp.StatusCode)
	}
}
