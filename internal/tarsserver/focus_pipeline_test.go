package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

const focusPlanReply = "Plan below.\n<focus-plan>" +
	`{"goal":"g","tasks":[{"title":"t1","done":"d1"}],"stages":["plan","build","pr","merge"],"verify":["make test"],"e2e":["make console-e2e"]}` +
	"</focus-plan>"

func focusRequest(t *testing.T, h http.Handler, method, path, body string, admin bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if admin {
		req.Header.Set("Tars-Debug-Auth-Role", "admin")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeInto(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
}

// focusSession makes a session with a fresh pipeline.
func focusSession(t *testing.T, store *session.Store, goal string) session.Session {
	t.Helper()
	sess, err := store.Create("focus")
	if err != nil {
		t.Fatal(err)
	}
	if err := focusStoreFor(store).Save(focuspipeline.New(sess.ID, goal, time.Now())); err != nil {
		t.Fatal(err)
	}
	return sess
}

// plannedFocusSession makes a session whose plan gate is open.
func plannedFocusSession(t *testing.T, store *session.Store) session.Session {
	t.Helper()
	sess := focusSession(t, store, "goal")
	if _, _, ok := focusAfterTurn(store, sess.ID, store.TranscriptPath(sess.ID), focusPlanReply, time.Now(), zerolog.Nop()); !ok {
		t.Fatal("focusAfterTurn found no pipeline")
	}
	return sess
}

func TestFocusCreate(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, zerolog.Nop())

	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", `{"goal":"g","cwd":`+jsonString(f.repo)+`}`, false); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin: %d %s", rec.Code, rec.Body.String())
	}
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", `{"goal":" ","cwd":`+jsonString(f.repo)+`}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("no goal: %d", rec.Code)
	}
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", `{"goal":"g","cwd":"relative/dir"}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad cwd: %d", rec.Code)
	}
	if list, _ := f.store.List(); len(list) != 0 {
		t.Fatalf("failed creates left sessions: %+v", list)
	}

	goal := "Add a focus mode pipeline view so development has stages, gates and cards"
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", `{"goal":`+jsonString(goal)+`,"cwd":`+jsonString(f.repo)+`,"isolate":true}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		SessionID string                 `json:"session_id"`
		Pipeline  focuspipeline.Pipeline `json:"pipeline"`
	}
	decodeInto(t, rec, &out)
	if out.SessionID == "" || out.Pipeline.Goal != goal || out.Pipeline.Current != focuspipeline.StagePlan {
		t.Fatalf("out = %+v", out)
	}
	sess, err := f.store.Get(out.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Worktree == nil || sess.CurrentDir != sess.Worktree.Dir {
		t.Fatalf("session not isolated: %+v", sess)
	}
	if n := len([]rune(sess.Title)); n == 0 || n > focusTitleRunes || !strings.HasPrefix(goal, strings.TrimSuffix(sess.Title, "…")) {
		t.Fatalf("title = %q", sess.Title)
	}
	if _, ok, _ := focusStoreFor(f.store).Get(out.SessionID); !ok {
		t.Fatal("pipeline not saved")
	}
}

func TestFocusListAndGet(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, zerolog.Nop())
	live := plannedFocusSession(t, f.store)
	// A pipeline whose session is gone never shows.
	if err := focusStoreFor(f.store).Save(focuspipeline.New("ghost", "g", time.Now())); err != nil {
		t.Fatal(err)
	}

	rec := focusRequest(t, h, http.MethodGet, "/v1/focus/pipelines", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var items []focusListItem
	decodeInto(t, rec, &items)
	if len(items) != 1 || items[0].SessionID != live.ID || items[0].Title != "focus" || items[0].OpenGate != focuspipeline.GatePlan || items[0].NeedsInput != 1 || items[0].Current != focuspipeline.StagePlan {
		t.Fatalf("items = %+v", items)
	}

	if rec := focusRequest(t, h, http.MethodGet, "/v1/focus/pipelines/"+live.ID, "", false); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"open_gate":"plan"`) {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	for _, id := range []string{"ghost", "nope"} {
		if rec := focusRequest(t, h, http.MethodGet, "/v1/focus/pipelines/"+id, "", false); rec.Code != http.StatusNotFound {
			t.Fatalf("get %s: %d", id, rec.Code)
		}
	}
	plain, _ := f.store.Create("plain")
	if rec := focusRequest(t, h, http.MethodGet, "/v1/focus/pipelines/"+plain.ID, "", false); rec.Code != http.StatusNotFound {
		t.Fatalf("session without pipeline: %d", rec.Code)
	}
}

func TestFocusPlanGate(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, zerolog.Nop())
	sess := plannedFocusSession(t, f.store)
	gateURL := "/v1/focus/pipelines/" + sess.ID + "/gates/plan"

	if rec := focusRequest(t, h, http.MethodPost, gateURL, `{"action":"yolo"}`, false); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad action: %d", rec.Code)
	}
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+sess.ID+"/gates/merge", `{"action":"approve"}`, false); rec.Code != http.StatusConflict {
		t.Fatalf("wrong gate: %d", rec.Code)
	}

	rec := focusRequest(t, h, http.MethodPost, gateURL, `{"action":"approve"}`, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Pipeline   focuspipeline.Pipeline `json:"pipeline"`
		NextPrompt string                 `json:"next_prompt"`
	}
	decodeInto(t, rec, &out)
	if out.NextPrompt != "Plan approved. Start the build stage with task 1." || out.Pipeline.Current != focuspipeline.StageBuild {
		t.Fatalf("out = %+v", out)
	}
	if st, ok := out.Pipeline.Stage(focuspipeline.StageReview); !ok || st.Status != focuspipeline.StatusSkipped {
		t.Fatalf("review = %+v", st)
	}

	tasks, err := f.store.GetTasks(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tasks.Contract == nil || tasks.Contract.Status != session.ContractStatusApproved ||
		strings.Join(tasks.Contract.VerificationCommands, ",") != "make test,make console-e2e" || len(tasks.Tasks) != 1 {
		t.Fatalf("tasks = %+v contract = %+v", tasks.Tasks, tasks.Contract)
	}

	// A double click or a stale tab: 409 with the current pipeline.
	rec = focusRequest(t, h, http.MethodPost, gateURL, `{"action":"approve"}`, false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second approve: %d", rec.Code)
	}
	var conflict struct {
		Error    string                 `json:"error"`
		Pipeline focuspipeline.Pipeline `json:"pipeline"`
	}
	decodeInto(t, rec, &conflict)
	if conflict.Error == "" || conflict.Pipeline.Current != focuspipeline.StageBuild {
		t.Fatalf("conflict = %+v", conflict)
	}
	saved, _, _ := focusStoreFor(f.store).Get(sess.ID)
	if !saved.UpdatedAt.Equal(out.Pipeline.UpdatedAt) {
		t.Fatal("a closed gate changed the pipeline")
	}
}

func TestFocusPlanGateEditsAndRequestChanges(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, zerolog.Nop())

	sess := plannedFocusSession(t, f.store)
	gateURL := "/v1/focus/pipelines/" + sess.ID + "/gates/plan"
	if rec := focusRequest(t, h, http.MethodPost, gateURL, `{"action":"approve","edits":{"goal":"x","tasks":[]}}`, false); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty edits: %d", rec.Code)
	}
	rec := focusRequest(t, h, http.MethodPost, gateURL, `{"action":"request_changes","note":"split t1"}`, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "split t1") {
		t.Fatalf("request changes: %d %s", rec.Code, rec.Body.String())
	}
	if tasks, _ := f.store.GetTasks(sess.ID); tasks.Contract != nil {
		t.Fatal("request_changes must not write a contract")
	}

	edited := plannedFocusSession(t, f.store)
	rec = focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+edited.ID+"/gates/plan",
		`{"action":"approve","edits":{"goal":"edited","tasks":[{"title":"a","done":"b"},{"title":"c","done":"d"}],"stages":["plan","build"],"verify":["go test ./..."]}}`, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve edits: %d %s", rec.Code, rec.Body.String())
	}
	tasks, _ := f.store.GetTasks(edited.ID)
	if len(tasks.Tasks) != 2 || tasks.Plan == nil || tasks.Plan.Goal != "edited" || tasks.Contract.VerificationCommands[0] != "go test ./..." {
		t.Fatalf("tasks = %+v", tasks)
	}
}

func TestFocusCards(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, zerolog.Nop())
	sess := focusSession(t, f.store, "g")
	reply := focusPlanReply + "\n<focus-report>{\"summary\":\"s\",\"decisions\":[{\"id\":\"d1\",\"question\":\"Which store?\",\"options\":[\"file\",\"sqlite\"]}]}</focus-report>"
	p, _, ok := focusAfterTurn(f.store, sess.ID, f.store.TranscriptPath(sess.ID), reply, time.Now(), zerolog.Nop())
	if !ok || len(p.Cards) != 3 {
		t.Fatalf("cards = %+v", p.Cards)
	}
	base := "/v1/focus/pipelines/" + sess.ID + "/cards/"
	report, decision := p.Cards[1].ID, p.Cards[2].ID

	if rec := focusRequest(t, h, http.MethodPost, base+report, `{"state":"seen"}`, false); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"next_prompt":""`) {
		t.Fatalf("seen: %d %s", rec.Code, rec.Body.String())
	}
	rec := focusRequest(t, h, http.MethodPost, base+decision, `{"state":"decided","decision":"file"}`, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"next_prompt":"Which store? → file"`) {
		t.Fatalf("decide: %d %s", rec.Code, rec.Body.String())
	}
	if rec := focusRequest(t, h, http.MethodPost, base+decision, `{"state":"decided","decision":"sqlite"}`, false); rec.Code != http.StatusConflict {
		t.Fatalf("re-decide: %d", rec.Code)
	}
	if rec := focusRequest(t, h, http.MethodPost, base+"c99", `{"state":"seen"}`, false); rec.Code != http.StatusNotFound {
		t.Fatalf("missing card: %d", rec.Code)
	}
	if rec := focusRequest(t, h, http.MethodPost, base+report, `{"state":"bogus"}`, false); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad state: %d", rec.Code)
	}
}

func TestAppendFocusGuidance(t *testing.T) {
	store := session.NewStore(t.TempDir())
	plain, _ := store.Create("plain")
	sess := focusSession(t, store, "ship it")
	log := zerolog.Nop()

	if got := appendFocusGuidance("hello", store, plain.ID, log); got != "hello" {
		t.Fatalf("no pipeline: %q", got)
	}
	got := appendFocusGuidance("hello\n", store, sess.ID, log)
	if !strings.HasPrefix(got, "hello\n\n<focus-stage>\n") || !strings.HasSuffix(got, "\n</focus-stage>") || !strings.Contains(got, "<focus-plan>") {
		t.Fatalf("with pipeline: %q", got)
	}
	if got := appendFocusGuidance("/review now", store, sess.ID, log); got != "/review now" {
		t.Fatalf("slash command: %q", got)
	}
	// A stopped pipeline gives no guidance.
	if _, _, err := focusStoreFor(store).Update(sess.ID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p.Stages[0].Status = focuspipeline.StatusBlocked
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := appendFocusGuidance("hello", store, sess.ID, log); got != "hello" {
		t.Fatalf("stopped: %q", got)
	}
}

func TestFocusChatTurn(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess := focusSession(t, store, "ship it")
	client := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: focusPlanReply}}}
	handler := newChatAPIHandlerWithRuntimeConfig(root, store, client, nil, zerolog.Nop(), 2, nil, "", defaultChatToolingOptions())

	rec := focusRequest(t, handler, http.MethodPost, "/v1/chat", `{"session_id":"`+sess.ID+`","message":"plan it"}`, false)
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"pipeline"`) || !strings.Contains(body, `"open_gate":"plan"`) {
		t.Fatalf("no pipeline event in %s", body)
	}
	if strings.Index(body, `"type":"pipeline"`) > strings.Index(body, `"type":"done"`) {
		t.Fatal("pipeline event must come before done")
	}
	client.mu.Lock()
	seen := client.seenMessages
	client.mu.Unlock()
	if len(seen) == 0 {
		t.Fatal("llm not called")
	}
	last := seen[0][len(seen[0])-1]
	if last.Role != "user" || !strings.Contains(last.Content, "plan it\n\n<focus-stage>") || !strings.Contains(last.Content, "current stage: plan") {
		t.Fatalf("user message = %q", last.Content)
	}
	p, _, _ := focusStoreFor(store).Get(sess.ID)
	if p.OpenGate != focuspipeline.GatePlan || len(p.Cards) != 1 || p.Cards[0].Turn != 1 {
		t.Fatalf("pipeline = %+v", p)
	}

	// A reply without its block: a notice and a re-request, nothing else.
	client.mu.Lock()
	client.response = llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "sorry, no block"}}
	client.mu.Unlock()
	if _, _, err := focusStoreFor(store).Update(sess.ID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		next, _, err := focuspipeline.Apply(p, focuspipeline.Event{Kind: focuspipeline.EventGate, Gate: "plan", Action: "approve"}, time.Now())
		return next, err
	}); err != nil {
		t.Fatal(err)
	}
	rec = focusRequest(t, handler, http.MethodPost, "/v1/chat", `{"session_id":"`+sess.ID+`","message":"go"}`, false)
	if !strings.Contains(rec.Body.String(), `"next_prompt":"Your last reply did not end`) {
		t.Fatalf("no re-request in %s", rec.Body.String())
	}
	p, _, _ = focusStoreFor(store).Get(sess.ID)
	last2 := p.Cards[len(p.Cards)-1]
	if last2.Kind != focuspipeline.CardNotice || last2.Turn != 2 || p.Current != focuspipeline.StageBuild {
		t.Fatalf("pipeline = %+v", p)
	}

	// A session without a pipeline gets no event.
	plain, _ := store.Create("plain")
	rec = focusRequest(t, handler, http.MethodPost, "/v1/chat", `{"session_id":"`+plain.ID+`","message":"hi"}`, false)
	if strings.Contains(rec.Body.String(), `"type":"pipeline"`) {
		t.Fatal("pipeline event for a plain session")
	}
}

func TestFocusPipelineRemovedWithSession(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	var other []string
	attachSessionDeleteHooks(store,
		nil,
		func(id string) { other = append(other, id) },
		focusPipelineCleanup(store, zerolog.Nop()),
	)
	sess := focusSession(t, store, "g")
	path := filepath.Join(root, "sessions", sess.ID+".pipeline.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("pipeline survived its session: %v", err)
	}
	if len(other) != 1 || other[0] != sess.ID {
		t.Fatalf("chained hook not run: %v", other)
	}

	// Orphans left from before are swept, live ones kept.
	live := focusSession(t, store, "g")
	if err := focusStoreFor(store).Save(focuspipeline.New("gone", "g", time.Now())); err != nil {
		t.Fatal(err)
	}
	if n := sweepOrphanFocusPipelines(store, zerolog.Nop()); n != 1 {
		t.Fatalf("swept %d", n)
	}
	if _, ok, _ := focusStoreFor(store).Get(live.ID); !ok {
		t.Fatal("live pipeline swept")
	}
}

func TestFocusAdvance(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, zerolog.Nop())
	sess := plannedFocusSession(t, f.store)
	advanceURL := "/v1/focus/pipelines/" + sess.ID + "/advance"

	// The plan gate is open: only the gate may pass the plan stage.
	rec := focusRequest(t, h, http.MethodPost, advanceURL, `{"stage":"plan"}`, false)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"open_gate":"plan"`) {
		t.Fatalf("advance with open gate: %d %s", rec.Code, rec.Body.String())
	}
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+sess.ID+"/gates/plan", `{"action":"approve"}`, false); rec.Code != http.StatusOK {
		t.Fatalf("approve: %d", rec.Code)
	}

	rec = focusRequest(t, h, http.MethodPost, advanceURL, `{"stage":"build"}`, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("advance: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Pipeline   focuspipeline.Pipeline `json:"pipeline"`
		NextPrompt string                 `json:"next_prompt"`
	}
	decodeInto(t, rec, &out)
	// focusPlanReply skips review and pr_review.
	if out.Pipeline.Current != focuspipeline.StagePR || out.NextPrompt != "Approved. Start the pr stage." {
		t.Fatalf("out = %+v", out)
	}
	if saved, _, _ := focusStoreFor(f.store).Get(sess.ID); saved.Current != focuspipeline.StagePR {
		t.Fatalf("saved current = %s", saved.Current)
	}

	// A stale tab passing build again: 409 with the current pipeline.
	rec = focusRequest(t, h, http.MethodPost, advanceURL, `{"stage":"build"}`, false)
	var conflict struct {
		Error    string                 `json:"error"`
		Pipeline focuspipeline.Pipeline `json:"pipeline"`
	}
	decodeInto(t, rec, &conflict)
	if rec.Code != http.StatusConflict || conflict.Error == "" || conflict.Pipeline.Current != focuspipeline.StagePR {
		t.Fatalf("stale advance: %d %s", rec.Code, rec.Body.String())
	}

	if rec := focusRequest(t, h, http.MethodPost, advanceURL, `{`, false); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad body: %d", rec.Code)
	}
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/nope/advance", `{"stage":"build"}`, false); rec.Code != http.StatusNotFound {
		t.Fatalf("missing: %d", rec.Code)
	}
}

func TestFocusStop(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, zerolog.Nop())
	sess := plannedFocusSession(t, f.store)
	stopURL := "/v1/focus/pipelines/" + sess.ID + "/stop"

	rec := focusRequest(t, h, http.MethodPost, stopURL, "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Pipeline   focuspipeline.Pipeline `json:"pipeline"`
		NextPrompt string                 `json:"next_prompt"`
	}
	decodeInto(t, rec, &out)
	st, _ := out.Pipeline.Stage(focuspipeline.StagePlan)
	if st.Status != focuspipeline.StatusBlocked || out.Pipeline.OpenGate != "" || out.NextPrompt != "" {
		t.Fatalf("out = %+v", out)
	}
	if c := out.Pipeline.Cards[0]; c.State != focuspipeline.CardDecided || c.Decision != focuspipeline.GateStop {
		t.Fatalf("gate card = %+v", c)
	}

	// Already stopped: 409 with the pipeline, and it stays as it was.
	rec = focusRequest(t, h, http.MethodPost, stopURL, "", false)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"status":"blocked"`) {
		t.Fatalf("second stop: %d %s", rec.Code, rec.Body.String())
	}
	if saved, _, _ := focusStoreFor(f.store).Get(sess.ID); !saved.UpdatedAt.Equal(out.Pipeline.UpdatedAt) {
		t.Fatal("a refused stop changed the pipeline")
	}
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/nope/stop", "", false); rec.Code != http.StatusNotFound {
		t.Fatalf("missing: %d", rec.Code)
	}
}
