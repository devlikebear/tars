package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestSessionAPI_AutomationConsentRoundTrip(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))

	getReq := httptest.NewRequest(http.MethodGet, "/v1/admin/sessions/"+sess.ID+"/automation-consent", nil)
	getReq.Header.Set("Tars-Debug-Auth-Role", "admin")
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected initial consent 200, got %d body=%q", getRec.Code, getRec.Body.String())
	}
	var initial session.SessionAutomationConsent
	if err := json.Unmarshal(getRec.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial consent: %v", err)
	}
	if initial.AutoResume || initial.GitMutations || initial.AutonomousMutations {
		t.Fatalf("expected conservative defaults, got %+v", initial)
	}
	if initial.UpdatedAt != nil {
		t.Fatalf("expected no updated_at for default consent, got %+v", initial.UpdatedAt)
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/admin/sessions/"+sess.ID+"/automation-consent", strings.NewReader(`{
		"auto_resume": true,
		"auto_resume_after_minutes": 12,
		"allowed_resume_modes": ["move_to_next_task"],
		"git_mutations": true,
		"autonomous_mutations": false
	}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Header.Set("Tars-Debug-Auth-Role", "admin")
	patchRec := httptest.NewRecorder()
	handler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected patch consent 200, got %d body=%q", patchRec.Code, patchRec.Body.String())
	}
	var updated session.SessionAutomationConsent
	if err := json.Unmarshal(patchRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated consent: %v", err)
	}
	if !updated.AutoResume || !updated.GitMutations || updated.AutonomousMutations {
		t.Fatalf("unexpected updated consent: %+v", updated)
	}
	if !updated.AutoResumeEnabled || updated.AutoResumeAfterMinutes != 12 {
		t.Fatalf("expected explicit auto-resume policy to round trip, got %+v", updated)
	}
	if len(updated.AllowedResumeModes) != 1 || updated.AllowedResumeModes[0] != session.AutoResumeModeMoveToNextTask {
		t.Fatalf("expected allowed resume mode to round trip, got %+v", updated.AllowedResumeModes)
	}
	if updated.UpdatedAt == nil || updated.UpdatedAt.IsZero() {
		t.Fatalf("expected updated_at")
	}
}

func TestSessionAPI_StyleControlRoundTrip(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defaults := sessionStyleValues{Directness: 78, Humor: 18, Caution: 66, Autonomy: 35}
	handler := newSessionAPIHandlerWithUsageAndStyleDefaults(store, zerolog.New(io.Discard), nil, defaults)

	getReq := httptest.NewRequest(http.MethodGet, "/v1/admin/sessions/"+sess.ID+"/style", nil)
	getReq.Header.Set("Tars-Debug-Auth-Role", "admin")
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected initial style 200, got %d body=%q", getRec.Code, getRec.Body.String())
	}
	var initial sessionStyleResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial style: %v", err)
	}
	if initial.Effective.Directness != 78 || initial.Effective.Autonomy != 35 {
		t.Fatalf("expected defaults in effective style, got %+v", initial.Effective)
	}
	if initial.StyleControl != nil {
		t.Fatalf("expected no session override by default, got %+v", initial.StyleControl)
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/admin/sessions/"+sess.ID+"/style", strings.NewReader(`{
		"directness": 120,
		"humor": -8,
		"caution": 40,
		"autonomy": 95
	}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Header.Set("Tars-Debug-Auth-Role", "admin")
	patchRec := httptest.NewRecorder()
	handler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected patch style 200, got %d body=%q", patchRec.Code, patchRec.Body.String())
	}
	var updated sessionStyleResponse
	if err := json.Unmarshal(patchRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated style: %v", err)
	}
	if updated.StyleControl == nil || updated.StyleControl.UpdatedAt == nil {
		t.Fatalf("expected persisted style override with updated_at, got %+v", updated.StyleControl)
	}
	if updated.Effective.Directness != 100 || updated.Effective.Humor != 0 ||
		updated.Effective.Caution != 40 || updated.Effective.Autonomy != 95 {
		t.Fatalf("unexpected effective style after patch: %+v", updated.Effective)
	}
	if len(updated.Preview) == 0 || !strings.Contains(strings.Join(updated.Preview, " "), "direct") {
		t.Fatalf("expected concise preview lines, got %+v", updated.Preview)
	}
}
