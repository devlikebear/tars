package sessiontranscripts_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/pkg/memory"
	"github.com/devlikebear/tars/pkg/session"
	"github.com/devlikebear/tars/pkg/tools"
	"github.com/devlikebear/tars/pkg/tools/sessiontranscripts"
)

// The adapter is the only place pkg/tools and the session store meet, so the
// contract is checked end to end here: a real store, a real transcript, and
// the memory search tool finding it through the adapter.
func TestAdapterLetsMemorySearchReadARealStore(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("test session")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	path := store.TranscriptPath(sess.ID)
	if err := session.AppendMessage(path, session.Message{
		Role: "user", Content: "I love cooking pasta with tomato sauce",
		Timestamp: time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	src := sessiontranscripts.New(store)

	refs, err := src.ListTranscripts()
	if err != nil || len(refs) != 1 || refs[0].ID != sess.ID {
		t.Fatalf("ListTranscripts = %+v, %v; want the one session", refs, err)
	}
	msgs, err := src.ReadTranscript(sess.ID)
	if err != nil || len(msgs) != 1 || msgs[0].Role != "user" || !strings.Contains(msgs[0].Content, "pasta") {
		t.Fatalf("ReadTranscript = %+v, %v", msgs, err)
	}

	tool := tools.NewMemorySearchToolWithTranscripts(root, memory.NewFileBackend(root, nil), src)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"pasta","include_sessions":true}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(result.Text(), "session:"+sess.ID) {
		t.Fatalf("expected a session hit through the adapter, got %s", result.Text())
	}
}

func TestNilStoreIsAnEmptySourceNotANilInterface(t *testing.T) {
	src := sessiontranscripts.New(nil)
	if src == nil {
		t.Fatal("New(nil) must return a usable source")
	}
	refs, err := src.ListTranscripts()
	if err != nil || len(refs) != 0 {
		t.Fatalf("ListTranscripts on nil store = %+v, %v; want empty, nil", refs, err)
	}
	msgs, err := src.ReadTranscript("anything")
	if err != nil || len(msgs) != 0 {
		t.Fatalf("ReadTranscript on nil store = %+v, %v; want empty, nil", msgs, err)
	}
}
