package tarsserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/rs/zerolog"
)

func TestSkillExtractionAPIExtractsAndApprovesDraft(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	sess, err := store.Create("release")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	messages := []session.Message{
		{ID: "m1", Role: "user", Content: "When we ship TARS issues, use GitHub Flow: PR, CI, squash merge, release verification.", Timestamp: now},
		{ID: "m2", Role: "assistant", Content: "I will run the repeatable GitHub release workflow and verify Homebrew.", Timestamp: now.Add(time.Minute)},
		{ID: "m3", Role: "user", Content: "This PR and release verification workflow should become a reusable skill.", Timestamp: now.Add(2 * time.Minute)},
	}
	if err := session.RewriteMessages(store.TranscriptPath(sess.ID), messages); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	ledger := openSkillLifecycleTestLedger(t, workspace)
	handler := newSkillExtractionAPIHandler(workspace, store, nil, zerolog.New(io.Discard), nil, ledger)
	extractReq := httptest.NewRequest(http.MethodPost, "/v1/admin/skills/extractions/extract", strings.NewReader(`{"session_id":"`+sess.ID+`"}`))
	extractReq.Header.Set("Content-Type", "application/json")
	extractRec := httptest.NewRecorder()
	handler.ServeHTTP(extractRec, extractReq)
	if extractRec.Code != http.StatusOK {
		t.Fatalf("expected extract 200, got %d body=%q", extractRec.Code, extractRec.Body.String())
	}
	var extracted struct {
		Candidates []skill.ExtractionCandidate `json:"candidates"`
	}
	if err := json.Unmarshal(extractRec.Body.Bytes(), &extracted); err != nil {
		t.Fatalf("decode extract response: %v", err)
	}
	if len(extracted.Candidates) == 0 || extracted.Candidates[0].Name != "github-release-flow" {
		t.Fatalf("unexpected extracted candidates: %+v", extracted.Candidates)
	}

	reviewed := postSkillLifecycleAction(t, handler, extracted.Candidates[0].ID, "approve")
	if reviewed.Candidate.Status != skill.ExtractionCandidateStatusApproved || reviewed.Capability.State != "canary" {
		t.Fatalf("expected approved candidate in canary, got %+v", reviewed)
	}
	if _, err := os.Stat(filepath.Join(workspace, "skills", reviewed.Capability.CapabilityName)); !os.IsNotExist(err) {
		t.Fatalf("approval activated skill before promotion: %v", err)
	}
	promoted := postSkillLifecycleAction(t, handler, extracted.Candidates[0].ID, "promote")
	skillFile := filepath.Join(workspace, "skills", promoted.Capability.CapabilityName, "SKILL.md")
	raw, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("read saved skill: %v", err)
	}
	if !strings.Contains(string(raw), "github-release-flow") || !strings.Contains(string(raw), "recommended_tools") {
		t.Fatalf("unexpected saved skill content: %s", raw)
	}
}

func TestSkillExtractionApproveReloadErrorIsWarningOnly(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	sess, err := store.Create("warn-test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	messages := []session.Message{
		{ID: "m1", Role: "user", Content: "Repeatable GitHub PR workflow each time we ship features.", Timestamp: now},
		{ID: "m2", Role: "assistant", Content: "I will run the release steps and verify.", Timestamp: now.Add(time.Minute)},
	}
	if err := session.RewriteMessages(store.TranscriptPath(sess.ID), messages); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	provider := &mockExtensionsProvider{reloadErr: fmt.Errorf("simulated reload failure")}
	ledger := openSkillLifecycleTestLedger(t, workspace)
	handler := newSkillExtractionAPIHandler(workspace, store, nil, zerolog.New(io.Discard), provider, ledger)

	extractReq := httptest.NewRequest(http.MethodPost, "/v1/admin/skills/extractions/extract", strings.NewReader(`{"session_id":"`+sess.ID+`"}`))
	extractReq.Header.Set("Content-Type", "application/json")
	extractRec := httptest.NewRecorder()
	handler.ServeHTTP(extractRec, extractReq)
	var extracted struct {
		Candidates []skill.ExtractionCandidate `json:"candidates"`
	}
	if err := json.Unmarshal(extractRec.Body.Bytes(), &extracted); err != nil || len(extracted.Candidates) == 0 {
		t.Fatalf("extract failed: %v candidates=%+v", err, extracted.Candidates)
	}

	postSkillLifecycleAction(t, handler, extracted.Candidates[0].ID, "approve")
	// Reload only happens after activation. A reload failure remains a warning.
	postSkillLifecycleAction(t, handler, extracted.Candidates[0].ID, "promote")
}

func TestSkillExtractionApproveTriggersReload(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	sess, err := store.Create("release")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	messages := []session.Message{
		{ID: "m1", Role: "user", Content: "Repeatable GitHub release workflow with PR, CI, merge, Homebrew verification.", Timestamp: now},
		{ID: "m2", Role: "assistant", Content: "I'll run the GitHub release steps and verify Homebrew.", Timestamp: now.Add(time.Minute)},
	}
	if err := session.RewriteMessages(store.TranscriptPath(sess.ID), messages); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	provider := &mockExtensionsProvider{}
	ledger := openSkillLifecycleTestLedger(t, workspace)
	handler := newSkillExtractionAPIHandler(workspace, store, nil, zerolog.New(io.Discard), provider, ledger)

	extractReq := httptest.NewRequest(http.MethodPost, "/v1/admin/skills/extractions/extract", strings.NewReader(`{"session_id":"`+sess.ID+`"}`))
	extractReq.Header.Set("Content-Type", "application/json")
	extractRec := httptest.NewRecorder()
	handler.ServeHTTP(extractRec, extractReq)
	if extractRec.Code != http.StatusOK {
		t.Fatalf("extract expected 200, got %d", extractRec.Code)
	}
	var extracted struct {
		Candidates []skill.ExtractionCandidate `json:"candidates"`
	}
	if err := json.Unmarshal(extractRec.Body.Bytes(), &extracted); err != nil || len(extracted.Candidates) == 0 {
		t.Fatalf("extract failed: %v, candidates=%+v", err, extracted.Candidates)
	}

	postSkillLifecycleAction(t, handler, extracted.Candidates[0].ID, "approve")
	if provider.reloadCount != 0 {
		t.Fatal("did not expect provider.Reload() before promotion")
	}
	postSkillLifecycleAction(t, handler, extracted.Candidates[0].ID, "promote")
	if provider.reloadCount == 0 {
		t.Fatal("expected provider.Reload() to be called after skill promotion")
	}
}

func TestSkillExtractionApprovedSkillHasNoEvidenceSection(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	sess, err := store.Create("evidence-test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	messages := []session.Message{
		{ID: "m1", Role: "user", Content: "Run the repeatable deploy workflow each time we push.", Timestamp: now},
		{ID: "m2", Role: "assistant", Content: "I will deploy and verify.", Timestamp: now.Add(time.Minute)},
	}
	if err := session.RewriteMessages(store.TranscriptPath(sess.ID), messages); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	ledger := openSkillLifecycleTestLedger(t, workspace)
	handler := newSkillExtractionAPIHandler(workspace, store, nil, zerolog.New(io.Discard), nil, ledger)

	extractReq := httptest.NewRequest(http.MethodPost, "/v1/admin/skills/extractions/extract", strings.NewReader(`{"session_id":"`+sess.ID+`"}`))
	extractReq.Header.Set("Content-Type", "application/json")
	extractRec := httptest.NewRecorder()
	handler.ServeHTTP(extractRec, extractReq)
	var extracted struct {
		Candidates []skill.ExtractionCandidate `json:"candidates"`
	}
	if err := json.Unmarshal(extractRec.Body.Bytes(), &extracted); err != nil || len(extracted.Candidates) == 0 {
		t.Fatalf("extract failed: %v candidates=%+v", err, extracted.Candidates)
	}

	postSkillLifecycleAction(t, handler, extracted.Candidates[0].ID, "approve")
	promoted := postSkillLifecycleAction(t, handler, extracted.Candidates[0].ID, "promote")
	skillFile := filepath.Join(workspace, "skills", promoted.Capability.CapabilityName, "SKILL.md")
	content, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("read skill file: %v", err)
	}
	if strings.Contains(string(content), "## Evidence") || strings.Contains(string(content), "## Provenance") {
		t.Fatalf("extracted SKILL.md must not contain Evidence or Provenance sections, got:\n%s", content)
	}
}

func TestParseLLMSkillExtractionCandidatesAddsEvidence(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	sess := session.Session{ID: "sess_llm", Title: "Browser smoke workflow"}
	messages := []session.Message{
		{ID: "m1", Role: "user", Content: "Please run a browser smoke check for the new console panel.", Timestamp: now},
		{ID: "m2", Role: "assistant", Content: "I will open the local console with Playwright and inspect the UI.", Timestamp: now.Add(time.Minute)},
		{ID: "m3", Role: "assistant", Content: "The console has no browser errors after the smoke test.", Timestamp: now.Add(2 * time.Minute)},
	}
	raw := `{"candidates":[{"name":"browser-smoke-check","title":"Browser Smoke Check","trigger":"Use after frontend work","summary":"Run the repeated browser smoke workflow.","use_case":"validate the UI in a real browser","recommended_tools":["bash"],"message_range":"m1..m3"}]}`

	candidates := parseLLMSkillExtractionCandidates(sess, messages, raw, 5)
	if len(candidates) != 1 {
		t.Fatalf("expected one LLM skill candidate, got %+v", candidates)
	}
	got := candidates[0]
	if got.Provenance.MessageRange != "m1..m3" || got.RepeatedCount != 3 || len(got.Evidence) != 3 {
		t.Fatalf("expected range evidence from transcript, got %+v", got)
	}
	if got.Evidence[0].MessageID != "m1" || !strings.Contains(got.Evidence[2].Snippet, "console") {
		t.Fatalf("unexpected evidence snippets: %+v", got.Evidence)
	}
}
