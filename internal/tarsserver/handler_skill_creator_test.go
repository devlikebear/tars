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
	handler := newSkillCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil)

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
	handler := newSkillCreatorAPIHandler(filepath.Join(t.TempDir(), "workspace"), zerolog.New(ioDiscard{}), nil)

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
