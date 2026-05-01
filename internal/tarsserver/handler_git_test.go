package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestGitAPIStatusAndDiffUseSessionCurrentDir(t *testing.T) {
	workspace := t.TempDir()
	repo := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runHandlerGit(t, repo, "init", "-b", "main")
	runHandlerGit(t, repo, "config", "user.email", "tars@example.test")
	runHandlerGit(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runHandlerGit(t, repo, "add", "README.md")
	runHandlerGit(t, repo, "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\nchanged\n"), 0o644); err != nil {
		t.Fatalf("modify readme: %v", err)
	}

	store := session.NewStore(workspace)
	sess, err := store.Create("git")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{repo}, repo); err != nil {
		t.Fatalf("set workdirs: %v", err)
	}
	handler := newGitAPIHandler(workspace, store, zerolog.New(io.Discard))

	statusReq := httptest.NewRequest(http.MethodGet, "/v1/git/status?session_id="+sess.ID, nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%q", statusRec.Code, statusRec.Body.String())
	}
	var status struct {
		IsGit  bool   `json:"is_git"`
		Root   string `json:"root"`
		Branch string `json:"branch"`
		Files  []struct {
			Path     string `json:"path"`
			Status   string `json:"status"`
			Unstaged bool   `json:"unstaged"`
		} `json:"files"`
	}
	if err := json.Unmarshal(statusRec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !status.IsGit || status.Root != canonicalHandlerGitPath(t, repo) || status.Branch != "main" {
		t.Fatalf("unexpected status payload: %+v", status)
	}
	if len(status.Files) != 1 || status.Files[0].Path != "README.md" || !status.Files[0].Unstaged || status.Files[0].Status != "modified" {
		t.Fatalf("expected README modification, got %+v", status.Files)
	}

	diffReq := httptest.NewRequest(http.MethodGet, "/v1/git/diff?session_id="+sess.ID+"&path=README.md", nil)
	diffRec := httptest.NewRecorder()
	handler.ServeHTTP(diffRec, diffReq)
	if diffRec.Code != http.StatusOK {
		t.Fatalf("expected diff 200, got %d body=%q", diffRec.Code, diffRec.Body.String())
	}
	var diff struct {
		Path  string `json:"path"`
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal(diffRec.Body.Bytes(), &diff); err != nil {
		t.Fatalf("decode diff: %v", err)
	}
	if diff.Path != "README.md" || !strings.Contains(diff.Patch, "+changed") {
		t.Fatalf("unexpected diff payload: %+v", diff)
	}

	logReq := httptest.NewRequest(http.MethodGet, "/v1/git/log?session_id="+sess.ID, nil)
	logRec := httptest.NewRecorder()
	handler.ServeHTTP(logRec, logReq)
	if logRec.Code != http.StatusOK {
		t.Fatalf("expected log 200, got %d body=%q", logRec.Code, logRec.Body.String())
	}
	var log struct {
		Commits []struct {
			Subject string `json:"subject"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(logRec.Body.Bytes(), &log); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	if len(log.Commits) != 1 || log.Commits[0].Subject != "initial" {
		t.Fatalf("unexpected log payload: %+v", log)
	}

	branchesReq := httptest.NewRequest(http.MethodGet, "/v1/git/branches?session_id="+sess.ID, nil)
	branchesRec := httptest.NewRecorder()
	handler.ServeHTTP(branchesRec, branchesReq)
	if branchesRec.Code != http.StatusOK {
		t.Fatalf("expected branches 200, got %d body=%q", branchesRec.Code, branchesRec.Body.String())
	}
	var branches struct {
		Branches []struct {
			Name    string `json:"name"`
			Current bool   `json:"current"`
		} `json:"branches"`
	}
	if err := json.Unmarshal(branchesRec.Body.Bytes(), &branches); err != nil {
		t.Fatalf("decode branches: %v", err)
	}
	if len(branches.Branches) == 0 || branches.Branches[0].Name != "main" || !branches.Branches[0].Current {
		t.Fatalf("unexpected branches payload: %+v", branches)
	}
}

func runHandlerGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func canonicalHandlerGitPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(resolved)
}
