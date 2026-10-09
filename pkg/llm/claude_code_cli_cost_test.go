package llm

import (
	"context"
	"math"
	"path/filepath"
	"testing"
)

func sameCost(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestClaudeCodeSessionCostsChargeEachCallItsOwnPart(t *testing.T) {
	l := newClaudeCodeSessionCosts(8)
	steps := []struct {
		name      string
		resume    string
		session   string
		total     float64
		wantSpent float64
	}{
		{"a fresh session is charged its total", "", "s1", 0.5, 0.5},
		{"a resumed call is charged the growth", "s1", "s1", 0.8, 0.3},
		{"a call that spent nothing is charged nothing", "s1", "s1", 0.8, 0},
		{"another session has its own total", "", "s2", 0.2, 0.2},
		{"a counter that started over is charged as it reads", "s1", "s1", 0.1, 0.1},
		{"a resume that moved to a new id still knows the old total", "s2", "s3", 0.9, 0.7},
		{"the new id carries on from there", "s3", "s3", 1.0, 0.1},
		{"an unknown earlier total charges nothing rather than the whole session", "gone", "gone", 4.2, 0},
		{"and is known from then on", "gone", "gone", 4.5, 0.3},
		{"no reported cost stays unreported", "s1", "s1", 0, 0},
	}
	for _, s := range steps {
		if got := l.callCost(s.resume, s.session, s.total); !sameCost(got, s.wantSpent) {
			t.Fatalf("%s: cost = %v, want %v", s.name, got, s.wantSpent)
		}
	}
}

func TestClaudeCodeSessionCostsForgetTheOldestSession(t *testing.T) {
	l := newClaudeCodeSessionCosts(2)
	l.callCost("", "a", 1)
	l.callCost("", "b", 1)
	l.callCost("", "c", 1)
	if len(l.totals) != 2 || len(l.order) != 2 {
		t.Fatalf("kept %d totals, %d ids; want 2", len(l.totals), len(l.order))
	}
	if got := l.callCost("a", "a", 3); got != 0 {
		t.Fatalf("a forgotten session's resume = %v, want 0 (unknown)", got)
	}
}

// Through the provider: the CLI reports the session's running total on
// every --resume, and each call's usage carries only what it added.
func TestClaudeCodeCLIChat_ResumedCallIsChargedTheGrowthOfTheSessionTotal(t *testing.T) {
	saved := claudeCodeSessionCosts
	claudeCodeSessionCosts = newClaudeCodeSessionCosts(8)
	t.Cleanup(func() { claudeCodeSessionCosts = saved })

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "claude")
	// The total grows with each run: 0.50, 0.80, then 0.80 again on an error.
	ccWriteStub(t, scriptPath, `#!/bin/sh
count=$(cat "`+dir+`/runs" 2>/dev/null || echo 0)
count=$((count + 1))
echo "$count" > "`+dir+`/runs"
printf '%s\n' '{"type":"system","subtype":"init","session_id":"sess-cost"}'
case "$count" in
  1) printf '%s\n' '{"type":"result","is_error":false,"result":"one","session_id":"sess-cost","total_cost_usd":0.5,"usage":{"input_tokens":1,"output_tokens":2}}' ;;
  2) printf '%s\n' '{"type":"result","is_error":false,"result":"two","session_id":"sess-cost","total_cost_usd":0.8,"usage":{"input_tokens":3,"output_tokens":4}}' ;;
  *) printf '%s\n' '{"type":"result","is_error":true,"result":"limit reached","session_id":"sess-cost","total_cost_usd":0.8,"usage":{}}' ;;
esac
`)
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)
	client := ccNewClient(t, dir)

	first, err := client.Chat(context.Background(), ccUserMsg(), ChatOptions{PersistSession: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.SessionID != "sess-cost" || !sameCost(first.Usage.CostUSD, 0.5) {
		t.Fatalf("first call: session %q cost %v, want sess-cost 0.5", first.SessionID, first.Usage.CostUSD)
	}
	second, err := client.Chat(context.Background(), ccUserMsg(), ChatOptions{ResumeSessionID: first.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if !sameCost(second.Usage.CostUSD, 0.3) || second.Usage.InputTokens != 3 {
		t.Fatalf("resumed call: usage %+v, want cost 0.3 (0.8 - 0.5)", second.Usage)
	}
	_, err = client.Chat(context.Background(), ccUserMsg(), ChatOptions{ResumeSessionID: first.SessionID})
	if err == nil {
		t.Fatal("the third call must fail")
	}
	if spent, ok := PartialUsageFromError(err); ok && spent.Usage.CostUSD != 0 {
		t.Fatalf("a failed call that spent nothing was charged %v", spent.Usage.CostUSD)
	}
}
