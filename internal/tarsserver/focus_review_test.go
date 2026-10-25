package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/pkg/session"
	"github.com/rs/zerolog"
)

// focusReviewRepo is a git repository with a committed b.go; it returns
// the repo and its HEAD.
func focusReviewRepo(t *testing.T) (string, string) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	repo := t.TempDir()
	gitRun(t, repo, "init", "-q", "-b", "main")
	gitRun(t, repo, "config", "commit.gpgSign", "false")
	gitRun(t, repo, "config", "core.autocrlf", "false")
	lines := make([]string, 0, 30)
	for i := 1; i <= 30; i++ {
		lines = append(lines, "line "+strconv.Itoa(i))
	}
	writeRepoFile(t, repo, "b.go", strings.Join(lines, "\n")+"\n")
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", "init")
	return repo, strings.TrimSpace(gitRun(t, repo, "rev-parse", "HEAD"))
}

func writeRepoFile(t *testing.T, repo, name, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// reviewingFocusSession puts a session (cwd: repo, when given) in review
// with verify and end-to-end commands.
func reviewingFocusSession(t *testing.T, store *session.Store, repo, base string) session.Session {
	t.Helper()
	sess := buildingFocusSession(t, store)
	if repo != "" {
		if err := store.SetWorkDirs(sess.ID, []string{repo}, repo); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := focusStoreFor(store).Update(sess.ID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p.Plan.Stages = []focuspipeline.StageID{focuspipeline.StagePlan, focuspipeline.StageBuild, focuspipeline.StageReview}
		p.Plan.Verify = []string{"make test"}
		p.Plan.E2E = []string{"make console-e2e"}
		p.BaseCommit = base
		for i := range p.Stages {
			switch p.Stages[i].ID {
			case focuspipeline.StageBuild:
				p.Stages[i].Status = focuspipeline.StatusDone
			case focuspipeline.StageReview:
				p.Stages[i].Status, p.Stages[i].Iteration, p.Stages[i].Limit = focuspipeline.StatusActive, 1, 2
			case focuspipeline.StagePR, focuspipeline.StagePRReview, focuspipeline.StageMerge:
				p.Stages[i].Status = focuspipeline.StatusSkipped
			}
		}
		p.Current = focuspipeline.StageReview
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
	return sess
}

func TestFocusBaseCommitRecordedAtFirstTurn(t *testing.T) {
	repo, base := focusReviewRepo(t)
	store := session.NewStore(t.TempDir())
	sess := focusSession(t, store, "goal")
	if err := store.SetWorkDirs(sess.ID, []string{repo}, repo); err != nil {
		t.Fatal(err)
	}
	if _, mark := appendFocusGuidance("go", store, sess.ID, zerolog.Nop()); mark == nil {
		t.Fatal("no guidance")
	}
	if got := pipelineOf(t, store, sess.ID).BaseCommit; got != base {
		t.Fatalf("base = %q want %q", got, base)
	}
	writeRepoFile(t, repo, "c.go", "x\n")
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-qm", "later")
	appendFocusGuidance("again", store, sess.ID, zerolog.Nop())
	if got := pipelineOf(t, store, sess.ID).BaseCommit; got != base {
		t.Fatalf("base moved to %q", got)
	}

	plain := focusSession(t, store, "no repo")
	appendFocusGuidance("go", store, plain.ID, zerolog.Nop())
	if got := pipelineOf(t, store, plain.ID).BaseCommit; got != "" {
		t.Fatalf("base outside a repo = %q", got)
	}
}

func TestFocusReviewFindingsCarryExcerpts(t *testing.T) {
	repo, base := focusReviewRepo(t)
	content, _ := os.ReadFile(filepath.Join(repo, "b.go"))
	writeRepoFile(t, repo, "b.go", strings.Replace(string(content), "line 5\n", "line five\n", 1))
	store := session.NewStore(t.TempDir())
	sess := reviewingFocusSession(t, store, repo, base)
	reply := "found one\n<focus-findings>[{\"id\":\"f1\",\"severity\":\"high\",\"file\":\"b.go\",\"line\":5,\"title\":\"renamed\",\"scenario\":\"x\"}]</focus-findings>"
	p, act, ok := focusAfterTurn(store, sess.ID, store.TranscriptPath(sess.ID), reply, currentFocusMark(t, store, sess.ID), time.Now(), zerolog.Nop())
	if !ok || act.Kind != focuspipeline.ActionNone || p.OpenGate != focuspipeline.GateTriage {
		t.Fatalf("ok = %v act = %+v gate = %q", ok, act, p.OpenGate)
	}
	var f focuspipeline.Finding
	if err := json.Unmarshal(p.Cards[len(p.Cards)-1].Payload, &f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.Excerpt, "+line five") {
		t.Fatalf("excerpt = %q", f.Excerpt)
	}
}

// recordingVerifier passes every command and records them.
type recordingVerifier struct {
	mu       sync.Mutex
	commands []string
}

func (v *recordingVerifier) verify(_ context.Context, _ string, command string) (focuspipeline.VerificationResult, error) {
	v.mu.Lock()
	v.commands = append(v.commands, command)
	v.mu.Unlock()
	return focuspipeline.VerificationResult{Command: command, Passed: true}, nil
}

func (v *recordingVerifier) seen() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.commands...)
}

const (
	focusTwoFindings = "review\n<focus-findings>[" +
		`{"id":"f1","severity":"high","file":"a.go","line":3,"title":"nil deref","scenario":"nil input → panic"},` +
		`{"id":"f2","severity":"low","file":"b.go","line":9,"title":"typo","scenario":"label reads wrong"}` +
		"]</focus-findings>"
	focusNoFindings = "clean\n<focus-findings>[]</focus-findings>"
)

func TestFocusDriverReviewRunsEndToEndAndTriages(t *testing.T) {
	var reviews int
	reply := func(_ int, prompt string) string {
		if strings.HasPrefix(prompt, "Fix these findings") {
			return focusReport("fixed", false)
		}
		reviews++
		if reviews == 1 {
			return focusTwoFindings
		}
		return focusNoFindings
	}
	d, turns, store, _ := testFocusDriver(t, reply, &fakeVerifier{result: passAll})
	verifier := &recordingVerifier{}
	d.verify = verifier.verify
	// The plan's e2e item ("make console-e2e") is checked through the same
	// recording fake here: this test cares that both lists ran, in order,
	// not that one of them is really computer_use underneath.
	d.e2e = focusE2ERunner(verifier.verify)
	sess := reviewingFocusSession(t, store, "", "")
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())

	d.start(sess.ID, sendTurn("Start the review stage."), "")
	waitDriverIdle(t, d, sess.ID)
	p := pipelineOf(t, store, sess.ID)
	if p.OpenGate != focuspipeline.GateTriage || len(p.Review.Triage) != 2 {
		t.Fatalf("triage not open: %+v", p)
	}
	if len(verifier.seen()) != 0 {
		t.Fatal("verification ran before triage")
	}
	first, second := p.Review.Triage[0], p.Review.Triage[1]

	path := "/v1/focus/pipelines/" + sess.ID + "/cards/"
	if rec := focusRequest(t, h, http.MethodPost, path+first, `{"state":"decided","decision":"maybe"}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid decision: %d %s", rec.Code, rec.Body.String())
	}
	if rec := focusRequest(t, h, http.MethodPost, path+first, `{"state":"decided","decision":"fix"}`, true); rec.Code != http.StatusOK {
		t.Fatalf("fix: %d %s", rec.Code, rec.Body.String())
	}
	rec := focusRequest(t, h, http.MethodPost, path+first, `{"state":"decided","decision":"dismiss"}`, true)
	if rec.Code != http.StatusConflict {
		t.Fatalf("double decision: %d %s", rec.Code, rec.Body.String())
	}
	var conflict struct {
		Pipeline focuspipeline.Pipeline `json:"pipeline"`
	}
	decodeInto(t, rec, &conflict)
	if conflict.Pipeline.OpenGate != focuspipeline.GateTriage {
		t.Fatalf("409 must carry the current pipeline: %+v", conflict.Pipeline)
	}
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+sess.ID+"/gates/triage", `{"action":"approve"}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("approve on triage: %d %s", rec.Code, rec.Body.String())
	}
	rec = focusRequest(t, h, http.MethodPost, path+second, `{"state":"decided","decision":"dismiss"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("dismiss: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		NextPrompt string `json:"next_prompt"`
	}
	decodeInto(t, rec, &resp)
	if !strings.Contains(resp.NextPrompt, "a.go:3") || strings.Contains(resp.NextPrompt, "b.go:9") {
		t.Fatalf("fix prompt = %q", resp.NextPrompt)
	}

	waitFor(t, "review done", func() bool {
		return statuses(pipelineOf(t, store, sess.ID))[focuspipeline.StageReview] == focuspipeline.StatusDone
	})
	waitDriverIdle(t, d, sess.ID)
	want := []string{"make test", "make console-e2e", "make test", "make console-e2e"}
	if got := verifier.seen(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("verified %q want %q", got, want)
	}
	prompts := turns.seen()
	if len(prompts) != 3 || !strings.HasPrefix(prompts[1], "Fix these findings") || !strings.Contains(prompts[2], "Review the changes again") {
		t.Fatalf("prompts = %q", prompts)
	}
}

func statuses(p focuspipeline.Pipeline) map[focuspipeline.StageID]focuspipeline.StageStatus {
	out := map[focuspipeline.StageID]focuspipeline.StageStatus{}
	for _, s := range p.Stages {
		out[s.ID] = s.Status
	}
	return out
}

// focusReviewBlockedAtLimit puts the session's review stage into a real
// limit-1 blocked gate (BlockedLimit) through the production hooks
// (appendFocusGuidanceAt/focusAfterTurn), after a normal (non-instructed)
// fix turn that actually edits repo's working tree: that turn's own
// Fixed=true is unaffected by tars dogfooding fix #2, exactly as before.
func focusReviewBlockedAtLimit(t *testing.T, repo, base string) (*session.Store, string) {
	t.Helper()
	store := session.NewStore(t.TempDir())
	sess := reviewingFocusSession(t, store, repo, base)
	if _, _, err := focusStoreFor(store).Update(sess.ID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		for i := range p.Stages {
			if p.Stages[i].ID == focuspipeline.StageReview {
				p.Stages[i].Limit = 1
			}
		}
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}

	reply := "found\n<focus-findings>[{\"id\":\"f1\",\"severity\":\"high\",\"file\":\"b.go\",\"line\":5,\"title\":\"renamed\",\"scenario\":\"x\"}]</focus-findings>"
	mark := currentFocusMark(t, store, sess.ID)
	transcript := store.TranscriptPath(sess.ID)
	if err := session.AppendMessage(transcript, session.Message{Role: "assistant", Content: reply, Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	p, _, ok := focusAfterTurn(store, sess.ID, transcript, reply, mark, time.Now(), zerolog.Nop())
	if !ok || p.OpenGate != focuspipeline.GateTriage {
		t.Fatalf("triage not open: ok=%v %+v", ok, p)
	}

	p, _, err := focuspipeline.SetCardState(p, p.Review.Triage[0], focuspipeline.CardDecided, focuspipeline.DecisionFix, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := focusStoreFor(store).Save(p); err != nil {
		t.Fatal(err)
	}
	if !p.Review.Fixing || p.Review.FixingInstructed {
		t.Fatalf("expected a normal (non-instructed) fix turn: %+v", p.Review)
	}

	// The normal fix turn really edits the repo.
	writeRepoFile(t, repo, "b.go", "fixed line 5\n")
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-qm", "normal fix")

	mark = currentFocusMark(t, store, sess.ID)
	if mark.FixBeforeState != "" {
		t.Fatalf("a non-instructed fix turn must not get FixBeforeState: %+v", mark)
	}
	fixReply := focusReport("fixed it", false)
	if err := session.AppendMessage(transcript, session.Message{Role: "assistant", Content: fixReply, Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	p, act, ok := focusAfterTurn(store, sess.ID, transcript, fixReply, mark, time.Now(), zerolog.Nop())
	if !ok || act.Kind != focuspipeline.ActionRunVerification || !p.Review.Fixed {
		t.Fatalf("normal fix turn should still set Fixed: ok=%v act=%+v review=%+v", ok, act, p.Review)
	}

	p, _, err = focuspipeline.Apply(p, focuspipeline.Event{
		Kind: focuspipeline.EventVerification,
		Verification: &focuspipeline.Verification{Passed: true, Results: []focuspipeline.VerificationResult{
			{Command: "make test", Passed: true},
		}},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := focusStoreFor(store).Save(p); err != nil {
		t.Fatal(err)
	}
	if p.OpenGate != focuspipeline.GateBlocked {
		t.Fatalf("expected the limit block (iteration 1 >= limit 1): %+v", p)
	}
	return store, sess.ID
}

// TestFocusFixUnchangedFromRealGitState is tars dogfooding fix #2, tested
// through the real production hooks against a real temporary git
// repository: an instruct turn at the review blocked gate that truly
// changes nothing lets verification passing end the stage instead of
// reopening the same limit block; one that does edit the repo keeps
// counting as a fix, same as before.
func TestFocusFixUnchangedFromRealGitState(t *testing.T) {
	t.Run("an instruct turn that touches nothing ends the stage", func(t *testing.T) {
		repo, base := focusReviewRepo(t)
		store, sessID := focusReviewBlockedAtLimit(t, repo, base)

		p, _, err := focuspipeline.Apply(pipelineOf(t, store, sessID), focuspipeline.Event{
			Kind: focuspipeline.EventGate, Gate: focuspipeline.GateBlocked, Action: focuspipeline.GateInstruct,
			Note: "double check only; report nothing if there is nothing left to fix",
		}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := focusStoreFor(store).Save(p); err != nil {
			t.Fatal(err)
		}
		if !p.Review.Fixing || !p.Review.FixingInstructed {
			t.Fatalf("expected an instructed fix turn: %+v", p.Review)
		}

		// The real production hook (appendFocusGuidanceAt) reads the
		// working tree now, before the turn runs.
		message, mark := appendFocusGuidanceAt("go", store, sessID, "", zerolog.Nop())
		if mark == nil || mark.FixBeforeState == "" {
			t.Fatalf("no FixBeforeState captured for an instructed fix turn: %+v", mark)
		}
		transcript := store.TranscriptPath(sessID)
		if err := session.AppendMessage(transcript, session.Message{Role: "user", Content: message, Timestamp: time.Now()}); err != nil {
			t.Fatal(err)
		}
		// The agent's turn edits nothing in the repo.
		reply := focusReport("nothing to fix", false)
		if err := session.AppendMessage(transcript, session.Message{Role: "assistant", Content: reply, Timestamp: time.Now()}); err != nil {
			t.Fatal(err)
		}
		p, act, ok := focusAfterTurn(store, sessID, transcript, reply, mark, time.Now(), zerolog.Nop())
		if !ok || act.Kind != focuspipeline.ActionRunVerification || p.Review.Fixed {
			t.Fatalf("after an unchanged instruct fix turn: ok=%v act=%+v review=%+v", ok, act, p.Review)
		}
		foundNotice := false
		for _, c := range p.Cards {
			if c.Title == focuspipeline.NoticeFixUnchanged {
				foundNotice = true
			}
		}
		if !foundNotice {
			t.Fatalf("no 'fix turn changed nothing' notice card: %+v", p.Cards)
		}

		p, _, err = focuspipeline.Apply(p, focuspipeline.Event{
			Kind: focuspipeline.EventVerification,
			Verification: &focuspipeline.Verification{Passed: true, Results: []focuspipeline.VerificationResult{
				{Command: "make test", Passed: true},
			}},
		}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if p.OpenGate == focuspipeline.GateBlocked {
			t.Fatalf("verification passing should end the stage, not reblock: %+v", p)
		}
	})

	t.Run("an instruct turn that edits the repo still counts as fixed", func(t *testing.T) {
		repo, base := focusReviewRepo(t)
		store, sessID := focusReviewBlockedAtLimit(t, repo, base)

		p, _, err := focuspipeline.Apply(pipelineOf(t, store, sessID), focuspipeline.Event{
			Kind: focuspipeline.EventGate, Gate: focuspipeline.GateBlocked, Action: focuspipeline.GateInstruct,
			Note: "fix it properly this time",
		}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := focusStoreFor(store).Save(p); err != nil {
			t.Fatal(err)
		}

		message, mark := appendFocusGuidanceAt("go", store, sessID, "", zerolog.Nop())
		if mark == nil || mark.FixBeforeState == "" {
			t.Fatalf("no FixBeforeState captured: %+v", mark)
		}
		transcript := store.TranscriptPath(sessID)
		if err := session.AppendMessage(transcript, session.Message{Role: "user", Content: message, Timestamp: time.Now()}); err != nil {
			t.Fatal(err)
		}
		// This time the agent's turn really edits the repo.
		writeRepoFile(t, repo, "b.go", "really fixed\n")
		gitRun(t, repo, "add", "-A")
		gitRun(t, repo, "commit", "-qm", "instructed fix")
		reply := focusReport("fixed for real", false)
		if err := session.AppendMessage(transcript, session.Message{Role: "assistant", Content: reply, Timestamp: time.Now()}); err != nil {
			t.Fatal(err)
		}
		p, act, ok := focusAfterTurn(store, sessID, transcript, reply, mark, time.Now(), zerolog.Nop())
		if !ok || act.Kind != focuspipeline.ActionRunVerification || !p.Review.Fixed {
			t.Fatalf("after a changed instruct fix turn: ok=%v act=%+v review=%+v", ok, act, p.Review)
		}
		for _, c := range p.Cards {
			if c.Title == focuspipeline.NoticeFixUnchanged {
				t.Fatalf("no notice card expected when the repo actually changed: %+v", c)
			}
		}
	})
}
