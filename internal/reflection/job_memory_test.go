package reflection

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
)

type fakeSessionSource struct {
	sessions []session.Session
	dir      string
	err      error
}

func (f *fakeSessionSource) ListAll() ([]session.Session, error) {
	return f.sessions, f.err
}

func (f *fakeSessionSource) TranscriptPath(id string) string {
	return filepath.Join(f.dir, id+".jsonl")
}

func writeTranscript(t *testing.T, dir, id string, msgs []session.Message) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, id+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, m := range msgs {
		if err := enc.Encode(m); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
}

type fakeLLM struct {
	chatCount int
	response  string
	err       error
}

func (f *fakeLLM) Ask(ctx context.Context, prompt string) (string, error) { return "", nil }
func (f *fakeLLM) Chat(ctx context.Context, msgs []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	f.chatCount++
	if f.err != nil {
		return llm.ChatResponse{}, f.err
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: f.response}}, nil
}

func newTestWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := memory.EnsureWorkspace(dir); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	return dir
}

// --- pairTurns ---

func TestPairTurnsBasic(t *testing.T) {
	msgs := []session.Message{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
	}
	turns := pairTurns(msgs)
	if len(turns) != 2 {
		t.Fatalf("want 2 turns, got %d", len(turns))
	}
	if turns[0].UserMessage != "q1" || turns[0].AssistantMessage != "a1" {
		t.Errorf("turn 0: %+v", turns[0])
	}
}

func TestPairTurnsIgnoresToolMessages(t *testing.T) {
	msgs := []session.Message{
		{Role: "user", Content: "q"},
		{Role: "tool", Content: "intermediate"},
		{Role: "assistant", Content: "a"},
	}
	turns := pairTurns(msgs)
	if len(turns) != 1 || turns[0].UserMessage != "q" || turns[0].AssistantMessage != "a" {
		t.Errorf("turns = %+v", turns)
	}
}

func TestPairTurnsDropsOrphanUser(t *testing.T) {
	msgs := []session.Message{
		{Role: "user", Content: "q1"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
	}
	turns := pairTurns(msgs)
	if len(turns) != 1 || turns[0].UserMessage != "q2" {
		t.Errorf("turns = %+v", turns)
	}
}

// --- MemoryJob end-to-end ---

func TestMemoryJobExtractsExperiences(t *testing.T) {
	workspace := newTestWorkspace(t)
	transcriptDir := filepath.Join(workspace, "sessions")
	writeTranscript(t, transcriptDir, "sess1", []session.Message{
		{Role: "user", Content: "I prefer dark mode for terminals"},
		{Role: "assistant", Content: "Noted."},
		{Role: "user", Content: "Please implement the feature"},
		{Role: "assistant", Content: "Task completed."},
	})

	src := &fakeSessionSource{
		dir: transcriptDir,
		sessions: []session.Session{
			{ID: "sess1", UpdatedAt: time.Now()},
		},
	}
	job := &MemoryJob{
		WorkspaceDir: workspace,
		Sessions:     src,
		Now:          func() time.Time { return time.Now() },
	}
	result, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !result.Success {
		t.Fatalf("success=false: %+v", result)
	}
	if !result.Changed {
		t.Errorf("expected Changed=true")
	}
	expAdded := result.Details["experiences_added"].(int)
	if expAdded < 2 {
		t.Errorf("experiences_added = %d, want >= 2", expAdded)
	}
}

func TestMemoryJobSkipsOldSessions(t *testing.T) {
	workspace := newTestWorkspace(t)
	transcriptDir := filepath.Join(workspace, "sessions")
	writeTranscript(t, transcriptDir, "old", []session.Message{
		{Role: "user", Content: "I prefer dark mode"},
		{Role: "assistant", Content: "ok"},
	})

	src := &fakeSessionSource{
		dir: transcriptDir,
		sessions: []session.Session{
			{ID: "old", UpdatedAt: time.Now().Add(-48 * time.Hour)},
		},
	}
	job := &MemoryJob{
		WorkspaceDir: workspace,
		Sessions:     src,
		Lookback:     24 * time.Hour,
		Now:          func() time.Time { return time.Now() },
	}
	result, _ := job.Run(context.Background())
	scanned := result.Details["sessions_scanned"].(int)
	if scanned != 0 {
		t.Errorf("sessions_scanned = %d, want 0", scanned)
	}
}

func TestMemoryJobCapsTurnsPerSession(t *testing.T) {
	workspace := newTestWorkspace(t)
	transcriptDir := filepath.Join(workspace, "sessions")
	// 10 turns of identical preference text — only the first cap should be processed.
	var msgs []session.Message
	for i := 0; i < 10; i++ {
		msgs = append(msgs,
			session.Message{Role: "user", Content: "I prefer setup " + string(rune('A'+i))},
			session.Message{Role: "assistant", Content: "ok"},
		)
	}
	writeTranscript(t, transcriptDir, "chatty", msgs)

	src := &fakeSessionSource{
		dir:      transcriptDir,
		sessions: []session.Session{{ID: "chatty", UpdatedAt: time.Now()}},
	}
	job := &MemoryJob{
		WorkspaceDir:       workspace,
		Sessions:           src,
		MaxTurnsPerSession: 3,
		Now:                func() time.Time { return time.Now() },
	}
	result, _ := job.Run(context.Background())
	processed := result.Details["turns_processed"].(int)
	if processed != 3 {
		t.Errorf("turns_processed = %d, want 3", processed)
	}
}

func TestMemoryJobNilSessionSource(t *testing.T) {
	job := &MemoryJob{WorkspaceDir: t.TempDir()}
	result, _ := job.Run(context.Background())
	if result.Success {
		t.Error("nil session source should fail")
	}
}

func TestMemoryJobSessionListError(t *testing.T) {
	job := &MemoryJob{
		WorkspaceDir: t.TempDir(),
		Sessions:     &fakeSessionSource{err: errors.New("io boom")},
	}
	_, err := job.Run(context.Background())
	if err == nil {
		t.Error("expected list error to propagate")
	}
}

func TestMemoryJobLLMKnowledgeCompile(t *testing.T) {
	workspace := newTestWorkspace(t)
	transcriptDir := filepath.Join(workspace, "sessions")
	writeTranscript(t, transcriptDir, "s", []session.Message{
		{Role: "user", Content: "Our workflow policy: always use fixtures for tests"},
		{Role: "assistant", Content: "Understood."},
	})

	fakeResp := `{"notes":[{"slug":"test-fixtures","title":"Use fixtures for tests","kind":"workflow","summary":"always use fixtures"}],"edges":[]}`
	fakeClient := &fakeLLM{response: fakeResp}

	src := &fakeSessionSource{
		dir:      transcriptDir,
		sessions: []session.Session{{ID: "s", UpdatedAt: time.Now()}},
	}
	job := &MemoryJob{
		WorkspaceDir: workspace,
		Sessions:     src,
		LLMClient:    fakeClient,
		Now:          func() time.Time { return time.Now() },
	}
	result, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if fakeClient.chatCount != 1 {
		t.Errorf("llm chat called %d times, want 1", fakeClient.chatCount)
	}
	kb := result.Details["kb_updates"].(int)
	if kb != 1 {
		t.Errorf("kb_updates = %d, want 1", kb)
	}
}

// --- derivation unit tests ---

func TestDeriveUserExperiencePreference(t *testing.T) {
	_, ok := deriveUserExperience("s1", "I prefer tabs over spaces", time.Now())
	if !ok {
		t.Error("should detect preference")
	}
}

func TestDeriveAssistantExperienceCompletion(t *testing.T) {
	_, ok := deriveAssistantExperience("s1", "Task completed successfully", time.Now())
	if !ok {
		t.Error("should detect completion")
	}
}

func TestDeriveAssistantExperienceResolved(t *testing.T) {
	_, ok := deriveAssistantExperience("s1", "Issue resolved", time.Now())
	if !ok {
		t.Error("should detect resolution")
	}
}

func TestShouldCompileKnowledgeHints(t *testing.T) {
	if !shouldCompileKnowledge(turn{UserMessage: "Our policy is X", AssistantMessage: "ok"}) {
		t.Error("policy hint should trigger")
	}
	if shouldCompileKnowledge(turn{UserMessage: "random small talk", AssistantMessage: "hi"}) {
		t.Error("small talk should not trigger")
	}
}

func TestTrimText(t *testing.T) {
	if got := trimText("  hello\nworld  ", 0); got != "hello world" {
		t.Errorf("got %q", got)
	}
	if got := trimText("abcdefghij", 5); got != "abcde..." {
		t.Errorf("got %q", got)
	}
}

func TestAppendExperienceDedupes(t *testing.T) {
	workspace := newTestWorkspace(t)
	exp := memory.Experience{
		Timestamp:     time.Now().UTC(),
		Category:      "preference",
		Summary:       "I prefer dark mode",
		Tags:          []string{"auto"},
		SourceSession: "s1",
		Importance:    6,
		Auto:          true,
	}
	if !appendExperienceIfNew(workspace, exp) {
		t.Error("first append should succeed")
	}
	if appendExperienceIfNew(workspace, exp) {
		t.Error("duplicate append should return false")
	}
}
