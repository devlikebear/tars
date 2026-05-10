package tarsserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/rs/zerolog"
)

func TestSkillCreatorAPI_DraftAndSaveLocal(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	handler := newSkillCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil, nil)

	reqBody := map[string]any{
		"name":              "docker-log-shipper",
		"description":       "Collect recent Docker ERROR logs and send them to Slack.",
		"category":          "system",
		"language":          "python",
		"layout":            "single_file",
		"use_case":          "Docker 컨테이너의 최근 1시간 ERROR 로그를 추출해 슬랙으로 보낸다",
		"recommended_tools": []string{"bash"},
	}
	draftRec := postJSON(t, handler, "/v1/admin/skills/draft", reqBody)
	if draftRec.Code != http.StatusOK {
		t.Fatalf("expected draft 200, got %d body=%q", draftRec.Code, draftRec.Body.String())
	}

	var draft skillCreatorDraftResponse
	if err := json.Unmarshal(draftRec.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	if draft.Name != "docker-log-shipper" {
		t.Fatalf("unexpected draft name: %q", draft.Name)
	}
	if !skillCreatorContainsString(draft.RecommendedTools, "bash") {
		t.Fatalf("expected bash in recommended tools, got %+v", draft.RecommendedTools)
	}
	if !draftContainsFile(draft.Files, "SKILL.md", "recommended_tools: [bash]") {
		t.Fatalf("expected SKILL.md with recommended tools, files=%+v", draft.Files)
	}
	if !draftContainsFile(draft.Files, "docker-log-shipper.py", "argparse") {
		t.Fatalf("expected python companion CLI stub, files=%+v", draft.Files)
	}

	saveRec := postJSON(t, handler, "/v1/admin/skills/save-local", draft)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("expected save 200, got %d body=%q", saveRec.Code, saveRec.Body.String())
	}
	var saved skillCreatorSaveResponse
	if err := json.Unmarshal(saveRec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode save response: %v", err)
	}
	if !saved.Saved || saved.Path == "" || len(saved.Files) != 2 {
		t.Fatalf("unexpected save response: %+v", saved)
	}

	skillPath := filepath.Join(workspaceDir, "skills", "docker-log-shipper", "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read saved skill: %v", err)
	}
	if !strings.Contains(string(data), "Collect recent Docker ERROR logs") {
		t.Fatalf("saved SKILL.md missing description: %s", string(data))
	}
	cliPath := filepath.Join(workspaceDir, "skills", "docker-log-shipper", "docker-log-shipper.py")
	if _, err := os.Stat(cliPath); err != nil {
		t.Fatalf("expected companion CLI file: %v", err)
	}
}

func TestSkillCreatorAPI_RejectsUnsafeNamesAndPaths(t *testing.T) {
	handler := newSkillCreatorAPIHandler(filepath.Join(t.TempDir(), "workspace"), zerolog.New(ioDiscard{}), nil, nil)

	badDraft := postJSON(t, handler, "/v1/admin/skills/draft", map[string]any{
		"name":        "../escape",
		"description": "bad",
		"language":    "shell",
		"layout":      "single_file",
		"use_case":    "bad",
	})
	if badDraft.Code != http.StatusBadRequest {
		t.Fatalf("expected unsafe name to fail, got %d body=%q", badDraft.Code, badDraft.Body.String())
	}

	badSave := postJSON(t, handler, "/v1/admin/skills/save-local", skillCreatorDraftResponse{
		Name: "safe-skill",
		Files: []skillCreatorFile{
			{Path: "SKILL.md", Content: "# Safe"},
			{Path: "../escape.sh", Content: "echo bad"},
		},
	})
	if badSave.Code != http.StatusBadRequest {
		t.Fatalf("expected unsafe file path to fail, got %d body=%q", badSave.Code, badSave.Body.String())
	}
}

func TestSkillCreatorAPI_TestRunsCompanionCLIInSandbox(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	handler := newSkillCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil, nil)

	draft := skillCreatorDraftResponse{
		Name:        "probe-skill",
		Description: "Probe generated skill.",
		Language:    "shell",
		Layout:      "single_file",
		UseCase:     "hello sandbox",
		Files: []skillCreatorFile{
			{Path: "SKILL.md", Content: "---\nname: probe-skill\nrecommended_tools: [bash]\n---\n# Probe\n"},
			{Path: "probe-skill.sh", Content: "#!/usr/bin/env bash\nset -euo pipefail\necho \"skill-test:$*\"\n"},
		},
	}
	rec := postJSON(t, handler, "/v1/admin/skills/test", draft)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected test 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var result skillCreatorTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode test response: %v", err)
	}
	if !result.Success || result.ExitCode != 0 {
		t.Fatalf("expected successful test result, got %+v", result)
	}
	if !strings.Contains(result.Stdout, "skill-test:hello sandbox") {
		t.Fatalf("expected CLI stdout to include use case, got %q", result.Stdout)
	}
	if result.SessionKind != "worker" || !result.Hidden {
		t.Fatalf("expected hidden worker sandbox metadata, got %+v", result)
	}
	if len(result.ToolTrail) == 0 || result.ToolTrail[0].Tool != "bash" {
		t.Fatalf("expected bash tool trail, got %+v", result.ToolTrail)
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, "skills", "probe-skill")); !os.IsNotExist(err) {
		t.Fatalf("sandbox test should not write into workspace/skills, err=%v", err)
	}
}

func TestSkillCreatorAPI_SaveLocalTriggersReload(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	provider := &mockExtensionsProvider{}
	handler := newSkillCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil, provider)

	draft := skillCreatorDraftResponse{
		Name:        "reload-test-skill",
		Description: "Test reload trigger",
		Language:    "shell",
		Layout:      "single_file",
		UseCase:     "test",
		Files: []skillCreatorFile{
			{Path: "SKILL.md", Content: "---\nname: reload-test-skill\ndescription: Test reload trigger\n---\n# Test\n"},
			{Path: "reload-test-skill.sh", Content: "#!/bin/bash\necho test\n"},
		},
	}
	rec := postJSON(t, handler, "/v1/admin/skills/save-local", draft)
	if rec.Code != http.StatusOK {
		t.Fatalf("save-local expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if provider.reloadCount == 0 {
		t.Fatal("expected provider.Reload() to be called after save-local")
	}
}

func TestSkillCreatorAPI_WorkspaceCRUD_WithReload(t *testing.T) {
	workspaceDir := t.TempDir()
	provider := &mockExtensionsProvider{}
	handler := newSkillCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil, provider)

	const skillName = "reload-crud-skill"
	skillDir := filepath.Join(workspaceDir, "skills", skillName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// PUT triggers reload
	provider.reloadCount = 0
	putBody, _ := json.Marshal(map[string]string{"content": "---\nname: reload-crud-skill\n---\n# Updated\n"})
	putReq := httptest.NewRequest(http.MethodPut, "/v1/admin/skills/"+skillName, bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d", putRec.Code)
	}
	if provider.reloadCount == 0 {
		t.Fatal("expected reload after PUT")
	}

	// DELETE triggers reload
	provider.reloadCount = 0
	delReq := httptest.NewRequest(http.MethodDelete, "/v1/admin/skills/"+skillName, nil)
	delRec := httptest.NewRecorder()
	handler.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE expected 200, got %d", delRec.Code)
	}
	if provider.reloadCount == 0 {
		t.Fatal("expected reload after DELETE")
	}
}

func TestSkillCreatorAPI_WorkspaceCRUD(t *testing.T) {
	workspaceDir := t.TempDir()
	handler := newSkillCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil, nil)

	const skillName = "my-test-skill"
	skillDir := filepath.Join(workspaceDir, "skills", skillName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("setup skill dir: %v", err)
	}
	initialContent := "---\nname: my-test-skill\ndescription: Test skill\n---\n# Test\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(initialContent), 0o644); err != nil {
		t.Fatalf("setup skill file: %v", err)
	}

	// GET — fetch content
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/skills/"+skillName, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var getResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if getResp["name"] != skillName {
		t.Fatalf("expected name=%q, got %q", skillName, getResp["name"])
	}
	if !strings.Contains(getResp["content"], "Test skill") {
		t.Fatalf("expected content to contain 'Test skill', got %q", getResp["content"])
	}

	// PUT — update content
	updatedContent := "---\nname: my-test-skill\ndescription: Updated skill\n---\n# Updated\n"
	putBody, _ := json.Marshal(map[string]string{"content": updatedContent})
	putReq := httptest.NewRequest(http.MethodPut, "/v1/admin/skills/"+skillName, bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d body=%q", putRec.Code, putRec.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read updated skill: %v", err)
	}
	if !strings.Contains(string(data), "Updated skill") {
		t.Fatalf("expected updated content, got %q", string(data))
	}

	// DELETE — remove skill directory
	delReq := httptest.NewRequest(http.MethodDelete, "/v1/admin/skills/"+skillName, nil)
	delRec := httptest.NewRecorder()
	handler.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE expected 200, got %d body=%q", delRec.Code, delRec.Body.String())
	}
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Fatalf("expected skill directory to be removed")
	}

	// GET after delete → 404
	req404 := httptest.NewRequest(http.MethodGet, "/v1/admin/skills/"+skillName, nil)
	rec404 := httptest.NewRecorder()
	handler.ServeHTTP(rec404, req404)
	if rec404.Code != http.StatusNotFound {
		t.Fatalf("GET after delete expected 404, got %d", rec404.Code)
	}

	// DELETE non-existent → 404
	delReq2 := httptest.NewRequest(http.MethodDelete, "/v1/admin/skills/"+skillName, nil)
	delRec2 := httptest.NewRecorder()
	handler.ServeHTTP(delRec2, delReq2)
	if delRec2.Code != http.StatusNotFound {
		t.Fatalf("DELETE non-existent expected 404, got %d", delRec2.Code)
	}
}

func TestSkillCreatorAPI_WorkspaceCRUD_EmptyName(t *testing.T) {
	handler := newSkillCreatorAPIHandler(t.TempDir(), zerolog.New(ioDiscard{}), nil, nil)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/v1/admin/skills/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s with empty name expected 400, got %d", method, rec.Code)
		}
	}
}

func TestSkillCreatorAPI_WorkspaceCRUD_MethodNotAllowed(t *testing.T) {
	workspaceDir := t.TempDir()
	handler := newSkillCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil, nil)

	const skillName = "method-check-skill"
	skillDir := filepath.Join(workspaceDir, "skills", skillName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/v1/admin/skills/"+skillName, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PATCH expected 405, got %d", rec.Code)
	}
}

func TestSkillCreatorAPI_WorkspaceCRUD_ReloadErrorIsWarningOnly(t *testing.T) {
	workspaceDir := t.TempDir()
	provider := &mockExtensionsProvider{reloadErr: fmt.Errorf("simulated reload failure")}
	handler := newSkillCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil, provider)

	const skillName = "reload-warn-skill"
	skillDir := filepath.Join(workspaceDir, "skills", skillName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// PUT with failing reload — should still return 200
	putBody, _ := json.Marshal(map[string]string{"content": "---\nname: reload-warn-skill\n---\n# Warn\n"})
	putReq := httptest.NewRequest(http.MethodPut, "/v1/admin/skills/"+skillName, bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT with failing reload expected 200, got %d", putRec.Code)
	}

	// DELETE with failing reload — should still return 200
	delReq := httptest.NewRequest(http.MethodDelete, "/v1/admin/skills/"+skillName, nil)
	delRec := httptest.NewRecorder()
	handler.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE with failing reload expected 200, got %d", delRec.Code)
	}
}

func TestSkillCreatorAPI_SaveLocalReloadErrorIsWarningOnly(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	provider := &mockExtensionsProvider{reloadErr: fmt.Errorf("simulated reload failure")}
	handler := newSkillCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil, provider)

	draft := skillCreatorDraftResponse{
		Name:        "reload-warn-save",
		Description: "Test",
		Language:    "shell",
		Layout:      "single_file",
		UseCase:     "test",
		Files: []skillCreatorFile{
			{Path: "SKILL.md", Content: "---\nname: reload-warn-save\ndescription: Test\n---\n# Test\n"},
			{Path: "reload-warn-save.sh", Content: "#!/bin/bash\necho test\n"},
		},
	}
	rec := postJSON(t, handler, "/v1/admin/skills/save-local", draft)
	if rec.Code != http.StatusOK {
		t.Fatalf("save-local with failing reload expected 200, got %d", rec.Code)
	}
}

func TestSkillCreatorAPI_WorkspaceCRUD_PutMkdirAllFailure(t *testing.T) {
	workspaceDir := t.TempDir()
	handler := newSkillCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil, nil)

	const skillName = "mkdirall-fail-skill"
	// Create a FILE at the skillDir path to block os.MkdirAll
	skillsDir := filepath.Join(workspaceDir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, skillName), []byte("I am a file"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	putBody, _ := json.Marshal(map[string]string{"content": "# Test"})
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/skills/"+skillName, bytes.NewReader(putBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("PUT expected 500 when skillDir is a file, got %d", rec.Code)
	}
}

func TestSkillCreatorAPI_WorkspaceCRUD_PutWriteFileFailure(t *testing.T) {
	workspaceDir := t.TempDir()
	handler := newSkillCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil, nil)

	const skillName = "writefile-fail-skill"
	// Create SKILL.md as a directory (not a file) to cause os.WriteFile to fail
	skillMDAsDir := filepath.Join(workspaceDir, "skills", skillName, "SKILL.md")
	if err := os.MkdirAll(skillMDAsDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	putBody, _ := json.Marshal(map[string]string{"content": "# Test"})
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/skills/"+skillName, bytes.NewReader(putBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("PUT expected 500 when SKILL.md is a directory, got %d", rec.Code)
	}
}

func TestSkillCreatorAPI_WorkspaceCRUD_InvalidName(t *testing.T) {
	handler := newSkillCreatorAPIHandler(t.TempDir(), zerolog.New(ioDiscard{}), nil, nil)

	// Names with uppercase or underscore fail validateSkillCreatorName.
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/v1/admin/skills/Invalid_Name", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s with invalid name expected 400, got %d", method, rec.Code)
		}
	}
}

func TestSkillCreatorAPI_WorkspaceCRUD_PutEmptyContent(t *testing.T) {
	workspaceDir := t.TempDir()
	handler := newSkillCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil, nil)

	putBody, _ := json.Marshal(map[string]string{"content": "   "})
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/skills/some-skill", bytes.NewReader(putBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT with empty content expected 400, got %d body=%q", rec.Code, rec.Body.String())
	}
}

// TestSkillCreatorAPI_WorkspaceCRUD_RenamedSkill covers the case where the
// SKILL.md frontmatter `name` was changed but the source directory under
// workspace/skills/ still has the original name. The admin UI lists the
// skill by its frontmatter name and calls /v1/admin/skills/<frontmatter-name>;
// the handler must resolve to the actual on-disk file via the snapshot.
func TestSkillCreatorAPI_WorkspaceCRUD_RenamedSkill(t *testing.T) {
	workspaceDir := t.TempDir()
	const sourceDirName = "claude-code-cli"
	const newName = "claude-code-cli2"

	skillDir := filepath.Join(workspaceDir, "skills", sourceDirName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("setup skill dir: %v", err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	original := "---\nname: " + newName + "\ndescription: Renamed\n---\n# Body\n"
	if err := os.WriteFile(skillFile, []byte(original), 0o644); err != nil {
		t.Fatalf("setup skill file: %v", err)
	}

	provider := &mockExtensionsProvider{
		snapshot: extensions.Snapshot{
			Skills: []skill.Definition{
				// Different name — must be skipped (covers the name-mismatch continue).
				{Name: "other-skill", Source: skill.SourceWorkspace, FilePath: filepath.Join(workspaceDir, "skills", "other-skill", "SKILL.md")},
				// Same name but bundled — must be skipped (covers the source-mismatch continue).
				{Name: newName, Source: skill.SourceBundled, FilePath: filepath.Join(workspaceDir, "skills", "ignored", "SKILL.md")},
				// Same name, workspace, but blank FilePath — must be skipped (covers the empty-path continue).
				{Name: newName, Source: skill.SourceWorkspace, FilePath: ""},
				// The real entry — its on-disk path must win.
				{Name: newName, Source: skill.SourceWorkspace, FilePath: skillFile},
			},
		},
	}
	handler := newSkillCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil, provider)

	// GET via the new (frontmatter) name must succeed and return the source file's content.
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/skills/"+newName, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var getResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if getResp["path"] != skillFile {
		t.Fatalf("expected path=%q, got %q", skillFile, getResp["path"])
	}
	if !strings.Contains(getResp["content"], "name: "+newName) {
		t.Fatalf("expected content to come from source dir, got %q", getResp["content"])
	}

	// PUT via the new name must update the original source file (not create a new directory).
	updated := "---\nname: " + newName + "\ndescription: Updated body\n---\n# After update\n"
	putBody, _ := json.Marshal(map[string]string{"content": updated})
	putReq := httptest.NewRequest(http.MethodPut, "/v1/admin/skills/"+newName, bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d body=%q", putRec.Code, putRec.Body.String())
	}
	got, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}
	if !strings.Contains(string(got), "Updated body") {
		t.Fatalf("expected source file to be updated, got %q", string(got))
	}
	// A new directory under the new name must not have been created.
	if _, err := os.Stat(filepath.Join(workspaceDir, "skills", newName)); !os.IsNotExist(err) {
		t.Fatalf("expected no directory at workspace/skills/%s, got err=%v", newName, err)
	}
}

// TestResolveWorkspaceSkillPaths_RejectsTraversal ensures a malicious or
// stale snapshot pointing outside <workspace>/skills/ falls back to the
// legacy <workspace>/skills/<name>/ layout instead of operating on the
// out-of-tree path.
func TestResolveWorkspaceSkillPaths_RejectsTraversal(t *testing.T) {
	workspaceDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "evil", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatalf("setup outside dir: %v", err)
	}
	if err := os.WriteFile(outside, []byte("# Evil\n"), 0o644); err != nil {
		t.Fatalf("setup outside file: %v", err)
	}

	provider := &mockExtensionsProvider{
		snapshot: extensions.Snapshot{
			Skills: []skill.Definition{{
				Name:     "evil",
				Source:   skill.SourceWorkspace,
				FilePath: outside,
			}},
		},
	}
	dir, file := resolveWorkspaceSkillPaths(provider, workspaceDir, "evil")
	wantDir := filepath.Join(workspaceDir, "skills", "evil")
	wantFile := filepath.Join(wantDir, "SKILL.md")
	if dir != wantDir || file != wantFile {
		t.Fatalf("expected fallback to %q / %q, got %q / %q", wantDir, wantFile, dir, file)
	}
}

func postJSON(t *testing.T, handler http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func draftContainsFile(files []skillCreatorFile, path string, want string) bool {
	for _, file := range files {
		if file.Path == path && strings.Contains(file.Content, want) {
			return true
		}
	}
	return false
}

func skillCreatorContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
