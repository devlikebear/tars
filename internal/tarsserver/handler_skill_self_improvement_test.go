package tarsserver

import (
	"context"
	"encoding/json"
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
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func TestSkillSelfImprovementRequiresEvaluationApprovalAndCanaryBeforePromotion(t *testing.T) {
	workspace := t.TempDir()
	sessionStore := session.NewStore(workspace)
	ledger := openSkillLifecycleTestLedger(t, workspace)
	provider := &mockExtensionsProvider{}
	sess, err := sessionStore.Create("review lifecycle")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	messages := []session.Message{
		{ID: "m1", Role: "user", Content: "Use a repeatable GitHub PR review workflow for every release.", Timestamp: now},
		{ID: "m2", Role: "assistant", Content: "I will review CI, the PR diff, and release verification.", Timestamp: now.Add(time.Minute)},
		{ID: "m3", Role: "user", Content: "Turn the repeated GitHub release review workflow into a reusable skill.", Timestamp: now.Add(2 * time.Minute)},
	}
	if err := session.RewriteMessages(sessionStore.TranscriptPath(sess.ID), messages); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	handler := newSkillExtractionAPIHandler(workspace, sessionStore, nil, zerolog.New(io.Discard), provider, ledger)

	candidate := extractSkillLifecycleCandidate(t, handler, sess.ID)
	evaluated := postSkillLifecycleAction(t, handler, candidate.ID, "evaluate")
	if evaluated.Capability.State != workstore.CapabilityStateShadow {
		t.Fatalf("evaluated state = %s, want shadow", evaluated.Capability.State)
	}
	if len(evaluated.Evaluations) != 3 {
		t.Fatalf("evaluation count = %d, want sandbox/offline/shadow", len(evaluated.Evaluations))
	}
	activeSkill := filepath.Join(workspace, "skills", evaluated.Capability.CapabilityName, "SKILL.md")
	if _, err := os.Stat(activeSkill); !os.IsNotExist(err) {
		t.Fatalf("evaluation wrote active skill before approval: %v", err)
	}

	approved := postSkillLifecycleAction(t, handler, candidate.ID, "approve")
	if approved.Capability.State != workstore.CapabilityStateCanary || approved.Capability.ApprovalID == "" {
		t.Fatalf("approved capability = %+v, want approved canary", approved.Capability)
	}
	if _, err := os.Stat(activeSkill); !os.IsNotExist(err) {
		t.Fatalf("approval wrote active skill before promotion: %v", err)
	}
	if provider.reloadCount != 0 {
		t.Fatalf("provider reload count before promotion = %d, want 0", provider.reloadCount)
	}

	promoted := postSkillLifecycleAction(t, handler, candidate.ID, "promote")
	if promoted.Capability.State != workstore.CapabilityStatePromoted {
		t.Fatalf("promoted state = %s, want promoted", promoted.Capability.State)
	}
	content, err := os.ReadFile(activeSkill)
	if err != nil {
		t.Fatalf("read promoted skill: %v", err)
	}
	if !strings.Contains(string(content), "github-release-flow") {
		t.Fatalf("promoted skill content = %q", content)
	}
	if provider.reloadCount != 1 {
		t.Fatalf("provider reload count after promotion = %d, want 1", provider.reloadCount)
	}

	rolledBack := postSkillLifecycleAction(t, handler, candidate.ID, "rollback")
	if rolledBack.Capability.State != workstore.CapabilityStateRolledBack {
		t.Fatalf("rollback state = %s, want rolled_back", rolledBack.Capability.State)
	}
	if _, err := os.Stat(activeSkill); !os.IsNotExist(err) {
		t.Fatalf("first-version rollback retained active skill: %v", err)
	}
	if provider.reloadCount != 2 {
		t.Fatalf("provider reload count after rollback = %d, want 2", provider.reloadCount)
	}
}

func TestSkillSelfImprovementMigratesLegacyInboxWithoutMutatingIt(t *testing.T) {
	workspace := t.TempDir()
	ledger := openSkillLifecycleTestLedger(t, workspace)
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	legacy := skill.ExtractionCandidate{
		ID:            "skillcand_legacy",
		Status:        skill.ExtractionCandidateStatusPending,
		Name:          "legacy-review",
		Title:         "Legacy Review",
		Summary:       "Review a legacy workflow.",
		UseCase:       "review legacy work",
		SourceSession: "legacy-session",
		Provenance: skill.ExtractionProvenance{
			Source:      "session",
			SessionID:   "legacy-session",
			ExtractedAt: now,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, _, err := skill.AppendExtractionCandidatesIfNew(workspace, []skill.ExtractionCandidate{legacy}); err != nil {
		t.Fatalf("seed legacy inbox: %v", err)
	}
	inboxPath := skill.ExtractionInboxPath(workspace)
	before, err := os.ReadFile(inboxPath)
	if err != nil {
		t.Fatalf("read seeded inbox: %v", err)
	}
	handler := newSkillExtractionAPIHandler(workspace, session.NewStore(workspace), nil, zerolog.New(io.Discard), nil, ledger)
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/skills/extractions?status=all", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list legacy inbox = %d: %s", rec.Code, rec.Body.String())
	}
	after, err := os.ReadFile(inboxPath)
	if err != nil {
		t.Fatalf("read migrated inbox: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("legacy inbox changed during migration\nbefore=%s\nafter=%s", before, after)
	}
	versions, err := ledger.ListCapabilityVersions(context.Background(), workstore.ListCapabilityVersionsFilter{WorkspaceID: defaultWorkspaceID})
	if err != nil {
		t.Fatalf("list migrated capability versions: %v", err)
	}
	if len(versions) != 1 || versions[0].CandidateID != legacy.ID || versions[0].State != workstore.CapabilityStateCandidate {
		t.Fatalf("migrated versions = %+v", versions)
	}
	if _, err := ledger.RecordCapabilityOutcome(context.Background(), workstore.RecordCapabilityOutcomeInput{
		WorkspaceID: versions[0].WorkspaceID, CapabilityVersionID: versions[0].ID,
		WorkID: versions[0].WorkID, IdempotencyKey: "observed-work:1",
		Status: workstore.CapabilityOutcomeSucceeded, VerifierStatus: workstore.ProofStatusPassed,
		CostUSD: 0.02, LatencyMS: 125, ActorID: "scheduler",
	}); err != nil {
		t.Fatalf("record observed capability outcome: %v", err)
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/v1/admin/skills/extractions?status=all", nil))
	var payload skillExtractionListResponse
	if err := json.Unmarshal(second.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode capability inbox response: %v", err)
	}
	if len(payload.Outcomes) != 1 || payload.Outcomes[0].CapabilityVersionID != versions[0].ID {
		t.Fatalf("capability inbox outcomes = %+v", payload.Outcomes)
	}
	versions, err = ledger.ListCapabilityVersions(context.Background(), workstore.ListCapabilityVersionsFilter{WorkspaceID: defaultWorkspaceID})
	if err != nil || len(versions) != 1 {
		t.Fatalf("idempotent migration versions = %+v err=%v", versions, err)
	}
}

func TestSkillSelfImprovementVersionsPermissionExpansionAndRestoresKnownGood(t *testing.T) {
	workspace := t.TempDir()
	ledger := openSkillLifecycleTestLedger(t, workspace)
	handler := newSkillExtractionAPIHandler(workspace, session.NewStore(workspace), nil, zerolog.New(io.Discard), nil, ledger)
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	candidates := []skill.ExtractionCandidate{
		{
			ID: "skillcand_version_1", Status: skill.ExtractionCandidateStatusPending,
			Name: "versioned-review", Title: "Versioned Review", Summary: "First known-good review flow.",
			UseCase: "run the first review flow", RecommendedTools: []string{"bash"}, SourceSession: "session-v1",
			CreatedAt: now, UpdatedAt: now,
			Provenance: skill.ExtractionProvenance{Source: "session", SessionID: "session-v1", ExtractedAt: now},
		},
		{
			ID: "skillcand_version_2", Status: skill.ExtractionCandidateStatusPending,
			Name: "versioned-review", Title: "Versioned Review", Summary: "Second review flow with memory evidence.",
			UseCase: "run the second review flow", RecommendedTools: []string{"bash", "memory_search"}, SourceSession: "session-v2",
			CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute),
			Provenance: skill.ExtractionProvenance{Source: "user_correction", SessionID: "session-v2", ExtractedAt: now.Add(time.Minute)},
		},
	}
	if _, _, err := skill.AppendExtractionCandidatesIfNew(workspace, candidates); err != nil {
		t.Fatalf("seed version candidates: %v", err)
	}
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/v1/admin/skills/extractions?status=all", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("sync candidates = %d: %s", listRec.Code, listRec.Body.String())
	}

	postSkillLifecycleAction(t, handler, candidates[0].ID, "approve")
	v1 := postSkillLifecycleAction(t, handler, candidates[0].ID, "promote")
	activePath := filepath.Join(workspace, "skills", "versioned-review", "SKILL.md")
	v1Content, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("read v1 skill: %v", err)
	}

	approvedV2 := postSkillLifecycleAction(t, handler, candidates[1].ID, "approve")
	if approvedV2.Capability.Version != 2 || approvedV2.Capability.RollbackTargetID != v1.Capability.ID {
		t.Fatalf("v2 capability = %+v, want rollback target %s", approvedV2.Capability, v1.Capability.ID)
	}
	permissionExpansionFound := false
	for _, evaluation := range approvedV2.Evaluations {
		var report struct {
			PermissionExpansion []string `json:"permission_expansion"`
		}
		if json.Unmarshal(evaluation.ReportJSON, &report) == nil {
			for _, permission := range report.PermissionExpansion {
				if permission == "tool:memory_search" {
					permissionExpansionFound = true
				}
			}
		}
	}
	if !permissionExpansionFound {
		t.Fatalf("v2 evaluations did not expose permission expansion: %+v", approvedV2.Evaluations)
	}
	postSkillLifecycleAction(t, handler, candidates[1].ID, "promote")
	v2Content, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("read v2 skill: %v", err)
	}
	if string(v2Content) == string(v1Content) || !strings.Contains(string(v2Content), "Second review flow") {
		t.Fatalf("v2 content did not replace v1:\n%s", v2Content)
	}
	postSkillLifecycleAction(t, handler, candidates[1].ID, "rollback")
	restored, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("read restored skill: %v", err)
	}
	if string(restored) != string(v1Content) {
		t.Fatalf("rollback did not restore exact known-good v1")
	}
}

type skillLifecycleTestResponse struct {
	Candidate   skill.ExtractionCandidate   `json:"candidate"`
	Capability  workstore.CapabilityVersion `json:"capability"`
	Evaluations []workstore.EvaluationRun   `json:"evaluations"`
}

func openSkillLifecycleTestLedger(t *testing.T, workspace string) *workstore.Store {
	t.Helper()
	ledger, err := workstore.Open(context.Background(), filepath.Join(workspace, "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	t.Cleanup(func() { _ = ledger.Close() })
	return ledger
}

func extractSkillLifecycleCandidate(t *testing.T, handler http.Handler, sessionID string) skill.ExtractionCandidate {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/skills/extractions/extract", strings.NewReader(`{"session_id":"`+sessionID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("extract candidate = %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Candidates []skill.ExtractionCandidate `json:"candidates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || len(response.Candidates) == 0 {
		t.Fatalf("decode extracted candidates: %v %+v", err, response.Candidates)
	}
	return response.Candidates[0]
}

func postSkillLifecycleAction(t *testing.T, handler http.Handler, candidateID, action string) skillLifecycleTestResponse {
	t.Helper()
	body, err := json.Marshal(map[string]string{"id": candidateID, "action": action})
	if err != nil {
		t.Fatalf("marshal lifecycle action: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/skills/extractions/review", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s capability = %d: %s", action, rec.Code, rec.Body.String())
	}
	var response skillLifecycleTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode %s response: %v", action, err)
	}
	return response
}
