package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/focusprobe"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

const (
	soMainReview = "Reviewed.\n<focus-findings>" +
		`[{"id":"f1","severity":"high","file":"a.go","line":3,"title":"nil deref","scenario":"nil input → panic"}]` +
		"</focus-findings>"
	soSecondReview = "Second look.\n<focus-findings>" +
		`[{"id":"s1","severity":"medium","file":"a.go","line":5,"title":"missing nil check","scenario":"same"},` +
		`{"id":"s2","severity":"high","file":"c.go","line":1,"title":"leak","scenario":"open file never closed"}]` +
		"</focus-findings>"
	soNoFindings = "Reviewed.\n<focus-findings>[]</focus-findings>"
)

// soRepo is a git repository with one commit and an uncommitted change.
func soRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	repo := realTempDir(t)
	gitRun(t, repo, "init", "-q", "-b", "main")
	gitRun(t, repo, "config", "commit.gpgSign", "false")
	gitRun(t, repo, "config", "core.autocrlf", "false")
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package a\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", "init")
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package a\n\nfunc A() { var p *int; _ = *p }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

// soReviewSession makes a session in repo whose pipeline passed build
// into review, opted in to the second opinion review on tier ("" = not).
func soReviewSession(t *testing.T, store *session.Store, repo, tier string) session.Session {
	t.Helper()
	sess := focusSession(t, store, "goal")
	if repo != "" {
		if err := store.SetWorkDirs(sess.ID, []string{repo}, repo); err != nil {
			t.Fatal(err)
		}
	}
	plan := &focuspipeline.Plan{
		Goal: "g", Tasks: []focuspipeline.PlanTask{{Title: "t1", Done: "d1"}},
		Stages: []focuspipeline.StageID{focuspipeline.StagePlan, focuspipeline.StageBuild, focuspipeline.StageReview, focuspipeline.StagePR},
		Verify: []string{"make test"},
	}
	now := time.Now()
	if _, _, err := focusStoreFor(store).Update(sess.ID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		steps := []focuspipeline.Event{
			{Kind: focuspipeline.EventTurnCompleted, Turn: 1, Blocks: focuspipeline.Blocks{Plan: plan}},
			{Kind: focuspipeline.EventGate, Gate: focuspipeline.GatePlan, Action: focuspipeline.GateApprove},
			{Kind: focuspipeline.EventTurnCompleted, Turn: 2, Blocks: focuspipeline.Blocks{Report: &focuspipeline.Report{Summary: "built", TasksDone: true}}},
			{Kind: focuspipeline.EventVerification, Verification: &focuspipeline.Verification{Passed: true}},
		}
		for _, ev := range steps {
			next, _, err := focuspipeline.Apply(p, ev, now)
			if err != nil {
				return p, err
			}
			p = next
		}
		p.PendingTurn = ""
		if tier != "" {
			p.SecondOpinion = &focuspipeline.SecondOpinion{Tier: tier}
		}
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
	if p := pipelineOf(t, store, sess.ID); p.Current != focuspipeline.StageReview {
		t.Fatalf("not in review: %s", p.Current)
	}
	return sess
}

// fakeSecondOpinion answers second opinion turns with reply(n, prompt).
type fakeSecondOpinion struct {
	reply func(ctx context.Context, n int, prompt string) (string, error)

	mu      sync.Mutex
	prompts []string
	session []string
}

func (f *fakeSecondOpinion) run(ctx context.Context, sessionID, prompt string) (string, error) {
	f.mu.Lock()
	f.prompts = append(f.prompts, prompt)
	f.session = append(f.session, sessionID)
	n := len(f.prompts)
	f.mu.Unlock()
	return f.reply(ctx, n, prompt)
}

func (f *fakeSecondOpinion) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompts...)
}

func soAnswers(replies ...string) *fakeSecondOpinion {
	return &fakeSecondOpinion{reply: func(_ context.Context, n int, _ string) (string, error) {
		return replies[min(n, len(replies))-1], nil
	}}
}

func testSecondOpinionDriver(t *testing.T, mainReply string, so *fakeSecondOpinion) (*focusDriver, *fakeFocusTurns, *session.Store, string, string) {
	t.Helper()
	store := session.NewStore(t.TempDir())
	repo := soRepo(t)
	sess := soReviewSession(t, store, repo, "light")
	d := newFocusDriver(zerolog.Nop())
	t.Cleanup(func() { d.Close(context.Background()) })
	d.idlePoll = 5 * time.Millisecond
	d.sessions = store
	d.feeds = newChatTurnFeeds()
	d.activity = newChatActivity(store, nil)
	d.cancels = newChatCancelRegistry()
	turns := &fakeFocusTurns{t: t, store: store, driver: d, reply: func(n int, _ string) string {
		if n == 1 {
			return mainReply
		}
		return focusReport("next", false)
	}}
	d.runTurn = turns.run
	d.verify = (&fakeVerifier{result: passAll}).verify
	d.secondOpinionTurn = so.run
	d.secondOpinionTiers = func(string) (bool, error) { return false, nil }
	return d, turns, store, sess.ID, repo
}

func noticesOf(p focuspipeline.Pipeline, title string) []map[string]any {
	var out []map[string]any
	for _, c := range p.Cards {
		if c.Kind == focuspipeline.CardNotice && c.Title == title {
			var payload map[string]any
			_ = json.Unmarshal(c.Payload, &payload)
			out = append(out, payload)
		}
	}
	return out
}

func findingsOf(t *testing.T, p focuspipeline.Pipeline) []focuspipeline.Finding {
	t.Helper()
	var out []focuspipeline.Finding
	for _, c := range p.Cards {
		if c.Kind == focuspipeline.CardFinding {
			var f focuspipeline.Finding
			if err := json.Unmarshal(c.Payload, &f); err != nil {
				t.Fatal(err)
			}
			out = append(out, f)
		}
	}
	return out
}

func TestFocusSecondOpinionJoinsTheTriage(t *testing.T) {
	so := soAnswers(soSecondReview)
	d, turns, store, id, repo := testSecondOpinionDriver(t, soMainReview, so)
	d.start(id, sendTurn("Start the review stage."), "")
	waitDriverIdle(t, d, id)

	p := pipelineOf(t, store, id)
	if p.OpenGate != focuspipeline.GateTriage || p.Review.SecondOpinionOwed || len(turns.seen()) != 1 {
		t.Fatalf("gate = %q owed = %v turns = %d", p.OpenGate, p.Review.SecondOpinionOwed, len(turns.seen()))
	}
	findings := findingsOf(t, p)
	if len(findings) != 2 || len(findings[0].ConfirmedBy) != 1 || findings[1].Source != focuspipeline.FindingSourceSecondOpinion || findings[1].File != "c.go" {
		t.Fatalf("findings = %+v", findings)
	}
	prompts := so.seen()
	if len(prompts) != 1 || !strings.Contains(prompts[0], "+func A() { var p *int; _ = *p }") || strings.Contains(prompts[0], "nil deref") {
		t.Fatalf("second opinion prompt:\n%s", prompts)
	}

	// The worker session: hidden, read-only, on the tier, in the folder.
	soID := p.SecondOpinion.SessionID
	worker, err := store.Get(soID)
	if err != nil {
		t.Fatal(err)
	}
	if !worker.Hidden || worker.Kind != "worker" || worker.PermissionMode != chatPermissionModePlan || worker.TierPin != "light" ||
		worker.CurrentDir != repo || worker.Isolation != session.IsolationOff || so.session[0] != soID {
		t.Fatalf("worker session = %+v", worker)
	}
	parent, _ := store.Get(id)
	again, err := d.secondOpinionSession(id, parent, "heavy")
	if err != nil || again != soID {
		t.Fatalf("the session is reused: %q %v", again, err)
	}
	if worker, _ = store.Get(soID); worker.TierPin != "heavy" {
		t.Fatalf("tier pin follows the pipeline: %q", worker.TierPin)
	}

	// Deleting the pipeline's session removes the worker session too.
	store.SetDeleteHook(focusPipelineCleanup(store, zerolog.Nop()))
	if err := store.Delete(id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the second opinion session delete", func() bool { _, err := store.Get(soID); return err != nil })
}

func TestFocusSecondOpinionFailures(t *testing.T) {
	tests := []struct {
		name       string
		so         *fakeSecondOpinion
		wantReason string
		wantCalls  int
	}{
		{"timeout", &fakeSecondOpinion{reply: func(ctx context.Context, _ int, _ string) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		}}, focuspipeline.SecondOpinionTimeout, 1},
		{"error", &fakeSecondOpinion{reply: func(context.Context, int, string) (string, error) {
			return "", errors.New("provider exploded with secret detail")
		}}, focuspipeline.SecondOpinionError, 1},
		{"no block twice", soAnswers("I looked, all fine.", "Still nothing."), focuspipeline.SecondOpinionNoBlock, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prev := focusSecondOpinionTimeout
			focusSecondOpinionTimeout = 100 * time.Millisecond
			t.Cleanup(func() { focusSecondOpinionTimeout = prev })
			d, _, store, id, _ := testSecondOpinionDriver(t, soNoFindings, tc.so)
			d.start(id, sendTurn("Start the review stage."), "")
			waitDriverIdle(t, d, id)

			p := pipelineOf(t, store, id)
			notices := noticesOf(p, focuspipeline.NoticeSecondOpinionFailed)
			if len(notices) != 1 || notices[0]["reason"] != tc.wantReason || notices[0]["tier"] != "light" || len(notices[0]) != 2 {
				t.Fatalf("notices = %v", notices)
			}
			// No finding from either review: verification ran and the
			// stage ended, as without a second opinion.
			if p.Current != focuspipeline.StagePR || len(tc.so.seen()) != tc.wantCalls {
				t.Fatalf("current = %s calls = %d", p.Current, len(tc.so.seen()))
			}
			if tc.wantCalls == 2 && tc.so.seen()[1] != focuspipeline.SecondOpinionReask {
				t.Fatalf("re-ask = %q", tc.so.seen()[1])
			}
		})
	}
}

func TestFocusSecondOpinionReaskSucceeds(t *testing.T) {
	so := soAnswers("Forgot the block.", soSecondReview)
	d, _, store, id, _ := testSecondOpinionDriver(t, soNoFindings, so)
	d.start(id, sendTurn("Start the review stage."), "")
	waitDriverIdle(t, d, id)
	p := pipelineOf(t, store, id)
	if len(so.seen()) != 2 || p.OpenGate != focuspipeline.GateTriage || len(findingsOf(t, p)) != 2 {
		t.Fatalf("calls = %d gate = %q findings = %d", len(so.seen()), p.OpenGate, len(findingsOf(t, p)))
	}
}

func TestFocusSecondOpinionUnavailableTier(t *testing.T) {
	so := soAnswers(soSecondReview)
	d, _, store, id, _ := testSecondOpinionDriver(t, soNoFindings, so)
	d.secondOpinionTiers = func(string) (bool, error) { return false, errors.New("tier light is gone") }
	d.start(id, sendTurn("Start the review stage."), "")
	waitDriverIdle(t, d, id)
	p := pipelineOf(t, store, id)
	if n := noticesOf(p, focuspipeline.NoticeSecondOpinionFailed); len(n) != 1 || n[0]["reason"] != focuspipeline.SecondOpinionUnavailable || len(so.seen()) != 0 {
		t.Fatalf("notices = %v calls = %d", n, len(so.seen()))
	}
}

func TestFocusSecondOpinionCancelLeavesItOwed(t *testing.T) {
	started := make(chan struct{})
	so := &fakeSecondOpinion{reply: func(ctx context.Context, _ int, _ string) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}}
	d, _, store, id, _ := testSecondOpinionDriver(t, soMainReview, so)
	d.start(id, sendTurn("Start the review stage."), "")
	<-started
	if !d.cancels.Running(id) {
		t.Fatal("the step claims the pipeline's session while it runs")
	}
	if !d.cancel(id) {
		t.Fatal("no run to cancel")
	}
	p := pipelineOf(t, store, id)
	if !p.Review.SecondOpinionOwed || len(noticesOf(p, focuspipeline.NoticeSecondOpinionFailed)) != 0 || p.SecondOpinion.Done != nil {
		t.Fatalf("a cancelled second opinion is still owed: %+v", p.Review)
	}
	// A restart now raises the interrupted gate; its retry runs it again.
	if n := interruptFocusPipelines(store, time.Now(), zerolog.Nop()); n != 1 {
		t.Fatalf("interrupted = %d", n)
	}
	if p = pipelineOf(t, store, id); p.OpenGate != focuspipeline.GateBlocked {
		t.Fatalf("gate = %q", p.OpenGate)
	}
}

func TestFocusSecondOpinionStepAllowed(t *testing.T) {
	store := session.NewStore(t.TempDir())
	owed := soReviewSession(t, store, "", "light")
	if _, _, err := focusStoreFor(store).Update(owed.ID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p.Review.SecondOpinionOwed = true
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
	idle := soReviewSession(t, store, "", "light")
	d := newFocusDriver(zerolog.Nop())
	d.sessions = store
	act := focuspipeline.Action{Kind: focuspipeline.ActionSecondOpinion}
	if !d.stepAllowed(owed.ID, act) || d.stepAllowed(idle.ID, act) {
		t.Fatal("a second opinion runs only while one is owed")
	}
}

// The real runner: one read-only turn on the worker session, through the
// chat turn path, that never writes to the pipeline's own transcript.
func TestFocusSecondOpinionRunsAReadOnlyTurn(t *testing.T) {
	client := &mockLLMClient{responses: []llm.ChatResponse{
		{Message: llm.ChatMessage{Role: "assistant", Content: soMainReview}},
		{Message: llm.ChatMessage{Role: "assistant", Content: soSecondReview}},
	}}
	_, d, store := focusChatHarness(t, client, &fakeVerifier{result: passAll})
	d.secondOpinionTiers = func(string) (bool, error) { return false, nil }
	repo := soRepo(t)
	sess := soReviewSession(t, store, repo, "light")
	d.start(sess.ID, sendTurn("Start the review stage."), "")
	waitFor(t, "both reviews", func() bool {
		client.mu.Lock()
		defer client.mu.Unlock()
		return client.callCount == 2
	})
	waitDriverIdle(t, d, sess.ID)

	p := pipelineOf(t, store, sess.ID)
	if p.OpenGate != focuspipeline.GateTriage || len(findingsOf(t, p)) != 2 {
		t.Fatalf("gate = %q findings = %d", p.OpenGate, len(findingsOf(t, p)))
	}
	messages, _ := session.ReadMessages(store.TranscriptPath(p.SecondOpinion.SessionID))
	if len(messages) != 2 || !strings.Contains(messages[0].Content, "second opinion") || strings.Contains(messages[0].Content, "<focus-stage>") {
		t.Fatalf("worker transcript = %+v", messages)
	}
	parent, _ := session.ReadMessages(store.TranscriptPath(sess.ID))
	for _, m := range parent {
		if strings.Contains(m.Content, "second opinion") {
			t.Fatal("the second opinion wrote to the pipeline's session")
		}
	}
	client.mu.Lock()
	workDir := client.seenWorkDirs[1]
	client.mu.Unlock()
	if workDir != repo {
		t.Fatalf("second opinion ran in %q, want %q", workDir, repo)
	}
}

func TestFocusCreateSecondOpinion(t *testing.T) {
	f := newWorktreeFixture(t)
	d := newFocusDriver(zerolog.Nop())
	t.Cleanup(func() { d.Close(context.Background()) })
	d.secondOpinionTurn = func(context.Context, string, string) (string, error) { return "", nil }
	d.secondOpinionTiers = func(tier string) (bool, error) {
		switch tier {
		case "heavy":
			return true, nil
		case "light":
			return false, errors.New("tier light runs on antigravity-cli, which cannot keep a review read-only")
		}
		return false, nil
	}
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	create := func(extra string) (int, map[string]any) {
		rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", `{"goal":"g","cwd":`+jsonString(f.repo)+extra+`}`, true)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	for _, bad := range []string{`,"second_opinion":{"tier":"huge"}`, `,"second_opinion":{"tier":"light"}`, `,"second_opinion":{}`} {
		if code, out := create(bad); code != http.StatusBadRequest || !strings.Contains(out["error"].(string), "second_opinion") {
			t.Fatalf("%s: %d %v", bad, code, out)
		}
	}
	if list, _ := f.store.List(); len(list) != 0 {
		t.Fatalf("a refused second opinion left a session: %d", len(list))
	}

	code, out := create(`,"second_opinion":{"tier":" Standard "}`)
	if code != http.StatusCreated || out["second_opinion_warning"] != nil {
		t.Fatalf("standard: %d %v", code, out)
	}
	if p := pipelineOf(t, f.store, out["session_id"].(string)); p.SecondOpinion == nil || p.SecondOpinion.Tier != "standard" {
		t.Fatalf("stored = %+v", p.SecondOpinion)
	}
	if code, out = create(`,"second_opinion":{"tier":"heavy"}`); code != http.StatusCreated || out["second_opinion_warning"] != focusSecondOpinionSameModel {
		t.Fatalf("heavy: %d %v", code, out)
	}
	code, out = create(``)
	if code != http.StatusCreated {
		t.Fatalf("plain: %d", code)
	}
	raw, _ := os.ReadFile(filepath.Join(f.store.WorkspaceDir(), "sessions", out["session_id"].(string)+".pipeline.json"))
	if strings.Contains(string(raw), "second_opinion") {
		t.Fatalf("a pipeline that did not opt in records nothing:\n%s", raw)
	}

	// A server that cannot run the review refuses the option, not the task.
	plain := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	rec := focusRequest(t, plain, http.MethodPost, "/v1/focus/pipelines", `{"goal":"g","cwd":`+jsonString(f.repo)+`,"second_opinion":{"tier":"heavy"}}`, true)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no driver: %d", rec.Code)
	}
}

func TestNewFocusSecondOpinionTiers(t *testing.T) {
	cfg := config.Config{LLMConfig: config.LLMConfig{
		LLMProviders: map[string]config.LLMProviderSettings{
			"anth": {Kind: "anthropic", AuthMode: "api-key", APIKey: "k", BaseURL: "https://api.anthropic.com"},
			"oai":  {Kind: "openai", AuthMode: "api-key", APIKey: "k", BaseURL: "https://api.openai.com/v1"},
			"agy":  {Kind: "antigravity-cli"},
		},
		LLMTiers: map[string]config.LLMTierBinding{
			"heavy":    {Provider: "anth", Model: "claude-opus-5"},
			"standard": {Provider: "oai", Model: "gpt-5"},
			"light":    {Provider: "agy", Model: "gemini-flash"},
		},
		LLMDefaultTier:  "standard",
		LLMRoleDefaults: map[string]string{"chat_main": "heavy"},
	}}
	check := newFocusSecondOpinionTiers(cfg)
	if same, err := check("heavy"); err != nil || !same {
		t.Fatalf("heavy = %v %v", same, err)
	}
	if same, err := check("standard"); err != nil || same {
		t.Fatalf("standard = %v %v", same, err)
	}
	if _, err := check("light"); err == nil || !strings.Contains(err.Error(), "antigravity-cli") {
		t.Fatalf("light = %v", err)
	}
	if _, err := check("huge"); err == nil {
		t.Fatal("an unknown tier is refused")
	}
	delete(cfg.LLMTiers, "standard")
	if _, err := newFocusSecondOpinionTiers(cfg)("standard"); err == nil {
		t.Fatal("an unmapped tier is refused")
	}
}

func TestFocusSecondOpinionChange(t *testing.T) {
	cut := focusprobe.ReviewDiff{Base: "abc1234", Diff: "+a ``` b", Truncated: true, Stat: "stat line", Untracked: []string{"new.go"}}
	if got := focusSecondOpinionChange(cut); !strings.Contains(got, "cut below") || !strings.Contains(got, "ˋˋˋ") ||
		!strings.Contains(got, "- new.go") || !strings.Contains(got, "stat line") {
		t.Fatalf("change:\n%s", got)
	}
	if got := focusSecondOpinionChange(focusprobe.ReviewDiff{Base: "HEAD"}); !strings.Contains(got, "shows no change") {
		t.Fatalf("empty:\n%s", got)
	}
	if got := focusSecondOpinionChange(focusprobe.ReviewDiff{}); !strings.Contains(got, "could not read") {
		t.Fatalf("no git:\n%s", got)
	}
}
