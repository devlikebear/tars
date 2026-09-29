//go:build integration

package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// These tests drive a real, signed-in `claude` through the stream-json control
// protocol. They skip when the CLI is not installed. Each one runs a short
// haiku turn with user customizations switched off (--safe-mode, strict MCP,
// Bash only), which keeps a run to about a cent.
//
//	go test -tags integration ./pkg/llm/ -run TestClaudeCodeCLIControlLive -v

func newLiveClaudeCodeClient(t *testing.T) (Client, string) {
	t.Helper()
	if _, err := FindClaudeCodeCLIPath(); err != nil {
		t.Skipf("claude cli not available: %v", err)
	}
	dir := t.TempDir()
	client, err := NewProvider(ProviderOptions{Provider: "claude-code-cli", Model: "haiku", WorkDir: dir})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	return client, dir
}

func liveControlOptions(dir string, handler ClaudeCodePermissionHandler) ChatOptions {
	return ChatOptions{
		WorkDir:                     dir,
		ClaudeCodePermissionMode:    "default",
		ClaudeCodePermissionHandler: handler,
		ClaudeCodeHarness: &ClaudeCodeHarnessOptions{
			SafeMode:      true,
			StrictMCP:     true,
			DisableChrome: true,
			Tools:         []string{"Bash"},
		},
	}
}

// recorder keeps every prompt the CLI raised.
type recorder struct {
	mu   sync.Mutex
	reqs []ClaudeCodePermissionRequest
}

func (r *recorder) add(req ClaudeCodePermissionRequest) {
	r.mu.Lock()
	r.reqs = append(r.reqs, req)
	r.mu.Unlock()
}

func (r *recorder) all() []ClaudeCodePermissionRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ClaudeCodePermissionRequest(nil), r.reqs...)
}

func logPrompts(t *testing.T, reqs []ClaudeCodePermissionRequest) {
	t.Helper()
	for i, req := range reqs {
		t.Logf("prompt %d: tool=%s input=%s reason_type=%q reason=%q agent=%q suggestions=%s",
			i+1, req.ToolName, req.Input, req.DecisionReasonType, req.DecisionReason, req.AgentID, req.Suggestions)
	}
}

func TestClaudeCodeCLIControlLive_DenyKeepsToolFromRunning(t *testing.T) {
	client, dir := newLiveClaudeCodeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	var rec recorder
	resp, err := client.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "Run exactly this shell command with the Bash tool: touch denied.txt"},
	}, liveControlOptions(dir, func(_ context.Context, req ClaudeCodePermissionRequest) (ClaudeCodePermissionDecision, error) {
		rec.add(req)
		return ClaudeCodePermissionDecision{Message: "denied by the TARS integration test"}, nil
	}))
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	reqs := rec.all()
	logPrompts(t, reqs)
	t.Logf("reply=%q cost=$%.4f session=%s", resp.Message.Content, resp.Usage.CostUSD, resp.SessionID)

	if len(reqs) == 0 || reqs[0].ToolName != "Bash" {
		t.Fatalf("expected a Bash permission prompt, got %+v", reqs)
	}
	if _, err := os.Stat(filepath.Join(dir, "denied.txt")); !os.IsNotExist(err) {
		t.Fatalf("denied command ran anyway (stat err = %v)", err)
	}
}

// TestClaudeCodeCLIControlLive_SessionRuleStopsReprompting allows the first
// prompt together with a session-scoped `Bash(touch:*)` rule and checks that
// the second touch is not asked about again.
//
// It sends its own rule rather than echoing Claude Code's suggestions: those
// name the exact command ("touch first.txt"), default to the localSettings
// destination (a write to the project's .claude/settings.local.json), and
// include a setMode acceptEdits entry that would let the second call through
// for a different reason.
func TestClaudeCodeCLIControlLive_SessionRuleStopsReprompting(t *testing.T) {
	client, dir := newLiveClaudeCodeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	var rec recorder
	_, err := client.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "Use the Bash tool twice, as two separate tool calls one after the other: first run `touch first.txt`, then run `touch second.txt`."},
	}, liveControlOptions(dir, func(_ context.Context, req ClaudeCodePermissionRequest) (ClaudeCodePermissionDecision, error) {
		rec.add(req)
		return ClaudeCodePermissionDecision{Allow: true, UpdatedPermissions: []json.RawMessage{
			json.RawMessage(`{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"touch:*"}],"behavior":"allow","destination":"session"}`),
		}}, nil
	}))
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	reqs := rec.all()
	logPrompts(t, reqs)
	for _, name := range []string{"first.txt", "second.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was not created: %v", name, err)
		}
	}
	if len(reqs) != 1 {
		t.Fatalf("prompts = %d, want 1: the session rule should cover the second touch", len(reqs))
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("a session-scoped rule must not write project settings (stat .claude: %v)", err)
	}
}

func TestClaudeCodeCLIControlLive_ResumedTurnStillPrompts(t *testing.T) {
	client, dir := newLiveClaudeCodeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	var rec recorder
	allow := func(_ context.Context, req ClaudeCodePermissionRequest) (ClaudeCodePermissionDecision, error) {
		rec.add(req)
		return ClaudeCodePermissionDecision{Allow: true}, nil
	}
	first := liveControlOptions(dir, allow)
	first.PersistSession = true
	resp, err := client.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "Remember the word PINEAPPLE. Reply with just OK."},
	}, first)
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}
	if resp.SessionID == "" {
		t.Fatal("first turn returned no session id to resume")
	}

	second := liveControlOptions(dir, allow)
	second.ResumeSessionID = resp.SessionID
	resp2, err := client.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "Remember the word PINEAPPLE. Reply with just OK."},
		{Role: "assistant", Content: resp.Message.Content},
		{Role: "user", Content: "Use the Bash tool to run exactly: touch resumed.txt  — then tell me the word I asked you to remember."},
	}, second)
	if err != nil {
		t.Fatalf("resumed turn: %v", err)
	}
	reqs := rec.all()
	logPrompts(t, reqs)
	t.Logf("resumed reply=%q session=%s", resp2.Message.Content, resp2.SessionID)

	if len(reqs) == 0 {
		t.Fatal("the resumed turn raised no permission prompt")
	}
	if _, err := os.Stat(filepath.Join(dir, "resumed.txt")); err != nil {
		t.Fatalf("allowed command did not run: %v", err)
	}
}

// TestClaudeCodeCLIControlLive_SubagentPromptCarriesAgentID delegates a
// command to an async subagent. Its prompt must reach the handler with the
// subagent's id even when it arrives after the parent's first result.
func TestClaudeCodeCLIControlLive_SubagentPromptCarriesAgentID(t *testing.T) {
	client, dir := newLiveClaudeCodeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	var rec recorder
	opts := liveControlOptions(dir, func(_ context.Context, req ClaudeCodePermissionRequest) (ClaudeCodePermissionDecision, error) {
		rec.add(req)
		return ClaudeCodePermissionDecision{Allow: true}, nil
	})
	opts.ClaudeCodeHarness.Tools = []string{"Bash", "Agent"}
	resp, err := client.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "Use the Agent tool to launch a general-purpose subagent. The subagent must run exactly this shell command with its Bash tool: touch sub.txt . Do not run it yourself."},
	}, opts)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	reqs := rec.all()
	logPrompts(t, reqs)
	t.Logf("reply=%q cost=$%.4f", resp.Message.Content, resp.Usage.CostUSD)

	var fromAgent bool
	for _, req := range reqs {
		if req.ToolName == "Bash" && req.AgentID != "" {
			fromAgent = true
		}
	}
	if !fromAgent {
		t.Fatalf("no Bash prompt carried an agent id: %+v", reqs)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub.txt")); err != nil {
		t.Fatalf("the subagent's allowed command did not run: %v", err)
	}
}
