package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/checkpoint"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/focusprobe"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

const (
	promotionFindingKey = "finding:*.go|racy map"
	promotionVerifyKey  = "verify:make test"
	promotionReply      = `{"checks":[{"key":"finding:*.go|racy map","kind":"script","command":"sh scripts/race.sh",` +
		`"files":[{"path":"scripts/race.sh","content":"! grep -q racy a.go\n"}],"summary":"no racy map"}]}`
)

var promotionCardAt = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

// promotionRepo is a git repository for the replay's clone.
func promotionRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	repo := t.TempDir()
	gitRun(t, repo, "init", "-q", "-b", "main")
	gitRun(t, repo, "config", "commit.gpgSign", "false")
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", "init")
	return repo
}

// promotionFake is a promotion whose fix turn is t2 (t1 started before the
// finding), exporting a.go racy before the fix and safe after it, and whose
// runner is a plain shell (the sandbox has its own tests).
func promotionFake(entries []checkpoint.Entry) *focusPromotion {
	return &focusPromotion{
		run: func(ctx context.Context, dir, command string) (focusprobe.ReplayRun, error) {
			return focusprobe.RunReplayCommand(ctx, dir, "/bin/sh", []string{"-c", command})
		},
		turns: func(string) ([]checkpoint.Entry, error) { return entries, nil },
		export: func(_ context.Context, _, turnID, side, dest string) error {
			if turnID != "t2" {
				return errors.New("wrong turn")
			}
			content := "safe\n"
			if side == checkpoint.SideStart {
				content = "racy\n"
			}
			return os.WriteFile(filepath.Join(dest, "a.go"), []byte(content), 0o644)
		},
	}
}

func promotionEntries() []checkpoint.Entry {
	return []checkpoint.Entry{
		{TurnID: "t1", StartedAt: promotionCardAt.Add(-time.Hour), Start: "a", End: "b", Files: 1},
		{TurnID: "t1b", StartedAt: promotionCardAt.Add(time.Minute), Skipped: "too_large"},
		{TurnID: "t2", StartedAt: promotionCardAt.Add(2 * time.Minute), Start: "c", End: "d", Files: 2},
	}
}

// promotionDriver is harnessDriver with a pipeline opted in to verify
// promotion in repo, and two earlier pipelines in the ledger that ran into
// the same racy map and failing make test.
func promotionDriver(t *testing.T, f *fakeHarness, repo string, history int) (*focusDriver, *session.Store, string) {
	t.Helper()
	d, store, id := harnessDriver(t, f, repo)
	p := pipelineOf(t, store, id)
	p.Harness = nil
	p.Promotion = &focuspipeline.PromotionOptIn{Repo: repo}
	p.BaseCommit = strings.TrimSpace(gitRun(t, repo, "rev-parse", "HEAD"))
	for i := range p.Cards {
		p.Cards[i].CreatedAt = promotionCardAt
	}
	if err := focusStoreFor(store).Save(p); err != nil {
		t.Fatal(err)
	}
	d.promotion = promotionFake(promotionEntries())
	lib := promotionLibrary(t, store, repo)
	for i := 0; i < history; i++ {
		if _, err := lib.RecordRecurrence(focuspipeline.RecurrencePipeline{
			SessionID: "old" + string(rune('a'+i)),
			Hits: []focuspipeline.RecurrenceHit{
				{Key: promotionFindingKey, Kind: focuspipeline.RecurrenceFinding, Title: "racy map", File: "b.go"},
				{Key: promotionVerifyKey, Kind: focuspipeline.RecurrenceVerify, Command: "make test"},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	return d, store, id
}

func promotionLibrary(t *testing.T, store *session.Store, repo string) focuspipeline.HarnessLibrary {
	t.Helper()
	lib, err := focuspipeline.OpenHarnessLibrary(filepath.Join(store.WorkspaceDir(), "_shared"), repo)
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func TestFocusPromotionProposesAndEnforces(t *testing.T) {
	repo := promotionRepo(t)
	f := &fakeHarness{replies: []string{"not json", promotionReply}}
	d, store, id := promotionDriver(t, f, repo, 2)
	d.runPromotion(id)

	p := pipelineOf(t, store, id)
	if p.Promotion.Outcome != focuspipeline.PromotionProposed || p.Promotion.Proposal != "proposal-1" {
		t.Fatalf("promotion = %+v", p.Promotion)
	}
	notices := noticesOf(p, focuspipeline.NoticePromotionProposed)
	if len(notices) != 1 || notices[0]["checks"] != float64(2) || notices[0]["candidates"] != float64(2) || notices[0]["session_id"] != "proposal-1" {
		t.Fatalf("notices = %+v", notices)
	}
	// The verify failure needs no model; the finding was asked about once
	// more after a reply that was not JSON.
	if len(f.prompts) != 2 || !strings.Contains(f.prompts[0], "key: "+promotionFindingKey+" (in 3 of the last 10") || strings.Contains(f.prompts[0], "key: "+promotionVerifyKey) {
		t.Fatalf("prompts = %v", f.prompts)
	}
	req := f.opened[0]
	if req.Cwd != repo || !req.Isolate || !req.Start || req.GoalMode || req.VerifyPromotion || req.harnessFrom == nil || req.harnessFrom.SessionID != id ||
		!strings.Contains(req.Kickoff, "`sh scripts/race.sh`") || !strings.Contains(req.Kickoff, "it exited 1, after the fix it exited 0") ||
		!strings.Contains(req.Kickoff, "`make test`") || req.Goal != "Verify: enforce 2 recurring review findings as checks" {
		t.Fatalf("opened = %+v", req)
	}
	if len(f.events) != 1 || f.events[0].OpenPath != "/console/focus/proposal-1" || len(f.audits) != 1 || f.audits[0].Action != focusPromotionSource {
		t.Fatalf("events %+v audits %+v", f.events, f.audits)
	}
	lib := promotionLibrary(t, store, repo)
	ledger, _ := lib.Recurrence()
	if len(ledger.Pipelines) != 3 || ledger.Pipelines[2].SessionID != id {
		t.Fatalf("ledger = %+v", ledger)
	}
	if keys, _ := lib.CheckRecurrences(false); !keys[promotionFindingKey] || !keys[promotionVerifyKey] {
		t.Fatalf("library = %v", keys)
	}
	d.runPromotion(id)
	if len(f.prompts) != 2 {
		t.Fatal("the promotion ran twice")
	}

	// The promotion pipeline merges: its checks are enforced.
	prop := focuspipeline.New("proposal-1", "Verify", time.Now())
	prop.HarnessFrom = &focuspipeline.HarnessSource{SessionID: id, Repo: repo}
	prop.PR = &focuspipeline.PRInfo{URL: "https://example.test/pr/5", State: focuspipeline.PRStateMerged}
	d.harnessAfterFinish("proposal-1", prop)
	if keys, _ := lib.CheckRecurrences(true); len(keys) != 2 {
		t.Fatalf("enforced = %v", keys)
	}

	// A harness proposal no longer offers prose for an enforced defect:
	// the edit citing only the make test failure (E1) is dropped.
	f2 := &fakeHarness{replies: []string{harnessReply}}
	d2, store2, id2 := harnessDriver(t, f2, repo)
	d2.sessions = store
	if err := focusStoreFor(store).Save(harnessFinished(t, id2, repo)); err != nil {
		t.Fatal(err)
	}
	_ = store2
	d2.runHarness(id2)
	if h := pipelineOf(t, store, id2).Harness; h.Outcome != focuspipeline.HarnessNoEdits {
		t.Fatalf("harness = %+v", h)
	}
	if !strings.Contains(f2.prompts[0], "checked by the verify command `make test`") {
		t.Fatalf("the harness prompt does not show the enforced check:\n%s", f2.prompts[0])
	}
}

func TestFocusPromotionOutcomes(t *testing.T) {
	repo := promotionRepo(t)
	cases := []struct {
		name    string
		history int
		replies []string
		llmErr  error
		openErr error
		setup   func(d *focusDriver)
		outcome string
		reason  string // details["reasons"][finding key], or details["reason"]
	}{
		{name: "nothing recurs yet", history: 1, outcome: focuspipeline.PromotionNoCandidates},
		{name: "model fails", history: 2, llmErr: errors.New("boom"), outcome: focuspipeline.PromotionProposed, reason: "model"},
		{name: "never json", history: 2, replies: []string{"no", "still no"}, outcome: focuspipeline.PromotionProposed, reason: "invalid_reply"},
		{name: "no check", history: 2, replies: []string{`{"checks":[]}`}, outcome: focuspipeline.PromotionProposed, reason: focuspipeline.PromotionNoCheck},
		{name: "invalid check", history: 2, replies: []string{`{"checks":[{"key":"` + promotionFindingKey + `","kind":"magic","command":"x"}]}`}, outcome: focuspipeline.PromotionProposed, reason: focuspipeline.PromotionDropKind},
		{name: "fails on fix", history: 2, replies: []string{`{"checks":[{"key":"` + promotionFindingKey + `","kind":"lint","command":"exit 1"}]}`}, outcome: focuspipeline.PromotionProposed, reason: focusprobe.ReplayFailsOnFix},
		{name: "no fix turn", history: 2, replies: []string{promotionReply}, setup: func(d *focusDriver) { d.promotion.turns = func(string) ([]checkpoint.Entry, error) { return nil, nil } },
			outcome: focuspipeline.PromotionProposed, reason: focuspipeline.PromotionNoReplay},
		{name: "no checkpoints", history: 2, replies: []string{promotionReply}, setup: func(d *focusDriver) {
			d.promotion.turns = func(string) ([]checkpoint.Entry, error) { return nil, errors.New("gone") }
		}, outcome: focuspipeline.PromotionProposed, reason: focuspipeline.PromotionNoReplay},
		{name: "no model", history: 2, setup: func(d *focusDriver) { d.harnessLLM = nil }, outcome: focuspipeline.PromotionProposed, reason: "unavailable"},
		{name: "open fails", history: 2, replies: []string{promotionReply}, openErr: errors.New("x"), outcome: focuspipeline.PromotionFailed, reason: "open"},
		{name: "no runner", history: 2, setup: func(d *focusDriver) { d.promotion.run = nil }, outcome: focuspipeline.PromotionUnavailable, reason: "unavailable"},
		{name: "no repository", history: 0, setup: func(d *focusDriver) {
			p := pipelineOf(t, d.sessions, firstPipelineID(t, d))
			p.Promotion.Repo = "relative"
			_ = focusStoreFor(d.sessions).Save(p)
		}, outcome: focuspipeline.PromotionUnavailable, reason: "no_repository"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeHarness{replies: c.replies, err: c.llmErr, openErr: c.openErr}
			d, store, id := promotionDriver(t, f, repo, c.history)
			if c.setup != nil {
				c.setup(d)
			}
			d.runPromotion(id)
			p := pipelineOf(t, store, id)
			if p.Promotion.Outcome != c.outcome {
				t.Fatalf("outcome = %+v", p.Promotion)
			}
			if c.reason == "" {
				if len(p.Cards) != 2 {
					t.Fatalf("a card for %s", c.outcome)
				}
				return
			}
			details := f.audits[len(f.audits)-1].Details
			got, _ := details["reason"].(string)
			if reasons, ok := details["reasons"].(map[string]string); ok {
				got = reasons[promotionFindingKey]
			}
			if got != c.reason {
				t.Fatalf("reason = %q (%+v)", got, details)
			}
		})
	}
}

// firstPipelineID is the one pipeline the driver's store holds.
func firstPipelineID(t *testing.T, d *focusDriver) string {
	t.Helper()
	list, err := focusStoreFor(d.sessions).List()
	if err != nil || len(list) != 1 {
		t.Fatalf("pipelines: %d %v", len(list), err)
	}
	return list[0].SessionID
}

func TestFocusPromotionRemembersAttempts(t *testing.T) {
	repo := promotionRepo(t)
	f := &fakeHarness{replies: []string{`{"checks":[]}`}}
	d, store, id := promotionDriver(t, f, repo, 2)
	d.runPromotion(id)
	lib := promotionLibrary(t, store, repo)
	ledger, _ := lib.Recurrence()
	if a := ledger.Attempts[promotionFindingKey]; a.Count != 3 || a.Reason != focuspipeline.PromotionNoCheck {
		t.Fatalf("attempts = %+v", ledger.Attempts)
	}
	// The same counts again: the finding is not asked about again, and
	// the verify failure is already proposed.
	p := pipelineOf(t, store, id)
	p.Promotion.Outcome = ""
	if err := focusStoreFor(store).Save(p); err != nil {
		t.Fatal(err)
	}
	d.runPromotion(id)
	if p := pipelineOf(t, store, id); p.Promotion.Outcome != focuspipeline.PromotionNoCandidates || len(f.prompts) != 1 {
		t.Fatalf("again: %+v prompts %d", p.Promotion, len(f.prompts))
	}

	// A broken ledger or library fails the promotion.
	if err := os.WriteFile(filepath.Join(lib.Dir(), "recurrence.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	p.Promotion.Outcome = ""
	_ = focusStoreFor(store).Save(p)
	d.runPromotion(id)
	if p := pipelineOf(t, store, id); p.Promotion.Outcome != focuspipeline.PromotionFailed {
		t.Fatalf("broken ledger: %+v", p.Promotion)
	}
	if err := os.Remove(filepath.Join(lib.Dir(), "recurrence.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib.Dir(), "lessons.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	p.Promotion.Outcome = ""
	_ = focusStoreFor(store).Save(p)
	d.runPromotion(id)
	if p := pipelineOf(t, store, id); p.Promotion.Outcome != focuspipeline.PromotionFailed {
		t.Fatalf("broken library: %+v", p.Promotion)
	}
}

func TestFocusPromotionRunsInTheBackground(t *testing.T) {
	repo := promotionRepo(t)
	f := &fakeHarness{replies: []string{promotionReply}}
	d, store, id := promotionDriver(t, f, repo, 2)
	if n := d.resumePromotions(); n != 1 {
		t.Fatalf("resumed %d", n)
	}
	d.wg.Wait()
	if p := pipelineOf(t, store, id); p.Promotion.Outcome != focuspipeline.PromotionProposed {
		t.Fatalf("outcome = %+v", p.Promotion)
	}
	if n := d.resumePromotions(); n != 0 {
		t.Fatalf("resumed a recorded promotion: %d", n)
	}
	d.Close(context.Background())
	d.goPromotion(id)
	if len(d.promotion.busy) != 0 {
		t.Fatal("started after close")
	}
	var bare focusDriver
	if bare.resumePromotions() != 0 {
		t.Fatal("a driver without a store resumed something")
	}
	bare.goPromotion("x")

	f2 := &fakeHarness{replies: []string{promotionReply}}
	d2, store2, id2 := promotionDriver(t, f2, promotionRepo(t), 2)
	next := pipelineOf(t, store2, id2)
	prev := next
	prev.Stages = append([]focuspipeline.Stage(nil), next.Stages...)
	prev.Stages[len(prev.Stages)-1].Status = focuspipeline.StatusActive
	d2.pipelineFinished(id2, prev, next)
	d2.wg.Wait()
	if p := pipelineOf(t, store2, id2); p.Promotion.Outcome != focuspipeline.PromotionProposed {
		t.Fatalf("pipelineFinished: %+v", p.Promotion)
	}
}

func TestFixTurnAfter(t *testing.T) {
	if e, ok := fixTurnAfter(promotionEntries(), promotionCardAt); !ok || e.TurnID != "t2" {
		t.Fatalf("%+v %v", e, ok)
	}
	if _, ok := fixTurnAfter(promotionEntries(), promotionCardAt.Add(time.Hour)); ok {
		t.Fatal("no turn after")
	}
}

func TestNewFocusPromotion(t *testing.T) {
	if newFocusPromotion(nil, zerolog.Nop()) != nil {
		t.Fatal("no checkpoints, no promotion")
	}
	store, err := checkpoint.Open(t.TempDir(), checkpoint.Options{})
	if err != nil {
		t.Skip(err)
	}
	// Whether this host has a sandbox decides; either way it never
	// returns a promotion without a runner.
	if p := newFocusPromotion(store, zerolog.Nop()); p != nil && (p.run == nil || p.turns == nil || p.export == nil) {
		t.Fatalf("partial promotion: %+v", p)
	}
}

func TestFocusCreateVerifyPromotion(t *testing.T) {
	f := newWorktreeFixture(t)
	d := newFocusDriver(zerolog.Nop())
	t.Cleanup(func() { d.Close(context.Background()) })
	d.harnessLLM = func(context.Context, string) (string, error) { return "", nil }
	d.promotion = promotionFake(nil)
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	create := func(body string) (int, map[string]any) {
		rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", body, true)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	if err := os.WriteFile(filepath.Join(f.repo, ".tars", "verify.json"), []byte(`{"verify":["make lint"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out := create(`{"goal":"g","cwd":` + jsonString(f.repo) + `,"verify_promotion":true}`)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	p := pipelineOf(t, f.store, out["session_id"].(string))
	if p.Promotion == nil || p.Promotion.Repo != focusprobe.MainCheckout(context.Background(), f.repo) || p.Harness != nil {
		t.Fatalf("stored = %+v", p.Promotion)
	}
	if len(p.RepoVerify) != 1 || p.RepoVerify[0] != "make lint" {
		t.Fatalf("repo verify = %v", p.RepoVerify)
	}
	if code, out := create(`{"goal":"g","cwd":` + jsonString(t.TempDir()) + `,"verify_promotion":true}`); code != http.StatusBadRequest || !strings.Contains(out["error"].(string), "git repository") {
		t.Fatalf("not a repository: %d %v", code, out)
	}
	if code, out := create(`{"goal":"g","kind":"release","cwd":` + jsonString(f.repo) + `,"verify_promotion":true}`); code != http.StatusBadRequest || !strings.Contains(out["error"].(string), "release") {
		t.Fatalf("release: %d %v", code, out)
	}
	if err := os.Remove(filepath.Join(f.repo, ".tars", "verify.json")); err != nil {
		t.Fatal(err)
	}
	code, out = create(`{"goal":"g","cwd":` + jsonString(f.repo) + `}`)
	if code != http.StatusCreated {
		t.Fatalf("plain: %d", code)
	}
	raw, _ := os.ReadFile(filepath.Join(f.store.WorkspaceDir(), "sessions", out["session_id"].(string)+".pipeline.json"))
	if strings.Contains(string(raw), "verify_promotion") || strings.Contains(string(raw), "repo_verify") {
		t.Fatalf("a pipeline that did not opt in records nothing:\n%s", raw)
	}
	// Without a sandboxed runner the option is refused, not the task.
	d.promotion.run = nil
	if code, _ := create(`{"goal":"g","cwd":` + jsonString(f.repo) + `,"verify_promotion":true}`); code != http.StatusServiceUnavailable {
		t.Fatalf("no sandbox: %d", code)
	}
}
