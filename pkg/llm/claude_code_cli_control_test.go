package llm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// controlStub writes a claude stand-in that speaks the stream-json control
// protocol the way Claude Code 2.1.283 does for one turn: it answers
// initialize, reads the user message, asks can_use_tool, emits the turn, and
// then waits for stdin to close before exiting. The last step makes a
// provider that forgets to close stdin hang until the test's CLI timeout.
//
// initReply is the control_response body for initialize, with REQ_ID standing
// in for the request id the provider chose.
func controlStub(t *testing.T, dir, initReply string) string {
	t.Helper()
	path := func(name string) string { return shellQuote(filepath.Join(dir, name)) }
	script := `#!/bin/sh
printf '%s\n' "$@" > ` + path("args.txt") + `
IFS= read -r init
printf '%s\n' "$init" > ` + path("init.json") + `
id=$(printf '%s' "$init" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
printf '%s\n' '` + initReply + `' | sed "s/REQ_ID/$id/"
IFS= read -r user || exit 3
printf '%s\n' "$user" > ` + path("user.json") + `
printf '%s\n' '{"type":"system","subtype":"init","session_id":"sess-ctl"}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"touch x"}}]}}'
printf '%s\n' '{"type":"control_request","request_id":"cli_1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"touch x","description":"Create x"},"tool_use_id":"toolu_1","permission_suggestions":[{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"touch:*"}],"behavior":"allow","destination":"session"}],"decision_reason":"touch writes a file","decision_reason_type":"rule","title":"Run touch?","agent_id":"agent-7"}}'
IFS= read -r answer
printf '%s\n' "$answer" > ` + path("answer.json") + `
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"num_turns":2,"session_id":"sess-ctl","stop_reason":"end_turn","total_cost_usd":0.01,"usage":{"input_tokens":3,"output_tokens":2},"result":"done"}'
cat > /dev/null
`
	scriptPath := filepath.Join(dir, "claude")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	return scriptPath
}

const okInitReply = `{"type":"control_response","response":{"subtype":"success","request_id":"REQ_ID","response":{"commands":[]}}}`

func newControlStubClient(t *testing.T, initReply string) (Client, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_CODE_CLI_PATH", controlStub(t, dir, initReply))
	// A provider that never closes stdin would otherwise hang for 5 minutes.
	t.Setenv("CLAUDE_CODE_CLI_TIMEOUT", "10s")
	client, err := NewProvider(ProviderOptions{Provider: "claude-code-cli", Model: "sonnet", WorkDir: dir})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	return client, dir
}

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Base(path), err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode %s: %v\n%s", filepath.Base(path), err, data)
	}
	return out
}

// answerBody digs the response payload out of a control_response frame.
func answerBody(t *testing.T, frame map[string]any) map[string]any {
	t.Helper()
	resp, _ := frame["response"].(map[string]any)
	if frame["type"] != "control_response" || (resp["request_id"] != "cli_1" && resp["request_id"] != "cli_9") {
		t.Fatalf("answer frame = %v", frame)
	}
	return resp
}

func TestClaudeCodeCLIChat_PermissionHandlerAllowsThroughControlProtocol(t *testing.T) {
	client, dir := newControlStubClient(t, okInitReply)

	var got ClaudeCodePermissionRequest
	calls := 0
	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "make x"},
	}, ChatOptions{
		ClaudeCodePermissionHandler: func(_ context.Context, req ClaudeCodePermissionRequest) (ClaudeCodePermissionDecision, error) {
			calls++
			got = req
			return ClaudeCodePermissionDecision{Allow: true, UpdatedPermissions: req.Suggestions}, nil
		},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Message.Content != "done" || resp.SessionID != "sess-ctl" || resp.Usage.CostUSD != 0.01 {
		t.Fatalf("response = %+v", resp)
	}
	if len(resp.ProviderExecutedTools) != 1 || resp.ProviderExecutedTools[0].Name != "Bash" {
		t.Fatalf("provider executed tools = %+v", resp.ProviderExecutedTools)
	}

	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
	if got.ToolName != "Bash" || got.ToolUseID != "toolu_1" || got.Title != "Run touch?" ||
		got.DecisionReason != "touch writes a file" || got.DecisionReasonType != "rule" || got.AgentID != "agent-7" {
		t.Fatalf("request = %+v", got)
	}
	if string(got.Input) != `{"command":"touch x","description":"Create x"}` {
		t.Fatalf("input = %s", got.Input)
	}
	if len(got.Suggestions) != 1 {
		t.Fatalf("suggestions = %s", got.Suggestions)
	}

	body := answerBody(t, readJSONFile(t, filepath.Join(dir, "answer.json")))
	if body["subtype"] != "success" {
		t.Fatalf("answer = %v", body)
	}
	decision := body["response"].(map[string]any)
	if decision["behavior"] != "allow" {
		t.Fatalf("decision = %v", decision)
	}
	// Claude Code runs the tool with updatedInput, so an unmodified allow
	// must echo the original input rather than omit it.
	if input, _ := decision["updatedInput"].(map[string]any); input["command"] != "touch x" {
		t.Fatalf("updatedInput = %v", decision["updatedInput"])
	}
	perms, _ := decision["updatedPermissions"].([]any)
	if len(perms) != 1 || perms[0].(map[string]any)["destination"] != "session" {
		t.Fatalf("updatedPermissions = %v", decision["updatedPermissions"])
	}

	init := readJSONFile(t, filepath.Join(dir, "init.json"))
	if req, _ := init["request"].(map[string]any); init["type"] != "control_request" || req["subtype"] != "initialize" {
		t.Fatalf("first frame must be initialize, got %v", init)
	}
	user := readJSONFile(t, filepath.Join(dir, "user.json"))
	if msg, _ := user["message"].(map[string]any); user["type"] != "user" || !strings.Contains(asString(msg["content"]), "make x") {
		t.Fatalf("user frame = %v", user)
	}

	argsData, err := os.ReadFile(filepath.Join(dir, "args.txt"))
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(argsData)
	if extractFlagValue(args, "--input-format") != "stream-json" || extractFlagValue(args, "--permission-prompt-tool") != "stdio" {
		t.Fatalf("control flags missing:\n%s", args)
	}
	// The prompt travels on stdin now; leaving it in argv as well would send
	// the turn twice.
	if strings.Contains(args, "make x") {
		t.Fatalf("prompt must not be passed in argv:\n%s", args)
	}
	if extractFlagValue(args, "--system-prompt") == "" {
		t.Fatalf("system prompt flag missing:\n%s", args)
	}
}

func TestClaudeCodeCLIChat_PermissionHandlerDenies(t *testing.T) {
	client, dir := newControlStubClient(t, okInitReply)

	_, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "make x"}}, ChatOptions{
		ClaudeCodePermissionHandler: func(context.Context, ClaudeCodePermissionRequest) (ClaudeCodePermissionDecision, error) {
			return ClaudeCodePermissionDecision{Interrupt: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	decision := answerBody(t, readJSONFile(t, filepath.Join(dir, "answer.json")))["response"].(map[string]any)
	if decision["behavior"] != "deny" || decision["interrupt"] != true {
		t.Fatalf("decision = %v", decision)
	}
	// Claude Code requires a message on deny; an empty one gets a default.
	if strings.TrimSpace(asString(decision["message"])) == "" {
		t.Fatalf("deny message is empty: %v", decision)
	}
	if _, ok := decision["updatedInput"]; ok {
		t.Fatalf("deny must not carry updatedInput: %v", decision)
	}
}

func TestClaudeCodeCLIChat_PermissionHandlerErrorBecomesErrorResponse(t *testing.T) {
	client, dir := newControlStubClient(t, okInitReply)

	_, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "make x"}}, ChatOptions{
		ClaudeCodePermissionHandler: func(context.Context, ClaudeCodePermissionRequest) (ClaudeCodePermissionDecision, error) {
			return ClaudeCodePermissionDecision{}, errors.New("approval store unavailable")
		},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	body := answerBody(t, readJSONFile(t, filepath.Join(dir, "answer.json")))
	if body["subtype"] != "error" || !strings.Contains(asString(body["error"]), "approval store unavailable") {
		t.Fatalf("answer = %v", body)
	}
}

func TestClaudeCodeCLIChat_InitializeFailureFailsTheTurn(t *testing.T) {
	client, _ := newControlStubClient(t, `{"type":"control_response","response":{"subtype":"error","request_id":"REQ_ID","error":"bad hooks"}}`)

	_, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "make x"}}, ChatOptions{
		ClaudeCodePermissionHandler: func(context.Context, ClaudeCodePermissionRequest) (ClaudeCodePermissionDecision, error) {
			t.Error("handler must not run when initialize fails")
			return ClaudeCodePermissionDecision{}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "bad hooks") {
		t.Fatalf("err = %v, want the initialize error", err)
	}
}

func TestClaudeCodeCLIChat_WithoutPermissionHandlerKeepsPrintMode(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\n" +
		`printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"ok"}'` + "\n"
	scriptPath := filepath.Join(dir, "claude")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)
	client, err := NewProvider(ProviderOptions{Provider: "claude-code-cli", Model: "sonnet", WorkDir: dir})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi there"}}, ChatOptions{}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(argsData)
	if strings.Contains(args, "--permission-prompt-tool") || strings.Contains(args, "--input-format") {
		t.Fatalf("print mode must not enable the control protocol:\n%s", args)
	}
	if !strings.Contains(args, "hi there") {
		t.Fatalf("print mode passes the prompt in argv:\n%s", args)
	}
}

func TestClaudeCodeRunEnd(t *testing.T) {
	ev := func(s string) map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatalf("bad event %s: %v", s, err)
		}
		return m
	}
	const (
		result      = `{"type":"result","subtype":"success"}`
		agentStart  = `{"type":"system","subtype":"task_started","task_id":"a1","task_type":"local_agent"}`
		shellStart  = `{"type":"system","subtype":"task_started","task_id":"b1","task_type":"local_bash"}`
		agentNotify = `{"type":"system","subtype":"task_notification","task_id":"a1","status":"completed"}`
		agentPatch  = `{"type":"system","subtype":"task_updated","task_id":"a1","patch":{"status":"killed"}}`
		agentAlive  = `{"type":"system","subtype":"task_updated","task_id":"a1","patch":{"status":"running"}}`
		running     = `{"type":"system","subtype":"session_state_changed","state":"running"}`
		idle        = `{"type":"system","subtype":"session_state_changed","state":"idle"}`
		assistant   = `{"type":"assistant","message":{"content":[]}}`
	)
	for _, tc := range []struct {
		name   string
		events []string
		// endsAt is the index of the event that ends the run, -1 for never.
		endsAt int
	}{
		{"plain turn ends at its result", []string{assistant, result}, 1},
		{"background shell does not hold the run", []string{shellStart, result}, 1},
		{"agent in flight holds the run to the next result", []string{agentStart, result, agentNotify, assistant, result}, 4},
		{"terminal task_updated clears the agent", []string{agentStart, result, agentPatch, result}, 3},
		{"non-terminal task_updated keeps the agent", []string{agentStart, result, agentAlive, result}, -1},
		{"reported running state waits for idle", []string{running, result, idle}, 2},
		{"idle before any result does not end the run", []string{idle, assistant, running, result, idle}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var r claudeCodeRunEnd
			got := -1
			for i, e := range tc.events {
				if r.observe(ev(e)) && got < 0 {
					got = i
				}
			}
			if got != tc.endsAt {
				t.Fatalf("run ended at event %d, want %d", got, tc.endsAt)
			}
		})
	}
}

// TestClaudeCodeCLIChat_KeepsStdinOpenForAgentStillRunning mirrors what
// Claude Code 2.1.283 did live: an async subagent outlives the parent's first
// result, asks for permission afterwards, and its completion wakes the parent
// for a second result. Closing stdin at the first result would have left that
// prompt unanswered.
func TestClaudeCodeCLIChat_KeepsStdinOpenForAgentStillRunning(t *testing.T) {
	dir := t.TempDir()
	path := func(name string) string { return shellQuote(filepath.Join(dir, name)) }
	script := `#!/bin/sh
IFS= read -r init
id=$(printf '%s' "$init" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
printf '%s\n' "{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"$id\",\"response\":{}}}"
IFS= read -r user
printf '%s\n' '{"type":"system","subtype":"task_started","task_id":"a1","task_type":"local_agent","description":"sub"}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"agent launched"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"agent launched","session_id":"s1"}'
printf '%s\n' '{"type":"control_request","request_id":"cli_9","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"touch sub.txt"},"tool_use_id":"toolu_9","agent_id":"agent-sub"}}'
IFS= read -r answer || exit 4
printf '%s\n' "$answer" > ` + path("answer.json") + `
printf '%s\n' '{"type":"system","subtype":"task_notification","task_id":"a1","status":"completed"}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"agent finished"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"agent finished","session_id":"s1"}'
cat > /dev/null
`
	scriptPath := filepath.Join(dir, "claude")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)
	t.Setenv("CLAUDE_CODE_CLI_TIMEOUT", "10s")
	client, err := NewProvider(ProviderOptions{Provider: "claude-code-cli", Model: "sonnet", WorkDir: dir})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	var agentID string
	resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "delegate"}}, ChatOptions{
		ClaudeCodePermissionHandler: func(_ context.Context, req ClaudeCodePermissionRequest) (ClaudeCodePermissionDecision, error) {
			agentID = req.AgentID
			return ClaudeCodePermissionDecision{Allow: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if agentID != "agent-sub" {
		t.Fatalf("the subagent's prompt did not reach the handler (agent id %q)", agentID)
	}
	if decision := answerBody(t, readJSONFile(t, filepath.Join(dir, "answer.json"))); decision["subtype"] != "success" {
		t.Fatalf("answer = %v", decision)
	}
	if !strings.Contains(resp.Message.Content, "agent finished") {
		t.Fatalf("the follow-up turn is missing from the reply: %q", resp.Message.Content)
	}
}
