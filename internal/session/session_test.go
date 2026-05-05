package session

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestStoreCreateAndList(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	s1, err := store.Create("first session")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if s1.Title != "first session" {
		t.Fatalf("expected title 'first session', got %q", s1.Title)
	}
	if s1.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if s1.RootSessionID != s1.ID {
		t.Fatalf("expected root_session_id to default to self, got %q want %q", s1.RootSessionID, s1.ID)
	}

	s2, err := store.Create("second session")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	got, err := store.Get(s1.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != s1.ID || got.Title != s1.Title {
		t.Fatalf("unexpected session: %+v", got)
	}
	if got.RootSessionID != got.ID {
		t.Fatalf("expected fetched session root to default to self, got %+v", got)
	}

	_ = s2 // use s2
}

func TestStoreBackfillsLegacySessionLineageOnRead(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	raw := `{
  "legacy": {
    "id": "legacy",
    "title": "Legacy",
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z"
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "sessions", "sessions.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write legacy index: %v", err)
	}

	got, err := store.Get("legacy")
	if err != nil {
		t.Fatalf("get legacy: %v", err)
	}
	if got.RootSessionID != "legacy" {
		t.Fatalf("expected root_session_id backfill, got %+v", got)
	}

	listed, err := store.ListAll()
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(listed) != 1 || listed[0].RootSessionID != "legacy" {
		t.Fatalf("expected listed legacy root backfill, got %+v", listed)
	}
}

func TestStoreForkFromMessageCopiesTranscriptPrefixAndState(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	parent, err := store.Create("Parent session")
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := store.SetToolConfig(parent.ID, &SessionToolConfig{
		ToolsCustom:     true,
		ToolsEnabled:    []string{"bash"},
		SkillsCustom:    true,
		SkillsEnabled:   []string{"planner"},
		CommandsCustom:  true,
		CommandsEnabled: []string{"memo"},
		MCPEnabled:      []string{"filesystem"},
	}); err != nil {
		t.Fatalf("set tool config: %v", err)
	}
	if err := store.SetPromptOverride(parent.ID, "Keep answers terse."); err != nil {
		t.Fatalf("set prompt override: %v", err)
	}
	style := &SessionStyleControl{
		Directness: intPtr(88),
		Humor:      intPtr(12),
		Caution:    intPtr(72),
		Autonomy:   intPtr(64),
	}
	if err := store.SetStyleControl(parent.ID, style); err != nil {
		t.Fatalf("set style control: %v", err)
	}
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := store.SetWorkDirs(parent.ID, []string{projectDir}, projectDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}
	parent, err = store.Get(parent.ID)
	if err != nil {
		t.Fatalf("reload parent: %v", err)
	}
	if err := store.SaveTasks(parent.ID, SessionTasks{
		Plan:  &Plan{Goal: "Ship fork support", Status: PlanStatusExecuting},
		Tasks: []Task{{ID: "1", Title: "Add endpoint", Status: "pending"}},
	}); err != nil {
		t.Fatalf("save tasks: %v", err)
	}

	messages := []Message{
		{Role: "user", Content: "prepare workspace", Timestamp: time.Now().UTC()},
		{Role: "assistant", Content: "workspace ready", Timestamp: time.Now().UTC().Add(time.Second)},
		{Role: "user", Content: "branch from here", Timestamp: time.Now().UTC().Add(2 * time.Second)},
		{Role: "assistant", Content: "do not copy me", Timestamp: time.Now().UTC().Add(3 * time.Second)},
	}
	for _, msg := range messages {
		if err := AppendMessage(store.TranscriptPath(parent.ID), msg); err != nil {
			t.Fatalf("append message: %v", err)
		}
	}
	parentHistory, err := ReadMessages(store.TranscriptPath(parent.ID))
	if err != nil {
		t.Fatalf("read parent history: %v", err)
	}

	child, err := store.ForkFromMessage(parent.ID, parentHistory[2].ID, ForkOptions{Reason: "try alternative implementation"})
	if err != nil {
		t.Fatalf("fork from message: %v", err)
	}

	if child.ID == "" || child.ID == parent.ID {
		t.Fatalf("expected distinct child session id, got parent=%q child=%q", parent.ID, child.ID)
	}
	if child.ParentSessionID != parent.ID {
		t.Fatalf("expected parent lineage %q, got %+v", parent.ID, child)
	}
	if child.RootSessionID != parent.ID {
		t.Fatalf("expected root lineage %q, got %+v", parent.ID, child)
	}
	if child.ForkedFromMessageID != parentHistory[2].ID {
		t.Fatalf("expected fork message id %q, got %+v", parentHistory[2].ID, child)
	}
	if child.ForkedFromIndex == nil || *child.ForkedFromIndex != 2 {
		t.Fatalf("expected fork index 2, got %+v", child.ForkedFromIndex)
	}
	if child.ForkReason != "try alternative implementation" {
		t.Fatalf("expected fork reason to persist, got %q", child.ForkReason)
	}
	if child.ToolConfig == nil || !child.ToolConfig.ToolsCustom || child.ToolConfig.ToolsEnabled[0] != "bash" {
		t.Fatalf("expected tool config copy, got %+v", child.ToolConfig)
	}
	if child.PromptOverride != "Keep answers terse." {
		t.Fatalf("expected prompt override copy, got %q", child.PromptOverride)
	}
	if child.StyleControl == nil || *child.StyleControl.Directness != 88 || *child.StyleControl.Autonomy != 64 {
		t.Fatalf("expected style control copy, got %+v", child.StyleControl)
	}
	if child.CurrentDir != parent.CurrentDir {
		t.Fatalf("expected current dir copy %q, got %q", parent.CurrentDir, child.CurrentDir)
	}

	childHistory, err := ReadMessages(store.TranscriptPath(child.ID))
	if err != nil {
		t.Fatalf("read child history: %v", err)
	}
	if len(childHistory) != 3 {
		t.Fatalf("expected transcript prefix through selected message, got %d messages", len(childHistory))
	}
	for i := range childHistory {
		if childHistory[i].ID != parentHistory[i].ID || childHistory[i].Content != parentHistory[i].Content {
			t.Fatalf("unexpected child prefix at %d: got %+v want %+v", i, childHistory[i], parentHistory[i])
		}
	}

	childTasks, err := store.GetTasks(child.ID)
	if err != nil {
		t.Fatalf("read child tasks: %v", err)
	}
	if childTasks.Plan == nil || childTasks.Plan.Goal != "Ship fork support" || len(childTasks.Tasks) != 1 {
		t.Fatalf("expected copied tasks, got %+v", childTasks)
	}

	parentAfter, err := ReadMessages(store.TranscriptPath(parent.ID))
	if err != nil {
		t.Fatalf("read parent after fork: %v", err)
	}
	if len(parentAfter) != len(parentHistory) {
		t.Fatalf("expected original session unchanged, got %d messages", len(parentAfter))
	}
}

func TestDetectForkPromotionCandidatesUsesPostForkMessages(t *testing.T) {
	forkIndex := 1
	child := Session{
		ID:                  "child",
		ParentSessionID:     "parent",
		RootSessionID:       "parent",
		ForkedFromMessageID: "m2",
		ForkedFromIndex:     &forkIndex,
	}
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	messages := []Message{
		{ID: "m1", Role: "user", Content: "setup context"},
		{ID: "m2", Role: "assistant", Content: "context ready"},
		{ID: "m3", Role: "assistant", Content: "Decision: keep the git inspector read-only until approval flow lands."},
		{ID: "m4", Role: "tool", Content: "tool output should not become reusable memory"},
		{ID: "m5", Role: "user", Content: "I prefer concise release summaries with exact PR and tag links."},
		{ID: "m6", Role: "assistant", Content: "결정: 부모 transcript는 직접 수정하지 않고 Memory Inbox로 승격한다."},
	}

	candidates := DetectForkPromotionCandidates(child, messages, ForkPromotionOptions{Now: now})

	if len(candidates) != 3 {
		t.Fatalf("expected 3 promotion candidates, got %+v", candidates)
	}
	if candidates[0].MessageID != "m3" || candidates[0].MessageIndex != 2 {
		t.Fatalf("expected first candidate from post-fork assistant message, got %+v", candidates[0])
	}
	if candidates[0].Category != "decision" {
		t.Fatalf("expected decision category, got %+v", candidates[0])
	}
	if candidates[1].MessageID != "m5" || candidates[1].Category != "preference" {
		t.Fatalf("expected preference candidate from user message, got %+v", candidates[1])
	}
	if candidates[2].MessageID != "m6" || candidates[2].Category != "decision" {
		t.Fatalf("expected korean decision candidate, got %+v", candidates[2])
	}
	if candidates[0].ID == "" || candidates[0].ID == candidates[1].ID {
		t.Fatalf("expected stable distinct candidate ids, got %+v", candidates)
	}
	if !candidates[0].CreatedAt.Equal(now) {
		t.Fatalf("expected deterministic created_at, got %s", candidates[0].CreatedAt)
	}
}

func TestStoreDelete(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	s, err := store.Create("to delete")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Also create a transcript file to verify it gets cleaned up
	tPath := filepath.Join(dir, "sessions", s.ID+".jsonl")
	if err := AppendMessage(tPath, Message{Role: "user", Content: "test"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	if err := store.Delete(s.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions after delete, got %d", len(sessions))
	}

	_, err = store.Get(s.ID)
	if err == nil {
		t.Fatal("expected error getting deleted session")
	}
}

func TestStoreGetNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	_, err := store.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestStoreTouchAndLatest(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	first, err := store.Create("first")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := store.Create("second")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	now := time.Now().UTC().Add(2 * time.Minute)
	if err := store.Touch(first.ID, now); err != nil {
		t.Fatalf("touch first: %v", err)
	}

	latest, err := store.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.ID != first.ID {
		t.Fatalf("expected touched session to be latest, got %s", latest.ID)
	}

	if err := store.Touch(second.ID, now.Add(1*time.Minute)); err != nil {
		t.Fatalf("touch second: %v", err)
	}
	latest, err = store.Latest()
	if err != nil {
		t.Fatalf("latest second: %v", err)
	}
	if latest.ID != second.ID {
		t.Fatalf("expected second session to be latest after touch, got %s", latest.ID)
	}
}

func TestStoreLatestNoSessionsReturnsError(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.Latest(); err == nil {
		t.Fatalf("expected error when no sessions exist")
	}
}

func TestStoreEnsureMain_ReusesStableMainSession(t *testing.T) {
	store := NewStore(t.TempDir())
	first, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main first: %v", err)
	}
	second, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected stable main session, got %q and %q", first.ID, second.ID)
	}
	if first.Kind != "main" || first.Hidden {
		t.Fatalf("unexpected main session metadata: %+v", first)
	}
}

func TestStoreEnsureMain_FindsMainAfterTitleChange(t *testing.T) {
	store := NewStore(t.TempDir())
	first, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	// Simulate auto-title renaming the main session
	if err := store.SetTitle(first.ID, "user's first question here"); err != nil {
		t.Fatalf("set title: %v", err)
	}
	// EnsureMain should still find the same session by kind, not title
	second, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main after rename: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same main session after rename, got %q and %q", first.ID, second.ID)
	}
}

func TestStoreEnsureWorker_HidesWorkerSessionFromDefaultList(t *testing.T) {
	store := NewStore(t.TempDir())
	worker, err := store.EnsureWorker("proj_demo")
	if err != nil {
		t.Fatalf("ensure worker: %v", err)
	}
	if worker.Kind != "worker" || !worker.Hidden {
		t.Fatalf("unexpected worker session metadata: %+v", worker)
	}

	visible, err := store.List()
	if err != nil {
		t.Fatalf("list visible: %v", err)
	}
	if len(visible) != 0 {
		t.Fatalf("expected hidden worker excluded from visible list, got %+v", visible)
	}

	all, err := store.ListAll()
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 1 || all[0].ID != worker.ID {
		t.Fatalf("expected hidden worker in full list, got %+v", all)
	}
}

func TestStoreEnsureWorker_ReusesProjectWorkerSession(t *testing.T) {
	store := NewStore(t.TempDir())
	first, err := store.EnsureWorker("proj_demo")
	if err != nil {
		t.Fatalf("ensure worker first: %v", err)
	}
	second, err := store.EnsureWorker("proj_demo")
	if err != nil {
		t.Fatalf("ensure worker second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected stable worker session, got %q and %q", first.ID, second.ID)
	}
}

func TestStoreCreate_InitializesArtifactWorkDir(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	artifactDir := testCanonicalPath(t, filepath.Join(root, "artifacts", sess.ID))
	if !reflect.DeepEqual(sess.WorkDirs, []string{artifactDir}) {
		t.Fatalf("expected default work_dirs [%q], got %+v", artifactDir, sess.WorkDirs)
	}
	if sess.CurrentDir != artifactDir {
		t.Fatalf("expected current_dir %q, got %q", artifactDir, sess.CurrentDir)
	}
	if _, err := os.Stat(artifactDir); err != nil {
		t.Fatalf("expected artifact dir to exist: %v", err)
	}
}

func TestStoreSetWorkDirs_PreservesMandatoryArtifactDirFirst(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	extraDir := testCanonicalPath(t, filepath.Join(root, "games", "2d-survivors"))
	if err := os.MkdirAll(extraDir, 0o755); err != nil {
		t.Fatalf("mkdir extra dir: %v", err)
	}

	if err := store.SetWorkDirs(sess.ID, []string{extraDir}, extraDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	got, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	artifactDir := testCanonicalPath(t, filepath.Join(root, "artifacts", sess.ID))
	wantDirs := []string{artifactDir, extraDir}
	if !reflect.DeepEqual(got.WorkDirs, wantDirs) {
		t.Fatalf("expected work_dirs %+v, got %+v", wantDirs, got.WorkDirs)
	}
	if got.CurrentDir != extraDir {
		t.Fatalf("expected current_dir %q, got %q", extraDir, got.CurrentDir)
	}

	if err := store.SetWorkDirs(sess.ID, []string{}, ""); err != nil {
		t.Fatalf("reset work dirs: %v", err)
	}

	got, err = store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get session after reset: %v", err)
	}
	if !reflect.DeepEqual(got.WorkDirs, []string{artifactDir}) {
		t.Fatalf("expected mandatory artifact dir to remain, got %+v", got.WorkDirs)
	}
	if got.CurrentDir != artifactDir {
		t.Fatalf("expected current_dir to fall back to %q, got %q", artifactDir, got.CurrentDir)
	}
}

func TestStoreSetToolConfig_RoundTripsGroupFields(t *testing.T) {
	store := NewStore(t.TempDir())
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	config := &SessionToolConfig{
		ToolsCustom:      true,
		ToolsEnabled:     []string{"read_file", "list_dir"},
		ToolsDisabled:    []string{"exec"},
		ToolsAllowGroups: []string{"files", "web"},
		ToolsDenyGroups:  []string{"shell"},
		CommandsCustom:   true,
		CommandsEnabled:  []string{"memo"},
	}
	if err := store.SetToolConfig(sess.ID, config); err != nil {
		t.Fatalf("set tool config: %v", err)
	}

	reloaded, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if !reflect.DeepEqual(reloaded.ToolConfig, config) {
		t.Fatalf("unexpected tool config: got=%+v want=%+v", reloaded.ToolConfig, config)
	}
}

func TestStoreSetAutomationConsent_RoundTripsConservativeDefaults(t *testing.T) {
	store := NewStore(t.TempDir())
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	initial, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if initial.AutomationConsent != nil && initial.AutomationConsent.AllowsAutonomousMutation() {
		t.Fatalf("expected conservative automation consent defaults, got %+v", initial.AutomationConsent)
	}

	consent := &SessionAutomationConsent{
		AutoResume:          true,
		GitMutations:        true,
		AutonomousMutations: false,
	}
	if err := store.SetAutomationConsent(sess.ID, consent); err != nil {
		t.Fatalf("set automation consent: %v", err)
	}

	reloaded, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if reloaded.AutomationConsent == nil {
		t.Fatalf("expected automation consent to persist")
	}
	if !reloaded.AutomationConsent.AutoResume || !reloaded.AutomationConsent.GitMutations {
		t.Fatalf("expected consent toggles to persist, got %+v", reloaded.AutomationConsent)
	}
	if reloaded.AutomationConsent.AutonomousMutations {
		t.Fatalf("expected autonomous mutations to remain disabled, got %+v", reloaded.AutomationConsent)
	}
	if reloaded.AutomationConsent.UpdatedAt.IsZero() {
		t.Fatalf("expected updated_at to be set")
	}
}

func TestStoreSetAutomationConsent_NormalizesAutoResumePolicy(t *testing.T) {
	store := NewStore(t.TempDir())
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	consent := &SessionAutomationConsent{
		AutoResumeEnabled:      true,
		AutoResumeAfterMinutes: -5,
		AllowedResumeModes: []string{
			"bad-mode",
			AutoResumeModeMoveToNextTask,
			AutoResumeModeMoveToNextTask,
			AutoResumeModeRecordAssumptionAndProceed,
		},
	}
	if err := store.SetAutomationConsent(sess.ID, consent); err != nil {
		t.Fatalf("set automation consent: %v", err)
	}

	reloaded, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	got := reloaded.AutomationConsent
	if got == nil {
		t.Fatalf("expected automation consent")
	}
	if !got.AutoResume || !got.AutoResumeEnabled {
		t.Fatalf("expected legacy and explicit auto-resume flags to be synchronized, got %+v", got)
	}
	if got.AutoResumeAfterMinutes != DefaultAutoResumeAfterMinutes {
		t.Fatalf("after minutes = %d, want %d", got.AutoResumeAfterMinutes, DefaultAutoResumeAfterMinutes)
	}
	wantModes := []string{AutoResumeModeMoveToNextTask, AutoResumeModeRecordAssumptionAndProceed}
	if !reflect.DeepEqual(got.AllowedResumeModes, wantModes) {
		t.Fatalf("allowed modes = %+v, want %+v", got.AllowedResumeModes, wantModes)
	}
}

func TestStoreSetStyleControl_NormalizesSliderScores(t *testing.T) {
	store := NewStore(t.TempDir())
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	style := &SessionStyleControl{
		Directness: intPtr(125),
		Humor:      intPtr(-20),
		Caution:    intPtr(45),
		Autonomy:   intPtr(99),
	}
	if err := store.SetStyleControl(sess.ID, style); err != nil {
		t.Fatalf("set style control: %v", err)
	}

	reloaded, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	got := reloaded.StyleControl
	if got == nil {
		t.Fatalf("expected style control")
	}
	if *got.Directness != 100 || *got.Humor != 0 || *got.Caution != 45 || *got.Autonomy != 99 {
		t.Fatalf("unexpected normalized style control: %+v", got)
	}
	if got.UpdatedAt == nil || got.UpdatedAt.IsZero() {
		t.Fatalf("expected updated_at to be set")
	}
}

func TestStoreGet_LegacyToolConfigWithoutGroupFieldsStillLoads(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	createdAt := time.Now().UTC().Add(-time.Hour)
	updatedAt := createdAt.Add(10 * time.Minute)
	legacyID := "sess_legacy"

	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	data := []byte(`{
	  "sess_legacy": {
	    "id": "sess_legacy",
	    "title": "Legacy",
	    "tool_config": {
	      "tools_custom": true,
	      "tools_enabled": ["read_file"],
	      "tools_disabled": ["exec"]
	    },
	    "created_at": "` + createdAt.Format(time.RFC3339) + `",
	    "updated_at": "` + updatedAt.Format(time.RFC3339) + `"
	  }
	}`)
	if err := os.WriteFile(filepath.Join(root, "sessions", "sessions.json"), data, 0o644); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	sess, err := store.Get(legacyID)
	if err != nil {
		t.Fatalf("get legacy session: %v", err)
	}
	if sess.ToolConfig == nil {
		t.Fatalf("expected tool config to load")
	}
	if sess.ToolConfig.ToolsAllowGroups != nil {
		t.Fatalf("expected missing allow groups to remain nil, got %+v", sess.ToolConfig.ToolsAllowGroups)
	}
	if sess.ToolConfig.ToolsDenyGroups != nil {
		t.Fatalf("expected missing deny groups to remain nil, got %+v", sess.ToolConfig.ToolsDenyGroups)
	}
}

func TestStoreGet_MigratesLegacyNestedArtifactDir(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	legacyDir := filepath.Join(root, "workspace", "artifacts", sess.ID)
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	legacyFile := filepath.Join(legacyDir, "report.md")
	if err := os.WriteFile(legacyFile, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	if _, err := store.Get(sess.ID); err != nil {
		t.Fatalf("get session: %v", err)
	}

	migratedFile := filepath.Join(root, "artifacts", sess.ID, "report.md")
	if _, err := os.Stat(migratedFile); err != nil {
		t.Fatalf("expected migrated file at %s: %v", migratedFile, err)
	}
	if _, err := os.Stat(legacyFile); !os.IsNotExist(err) {
		t.Fatalf("expected legacy file to be removed, stat err=%v", err)
	}
}
