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

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/focusprobe"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
	pkgllm "github.com/devlikebear/tars/pkg/llm"
	"github.com/rs/zerolog"
)

const harnessReply = `{"edits":[` +
	`{"path":".tars/skills/race/SKILL.md","lesson":"Run the race detector before review","evidence":["E1"],"content":"---\nname: race\n---\nRun make test-race."},` +
	`{"path":"CLAUDE.md","section":"## Style","lesson":"Uncited prose","evidence":[],"content":"x"}` +
	`],"summary":"one lesson"}`

// fakeHarness records the harness proposal's model prompts and opened
// pipelines.
type fakeHarness struct {
	replies []string
	err     error
	openErr error

	mu      sync.Mutex
	prompts []string
	opened  []focusCreateRequest
	audits  []ops.AutomationAuditEntry
	events  []notificationEvent
}

func (f *fakeHarness) llm(_ context.Context, prompt string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompts = append(f.prompts, prompt)
	if f.err != nil {
		return "", f.err
	}
	return f.replies[min(len(f.prompts), len(f.replies))-1], nil
}

func (f *fakeHarness) open(_ context.Context, req focusCreateRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.openErr != nil {
		return "", f.openErr
	}
	f.opened = append(f.opened, req)
	return "proposal-" + string(rune('0'+len(f.opened))), nil
}

// harnessDriver is a driver whose store holds one pipeline finished merged,
// opted in to a harness proposal in repo, with a failed verification and a
// fixed finding behind it.
func harnessDriver(t *testing.T, f *fakeHarness, repo string) (*focusDriver, *session.Store, string) {
	t.Helper()
	store := session.NewStore(t.TempDir())
	sess, err := store.Create("focus")
	if err != nil {
		t.Fatal(err)
	}
	if err := focusStoreFor(store).Save(harnessFinished(t, sess.ID, repo)); err != nil {
		t.Fatal(err)
	}
	d := newFocusDriver(zerolog.Nop())
	t.Cleanup(func() { d.Close(context.Background()) })
	d.sessions = store
	d.harnessLLM = f.llm
	d.openPipeline = f.open
	d.harnessCost = func(string) float64 { return 2.5 }
	d.audit = func(e ops.AutomationAuditEntry) {
		f.mu.Lock()
		f.audits = append(f.audits, e)
		f.mu.Unlock()
	}
	d.notify = func(_ context.Context, e notificationEvent) {
		f.mu.Lock()
		f.events = append(f.events, e)
		f.mu.Unlock()
	}
	return d, store, sess.ID
}

func harnessFinished(t *testing.T, sessionID, repo string) focuspipeline.Pipeline {
	t.Helper()
	p := focuspipeline.New(sessionID, "Add export", time.Now())
	p.Plan = &focuspipeline.Plan{Goal: "g", Stages: focuspipeline.StageOrder, Verify: []string{"make test"}}
	for i := range p.Stages {
		p.Stages[i].Status = focuspipeline.StatusDone
	}
	p.Current = focuspipeline.StageMerge
	p.PR = &focuspipeline.PRInfo{Number: 3, URL: "https://example.test/pr/3", State: focuspipeline.PRStateMerged}
	raw, _ := json.Marshal(focuspipeline.FailureFact{Command: "make test", ExitCode: 1, Excerpt: "DATA RACE"})
	p.Cards = append(p.Cards, focuspipeline.Card{ID: "c1", Kind: focuspipeline.CardFailure, Stage: focuspipeline.StageReview, Payload: raw, State: focuspipeline.CardSeen})
	raw, _ = json.Marshal(focuspipeline.Finding{File: "a.go", Line: 2, Title: "racy map"})
	p.Cards = append(p.Cards, focuspipeline.Card{ID: "c2", Kind: focuspipeline.CardFinding, Stage: focuspipeline.StageReview, Title: "racy map", Payload: raw, State: focuspipeline.CardDecided, Decision: focuspipeline.DecisionFix})
	p.Harness = &focuspipeline.HarnessOptIn{Repo: repo}
	return p
}

func harnessRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "CLAUDE.md"), []byte("# Guide\n## Testing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestFocusHarnessProposesAndAdopts(t *testing.T) {
	repo := harnessRepo(t)
	f := &fakeHarness{replies: []string{harnessReply}}
	d, store, id := harnessDriver(t, f, repo)
	d.runHarness(id)

	p := pipelineOf(t, store, id)
	if p.Harness.Outcome != focuspipeline.HarnessProposed || p.Harness.Proposal != "proposal-1" {
		t.Fatalf("harness = %+v", p.Harness)
	}
	notices := noticesOf(p, focuspipeline.NoticeHarnessProposed)
	if len(notices) != 1 || notices[0]["session_id"] != "proposal-1" || notices[0]["lessons"] != float64(1) || notices[0]["dropped"] != float64(1) {
		t.Fatalf("notices = %+v", notices)
	}
	if len(f.prompts) != 1 || !strings.Contains(f.prompts[0], "CLAUDE.md sections: # Guide | ## Testing") ||
		!strings.Contains(f.prompts[0], "`make test` exited 1") || !strings.Contains(f.prompts[0], "recorded cost: $2.50") {
		t.Fatalf("prompt:\n%v", f.prompts)
	}
	req := f.opened[0]
	if req.Cwd != repo || !req.Isolate || !req.Start || req.GoalMode || req.harnessFrom == nil || req.harnessFrom.SessionID != id ||
		!strings.Contains(req.Kickoff, "Run the race detector before review") || strings.Contains(req.Kickoff, "Uncited prose") ||
		!strings.HasPrefix(req.Goal, "Harness: ") {
		t.Fatalf("opened = %+v", req)
	}
	if len(f.events) != 1 || f.events[0].OpenPath != "/console/focus/proposal-1" {
		t.Fatalf("events = %+v", f.events)
	}
	if len(f.audits) != 1 || f.audits[0].Result != focuspipeline.HarnessProposed || f.audits[0].Action != "focus_harness_proposal" {
		t.Fatalf("audits = %+v", f.audits)
	}
	lib, _ := focuspipeline.OpenHarnessLibrary(filepath.Join(store.WorkspaceDir(), "_shared"), repo)
	lessons, err := lib.Lessons()
	if err != nil || len(lessons) != 1 || lessons[0].Status != focuspipeline.HarnessLessonProposed || lessons[0].ProposalSession != "proposal-1" {
		t.Fatalf("library = %+v %v", lessons, err)
	}

	// Run again: it ran once.
	d.runHarness(id)
	if len(f.prompts) != 1 {
		t.Fatal("the proposal ran twice")
	}

	// The proposal pipeline merges: its lessons are adopted.
	prop := focuspipeline.New("proposal-1", "Harness", time.Now())
	prop.HarnessFrom = &focuspipeline.HarnessSource{SessionID: id, Repo: repo}
	prop.PR = &focuspipeline.PRInfo{URL: "https://example.test/pr/4", State: focuspipeline.PRStateMerged}
	d.harnessAfterFinish("proposal-1", prop)
	lessons, _ = lib.Lessons()
	if lessons[0].Status != focuspipeline.HarnessLessonAdopted || lessons[0].PRURL != "https://example.test/pr/4" {
		t.Fatalf("adopted = %+v", lessons[0])
	}
	if raw, err := os.ReadFile(filepath.Join(lib.Dir(), "EXPERIENCE.md")); err != nil || !strings.Contains(string(raw), "Run the race detector before review") {
		t.Fatalf("EXPERIENCE.md: %s %v", raw, err)
	}
	if last := f.audits[len(f.audits)-1]; last.Result != "adopted" || last.SessionID != id {
		t.Fatalf("adopt audit = %+v", last)
	}
	// An open (not merged) proposal, or one with no repository, adopts nothing.
	prop.PR.State = "OPEN"
	d.harnessAfterFinish("proposal-1", prop)
	prop.PR.State = focuspipeline.PRStateMerged
	prop.HarnessFrom.Repo = "relative"
	d.harnessAfterFinish("proposal-1", prop)
	if n := len(f.audits); n != 2 {
		t.Fatalf("audits = %d", n)
	}

	// A second pipeline teaching the same lesson proposes nothing.
	f2 := &fakeHarness{replies: []string{harnessReply}}
	d2, store2, id2 := harnessDriver(t, f2, repo)
	d2.sessions = store2
	// Same workspace library as the first run.
	sharedLib, _ := focuspipeline.OpenHarnessLibrary(filepath.Join(store2.WorkspaceDir(), "_shared"), repo)
	raw, _ := os.ReadFile(filepath.Join(lib.Dir(), "lessons.json"))
	if err := os.MkdirAll(sharedLib.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedLib.Dir(), "lessons.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	d2.runHarness(id2)
	if p := pipelineOf(t, store2, id2); p.Harness.Outcome != focuspipeline.HarnessNoEdits || len(f2.opened) != 0 ||
		len(noticesOf(p, focuspipeline.NoticeHarnessSkipped)) != 1 {
		t.Fatalf("repeat = %+v %d", p.Harness, len(f2.opened))
	}
	if !strings.Contains(f2.prompts[0], "[adopted] Run the race detector before review") {
		t.Fatalf("the library was not in the prompt:\n%s", f2.prompts[0])
	}
}

func TestFocusHarnessOutcomes(t *testing.T) {
	cases := []struct {
		name    string
		f       *fakeHarness
		setup   func(d *focusDriver, p *focuspipeline.Pipeline)
		outcome string
		reason  string
		calls   int
	}{
		{name: "no evidence", f: &fakeHarness{replies: []string{harnessReply}}, setup: func(_ *focusDriver, p *focuspipeline.Pipeline) {
			p.Cards = nil
		}, outcome: focuspipeline.HarnessNoEvidence},
		{name: "no model", f: &fakeHarness{}, setup: func(d *focusDriver, _ *focuspipeline.Pipeline) {
			d.harnessLLM = nil
		}, outcome: focuspipeline.HarnessUnavailable, reason: "unavailable"},
		{name: "no repository", f: &fakeHarness{}, setup: func(_ *focusDriver, p *focuspipeline.Pipeline) {
			p.Harness.Repo = "relative"
		}, outcome: focuspipeline.HarnessUnavailable, reason: "no_repository"},
		{name: "model error", f: &fakeHarness{err: errors.New("boom")}, outcome: focuspipeline.HarnessFailed, reason: "model", calls: 1},
		{name: "invalid twice", f: &fakeHarness{replies: []string{"nope"}}, outcome: focuspipeline.HarnessFailed, reason: "invalid_reply", calls: 2},
		{name: "invalid then valid", f: &fakeHarness{replies: []string{"nope", harnessReply}}, outcome: focuspipeline.HarnessProposed, calls: 2},
		{name: "nothing cited", f: &fakeHarness{replies: []string{`{"edits":[]}`}}, outcome: focuspipeline.HarnessNoEdits, calls: 1},
		{name: "open fails", f: &fakeHarness{replies: []string{harnessReply}, openErr: errors.New("409")}, outcome: focuspipeline.HarnessFailed, reason: "open", calls: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, store, id := harnessDriver(t, tc.f, harnessRepo(t))
			if tc.setup != nil {
				if _, _, err := focusStoreFor(store).Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
					tc.setup(d, &p)
					return p, nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			d.runHarness(id)
			p := pipelineOf(t, store, id)
			if p.Harness.Outcome != tc.outcome || len(tc.f.prompts) != tc.calls {
				t.Fatalf("outcome %q after %d calls, want %q after %d", p.Harness.Outcome, len(tc.f.prompts), tc.outcome, tc.calls)
			}
			if tc.reason != "" {
				notices := noticesOf(p, focuspipeline.NoticeHarnessSkipped)
				if len(notices) != 1 || notices[0]["reason"] != tc.reason {
					t.Fatalf("notices = %+v", notices)
				}
			}
			if tc.calls == 2 && !strings.Contains(tc.f.prompts[1], "was not the JSON object asked for") {
				t.Fatalf("re-ask = %s", tc.f.prompts[1])
			}
		})
	}
}

func TestFocusHarnessBrokenLibrary(t *testing.T) {
	repo := harnessRepo(t)
	f := &fakeHarness{replies: []string{harnessReply}}
	d, store, id := harnessDriver(t, f, repo)
	lib, _ := focuspipeline.OpenHarnessLibrary(d.sharedDir(), repo)
	if err := os.MkdirAll(lib.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib.Dir(), "lessons.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	d.runHarness(id)
	if p := pipelineOf(t, store, id); p.Harness.Outcome != focuspipeline.HarnessFailed || len(f.prompts) != 0 {
		t.Fatalf("outcome = %+v", p.Harness)
	}
}

func TestFocusHarnessRunsInTheBackground(t *testing.T) {
	f := &fakeHarness{replies: []string{harnessReply}}
	d, store, id := harnessDriver(t, f, harnessRepo(t))
	if n := d.resumeHarness(); n != 1 {
		t.Fatalf("resumed %d", n)
	}
	d.wg.Wait()
	if p := pipelineOf(t, store, id); p.Harness.Outcome != focuspipeline.HarnessProposed {
		t.Fatalf("outcome = %+v", p.Harness)
	}
	if n := d.resumeHarness(); n != 0 {
		t.Fatalf("resumed a recorded proposal: %d", n)
	}
	// A closed driver starts nothing.
	d.Close(context.Background())
	d.goHarness(id)
	if len(d.harnessBusy) != 0 {
		t.Fatal("started after close")
	}
	var nilStore focusDriver
	if nilStore.resumeHarness() != 0 || nilStore.sharedDir() != "" {
		t.Fatal("a driver without a store found something")
	}

	// pipelineFinished starts it on the transition to finished.
	f2 := &fakeHarness{replies: []string{harnessReply}}
	d2, store2, id2 := harnessDriver(t, f2, harnessRepo(t))
	next := pipelineOf(t, store2, id2)
	prev := next
	prev.Stages = append([]focuspipeline.Stage(nil), next.Stages...)
	prev.Stages[len(prev.Stages)-1].Status = focuspipeline.StatusActive
	d2.pipelineFinished(id2, prev, next)
	d2.wg.Wait()
	if p := pipelineOf(t, store2, id2); p.Harness.Outcome != focuspipeline.HarnessProposed {
		t.Fatalf("pipelineFinished: %+v", p.Harness)
	}
}

func TestFocusCreateHarnessProposal(t *testing.T) {
	f := newWorktreeFixture(t)
	d := newFocusDriver(zerolog.Nop())
	t.Cleanup(func() { d.Close(context.Background()) })
	d.harnessLLM = func(context.Context, string) (string, error) { return "", nil }
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	if d.openPipeline == nil {
		t.Fatal("the handler did not bind openPipeline")
	}
	create := func(h http.Handler, body string) (int, map[string]any) {
		rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", body, true)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	code, out := create(h, `{"goal":"g","cwd":`+jsonString(f.repo)+`,"harness_proposal":true}`)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	p := pipelineOf(t, f.store, out["session_id"].(string))
	if p.Harness == nil || p.Harness.Repo != focusprobe.MainCheckout(context.Background(), f.repo) || p.Harness.Outcome != "" {
		t.Fatalf("stored = %+v", p.Harness)
	}

	if code, out := create(h, `{"goal":"g","cwd":`+jsonString(t.TempDir())+`,"harness_proposal":true}`); code != http.StatusBadRequest || !strings.Contains(out["error"].(string), "git repository") {
		t.Fatalf("not a repository: %d %v", code, out)
	}
	if code, out := create(h, `{"goal":"g","kind":"release","cwd":`+jsonString(f.repo)+`,"harness_proposal":true}`); code != http.StatusBadRequest || !strings.Contains(out["error"].(string), "release") {
		t.Fatalf("release: %d %v", code, out)
	}
	code, out = create(h, `{"goal":"g","cwd":`+jsonString(f.repo)+`}`)
	if code != http.StatusCreated {
		t.Fatalf("plain: %d", code)
	}
	raw, _ := os.ReadFile(filepath.Join(f.store.WorkspaceDir(), "sessions", out["session_id"].(string)+".pipeline.json"))
	if strings.Contains(string(raw), "harness") {
		t.Fatalf("a pipeline that did not opt in records nothing:\n%s", raw)
	}

	// A server that cannot draft lessons refuses the option, not the task.
	d.harnessLLM = nil
	if code, _ := create(h, `{"goal":"g","cwd":`+jsonString(f.repo)+`,"harness_proposal":true}`); code != http.StatusServiceUnavailable {
		t.Fatalf("no model: %d", code)
	}

	// The server's own open: a proposal pipeline carries where it came from.
	id, err := d.openPipeline(context.Background(), focusCreateRequest{
		Goal: "Harness: x", Kickoff: "apply", Cwd: f.repo,
		harnessFrom: &focuspipeline.HarnessSource{SessionID: "src", Repo: f.repo},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p := pipelineOf(t, f.store, id); p.HarnessFrom == nil || p.HarnessFrom.SessionID != "src" || p.Kickoff != "apply" {
		t.Fatalf("proposal = %+v", p)
	}
	if _, err := d.openPipeline(context.Background(), focusCreateRequest{Cwd: f.repo}); err == nil || !strings.Contains(err.Error(), "400 goal is required") {
		t.Fatalf("bad open: %v", err)
	}
}

func TestNewFocusHarnessLLM(t *testing.T) {
	if newFocusHarnessLLM(nil) != nil {
		t.Fatal("a nil router gave a caller")
	}
	router, clients, err := pkgllm.NewFakeRouter(pkgllm.TierStandard, map[pkgllm.Role]pkgllm.Tier{pkgllm.RoleFocusHarness: pkgllm.TierHeavy})
	if err != nil {
		t.Fatal(err)
	}
	clients[pkgllm.TierHeavy].ChatResponse = pkgllm.ChatResponse{Message: pkgllm.ChatMessage{Role: "assistant", Content: `{"edits":[]}`}}
	ask := newFocusHarnessLLM(router)
	reply, err := ask(context.Background(), "p")
	if err != nil || reply != `{"edits":[]}` || clients[pkgllm.TierHeavy].ChatCalls != 1 || clients[pkgllm.TierStandard].ChatCalls != 0 {
		t.Fatalf("reply %q %v", reply, err)
	}
	if opts := clients[pkgllm.TierHeavy].LastChatOptions; opts.ClaudeCodePermissionMode != "plan" || opts.ToolChoice == nil {
		t.Fatalf("not decision-only: %+v", opts)
	}
	clients[pkgllm.TierHeavy].ChatResponse.Message.ToolCalls = []pkgllm.ToolCall{{ID: "t", Name: "bash"}}
	if _, err := ask(context.Background(), "p"); err == nil {
		t.Fatal("a tool call was accepted")
	}

	agy := &pkgllm.FakeClient{Label: "agy"}
	other := &pkgllm.FakeClient{Label: "other"}
	r2, err := pkgllm.NewRouter(pkgllm.RouterConfig{
		DefaultTier: pkgllm.TierStandard,
		Tiers: map[pkgllm.Tier]pkgllm.TierEntry{
			pkgllm.TierHeavy:    {Client: agy, Provider: "antigravity-cli"},
			pkgllm.TierStandard: {Client: other, Provider: "fake"},
			pkgllm.TierLight:    {Client: other, Provider: "fake"},
		},
		RoleDefaults: map[pkgllm.Role]pkgllm.Tier{pkgllm.RoleFocusHarness: pkgllm.TierHeavy},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newFocusHarnessLLM(r2)(context.Background(), "p"); err == nil || agy.ChatCalls != 0 {
		t.Fatalf("antigravity-cli ran: %v", err)
	}
	failing := &failingChatClient{}
	r3, _ := pkgllm.NewRouter(pkgllm.RouterConfig{
		DefaultTier: pkgllm.TierHeavy,
		Tiers: map[pkgllm.Tier]pkgllm.TierEntry{
			pkgllm.TierHeavy: {Client: failing, Provider: "fake"}, pkgllm.TierStandard: {Client: other}, pkgllm.TierLight: {Client: other},
		},
	})
	if _, err := newFocusHarnessLLM(r3)(context.Background(), "p"); err == nil {
		t.Fatal("a failed call returned no error")
	}
}

type failingChatClient struct{ pkgllm.FakeClient }

func (failingChatClient) Chat(context.Context, []pkgllm.ChatMessage, pkgllm.ChatOptions) (pkgllm.ChatResponse, error) {
	return pkgllm.ChatResponse{}, errors.New("down")
}

func TestFocusHarnessCost(t *testing.T) {
	if focusHarnessCost(nil) != nil {
		t.Fatal("no tracker should mean no cost source")
	}
	now := time.Now().UTC()
	tracker, err := usage.NewTracker(t.TempDir(), usage.TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if err := tracker.Record(usage.Entry{Timestamp: now, Provider: "anthropic", Model: "m", InputTokens: 1, EstimatedCostUSD: 0.75, PricingKnown: true, SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	cost := focusHarnessCost(tracker)
	if got := cost("s1"); got != 0.75 {
		t.Fatalf("cost = %v", got)
	}
	if got := cost("other"); got != 0 {
		t.Fatalf("other = %v", got)
	}
}
