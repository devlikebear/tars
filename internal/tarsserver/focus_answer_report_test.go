package tarsserver

import (
	"net/http"
	"testing"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

func TestFocusAnswersRecordWhereTheyCameFrom(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	fromPhone := plannedFocusSession(t, f.store)
	fromDesk := plannedFocusSession(t, f.store)

	if rec := sendTo(t, asRole(h, serverauth.RoleFocus), http.MethodPost, "/v1/focus/pipelines/"+fromPhone.ID+"/gates/plan", `{"action":"approve"}`); rec.Code != http.StatusOK {
		t.Fatalf("phone approve: %d %s", rec.Code, rec.Body.String())
	}
	// The body cannot claim a source: the server decides it.
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+fromDesk.ID+"/gates/plan", `{"action":"approve","by":"phone"}`, false); rec.Code != http.StatusOK {
		t.Fatalf("desk approve: %d %s", rec.Code, rec.Body.String())
	}
	gateBy := func(id string) string {
		p, _, err := focusStoreFor(f.store).Get(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range p.Cards {
			if c.Kind == focuspipeline.CardGate && c.State == focuspipeline.CardDecided {
				if c.DecidedAt == nil {
					t.Fatalf("%s: decided without a time", id)
				}
				return c.DecidedBy
			}
		}
		return ""
	}
	if got := gateBy(fromPhone.ID); got != focuspipeline.AnswerPhone {
		t.Errorf("phone gate decided by %q", got)
	}
	if got := gateBy(fromDesk.ID); got != focuspipeline.AnswerDesk {
		t.Errorf("desk gate decided by %q", got)
	}
	// A card answer goes through the same source (an unknown card is 404).
	if rec := sendTo(t, asRole(h, serverauth.RoleFocus), http.MethodPost, "/v1/focus/pipelines/"+fromPhone.ID+"/cards/nope", `{"state":"decided","decision":"x"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown card: %d", rec.Code)
	}

	rec := focusRequest(t, h, http.MethodGet, "/v1/focus/answer-report", "", true)
	var out struct {
		Days   int                        `json:"days"`
		Report focuspipeline.AnswerReport `json:"report"`
	}
	decodeInto(t, rec, &out)
	if rec.Code != http.StatusOK || out.Days != 30 {
		t.Fatalf("report: %d %s", rec.Code, rec.Body.String())
	}
	want := focuspipeline.AnswerCounts{Phone: 1, Desk: 1}
	if out.Report.Gates != want || out.Report.Pipelines != 2 || out.Report.PhonePipelines != 1 {
		t.Fatalf("report = %+v", out.Report)
	}
	for _, days := range []string{"0", "366", "x"} {
		if rec := focusRequest(t, h, http.MethodGet, "/v1/focus/answer-report?days="+days, "", true); rec.Code != http.StatusBadRequest {
			t.Errorf("days=%s: %d", days, rec.Code)
		}
	}
	if rec := focusRequest(t, h, http.MethodGet, "/v1/focus/answer-report?days=7", "", true); rec.Code != http.StatusOK {
		t.Errorf("days=7: %d", rec.Code)
	}
}

func TestFocusAnswerSource(t *testing.T) {
	phone := asRole(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(focusAnswerSource(r)))
	}), serverauth.RoleFocus)
	if got := sendTo(t, phone, http.MethodGet, "/", "").Body.String(); got != focuspipeline.AnswerPhone {
		t.Errorf("focus role: %q", got)
	}
	for _, role := range []string{serverauth.RoleAdmin, serverauth.RoleUser, ""} {
		h := asRole(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(focusAnswerSource(r)))
		}), role)
		if got := sendTo(t, h, http.MethodGet, "/", "").Body.String(); got != focuspipeline.AnswerDesk {
			t.Errorf("role %q: %q", role, got)
		}
	}
}
