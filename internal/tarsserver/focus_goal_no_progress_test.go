package tarsserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/ops"
)

// goalRepoDriver is goalDriver with the session working in a real git
// repository, and a verifier that always fails the same way.
func goalRepoDriver(t *testing.T, reply func(p focuspipeline.Pipeline, n int, prompt string) string, repo string) (*focusDriver, string, func() focuspipeline.Pipeline) {
	t.Helper()
	verifier := &fakeVerifier{result: func(_ int, command string) focuspipeline.VerificationResult {
		return focuspipeline.VerificationResult{Command: command, ExitCode: 1, Excerpt: "FAIL: the list is empty"}
	}}
	d, _, store, id := goalDriver(t, reply, verifier)
	if err := store.SetWorkDirs(id, []string{repo}, repo); err != nil {
		t.Fatal(err)
	}
	return d, id, func() focuspipeline.Pipeline { return pipelineOf(t, store, id) }
}

func goalRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "app.txt"), []byte("v0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitInit(t, repo, "main")
	return repo
}

// The team keeps saying nothing is wrong and changes nothing: goal mode
// ends after GoalEmptyRetryLimit empty retries, not after the budget.
func TestFocusGoalEndsOnEmptyRetries(t *testing.T) {
	repo := goalRepo(t)
	d, id, current := goalRepoDriver(t, stageReply, repo)
	var mu sync.Mutex
	var ended map[string]any
	var titles []string
	d.audit = func(e ops.AutomationAuditEntry) {
		if e.Result == "ended" {
			mu.Lock()
			ended = e.Details
			mu.Unlock()
		}
	}
	d.notify = func(_ context.Context, e notificationEvent) {
		mu.Lock()
		titles = append(titles, e.Title)
		mu.Unlock()
	}
	startGoal(t, d, d.sessions, id, 0)

	waitFor(t, "goal mode to end", func() bool { return !current().GoalActive() })
	waitDriverIdle(t, d, id)
	p := current()
	if p.GoalMode.EndReason != focuspipeline.GoalEndNoProgress || p.OpenGate != focuspipeline.GateBlocked {
		t.Fatalf("goal=%+v gate=%q", p.GoalMode, p.OpenGate)
	}
	if p.GoalMode.Pushes != focuspipeline.GoalEmptyRetryLimit || p.GoalMode.Pushes >= p.GoalMode.MaxPushes {
		t.Fatalf("pushes = %d of %d, want %d", p.GoalMode.Pushes, p.GoalMode.MaxPushes, focuspipeline.GoalEmptyRetryLimit)
	}
	if last := p.Cards[len(p.Cards)-1]; last.Title != focuspipeline.NoticeGoalEnded {
		t.Fatalf("last card = %+v", last)
	}
	waitFor(t, "the announcement", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(titles) > 0
	})
	mu.Lock()
	defer mu.Unlock()
	if titles[0] != "Goal mode stopped" || ended["reason"] != focuspipeline.GoalEndNoProgress || ended["empty_retries"] != focuspipeline.GoalEmptyRetryLimit {
		t.Fatalf("notices = %v, audit = %v", titles, ended)
	}
}

// A team that changes the tree on every round is still trying: goal mode
// keeps retrying until its budget, as before.
func TestFocusGoalKeepsRetryingWhenTheTreeChanges(t *testing.T) {
	repo := goalRepo(t)
	reply := func(p focuspipeline.Pipeline, n int, prompt string) string {
		if p.Current == focuspipeline.StagePlan {
			return focusGoalPlanReply
		}
		if err := os.WriteFile(filepath.Join(repo, "app.txt"), fmt.Appendf(nil, "v%d\n", n), 0o644); err != nil {
			t.Error(err)
		}
		return focusReport("done", true)
	}
	d, id, current := goalRepoDriver(t, reply, repo)
	startGoal(t, d, d.sessions, id, 4)

	waitFor(t, "goal mode to end", func() bool { return !current().GoalActive() })
	waitDriverIdle(t, d, id)
	p := current()
	if p.GoalMode.EndReason != focuspipeline.GoalEndExhausted || p.GoalMode.Pushes != 4 {
		t.Fatalf("goal=%+v", p.GoalMode)
	}
}
