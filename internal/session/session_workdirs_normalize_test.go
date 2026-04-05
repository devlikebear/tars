package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func testCanonicalPath(t *testing.T, value string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		t.Fatalf("abs path %q: %v", value, err)
	}
	current := abs
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			out := filepath.Clean(resolved)
			for i := len(suffix) - 1; i >= 0; i-- {
				out = filepath.Join(out, suffix[i])
			}
			return filepath.Clean(out)
		}
		if !os.IsNotExist(err) {
			return abs
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func TestStoreCreate_NormalizesArtifactWorkDirToAbsolutePath(t *testing.T) {
	rootParent := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(rootParent); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	store := NewStore("workspace")
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	artifactDir := testCanonicalPath(t, filepath.Join(rootParent, "workspace", "artifacts", sess.ID))
	if !reflect.DeepEqual(sess.WorkDirs, []string{artifactDir}) {
		t.Fatalf("expected absolute artifact dir [%q], got %+v", artifactDir, sess.WorkDirs)
	}
	if sess.CurrentDir != artifactDir {
		t.Fatalf("expected absolute current_dir %q, got %q", artifactDir, sess.CurrentDir)
	}
}

func TestStoreGet_NormalizesLegacyRelativeWorkDirsToAbsolutePath(t *testing.T) {
	rootParent := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(rootParent); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	store := NewStore("workspace")
	if err := os.MkdirAll(filepath.Join("workspace", "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

	const sessionID = "sess-legacy"
	legacyIndex := map[string]Session{
		sessionID: {
			ID:         sessionID,
			Title:      "legacy",
			WorkDirs:   []string{filepath.ToSlash(filepath.Join("workspace", "artifacts", sessionID))},
			CurrentDir: filepath.ToSlash(filepath.Join("workspace", "artifacts", sessionID)),
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		},
	}
	data, err := json.MarshalIndent(legacyIndex, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy index: %v", err)
	}
	if err := os.WriteFile(filepath.Join("workspace", "sessions", "sessions.json"), data, 0o644); err != nil {
		t.Fatalf("write legacy index: %v", err)
	}

	got, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get legacy session: %v", err)
	}

	artifactDir := testCanonicalPath(t, filepath.Join(rootParent, "workspace", "artifacts", sessionID))
	if !reflect.DeepEqual(got.WorkDirs, []string{artifactDir}) {
		t.Fatalf("expected normalized work_dirs [%q], got %+v", artifactDir, got.WorkDirs)
	}
	if got.CurrentDir != artifactDir {
		t.Fatalf("expected normalized current_dir %q, got %q", artifactDir, got.CurrentDir)
	}
}
