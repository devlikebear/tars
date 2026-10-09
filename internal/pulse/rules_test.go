package pulse

import (
	"context"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/pulse/autofix"
)

func TestDecideByRules(t *testing.T) {
	policy := DeciderPolicy{AllowedAutofixes: []string{"auto_continue_chat", "compress_old_logs"}, MinSeverity: SeverityWarn}
	stalled := func(ready bool) Signal {
		details := map[string]any{"autofix_candidate": "auto_continue_chat", "can_auto_resume": ready}
		if ready {
			details["has_auto_resume_candidate"] = true
		}
		return Signal{Kind: SignalKindStalledChat, Severity: SeverityWarn, Summary: "1 chat stalled", Details: details}
	}
	cron := Signal{Kind: SignalKindCronFailures, Severity: SeverityError, Summary: "2 cron jobs failing"}
	disk := Signal{Kind: SignalKindDiskUsage, Severity: SeverityWarn, Summary: "disk usage at 90.0%"}

	tests := []struct {
		name    string
		signals []Signal
		policy  DeciderPolicy
		action  Action
		autofix string
		title   string
	}{
		{name: "ready autofix runs", signals: []Signal{cron, stalled(true)}, policy: policy, action: ActionAutofix, autofix: "auto_continue_chat"},
		{name: "autofix not ready notifies", signals: []Signal{stalled(false)}, policy: policy, action: ActionNotify, title: "1 chat stalled"},
		{name: "autofix not allowed notifies", signals: []Signal{stalled(true)}, policy: DeciderPolicy{MinSeverity: SeverityWarn}, action: ActionNotify, title: "1 chat stalled"},
		{name: "disk warn compresses logs", signals: []Signal{disk}, policy: policy, action: ActionAutofix, autofix: "compress_old_logs"},
		{name: "disk warn without compression notifies", signals: []Signal{disk}, policy: DeciderPolicy{MinSeverity: SeverityWarn}, action: ActionNotify, title: "disk usage at 90.0%"},
		{name: "worst signal titles the notification", signals: []Signal{stalled(false), cron}, policy: policy, action: ActionNotify, title: "2 cron jobs failing"},
		{name: "below the floor is ignored", signals: []Signal{stalled(false)}, policy: DeciderPolicy{MinSeverity: SeverityError}, action: ActionIgnore},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := DecideByRules(tt.signals, tt.policy)
			if d.Action != tt.action || d.AutofixName != tt.autofix {
				t.Fatalf("decision = %+v, want action %s autofix %q", d, tt.action, tt.autofix)
			}
			if tt.title != "" && d.Title != tt.title {
				t.Fatalf("title = %q, want %q", d.Title, tt.title)
			}
		})
	}
}

func TestFingerprintIgnoresDriftingNumbers(t *testing.T) {
	signal := func(age int, session string, ready bool) []Signal {
		return []Signal{{Kind: SignalKindStalledChat, Severity: SeverityWarn, Summary: "stalled", Details: map[string]any{
			"age_minutes": age, "session_id": session, "can_auto_resume": ready,
			"sessions": []map[string]any{{"session_id": session, "age_minutes": age}},
		}}}
	}
	base := Fingerprint(signal(10, "s1", false))
	if got := Fingerprint(signal(703, "s1", false)); got != base {
		t.Fatal("an older age must not read as a new situation")
	}
	if got := Fingerprint(signal(10, "s2", false)); got == base {
		t.Fatal("another session must read as a new situation")
	}
	if got := Fingerprint(signal(10, "s1", true)); got == base {
		t.Fatal("an autofix becoming ready must read as a new situation")
	}
	escalated := signal(10, "s1", false)
	escalated[0].Severity = SeverityError
	if got := Fingerprint(escalated); got == base {
		t.Fatal("a higher severity must read as a new situation")
	}
}

// rulesRuntime wires a disk-usage signal to a rules-mode runtime whose LLM
// client fails the test when called.
func rulesRuntime(t *testing.T, cfg Config, allowed []string, fixer *fakeAutofix) (*Runtime, *captureNotifier, *fakeLLMClient) {
	t.Helper()
	scanner := buildScanner(
		ScannerSources{Ops: &fakeOpsStatus{status: ops.Status{DiskUsedPercent: 90}}},
		Thresholds{DiskUsedPercentWarn: 85},
		time.Now(),
	)
	client := &fakeLLMClient{}
	notif := &captureNotifier{}
	reg := autofix.NewRegistry()
	if fixer != nil {
		reg.Register(fixer)
	}
	cfg.Enabled = true
	rt := NewRuntime(cfg, Dependencies{
		Scanner:   scanner,
		Decider:   NewDecider(routerForClient(client), DeciderPolicy{AllowedAutofixes: allowed, MinSeverity: SeverityWarn}),
		Router:    NewNotifyRouter(notif, NotifyConfig{}),
		Autofixes: reg,
		State:     NewState(10),
	})
	return rt, notif, client
}

func TestRuntime_RulesNotifyOncePerSituation(t *testing.T) {
	rt, notif, client := rulesRuntime(t, Config{}, nil, nil)

	first := rt.RunOnce(context.Background())
	if first.DeciderInvoked || client.lastMessages != nil {
		t.Fatal("rules mode must not call the LLM")
	}
	if first.Decision == nil || first.Decision.Action != ActionNotify || !first.NotifyDelivered {
		t.Fatalf("first tick = %+v", first)
	}
	for i := 0; i < 3; i++ {
		out := rt.RunOnce(context.Background())
		if !out.Skipped || out.SkipReason != "signals_unchanged" {
			t.Fatalf("tick %d on the same signals = %+v", i+2, out)
		}
	}
	if len(notif.events) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notif.events))
	}

	// Past RedecideAfter the same situation is reported again.
	rt.mu.Lock()
	rt.lastDecidedAt = time.Now().Add(-7 * time.Hour)
	rt.mu.Unlock()
	if out := rt.RunOnce(context.Background()); out.Skipped || len(notif.events) != 2 {
		t.Fatalf("tick after the wait = %+v, notifications = %d", out, len(notif.events))
	}
}

func TestRuntime_IdleAutofixRetriesSoonerWithBackoff(t *testing.T) {
	rt, _, _ := rulesRuntime(t, Config{}, []string{"compress_old_logs"}, &fakeAutofix{name: "compress_old_logs"})

	if out := rt.RunOnce(context.Background()); out.AutofixAttempt != "compress_old_logs" || !out.AutofixOK {
		t.Fatalf("first tick = %+v", out)
	}
	age := func(d time.Duration) {
		rt.mu.Lock()
		rt.lastDecidedAt = time.Now().Add(-d)
		rt.mu.Unlock()
	}
	age(4 * time.Minute)
	if out := rt.RunOnce(context.Background()); !out.Skipped {
		t.Fatalf("retry before the first wait = %+v", out)
	}
	age(6 * time.Minute)
	if out := rt.RunOnce(context.Background()); out.Skipped {
		t.Fatalf("retry after the first wait = %+v", out)
	}
	// Second idle attempt: the wait doubles to 10 minutes.
	age(6 * time.Minute)
	if out := rt.RunOnce(context.Background()); !out.Skipped {
		t.Fatalf("retry before the doubled wait = %+v", out)
	}
}

func TestRuntime_LLMModeCallsOncePerSituation(t *testing.T) {
	rt, _, client := rulesRuntime(t, Config{UseLLM: true}, nil, nil)
	client.resp = makeToolCall(`{"action":"ignore","severity":"info"}`)

	if out := rt.RunOnce(context.Background()); !out.DeciderInvoked {
		t.Fatalf("first tick = %+v", out)
	}
	client.lastMessages = nil
	if out := rt.RunOnce(context.Background()); !out.Skipped || client.lastMessages != nil {
		t.Fatalf("second tick on the same signals called the LLM: %+v", out)
	}
}

func TestDecider_TextReplyForProvidersWithoutToolCalling(t *testing.T) {
	client := &fakeLLMClient{resp: llm.ChatResponse{Message: llm.ChatMessage{
		Role:    "assistant",
		Content: "```json\n{\"action\":\"notify\",\"severity\":\"warn\",\"title\":\"disk filling up\"}\n```",
	}}}
	decider := NewDecider(routerForClientWithProvider(client, "claude-code-cli"), DeciderPolicy{MinSeverity: SeverityWarn})

	d, err := decider.Decide(context.Background(), []Signal{{Kind: SignalKindDiskUsage, Severity: SeverityWarn, Summary: "disk usage at 90.0%"}})
	if err != nil {
		t.Fatal(err)
	}
	if d.Action != ActionNotify || d.Title != "disk filling up" {
		t.Fatalf("decision = %+v", d)
	}
	if len(client.lastOpts.Tools) != 0 {
		t.Fatal("a provider without tool calling must not be handed pulse_decide")
	}
	harness := client.lastOpts.ClaudeCodeHarness
	if harness == nil || harness.Tools == nil || len(harness.Tools) != 0 || harness.MaxTurns != 1 {
		t.Fatalf("the CLI call must be decision-only: %+v", harness)
	}

	client.resp.Message.Content = "I could not decide."
	if _, err := decider.Decide(context.Background(), []Signal{{Kind: SignalKindDiskUsage, Severity: SeverityWarn}}); err == nil {
		t.Fatal("a reply without a JSON object must be an error")
	}
}
