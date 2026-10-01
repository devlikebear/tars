package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// realTempDir is a temp dir with symlinks resolved (macOS /var →
// /private/var), as the session store records it.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

type qaCall struct{ session, question, context string }

func qaFixture(t *testing.T) (http.Handler, *focusDriver, *session.Store, session.Session, string) {
	t.Helper()
	store := session.NewStore(t.TempDir())
	sess := focusSession(t, store, "goal")
	// The plan turn, on record, so the card has a source to quote.
	transcript := store.TranscriptPath(sess.ID)
	_ = session.AppendMessage(transcript, session.Message{Role: "user", Content: "plan it", Timestamp: time.Now()})
	_ = session.AppendMessage(transcript, session.Message{Role: "assistant", Content: focusPlanReply, Timestamp: time.Now()})
	if _, _, ok := focusAfterTurn(store, sess.ID, transcript, focusPlanReply, currentFocusMark(t, store, sess.ID), time.Now(), zerolog.Nop()); !ok {
		t.Fatal("no pipeline")
	}
	cwd := realTempDir(t)
	if err := store.SetWorkDirs(sess.ID, []string{cwd}, cwd); err != nil {
		t.Fatal(err)
	}
	d := newFocusDriver(zerolog.Nop())
	t.Cleanup(func() { d.Close(context.Background()) })
	d.cancels = newChatCancelRegistry()
	return newFocusPipelineHandler(store, nil, d, zerolog.Nop()), d, store, sess, cwd
}

func askQA(t *testing.T, h http.Handler, sessionID, body string) (int, struct {
	QASessionID string `json:"qa_session_id"`
	Turn        int    `json:"turn"`
}) {
	t.Helper()
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+sessionID+"/qa", body, false)
	var out struct {
		QASessionID string `json:"qa_session_id"`
		Turn        int    `json:"turn"`
	}
	if rec.Code == http.StatusAccepted {
		decodeInto(t, rec, &out)
	}
	return rec.Code, out
}

func TestFocusQA(t *testing.T) {
	h, d, store, sess, cwd := qaFixture(t)
	var mu sync.Mutex
	var calls []qaCall
	release := make(chan struct{})
	d.qaTurn = func(_ context.Context, qaID, question, consoleContext string) error {
		_ = session.AppendMessage(store.TranscriptPath(qaID), session.Message{Role: "user", Content: question, Timestamp: time.Now()})
		mu.Lock()
		calls = append(calls, qaCall{qaID, question, consoleContext})
		mu.Unlock()
		<-release
		return nil
	}
	before := pipelineOf(t, store, sess.ID)

	if code, _ := askQA(t, h, sess.ID, `{"card_id":"c1","question":"  "}`); code != http.StatusBadRequest {
		t.Fatalf("empty question = %d", code)
	}
	if code, _ := askQA(t, h, sess.ID, `{"card_id":"c99","question":"why?"}`); code != http.StatusNotFound {
		t.Fatalf("unknown card = %d", code)
	}
	if code, _ := askQA(t, h, "missing", `{"card_id":"c1","question":"why?"}`); code != http.StatusNotFound {
		t.Fatalf("unknown pipeline = %d", code)
	}

	code, first := askQA(t, h, sess.ID, `{"card_id":"c1","question":"why make test?"}`)
	if code != http.StatusAccepted || first.QASessionID == "" || first.Turn != 1 {
		t.Fatalf("ask = %d %+v", code, first)
	}
	waitFor(t, "the Q&A turn", func() bool { mu.Lock(); defer mu.Unlock(); return len(calls) == 1 })
	// One question at a time per Q&A session.
	if code, _ := askQA(t, h, sess.ID, `{"card_id":"c1","question":"and?"}`); code != http.StatusConflict {
		t.Fatalf("overlapping question = %d", code)
	}
	release <- struct{}{}

	qa, err := store.Get(first.QASessionID)
	if err != nil {
		t.Fatal(err)
	}
	if qa.Kind != "worker" || !qa.Hidden || qa.PermissionMode != chatPermissionModePlan || qa.CurrentDir != cwd || qa.Isolation != session.IsolationOff {
		t.Fatalf("qa session = %+v", qa)
	}
	mu.Lock()
	call := calls[0]
	mu.Unlock()
	if call.session != first.QASessionID || call.question != "why make test?" || !strings.Contains(call.context, "Card c1 (gate, stage plan, turn 1): Approve plan") || !strings.Contains(call.context, "make test") || !strings.Contains(call.context, "Source reply (excerpt):\nPlan below.") || strings.Contains(call.context, "<focus-plan>") {
		t.Fatalf("call = %+v", call)
	}

	waitFor(t, "the first answer", func() bool {
		_, err := d.reserveQA(first.QASessionID)
		if err == nil {
			d.mu.Lock()
			delete(d.qaBusy, first.QASessionID)
			d.mu.Unlock()
		}
		return err == nil
	})
	code, second := askQA(t, h, sess.ID, `{"card_id":"c1","question":"and the e2e?"}`)
	if code != http.StatusAccepted || second.QASessionID != first.QASessionID || second.Turn != 2 {
		t.Fatalf("second ask = %d %+v", code, second)
	}
	release <- struct{}{}

	after := pipelineOf(t, store, sess.ID)
	if after.QASessionID != first.QASessionID || len(after.QATurns["c1"]) != 2 || after.QATurns["c1"][1] != 2 {
		t.Fatalf("qa fields = %q %v", after.QASessionID, after.QATurns)
	}
	after.QASessionID, after.QATurns, after.UpdatedAt = "", nil, before.UpdatedAt
	a, _ := json.Marshal(after)
	b, _ := json.Marshal(before)
	if string(a) != string(b) {
		t.Fatalf("Q&A changed the pipeline:\n%s\n%s", a, b)
	}
	if sessions, _ := store.List(); len(sessions) != 1 {
		t.Fatalf("the Q&A session must stay hidden: %d visible", len(sessions))
	}

	// Deleting the pipeline's session removes its Q&A session too.
	store.SetDeleteHook(focusPipelineCleanup(store, zerolog.Nop()))
	if err := store.Delete(sess.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the Q&A session delete", func() bool { _, err := store.Get(first.QASessionID); return err != nil })
}

func TestFocusQAUnavailableWithoutRunner(t *testing.T) {
	h, _, _, sess, _ := qaFixture(t)
	if code, _ := askQA(t, h, sess.ID, `{"card_id":"c1","question":"why?"}`); code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d", code)
	}
}

func TestFocusQARunsAReadOnlyTurn(t *testing.T) {
	client := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "Because the plan says so."}}}
	_, d, store := focusChatHarness(t, client, &fakeVerifier{result: passAll})
	api := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	sess := plannedFocusSession(t, store)
	cwd := realTempDir(t)
	if err := store.SetWorkDirs(sess.ID, []string{cwd}, cwd); err != nil {
		t.Fatal(err)
	}
	parentBefore, _ := session.ReadMessages(store.TranscriptPath(sess.ID))

	code, out := askQA(t, api, sess.ID, `{"card_id":"c1","question":"why?"}`)
	if code != http.StatusAccepted {
		t.Fatalf("ask = %d", code)
	}
	waitFor(t, "the answer", func() bool {
		messages, _ := session.ReadMessages(store.TranscriptPath(out.QASessionID))
		return len(messages) == 2
	})
	messages, _ := session.ReadMessages(store.TranscriptPath(out.QASessionID))
	if messages[1].Content != "Because the plan says so." || !strings.Contains(messages[0].Content, "<console-context>") || strings.Contains(messages[0].Content, "<focus-stage>") {
		t.Fatalf("qa transcript = %+v", messages)
	}
	client.mu.Lock()
	workDir := client.seenWorkDirs[0]
	client.mu.Unlock()
	if workDir != cwd {
		t.Fatalf("Q&A turn ran in %q, want %q", workDir, cwd)
	}
	parentAfter, _ := session.ReadMessages(store.TranscriptPath(sess.ID))
	if len(parentAfter) != len(parentBefore) {
		t.Fatal("Q&A wrote to the pipeline session")
	}
	if p := pipelineOf(t, store, sess.ID); len(p.Cards) != 1 || p.OpenGate != focuspipeline.GatePlan {
		t.Fatalf("Q&A changed the pipeline: %+v", p)
	}
}

func TestFocusCardExcerpt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	long := strings.Repeat("é", focusQAExcerptBytes) + " the end"
	for _, m := range []session.Message{
		{Role: "user", Content: "one"}, {Role: "assistant", Content: "first <focus-report>{}</focus-report>"},
		{Role: "user", Content: "two"}, {Role: "assistant", Content: long},
	} {
		m.Timestamp = time.Now()
		_ = session.AppendMessage(path, m)
	}
	if got := focusCardExcerpt(path, 1); got != "first" {
		t.Fatalf("turn 1 = %q", got)
	}
	got := focusCardExcerpt(path, 2)
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, " the end") || len(got) > focusQAExcerptBytes+len("…") || !utf8.ValidString(got) {
		t.Fatalf("turn 2 = %d bytes, valid %v", len(got), utf8.ValidString(got))
	}
	if focusCardExcerpt(path, 0) != "" || focusCardExcerpt(path, 9) != "" {
		t.Fatal("no turn, no excerpt")
	}
}

// R6: two first questions at once share one Q&A session.
func TestFocusQAConcurrentFirstQuestionsShareOneSession(t *testing.T) {
	h, d, store, sess, _ := qaFixture(t)
	d.qaTurn = func(context.Context, string, string, string) error { return nil }
	var wg sync.WaitGroup
	ids := make([]string, 8)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+sess.ID+"/qa", `{"card_id":"c1","question":"why?"}`, false)
			var out struct {
				QASessionID string `json:"qa_session_id"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &out)
			ids[i] = out.QASessionID
		}(i)
	}
	wg.Wait()
	want := pipelineOf(t, store, sess.ID).QASessionID
	for _, id := range ids {
		if id != "" && id != want {
			t.Fatalf("questions got sessions %v, pipeline has %s", ids, want)
		}
	}
	all, _ := store.ListAll()
	workers := 0
	for _, s := range all {
		if s.Kind == "worker" {
			workers++
		}
	}
	if workers != 1 {
		t.Fatalf("%d Q&A sessions were made", workers)
	}
}
