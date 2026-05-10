package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/rs/zerolog"
)

func newLocalSkillsTestHandler(t *testing.T, root string, provider *mockExtensionsProvider) (*session.Store, http.Handler) {
	t.Helper()
	store := session.NewStore(root)
	deps := localSkillsHandlerDeps{provider: provider, workspaceDir: root}
	handler := newSessionAPIHandlerFullWithLocalSkills(
		store,
		zerolog.New(io.Discard),
		nil,
		sessionStyleValues{},
		nil,
		nil,
		nil,
		deps,
	)
	return store, handler
}

func writeSessionLocalSkill(t *testing.T, cwd, name, body string) {
	t.Helper()
	dir := filepath.Join(cwd, ".tars", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	frontmatter := "---\nname: " + name + "\ndescription: " + name + " skill\nslash: " + name + "\nuser_invocable: true\n---\n\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(frontmatter+body), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func writeWorkspaceSkill(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write workspace skill: %v", err)
	}
}

func writeSessionLocalCommand(t *testing.T, cwd, name, body string) {
	t.Helper()
	dir := filepath.Join(cwd, ".tars", "commands")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write command: %v", err)
	}
}

func ensureSessionWithCwd(t *testing.T, store *session.Store, root string) (sessionID, cwd string) {
	t.Helper()
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	cwd = filepath.Join(root, "projects", "alpha")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{cwd}, cwd); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}
	return sess.ID, cwd
}

func adminGet(handler http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func adminPostJSON(handler http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestLocalSkillsList_ReturnsItemsAndCollisions(t *testing.T) {
	root := t.TempDir()
	store, handler := newLocalSkillsTestHandler(t, root, &mockExtensionsProvider{})
	sessionID, cwd := ensureSessionWithCwd(t, store, root)

	writeSessionLocalSkill(t, cwd, "alpha", "body")
	writeSessionLocalSkill(t, cwd, "beta", "body")
	// Commands under .tars/commands/ are intentionally not surfaced by
	// the v1 inbox API — confirm one is created but not returned.
	writeSessionLocalCommand(t, cwd, "deploy", "command body")
	writeWorkspaceSkill(t, root, "alpha", "existing")

	rec := adminGet(handler, "/v1/admin/sessions/"+sessionID+"/local-skills")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp localSkillListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasSuffix(resp.Cwd, filepath.Join("projects", "alpha")) {
		t.Fatalf("expected cwd ending with projects/alpha, got %q", resp.Cwd)
	}
	if resp.Counts.Skills != 2 || resp.Counts.Commands != 0 {
		t.Fatalf("unexpected counts: %+v", resp.Counts)
	}
	for _, item := range resp.Items {
		if item.Kind == "command" {
			t.Fatalf("commands should not be surfaced in v1 inbox: %+v", item)
		}
	}
	var alpha, beta *localSkillItem
	for i := range resp.Items {
		switch resp.Items[i].Name {
		case "alpha":
			alpha = &resp.Items[i]
		case "beta":
			beta = &resp.Items[i]
		}
	}
	if alpha == nil || !alpha.HasWorkspaceCollision {
		t.Fatalf("alpha should have workspace collision, got %+v", alpha)
	}
	if beta == nil || beta.HasWorkspaceCollision {
		t.Fatalf("beta should not have workspace collision, got %+v", beta)
	}
}

func TestLocalSkillsList_NoCwdReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	store, handler := newLocalSkillsTestHandler(t, root, &mockExtensionsProvider{})
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	rec := adminGet(handler, "/v1/admin/sessions/"+sess.ID+"/local-skills")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp localSkillListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Fatalf("expected empty list, got %+v", resp.Items)
	}
}

func TestLocalSkillsPromote_CopyCreatesAndReloadsOnce(t *testing.T) {
	root := t.TempDir()
	provider := &mockExtensionsProvider{}
	store, handler := newLocalSkillsTestHandler(t, root, provider)
	sessionID, cwd := ensureSessionWithCwd(t, store, root)
	writeSessionLocalSkill(t, cwd, "alpha", "body")
	writeSessionLocalSkill(t, cwd, "beta", "body")

	rec := adminPostJSON(handler, "/v1/admin/sessions/"+sessionID+"/local-skills/promote",
		`{"items":[{"name":"alpha"},{"name":"beta"}],"mode":"copy","on_conflict":"rename"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp localSkillPromoteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Promoted) != 2 || len(resp.Failed) != 0 {
		t.Fatalf("unexpected outcomes: %+v", resp)
	}
	for _, p := range resp.Promoted {
		if p.Action != skill.PromoteActionCreated {
			t.Fatalf("expected created, got %s", p.Action)
		}
		if p.SourceDeleted {
			t.Fatalf("copy mode should not delete source: %+v", p)
		}
	}
	if provider.reloadCount != 1 {
		t.Fatalf("expected single reload, got %d", provider.reloadCount)
	}
	for _, name := range []string{"alpha", "beta"} {
		if _, err := os.Stat(filepath.Join(root, "skills", name, "SKILL.md")); err != nil {
			t.Fatalf("workspace skill %s missing: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(cwd, ".tars", "skills", name, "SKILL.md")); err != nil {
			t.Fatalf("session skill %s should still exist: %v", name, err)
		}
	}
}

func TestLocalSkillsPromote_MoveDeletesSource(t *testing.T) {
	root := t.TempDir()
	store, handler := newLocalSkillsTestHandler(t, root, &mockExtensionsProvider{})
	sessionID, cwd := ensureSessionWithCwd(t, store, root)
	writeSessionLocalSkill(t, cwd, "gamma", "body")

	rec := adminPostJSON(handler, "/v1/admin/sessions/"+sessionID+"/local-skills/promote",
		`{"items":[{"name":"gamma"}],"mode":"move","on_conflict":"rename"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp localSkillPromoteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Promoted) != 1 || !resp.Promoted[0].SourceDeleted {
		t.Fatalf("expected source deleted: %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".tars", "skills", "gamma")); !os.IsNotExist(err) {
		t.Fatalf("session skill dir should be removed, err=%v", err)
	}
}

func TestLocalSkillsPromote_RenameOnCollision(t *testing.T) {
	root := t.TempDir()
	store, handler := newLocalSkillsTestHandler(t, root, &mockExtensionsProvider{})
	sessionID, cwd := ensureSessionWithCwd(t, store, root)
	writeSessionLocalSkill(t, cwd, "delta", "fresh body")
	writeWorkspaceSkill(t, root, "delta", "existing")

	rec := adminPostJSON(handler, "/v1/admin/sessions/"+sessionID+"/local-skills/promote",
		`{"items":[{"name":"delta"}],"mode":"copy","on_conflict":"rename"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp localSkillPromoteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Promoted) != 1 || resp.Promoted[0].TargetName != "delta-2" || resp.Promoted[0].Action != skill.PromoteActionRenamed {
		t.Fatalf("expected delta-2 renamed: %+v", resp)
	}
	if existing, err := os.ReadFile(filepath.Join(root, "skills", "delta", "SKILL.md")); err != nil || string(existing) != "existing" {
		t.Fatalf("original delta should be untouched: body=%q err=%v", string(existing), err)
	}
}

func TestLocalSkillsPromote_AbortReturnsFailedItem(t *testing.T) {
	root := t.TempDir()
	store, handler := newLocalSkillsTestHandler(t, root, &mockExtensionsProvider{})
	sessionID, cwd := ensureSessionWithCwd(t, store, root)
	writeSessionLocalSkill(t, cwd, "epsilon", "fresh")
	writeWorkspaceSkill(t, root, "epsilon", "existing")

	rec := adminPostJSON(handler, "/v1/admin/sessions/"+sessionID+"/local-skills/promote",
		`{"items":[{"name":"epsilon"}],"mode":"copy","on_conflict":"abort"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when nothing promoted, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp localSkillPromoteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Promoted) != 0 || len(resp.Failed) != 1 {
		t.Fatalf("expected only failed: %+v", resp)
	}
	if !strings.Contains(resp.Failed[0].Error, "already exists") {
		t.Fatalf("expected conflict error, got %q", resp.Failed[0].Error)
	}
}

func TestLocalSkillsPromote_OverwriteReplaces(t *testing.T) {
	root := t.TempDir()
	store, handler := newLocalSkillsTestHandler(t, root, &mockExtensionsProvider{})
	sessionID, cwd := ensureSessionWithCwd(t, store, root)
	writeSessionLocalSkill(t, cwd, "zeta", "fresh body")
	writeWorkspaceSkill(t, root, "zeta", "old body")

	rec := adminPostJSON(handler, "/v1/admin/sessions/"+sessionID+"/local-skills/promote",
		`{"items":[{"name":"zeta"}],"mode":"copy","on_conflict":"overwrite"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp localSkillPromoteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Promoted) != 1 || resp.Promoted[0].Action != skill.PromoteActionOverwritten {
		t.Fatalf("expected overwritten: %+v", resp)
	}
	body, err := os.ReadFile(filepath.Join(root, "skills", "zeta", "SKILL.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(body), "fresh body") {
		t.Fatalf("expected fresh body, got %q", string(body))
	}
}

func TestLocalSkillsPromote_NoCwdReturnsBadRequest(t *testing.T) {
	root := t.TempDir()
	store, handler := newLocalSkillsTestHandler(t, root, &mockExtensionsProvider{})
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	rec := adminPostJSON(handler, "/v1/admin/sessions/"+sess.ID+"/local-skills/promote",
		`{"items":[{"name":"alpha"}],"mode":"copy"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLocalSkillsPromote_ValidatesPayload(t *testing.T) {
	root := t.TempDir()
	store, handler := newLocalSkillsTestHandler(t, root, &mockExtensionsProvider{})
	sessionID, _ := ensureSessionWithCwd(t, store, root)

	cases := []struct {
		name string
		body string
		want string
	}{
		{"no items", `{"items":[],"mode":"copy"}`, "items is required"},
		{"bad mode", `{"items":[{"name":"x"}],"mode":"yeet"}`, "mode must be"},
		{"bad conflict", `{"items":[{"name":"x"}],"mode":"copy","on_conflict":"panic"}`, "on_conflict must be"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := adminPostJSON(handler, "/v1/admin/sessions/"+sessionID+"/local-skills/promote", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("expected error containing %q, got %s", tc.want, rec.Body.String())
			}
		})
	}
}

func TestLocalSkillsPromote_PartialFailureReportsItem(t *testing.T) {
	root := t.TempDir()
	provider := &mockExtensionsProvider{}
	store, handler := newLocalSkillsTestHandler(t, root, provider)
	sessionID, cwd := ensureSessionWithCwd(t, store, root)
	writeSessionLocalSkill(t, cwd, "ok", "body")
	// "missing" is requested but doesn't exist on disk.

	rec := adminPostJSON(handler, "/v1/admin/sessions/"+sessionID+"/local-skills/promote",
		`{"items":[{"name":"ok"},{"name":"missing"},{"name":""}],"mode":"copy","on_conflict":"rename"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (partial success), got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp localSkillPromoteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Promoted) != 1 || resp.Promoted[0].TargetName != "ok" {
		t.Fatalf("expected only ok promoted: %+v", resp.Promoted)
	}
	if len(resp.Failed) != 2 {
		t.Fatalf("expected 2 failures (missing + empty name), got %+v", resp.Failed)
	}
	if provider.reloadCount != 1 {
		t.Fatalf("expected 1 reload after partial success, got %d", provider.reloadCount)
	}
}

func TestLocalSkillsPromote_RejectsMissingSession(t *testing.T) {
	root := t.TempDir()
	_, handler := newLocalSkillsTestHandler(t, root, &mockExtensionsProvider{})

	rec := adminPostJSON(handler, "/v1/admin/sessions/nonexistent/local-skills/promote",
		`{"items":[{"name":"x"}],"mode":"copy"}`)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 4xx, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLocalSkillsPromote_RejectsMalformedJSON(t *testing.T) {
	root := t.TempDir()
	store, handler := newLocalSkillsTestHandler(t, root, &mockExtensionsProvider{})
	sessionID, _ := ensureSessionWithCwd(t, store, root)

	rec := adminPostJSON(handler, "/v1/admin/sessions/"+sessionID+"/local-skills/promote", `{not-json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLocalSkillsList_RejectsWrongMethod(t *testing.T) {
	root := t.TempDir()
	store, handler := newLocalSkillsTestHandler(t, root, &mockExtensionsProvider{})
	sessionID, _ := ensureSessionWithCwd(t, store, root)

	rec := adminPostJSON(handler, "/v1/admin/sessions/"+sessionID+"/local-skills", `{}`)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 on POST to list endpoint, got %d", rec.Code)
	}
}

func TestLocalSkillsDispatch_UnknownSubpathReturnsNotFound(t *testing.T) {
	root := t.TempDir()
	store, handler := newLocalSkillsTestHandler(t, root, &mockExtensionsProvider{})
	sessionID, _ := ensureSessionWithCwd(t, store, root)

	rec := adminGet(handler, "/v1/admin/sessions/"+sessionID+"/local-skills/garbage")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
