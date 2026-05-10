package tarsserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
