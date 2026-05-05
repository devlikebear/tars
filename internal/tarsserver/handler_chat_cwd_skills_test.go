package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestChatContext_AugmentsSnapshotWithCwdSkill(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Drop a SKILL.md under the active cwd's `.tars/skills/`. With no
	// global extensions manager, this skill is the only one the chat
	// context preview should see.
	cwd := sess.WorkDirs[0]
	skillsDir := filepath.Join(cwd, ".tars", "skills", "cwd-only")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(`---
name: cwd-only
description: only present in cwd
slash: cwd-only
user_invocable: true
---

cwd-only body
`), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	handler := newChatAPIHandlerWithRuntimeConfig(
		root, store, &mockLLMClient{}, nil, zerolog.Nop(), 4, nil, "", chatToolingOptions{},
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/context?session_id="+sess.ID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		SkillNames   []string `json:"skill_names"`
		SystemPrompt string   `json:"system_prompt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, n := range payload.SkillNames {
		if n == "cwd-only" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected cwd-only skill in payload, got %+v", payload.SkillNames)
	}
	if !strings.Contains(payload.SystemPrompt, "cwd-only") {
		t.Fatalf("expected cwd-only to appear in system prompt, got: %q", payload.SystemPrompt)
	}
}

func TestChatContext_CommandsAliasResolvesAgainstSnapshotSkill(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// First, drop the target skill in cwd .tars/skills/.
	cwd := sess.WorkDirs[0]
	skillsDir := filepath.Join(cwd, ".tars", "skills", "refactor")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(`---
name: refactor
description: refactor target
slash: refactor
user_invocable: true
---

refactor body
`), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	// Then drop a commands/ alias targeting it.
	commandsDir := filepath.Join(cwd, ".tars", "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(commandsDir, "tidy.md"), []byte(`---
target_skill: refactor
description: tidy alias
---
`), 0o600); err != nil {
		t.Fatalf("write alias: %v", err)
	}

	handler := newChatAPIHandlerWithRuntimeConfig(
		root, store, &mockLLMClient{}, nil, zerolog.Nop(), 4, nil, "", chatToolingOptions{},
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/context?session_id="+sess.ID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		SkillNames []string `json:"skill_names"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	gotRefactor := false
	gotTidy := false
	for _, n := range payload.SkillNames {
		switch n {
		case "refactor":
			gotRefactor = true
		case "tidy":
			gotTidy = true
		}
	}
	if !gotRefactor || !gotTidy {
		t.Fatalf("expected both refactor + tidy alias in skill list, got %+v", payload.SkillNames)
	}
}
