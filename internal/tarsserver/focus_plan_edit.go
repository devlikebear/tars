package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/devlikebear/tars/internal/apptool"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// Editing an approved plan (focuspipeline/planedit.go). Three ways in, one
// function:
//
//	POST /v1/focus/pipelines/{id}/plan   {goal?, verify?, e2e?, e2e_setup?, e2e_teardown?} → {pipeline, changed}
//	the focus_plan_edit chat tool         (native providers)
//	a <focus-plan-edit> block in a reply  (every provider; a CLI provider has no TARS tools)
//
// An edit changes no stage and starts no turn; the next verification reads
// the edited lists. It leaves a notice card and an automation audit entry.

// focusEditPlan applies edit to the session's approved plan and brings the
// session's task contract in line. actor names who asked, for the audit.
func focusEditPlan(sessions *session.Store, sessionID string, edit focuspipeline.PlanEdit, actor string, now time.Time, audit func(ops.AutomationAuditEntry), logger zerolog.Logger) (focuspipeline.Pipeline, bool, error) {
	store := focusStoreFor(sessions)
	if store == nil {
		return focuspipeline.Pipeline{}, false, focuspipeline.ErrNoApprovedPlan
	}
	changed := false
	p, ok, err := store.Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		next, did, err := focuspipeline.EditPlan(p, edit, now)
		changed = did
		return next, err
	})
	if err != nil {
		return p, false, err
	}
	if !ok {
		return p, false, focuspipeline.ErrNoApprovedPlan
	}
	if changed {
		focusPlanEdited(sessions, p, actor, audit, logger)
	}
	return p, changed, nil
}

// focusPlanEdited runs after an edit was stored (and the pipeline store's
// lock released — saving tasks takes the session index lock): the session
// contract's goal and commands follow the plan, and the edit is audited.
func focusPlanEdited(sessions *session.Store, p focuspipeline.Pipeline, actor string, audit func(ops.AutomationAuditEntry), logger zerolog.Logger) {
	if p.Plan == nil {
		return
	}
	if st, err := sessions.GetTasks(p.SessionID); err == nil && st.Contract != nil {
		fresh := focuspipeline.SessionTasks(*p.Plan, time.Now())
		st.Contract.Goal = fresh.Contract.Goal
		st.Contract.VerificationCommands = fresh.Contract.VerificationCommands
		if st.Plan != nil {
			st.Plan.Goal = fresh.Plan.Goal
		}
		if err := sessions.SaveTasks(p.SessionID, st); err != nil {
			logger.Warn().Err(err).Str("session_id", p.SessionID).Msg("focus: update task contract after plan edit failed")
		}
	}
	if audit == nil {
		return
	}
	details := map[string]any{}
	if n := len(p.Cards); n > 0 && p.Cards[n-1].Title == focuspipeline.NoticePlanEdited {
		details["card"] = p.Cards[n-1].Payload
	}
	audit(ops.AutomationAuditEntry{Actor: actor, Action: "focus_plan_edit", SessionID: p.SessionID, Result: "edited", Details: details})
}

// planEdit is POST /v1/focus/pipelines/{id}/plan.
func (a *focusAPI) planEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.load(w, id); !ok {
		return
	}
	var edit focuspipeline.PlanEdit
	if !decodeJSONBody(w, r, &edit) {
		return
	}
	if edit.Empty() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name at least one of goal, verify, e2e, e2e_setup, e2e_teardown"})
		return
	}
	p, changed, err := focusEditPlan(a.sessions, id, edit, "console", a.now(), a.driver.auditFunc(), a.logger)
	switch {
	case errors.Is(err, focuspipeline.ErrNoApprovedPlan):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "pipeline": p})
	case errors.Is(err, focuspipeline.ErrInvalidEdits):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "edit plan failed"})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"pipeline": p, "changed": changed})
	}
}

// auditFunc is the driver's automation audit, nil when there is none.
func (d *focusDriver) auditFunc() func(ops.AutomationAuditEntry) {
	if d == nil {
		return nil
	}
	return d.audit
}

// focusPlanEditable reports whether the session has a focus pipeline with
// an approved plan: only then is the focus_plan_edit tool registered, so
// no other chat carries its schema.
func focusPlanEditable(sessions *session.Store, sessionID string) bool {
	store := focusStoreFor(sessions)
	if store == nil {
		return false
	}
	p, ok, err := store.Get(sessionID)
	return err == nil && ok && p.Plan != nil && p.Current != focuspipeline.StagePlan && !focuspipeline.Finished(p)
}

// newFocusPlanEditor is the focus_plan_edit tool's way into focusEditPlan.
func newFocusPlanEditor(sessions *session.Store, sessionID string, driver *focusDriver, logger zerolog.Logger) apptool.FocusPlanEditor {
	return func(_ context.Context, edit focuspipeline.PlanEdit) (focuspipeline.Plan, []string, error) {
		p, changed, err := focusEditPlan(sessions, sessionID, edit, "focus_plan_edit", time.Now(), driver.auditFunc(), logger)
		if err != nil || p.Plan == nil {
			return focuspipeline.Plan{}, nil, err
		}
		var changes []string
		if n := len(p.Cards); changed && n > 0 {
			var payload struct {
				Changes []string `json:"changes"`
			}
			if json.Unmarshal(p.Cards[n-1].Payload, &payload) == nil {
				changes = payload.Changes
			}
		}
		return *p.Plan, changes, nil
	}
}
