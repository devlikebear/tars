package pulse

import (
	"context"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func TestScanner_AutoContinueGoal_DetectsCompletedOptedIn(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	src := &fakeChatSessionSource{
		root:     root,
		sessions: []session.Session{newOptedInSession("sess_goal", "ship feature", now.Add(-5*time.Minute))},
		tasks: map[string]session.SessionTasks{
			"sess_goal": {
				Plan: &session.Plan{
					Goal:                "ship the feature end-to-end",
					Status:              session.PlanStatusCompleted,
					AutoContinueEnabled: true,
				},
				Tasks: []session.Task{{ID: "t1", Title: "ship", Status: "completed"}},
			},
		},
	}

	sc := buildScanner(ScannerSources{ChatSessions: src}, Thresholds{}, now)
	got := sc.Scan(context.Background())
	if len(got) != 1 {
		t.Fatalf("want 1 signal, got %d (%+v)", len(got), got)
	}
	sig := got[0]
	if sig.Kind != SignalKindAutoContinueGoal {
		t.Fatalf("kind = %s, want %s", sig.Kind, SignalKindAutoContinueGoal)
	}
	if sig.Details["can_auto_continue"] != true {
		t.Fatalf("can_auto_continue = %+v", sig.Details["can_auto_continue"])
	}
	if sig.Details["autofix_candidate"] != "auto_continue_goal_plan" {
		t.Fatalf("autofix_candidate = %+v", sig.Details["autofix_candidate"])
	}
	if sig.Details["max_iterations"] != session.DefaultAutoContinueMaxIterations {
		t.Fatalf("max_iterations = %+v", sig.Details["max_iterations"])
	}
}

func TestScanner_AutoContinueGoal_SkipsWhenFlagOff(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	src := &fakeChatSessionSource{
		root:     root,
		sessions: []session.Session{newOptedInSession("sess_off", "off", now)},
		tasks: map[string]session.SessionTasks{
			"sess_off": {
				Plan: &session.Plan{
					Goal:                "x",
					Status:              session.PlanStatusCompleted,
					AutoContinueEnabled: false,
				},
			},
		},
	}
	sc := buildScanner(ScannerSources{ChatSessions: src}, Thresholds{}, now)
	for _, sig := range sc.Scan(context.Background()) {
		if sig.Kind == SignalKindAutoContinueGoal {
			t.Fatalf("should not emit auto-continue-goal signal when flag off: %+v", sig)
		}
	}
}

func TestScanner_AutoContinueGoal_SkipsWhenNotCompleted(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	src := &fakeChatSessionSource{
		root:     root,
		sessions: []session.Session{newOptedInSession("sess_exec", "executing", now)},
		tasks: map[string]session.SessionTasks{
			"sess_exec": {
				Plan: &session.Plan{
					Goal:                "x",
					Status:              session.PlanStatusExecuting,
					AutoContinueEnabled: true,
				},
				Tasks: []session.Task{{ID: "t", Status: "in_progress"}},
			},
		},
	}
	sc := buildScanner(ScannerSources{ChatSessions: src}, Thresholds{}, now)
	for _, sig := range sc.Scan(context.Background()) {
		if sig.Kind == SignalKindAutoContinueGoal {
			t.Fatalf("should not emit signal when plan still executing: %+v", sig)
		}
	}
}

func TestScanner_AutoContinueGoal_BlocksWhenConsentMissing(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	src := &fakeChatSessionSource{
		root: root,
		sessions: []session.Session{{
			ID:                "sess_no_consent",
			Title:             "no consent",
			UpdatedAt:         now,
			AutomationConsent: nil, // missing consent
		}},
		tasks: map[string]session.SessionTasks{
			"sess_no_consent": {
				Plan: &session.Plan{
					Goal:                "x",
					Status:              session.PlanStatusCompleted,
					AutoContinueEnabled: true,
				},
			},
		},
	}
	sc := buildScanner(ScannerSources{ChatSessions: src}, Thresholds{}, now)
	got := sc.Scan(context.Background())
	if len(got) != 1 {
		t.Fatalf("want 1 signal, got %d", len(got))
	}
	if got[0].Details["can_auto_continue"] != false {
		t.Fatalf("missing consent must block auto-continue: %+v", got[0].Details)
	}
}

func TestPlan_EffectiveAutoContinueMaxIterations_ClampsToHardCap(t *testing.T) {
	plan := &session.Plan{AutoContinueMaxIterations: 999}
	if got := plan.EffectiveAutoContinueMaxIterations(); got != session.AutoContinueIterationsHardCap {
		t.Fatalf("expected hard cap %d, got %d", session.AutoContinueIterationsHardCap, got)
	}
}

func TestPlan_EffectiveAutoContinueMaxIterations_FallsBackToDefault(t *testing.T) {
	plan := &session.Plan{}
	if got := plan.EffectiveAutoContinueMaxIterations(); got != session.DefaultAutoContinueMaxIterations {
		t.Fatalf("expected default %d, got %d", session.DefaultAutoContinueMaxIterations, got)
	}
}
