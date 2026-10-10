package tarsserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/focusprobe"
	"github.com/devlikebear/tars/internal/serverauth"
)

// The developer's own findings (#1196): what they found reading the diff
// goes in as a finding card of the review or pr_review round, instead of
// through a decision card's free answer or a typed instruction — neither of
// which reaches the fix prompt. When it is allowed and what "fix" does is
// focuspipeline.AddFinding's (finding_add.go).

type focusFindingRequest struct {
	Title    string `json:"title"`
	Scenario string `json:"scenario,omitempty"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	// Severity is high, medium or low; empty is medium.
	Severity string `json:"severity,omitempty"`
	// Decision is "" (decide it on the card) or "fix" (decided from the
	// start).
	Decision string `json:"decision,omitempty"`
}

// addFinding adds the developer's finding to the session's pipeline. A
// running turn does not refuse it: the card is not a turn, and a fix turn
// it asks for queues behind the running one like a card decision's.
func (a *focusAPI) addFinding(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	current, ok := a.load(w, id)
	if !ok {
		return
	}
	var req focusFindingRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	finding := focuspipeline.Finding{
		Severity: req.Severity, File: req.File, Line: req.Line, Title: req.Title, Scenario: req.Scenario,
	}
	if strings.TrimSpace(req.File) != "" {
		// The card shows the diff around file:line like the agent's
		// findings. Git runs here, outside the pipeline store's lock.
		if dir, err := a.sessions.GetCurrentDir(id); err == nil {
			finding = focusprobe.FindingExcerpts(r.Context(), dir, current.BaseCommit, []focuspipeline.Finding{finding})[0]
		}
	}
	act := focuspipeline.Action{Kind: focuspipeline.ActionNone}
	cardID := ""
	p, _, err := a.store().Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		next, action, added, err := focuspipeline.AddFinding(p, finding, req.Decision, a.now())
		act, cardID = action, added
		return next, err
	})
	switch {
	case errors.Is(err, focuspipeline.ErrInvalidFinding):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	case errors.Is(err, focuspipeline.ErrCannotAddFinding):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "pipeline": p})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.driver.start(id, act, serverauth.RoleFromRequest(r))
	resp := focusActionResponse(p, act)
	resp["card_id"] = cardID
	writeJSON(w, http.StatusCreated, resp)
}
