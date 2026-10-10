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
