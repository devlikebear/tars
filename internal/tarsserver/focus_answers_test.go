package tarsserver

import (
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/pkg/session"
	"github.com/rs/zerolog"
)

// addDecisionCards puts answered decision cards on the session's pipeline
// in stage (round 1) and returns their answers.
func addDecisionCards(t *testing.T, store *session.Store, id string, stage focuspipeline.StageID, questions ...string) []focuspipeline.Answer {
	t.Helper()
	var answers []focuspipeline.Answer
	if _, _, err := focusStoreFor(store).Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		for _, q := range questions {
			cardID := "q-" + q
			p.Cards = append(p.Cards, focuspipeline.Card{
				ID: cardID, Kind: focuspipeline.CardDecision, Stage: stage, Iteration: 1, Turn: 2,
				Title: q, State: focuspipeline.CardDecided, Decision: "yes", CreatedAt: time.Now(),
			})
			answers = append(answers, focuspipeline.Answer{CardID: cardID, Question: q, Answer: "yes"})
		}
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
	return answers
}

func answerAction(answers ...focuspipeline.Answer) focuspipeline.Action {
	return focuspipeline.Action{Kind: focuspipeline.ActionSendTurn, Prompt: focuspipeline.AnswersPrompt(answers), Answers: answers}
}

// Answers given while a turn runs go out as one turn.
func TestFocusDriverMergesQueuedAnswers(t *testing.T) {
	d, turns, store, id := testFocusDriver(t, asking, &fakeVerifier{result: passAll})
	answers := addDecisionCards(t, store, id, focuspipeline.StageBuild, "Rename?", "Keep the flag?")
	turns.gate = make(chan struct{})
	turns.block = func(n int) bool { return n == 1 }
	d.start(id, sendTurn("first"), "")
	waitFor(t, "first turn", func() bool { return len(turns.seen()) == 1 })
	d.start(id, answerAction(answers[0]), "")
	d.start(id, answerAction(answers[1]), "")
	close(turns.gate)
	waitDriverIdle(t, d, id)
	prompts := turns.seen()
	if len(prompts) != 2 {
		t.Fatalf("answers were not merged: %q", prompts)
	}
	for _, s := range []string{"Rename? → yes", "Keep the flag? → yes"} {
		if !strings.Contains(prompts[1], s) {
			t.Errorf("merged turn lacks %q:\n%s", s, prompts[1])
		}
	}
}

// An answer whose stage moved on is not sent as its own turn: it rides in
// the next queued turn's prompt as context.
func TestFocusDriverFoldsLateAnswersIntoTheNextTurn(t *testing.T) {
	d, turns, store, id := testFocusDriver(t, asking, &fakeVerifier{result: passAll})
	late := addDecisionCards(t, store, id, focuspipeline.StagePlan, "Which DB?")
	turns.gate = make(chan struct{})
	turns.block = func(n int) bool { return n == 1 }
	d.start(id, sendTurn("first"), "")
	waitFor(t, "first turn", func() bool { return len(turns.seen()) == 1 })
	d.start(id, answerAction(late...), "")
	d.start(id, sendTurn("Start the review stage."), "")
	close(turns.gate)
	waitDriverIdle(t, d, id)
	prompts := turns.seen()
	if len(prompts) != 2 || !strings.HasPrefix(prompts[1], "Start the review stage.") || !strings.Contains(prompts[1], "Which DB? → yes") || !strings.Contains(prompts[1], "For context") {
		t.Fatalf("prompts = %q", prompts)
	}
}

// With no turn to carry it, a late answer leaves a notice card instead.
func TestFocusDriverLateAnswerWithNothingFollowingLeavesANotice(t *testing.T) {
	d, turns, store, id := testFocusDriver(t, asking, &fakeVerifier{result: passAll})
	late := addDecisionCards(t, store, id, focuspipeline.StagePlan, "Which DB?")
	act := answerAction(late...)
	if _, _, err := focusStoreFor(store).Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p.PendingTurn = act.Prompt
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
	d.start(id, act, "")
	waitDriverIdle(t, d, id)
	if prompts := turns.seen(); len(prompts) != 0 {
		t.Fatalf("a late answer was sent as a turn: %q", prompts)
	}
	p := pipelineOf(t, store, id)
	last := p.Cards[len(p.Cards)-1]
	if last.Kind != focuspipeline.CardNotice || last.Title != focuspipeline.NoticeLateAnswers || !strings.Contains(string(last.Payload), "Which DB?") {
		t.Fatalf("last card = %+v", last)
	}
	if p.PendingTurn != "" {
		t.Fatalf("the dropped answer is still owed: %q", p.PendingTurn)
	}
}

// The live defect: two decisions asked by the fix turn, both answered while
// idle, become one answer turn in the review round; nothing reaches the pr
// stage as a stray turn.
func TestFocusReviewDecisionsAnsweredWhileIdle(t *testing.T) {
	reply := func(_ int, prompt string) string {
		switch {
		case strings.HasPrefix(prompt, "Fix these findings"):
			return "fixed\n<focus-report>{\"summary\":\"fixed\",\"decisions\":[" +
				"{\"id\":\"d1\",\"question\":\"Rename the field?\",\"options\":[\"yes\",\"no\"]}," +
				"{\"id\":\"d2\",\"question\":\"Keep the flag?\",\"options\":[\"keep\",\"drop\"]}]}</focus-report>"
		case strings.HasPrefix(prompt, "Answers to your questions"):
			return focusReport("applied", false)
		case strings.Contains(prompt, "Review the changes again"):
			return focusNoFindings
		case strings.Contains(prompt, "Start the"):
			return focusReport("next stage", false)
		default:
			return focusTwoFindings
		}
	}
	d, turns, store, _ := testFocusDriver(t, reply, &fakeVerifier{result: passAll})
	sess := reviewingFocusSession(t, store, "", "")
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	d.start(sess.ID, sendTurn("Start the review stage."), "")
	waitDriverIdle(t, d, sess.ID)
	p := pipelineOf(t, store, sess.ID)
	path := "/v1/focus/pipelines/" + sess.ID + "/cards/"
	focusRequest(t, h, "POST", path+p.Review.Triage[0], `{"state":"decided","decision":"fix"}`, true)
	focusRequest(t, h, "POST", path+p.Review.Triage[1], `{"state":"decided","decision":"dismiss"}`, true)
	waitDriverIdle(t, d, sess.ID)

	var decisions []string
	for _, c := range pipelineOf(t, store, sess.ID).Cards {
		if c.Kind == focuspipeline.CardDecision {
			decisions = append(decisions, c.ID)
		}
	}
	if len(decisions) != 2 {
		t.Fatalf("decision cards = %v", decisions)
	}
	if rec := focusRequest(t, h, "POST", path+decisions[0], `{"state":"decided","decision":"yes"}`, true); rec.Code != 200 {
		t.Fatalf("answer 1: %d %s", rec.Code, rec.Body.String())
	}
	if rec := focusRequest(t, h, "POST", path+decisions[1], `{"state":"decided","decision":"drop"}`, true); rec.Code != 200 {
		t.Fatalf("answer 2: %d %s", rec.Code, rec.Body.String())
	}
	waitFor(t, "review done", func() bool {
		return statuses(pipelineOf(t, store, sess.ID))[focuspipeline.StageReview] == focuspipeline.StatusDone
	})
	waitDriverIdle(t, d, sess.ID)

	var answerTurns int
	for _, prompt := range turns.seen() {
		if strings.Contains(prompt, "Rename the field?") || strings.Contains(prompt, "Keep the flag?") {
			answerTurns++
			if !strings.Contains(prompt, "Rename the field? → yes") || !strings.Contains(prompt, "Keep the flag? → drop") {
				t.Fatalf("answer turn lacks an answer: %q", prompt)
			}
		}
	}
	if answerTurns != 1 {
		t.Fatalf("answer turns = %d, prompts = %q", answerTurns, turns.seen())
	}
	for _, c := range pipelineOf(t, store, sess.ID).Cards {
		if c.Kind == focuspipeline.CardDecision && c.Stage != focuspipeline.StageReview {
			t.Fatalf("a decision card outside review: %+v", c)
		}
	}
}
