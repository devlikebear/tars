package agentruntime

import (
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/session"
)

func TestParentSessionExecutionRoot(t *testing.T) {
	store := session.NewStore(t.TempDir())
	parent, err := store.Create("chat in a project")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// A session with no cwd of its own leaves the run in the workspace.
	if got := parentSessionExecutionRoot(store, parent.ID); got != "" {
		t.Fatalf("expected no root for a session without a cwd, got %q", got)
	}

	project := t.TempDir()
	if err := store.SetWorkDirs(parent.ID, []string{project}, project); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}
	want, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	if got := parentSessionExecutionRoot(store, parent.ID); got != want {
		t.Fatalf("expected the parent's cwd %q, got %q", want, got)
	}

	if got := parentSessionExecutionRoot(store, "missing"); got != "" {
		t.Fatalf("expected no root for an unknown session, got %q", got)
	}
	if got := parentSessionExecutionRoot(store, " "); got != "" {
		t.Fatalf("expected no root without a parent, got %q", got)
	}
}
