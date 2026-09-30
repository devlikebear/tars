package llm

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestPartialUsageFromError(t *testing.T) {
	base := errors.New("cli timed out")
	spent := Usage{InputTokens: 3, CacheReadTokens: 100}
	err := withPartialUsage(base, spent, map[string]Usage{"claude-sonnet-4-6": spent})
	got, ok := PartialUsageFromError(fmt.Errorf("loop: %w", withUpstreamSession(err, "s")))
	if !ok || got.Usage != spent || got.ByModel["claude-sonnet-4-6"] != spent {
		t.Fatalf("partial = %+v, %v", got, ok)
	}
	if err.Error() != base.Error() || !errors.Is(err, base) {
		t.Fatalf("the wrapper must keep the message and chain: %v", err)
	}
	if withPartialUsage(base, Usage{}, nil) != base || withPartialUsage(nil, spent, nil) != nil {
		t.Fatal("nothing spent or no error: nothing to wrap")
	}
	if _, ok := PartialUsageFromError(base); ok {
		t.Fatal("plain errors carry no usage")
	}
}

// One API response arrives as several assistant events (a text block, then
// each tool_use block) repeating its message.id and usage.
const ccSplitResponses = `printf '%s\n' '{"type":"system","subtype":"init","session_id":"s"}'
printf '%s\n' '{"type":"assistant","message":{"id":"msg_1","model":"claude-sonnet-4-6","usage":{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":5000,"output_tokens":20},"content":[{"type":"text","text":"reading"}]}}'
printf '%s\n' '{"type":"assistant","message":{"id":"msg_1","model":"claude-sonnet-4-6","usage":{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":5000,"output_tokens":40},"content":[{"type":"tool_use","id":"t1","name":"Read","input":{}}]}}'
printf '%s\n' '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"x"}]}}'
printf '%s\n' '{"type":"assistant","message":{"id":"msg_2","model":"claude-sonnet-4-6","usage":{"input_tokens":2,"cache_creation_input_tokens":300,"cache_read_input_tokens":6000,"output_tokens":7},"content":[{"type":"tool_use","id":"t2","name":"Task","input":{}}]}}'
printf '%s\n' '{"type":"assistant","parent_tool_use_id":"t2","message":{"id":"msg_3","model":"claude-haiku-4-5-20251001","usage":{"input_tokens":50,"output_tokens":5},"content":[{"type":"text","text":"sub"}]}}'
`

// A call cut off before its result reports what its API requests spent:
// each message counted once, split by the model that spent it.
func TestClaudeCodeCLIChat_TimeoutReportsSpentUsage(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "claude")
	ccWriteStub(t, scriptPath, "#!/bin/sh\n"+ccSplitResponses+"sleep 30\n")
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)
	t.Setenv("CLAUDE_CODE_CLI_TIMEOUT", "1s")

	_, err := ccNewClient(t, dir).Chat(context.Background(), ccUserMsg(), ChatOptions{})
	if err == nil || !strings.Contains(err.Error(), "no output for 1s") {
		t.Fatalf("expected the idle timeout, got %v", err)
	}
	got, ok := PartialUsageFromError(err)
	if !ok {
		t.Fatalf("no usage on the error: %v", err)
	}
	want := Usage{InputTokens: 62, OutputTokens: 52, CacheWriteTokens: 1300, CacheReadTokens: 11000}
	if got.Usage != want {
		t.Fatalf("usage = %+v, want %+v", got.Usage, want)
	}
	if sonnet := got.ByModel["claude-sonnet-4-6"]; sonnet != (Usage{InputTokens: 12, OutputTokens: 47, CacheWriteTokens: 1300, CacheReadTokens: 11000}) {
		t.Fatalf("sonnet usage = %+v", sonnet)
	}
	if haiku := got.ByModel["claude-haiku-4-5-20251001"]; haiku != (Usage{InputTokens: 50, OutputTokens: 5}) {
		t.Fatalf("haiku usage = %+v", haiku)
	}
}

// An error result carries the CLI's own totals, cost included; those win.
func TestClaudeCodeCLIChat_ErrorResultReportsResultUsage(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "claude")
	ccWriteStub(t, scriptPath, "#!/bin/sh\n"+ccSplitResponses+
		`printf '%s\n' '{"type":"result","is_error":true,"result":"API Error: overloaded","total_cost_usd":0.42,"usage":{"input_tokens":70,"output_tokens":60,"cache_read_input_tokens":11000,"cache_creation_input_tokens":1300}}'`+"\n")
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	_, err := ccNewClient(t, dir).Chat(context.Background(), ccUserMsg(), ChatOptions{})
	got, ok := PartialUsageFromError(err)
	if !ok {
		t.Fatalf("no usage on the error: %v", err)
	}
	if got.Usage != (Usage{InputTokens: 70, OutputTokens: 60, CacheReadTokens: 11000, CacheWriteTokens: 1300, CostUSD: 0.42}) {
		t.Fatalf("usage = %+v", got.Usage)
	}
}

// A successful call keeps reporting the result's values, not the sum of
// the assistant events.
func TestClaudeCodeCLIChat_SuccessUsesResultUsage(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "claude")
	ccWriteStub(t, scriptPath, "#!/bin/sh\n"+ccSplitResponses+
		`printf '%s\n' '{"type":"result","is_error":false,"result":"done","total_cost_usd":0.5,"usage":{"input_tokens":1,"output_tokens":2}}'`+"\n")
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	resp, err := ccNewClient(t, dir).Chat(context.Background(), ccUserMsg(), ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Usage != (Usage{InputTokens: 1, OutputTokens: 2, CostUSD: 0.5}) {
		t.Fatalf("usage = %+v", resp.Usage)
	}
}

// A call that fails before any API request spent nothing to report.
func TestClaudeCodeCLIChat_FailureBeforeAnyRequestReportsNoUsage(t *testing.T) {
	client := ccSessionThenHang(t)
	_, err := client.Chat(context.Background(), ccUserMsg(), ChatOptions{})
	if _, ok := PartialUsageFromError(err); ok {
		t.Fatalf("expected no usage, got one on %v", err)
	}
}
