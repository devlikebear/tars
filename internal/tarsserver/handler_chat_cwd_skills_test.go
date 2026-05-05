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
	"github.com/devlikebear/tars/internal/skill"
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

	// Then drop a commands/ prompt. It should stay out of the skill list.
	commandsDir := filepath.Join(cwd, ".tars", "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(commandsDir, "tidy.md"), []byte(`---
target_skill: refactor
description: tidy command
---
Tidy only the files mentioned by the user.
`), 0o600); err != nil {
		t.Fatalf("write command: %v", err)
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
		CommandNames []string `json:"command_names"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	gotRefactor := false
	for _, n := range payload.SkillNames {
		switch n {
		case "refactor":
			gotRefactor = true
		case "tidy":
			t.Fatalf("did not expect command tidy in skill list: %+v", payload.SkillNames)
		}
	}
	gotTidyCommand := false
	for _, n := range payload.CommandNames {
		if n == "tidy" {
			gotTidyCommand = true
		}
	}
	if !gotRefactor || !gotTidyCommand {
		t.Fatalf("expected refactor skill + tidy command, got skills=%+v commands=%+v", payload.SkillNames, payload.CommandNames)
	}
}

func TestChatToolsEndpointIncludesSessionCwdSkills(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	skillsDir := filepath.Join(sess.CurrentDir, ".tars", "skills", "cwd-only")
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
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/tools?session_id="+sess.ID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		Skills []string `json:"skills"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, name := range payload.Skills {
		if name == "cwd-only" {
			return
		}
	}
	t.Fatalf("expected cwd-only in /v1/chat/tools skills, got %+v", payload.Skills)
}

func TestChatToolsEndpointIncludesSessionCwdStandaloneCommands(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	commandsDir := filepath.Join(sess.CurrentDir, ".tars", "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		t.Fatalf("mkdir commands: %v", err)
	}
	if err := os.WriteFile(filepath.Join(commandsDir, "메모.md"), []byte(`---
description: save a memory note
recommended_tools:
  - memory
---

# /메모

Save the text after the command.
`), 0o600); err != nil {
		t.Fatalf("write command: %v", err)
	}

	handler := newChatAPIHandlerWithRuntimeConfig(
		root, store, &mockLLMClient{}, nil, zerolog.Nop(), 4, nil, "", chatToolingOptions{},
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/tools?session_id="+sess.ID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		Skills   []string           `json:"skills"`
		Commands []skill.Definition `json:"commands"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, name := range payload.Skills {
		if name == "메모" {
			t.Fatalf("did not expect 메모 command in /v1/chat/tools skills: %+v", payload.Skills)
		}
	}
	for _, command := range payload.Commands {
		if command.Name == "메모" {
			return
		}
	}
	t.Fatalf("expected 메모 in /v1/chat/tools commands, got %+v", payload.Commands)
}
