package agentruntime

import (
	"context"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

// newRouterTestRun registers a running hidden-worker run whose summary
// note would go to the main session.
func newRouterTestRun(t *testing.T) (*Runtime, *session.Store, session.Session, *runState) {
	t.Helper()
	store := session.NewStore(t.TempDir())
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	worker, err := store.EnsureWorker("proj_router")
	if err != nil {
		t.Fatalf("ensure worker: %v", err)
	}
	fixedNow := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	rt := NewRuntime(RuntimeOptions{Enabled: true, SessionStore: store, Now: func() time.Time { return fixedNow }})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })
	state := &runState{
		run: Run{
			ID: "run_router", Status: RunStatusRunning, SessionID: worker.ID, Agent: "researcher",
			WorkspaceID: DefaultWorkspaceID, CreatedAt: fixedNow.Format(time.RFC3339), UpdatedAt: fixedNow.Format(time.RFC3339),
		},
		done: make(chan struct{}),
	}
	rt.runs[state.run.ID] = state
	rt.runOrder = append(rt.runOrder, state.run.ID)
	return rt, store, mainSession, state
}

func mainNoteCount(t *testing.T, store *session.Store, mainID string) int {
	t.Helper()
	msgs, err := session.ReadMessages(store.TranscriptPath(mainID))
	if err != nil {
		t.Fatalf("read main transcript: %v", err)
	}
	return len(msgs)
}

func TestResultRouterTakesUnawaitedRunAndSkipsNote(t *testing.T) {
	rt, store, mainSession, state := newRouterTestRun(t)
	var seen []FinishedRun
	rt.SetResultRouter(func(f FinishedRun) bool {
		seen = append(seen, f)
		return true
	})
	rt.mu.Lock()
	rt.finalizeRunLocked(state, "found three sources", PromptExecutionMetadata{}, nil, nil)
	rt.mu.Unlock()

	if len(seen) != 1 {
		t.Fatalf("router calls = %d, want 1", len(seen))
	}
	got := seen[0]
	if got.Awaited || got.NoteSessionID != mainSession.ID || got.Note == "" || got.Run.Status != RunStatusCompleted {
		t.Fatalf("finished run = %+v", got)
	}
	if n := mainNoteCount(t, store, mainSession.ID); n != 0 {
		t.Fatalf("main notes = %d, want 0 once the router took the result", n)
	}
}

func TestResultRouterDecliningKeepsNote(t *testing.T) {
	rt, store, mainSession, state := newRouterTestRun(t)
	rt.SetResultRouter(func(FinishedRun) bool { return false })
	rt.mu.Lock()
	rt.finalizeRunLocked(state, "", PromptExecutionMetadata{}, context.Canceled, nil)
	rt.mu.Unlock()
	if n := mainNoteCount(t, store, mainSession.ID); n != 1 {
		t.Fatalf("main notes = %d, want the note kept", n)
	}
}

func TestResultRouterSeesAwaitedRun(t *testing.T) {
	rt, _, _, state := newRouterTestRun(t)
	seen := make(chan FinishedRun, 1)
	rt.SetResultRouter(func(f FinishedRun) bool {
		seen <- f
		return false
	})
	waited := make(chan struct{})
	go func() {
		defer close(waited)
		_, _ = rt.Wait(context.Background(), state.run.ID)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		rt.mu.RLock()
		n := state.waiters
		rt.mu.RUnlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Wait never registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	rt.mu.Lock()
	rt.finalizeRunLocked(state, "done", PromptExecutionMetadata{}, nil, nil)
	rt.mu.Unlock()
	<-waited
	if f := <-seen; !f.Awaited {
		t.Fatalf("finished run = %+v, want Awaited", f)
	}
}

func TestWaitTimeoutUnregistersWaiter(t *testing.T) {
	rt, _, _, state := newRouterTestRun(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := rt.Wait(ctx, state.run.ID); err == nil {
		t.Fatal("Wait on a cancelled context returned nil error")
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	if state.waiters != 0 {
		t.Fatalf("waiters = %d after a cancelled Wait", state.waiters)
	}
}
