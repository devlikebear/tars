package tarsserver

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/rs/zerolog"
)

func TestFocusPlanEditIsAuditedAndRefusesBadInput(t *testing.T) {
	d, _, store, _ := testFocusDriver(t, func(int, string) string { return focusNoFindings }, &fakeVerifier{result: passAll})
	var audited []ops.AutomationAuditEntry
	d.audit = func(entry ops.AutomationAuditEntry) { audited = append(audited, entry) }
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	sess := reviewingFocusSession(t, store, "", "")
	path := "/v1/focus/pipelines/" + sess.ID + "/plan"

	if rec := focusRequest(t, h, http.MethodPost, path, `{"goal":"  "}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty goal: %d %s", rec.Code, rec.Body.String())
	}
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/missing/plan", `{"e2e":[]}`, true); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown session: %d %s", rec.Code, rec.Body.String())
	}
	if rec := focusRequest(t, h, http.MethodPost, path, `{"e2e":[]}`, true); rec.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body.String())
	}
	if len(audited) != 1 || audited[0].Action != "focus_plan_edit" || audited[0].SessionID != sess.ID || audited[0].Details["card"] == nil {
		t.Fatalf("audit = %+v", audited)
	}
	// The same edit again changes nothing and is not audited twice.
	if rec := focusRequest(t, h, http.MethodPost, path, `{"e2e":[]}`, true); rec.Code != http.StatusOK || len(audited) != 1 {
		t.Fatalf("no-op edit: %d, audited %d", rec.Code, len(audited))
	}
	if focusPlanEditable(nil, sess.ID) || (*focusDriver)(nil).auditFunc() != nil {
		t.Fatal("no store, no driver: nothing to edit or audit")
	}
	if _, _, err := focusEditPlan(nil, sess.ID, focuspipeline.PlanEdit{}, "test", time.Now(), nil, zerolog.Nop()); err == nil {
		t.Fatal("an edit without a store must fail")
	}
}

func TestFocusPlanEditBlockInAReplyUpdatesTheContract(t *testing.T) {
	_, _, store, _ := testFocusDriver(t, func(int, string) string { return focusNoFindings }, &fakeVerifier{result: passAll})
	sess := reviewingFocusSession(t, store, "", "")
	if err := store.SaveTasks(sess.ID, focuspipeline.SessionTasks(*pipelineOf(t, store, sess.ID).Plan, time.Now())); err != nil {
		t.Fatal(err)
	}
	reply := "dropped the live check as asked\n" +
		`<focus-plan-edit>{"e2e":[],"goal":"   "}</focus-plan-edit>` + "\n" + focusNoFindings
	p, act, ok := focusAfterTurn(store, sess.ID, store.TranscriptPath(sess.ID), reply, currentFocusMark(t, store, sess.ID), time.Now(), zerolog.Nop())
	if !ok || act.Kind != focuspipeline.ActionRunVerification {
		t.Fatalf("ok = %v act = %+v", ok, act)
	}
	// An edit the pipeline refuses (an empty goal) changes nothing and
	// says so on a notice card; the turn itself still counts.
	if len(p.Plan.E2E) != 1 {
		t.Fatalf("refused edit changed the plan: %+v", p.Plan)
	}
	refused := false
	for _, c := range p.Cards {
		refused = refused || (c.Kind == focuspipeline.CardNotice && strings.Contains(string(c.Payload), focuspipeline.TagPlanEdit))
	}
	if !refused {
		t.Fatalf("no notice for the refused edit: %+v", p.Cards)
	}

	if _, _, err := focusStoreFor(store).Update(sess.ID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p.AwaitingVerification = false
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
	reply = "dropped\n" + `<focus-plan-edit>{"e2e":[]}</focus-plan-edit>` + "\n" + focusNoFindings
	p, _, ok = focusAfterTurn(store, sess.ID, store.TranscriptPath(sess.ID), reply, currentFocusMark(t, store, sess.ID), time.Now(), zerolog.Nop())
	if !ok || len(p.Plan.E2E) != 0 {
		t.Fatalf("ok = %v plan = %+v", ok, p.Plan)
	}
	tasks, err := store.GetTasks(sess.ID)
	if err != nil || strings.Join(tasks.Contract.VerificationCommands, ",") != "make test" {
		t.Fatalf("contract = %+v err = %v", tasks.Contract, err)
	}
}
