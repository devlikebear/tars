package tarsserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/rs/zerolog"
)

// The developer's finding joins the open triage, shows the diff around its
// line, and reaches the fix turn with no second decision (#1196).
func TestFocusDeveloperFindingJoinsTheTriageAndTheFixTurn(t *testing.T) {
	repo, base := focusReviewRepo(t)
	content, _ := os.ReadFile(filepath.Join(repo, "b.go"))
	writeRepoFile(t, repo, "b.go", strings.Replace(string(content), "line 5\n", "line five\n", 1))
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
	d.e2e = focusE2ERunner((&recordingVerifier{}).verify)
	sess := reviewingFocusSession(t, store, repo, base)
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	path := "/v1/focus/pipelines/" + sess.ID

	d.start(sess.ID, sendTurn("Start the review stage."), "")
	waitDriverIdle(t, d, sess.ID)
	if p := pipelineOf(t, store, sess.ID); p.OpenGate != focuspipeline.GateTriage || len(p.Review.Triage) != 2 {
		t.Fatalf("triage not open: %+v", p)
	}

	if rec := focusRequest(t, h, http.MethodPost, path+"/findings", `{"title":" ","decision":"fix"}`, false); rec.Code != http.StatusBadRequest {
		t.Fatalf("no title: %d %s", rec.Code, rec.Body.String())
	}
	if rec := focusRequest(t, h, http.MethodPost, path+"/findings", `{"title":"x","decision":"dismiss"}`, false); rec.Code != http.StatusBadRequest {
		t.Fatalf("added as dismissed: %d %s", rec.Code, rec.Body.String())
	}
	rec := focusRequest(t, h, http.MethodPost, path+"/findings",
		`{"title":"renamed for no reason","scenario":"grep for line 5 finds nothing","file":"b.go","line":5,"severity":"high","decision":"fix"}`, false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
	}
	var added struct {
		Pipeline   focuspipeline.Pipeline `json:"pipeline"`
		NextPrompt string                 `json:"next_prompt"`
		CardID     string                 `json:"card_id"`
	}
	decodeInto(t, rec, &added)
	p := added.Pipeline
	if added.NextPrompt != "" || p.OpenGate != focuspipeline.GateTriage || len(p.Review.Triage) != 3 || p.Review.Triage[2] != added.CardID {
		t.Fatalf("after add: prompt %q gate %q triage %v card %q", added.NextPrompt, p.OpenGate, p.Review.Triage, added.CardID)
	}
	card := p.Cards[len(p.Cards)-1]
	var f focuspipeline.Finding
	if err := json.Unmarshal(card.Payload, &f); err != nil {
		t.Fatal(err)
	}
	if card.ID != added.CardID || card.State != focuspipeline.CardDecided || card.Decision != focuspipeline.DecisionFix ||
		f.Source != focuspipeline.FindingSourceDeveloper || !strings.Contains(f.Excerpt, "+line five") {
		t.Fatalf("card %+v finding %+v", card, f)
	}

	// The agent's two are dismissed: the fix turn is the developer's alone.
	for _, id := range p.Review.Triage[:2] {
		rec = focusRequest(t, h, http.MethodPost, path+"/cards/"+id, `{"state":"decided","decision":"dismiss"}`, false)
		if rec.Code != http.StatusOK {
			t.Fatalf("dismiss %s: %d %s", id, rec.Code, rec.Body.String())
		}
	}
	var resp struct {
		NextPrompt string `json:"next_prompt"`
	}
	decodeInto(t, rec, &resp)
	if !strings.Contains(resp.NextPrompt, "[high] b.go:5 — renamed for no reason (added by the developer)") || strings.Contains(resp.NextPrompt, "a.go:3") {
		t.Fatalf("fix prompt = %q", resp.NextPrompt)
	}
	waitFor(t, "review done", func() bool {
		return statuses(pipelineOf(t, store, sess.ID))[focuspipeline.StageReview] == focuspipeline.StatusDone
	})
	waitDriverIdle(t, d, sess.ID)
	if prompts := turns.seen(); len(prompts) != 3 || !strings.HasPrefix(prompts[1], "Fix these findings") {
		t.Fatalf("prompts = %q", prompts)
	}

	// The stage is over: nothing would pick a finding up any more.
	rec = focusRequest(t, h, http.MethodPost, path+"/findings", `{"title":"too late"}`, false)
	var conflict struct {
		Error    string                 `json:"error"`
		Pipeline focuspipeline.Pipeline `json:"pipeline"`
	}
	decodeInto(t, rec, &conflict)
	if rec.Code != http.StatusConflict || conflict.Error == "" || conflict.Pipeline.SessionID != sess.ID {
		t.Fatalf("after review: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusDeveloperFindingRefusedOutsideReview(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	sess := plannedFocusSession(t, f.store)
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+sess.ID+"/findings", `{"title":"x"}`, false)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "review and pr_review") {
		t.Fatalf("plan stage: %d %s", rec.Code, rec.Body.String())
	}
	if p := pipelineOf(t, f.store, sess.ID); len(p.Cards) != 1 {
		t.Fatalf("cards = %+v", p.Cards)
	}
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/nope/findings", `{"title":"x"}`, false); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown pipeline: %d", rec.Code)
	}
}
