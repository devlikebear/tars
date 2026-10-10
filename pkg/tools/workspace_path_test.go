package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// tempRoot returns t.TempDir() canonicalized through EvalSymlinks so assertions
// can compare against the same path resolveWorkspaceRoot would produce
// (macOS' /var/folders symlinks into /private/var/folders).
func tempRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("eval symlinks for temp dir: %v", err)
	}
	return resolved
}

// These tests pin down the path-injection trust boundary the workspace file
// tools rely on. They double as the documented sanitizer evidence referenced
// by the CodeQL go/path-injection dismissals in #804 — every read/list/edit/
// write/project-skill site routes its raw input through one of
// resolvePathWithPolicy or resolveWritePathWithPolicy before touching os.*.

func TestResolveWorkspacePath_RejectsDotDotTraversal(t *testing.T) {
	root := tempRoot(t)
	cases := []string{
		"../etc/passwd",
		"../../etc/passwd",
		"sub/../../escape",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			_, err := resolveWorkspacePath(root, raw)
			if err == nil {
				t.Fatalf("expected rejection for %q", raw)
			}
			if !strings.Contains(err.Error(), "escapes workspace") {
				t.Fatalf("expected escape error for %q, got %v", raw, err)
			}
		})
	}
}

func TestResolveWorkspacePath_RejectsAbsoluteOutsideWorkspace(t *testing.T) {
	root := tempRoot(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("hi"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := resolveWorkspacePath(root, outside); err == nil {
		t.Fatalf("expected rejection for absolute path outside workspace")
	}
}

func TestResolveWorkspacePath_RejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	root := tempRoot(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("classified"), 0o644); err != nil {
		t.Fatalf("seed outside: %v", err)
	}
	link := filepath.Join(root, "leak")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, err := resolveWorkspacePath(root, "leak")
	if err == nil {
		t.Fatalf("expected symlink-escape rejection")
	}
	if !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("expected escape error, got %v", err)
	}
}

func TestResolveWorkspaceWritePath_RejectsSymlinkAncestorEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	root := tempRoot(t)
	outside := t.TempDir()
	link := filepath.Join(root, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	// `outside/new.txt` does not exist yet, exercising the parent-walk in
	// resolveWorkspaceWritePath.
	_, err := resolveWorkspaceWritePath(root, "outside/new.txt")
	if err == nil {
		t.Fatalf("expected write rejection through symlinked ancestor")
	}
	if !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("expected escape error, got %v", err)
	}
}

func TestResolveWorkspaceWritePath_AllowsMissingChildPath(t *testing.T) {
	root := tempRoot(t)
	resolved, err := resolveWorkspaceWritePath(root, "subdir/new.txt")
	if err != nil {
		t.Fatalf("expected missing child path to be allowed inside workspace: %v", err)
	}
	if !strings.HasPrefix(resolved, root) {
		t.Fatalf("resolved path %q should live under workspace %q", resolved, root)
	}
}

func TestResolvePathWithPolicy_AbsoluteWithinAllowedDir(t *testing.T) {
	primary := tempRoot(t)
	extra := tempRoot(t)
	policy := NewPathPolicy(primary, []string{extra}, "")

	target := filepath.Join(extra, "ok.txt")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	resolved, err := resolvePathWithPolicy(policy, target)
	if err != nil {
		t.Fatalf("expected absolute path in allowed dir to resolve: %v", err)
	}
	if !strings.HasPrefix(resolved, extra) {
		t.Fatalf("resolved %q should be under %q", resolved, extra)
	}
}

func TestResolvePathWithPolicy_RejectsAbsoluteOutsideAllAllowedDirs(t *testing.T) {
	primary := tempRoot(t)
	extra := tempRoot(t)
	policy := NewPathPolicy(primary, []string{extra}, "")

	outside := filepath.Join(t.TempDir(), "leak.txt")
	if err := os.WriteFile(outside, []byte("hi"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := resolvePathWithPolicy(policy, outside)
	if err == nil {
		t.Fatalf("expected rejection for absolute path outside every allowed dir")
	}
	if !strings.Contains(err.Error(), "outside allowed directories") {
		t.Fatalf("expected outside-allowed error, got %v", err)
	}
}

func TestResolvePathWithPolicy_WorkspaceRootPrefixedRelative(t *testing.T) {
	// When the relative path begins with the workspace directory's basename
	// (e.g. user says "workspace/notes.md"), resolveWorkspaceRootPrefixedRelative
	// strips the prefix so the resolution still anchors inside the workspace.
	parent := tempRoot(t)
	root := filepath.Join(parent, "workspace")
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "todo.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	policy := NewPathPolicy(root, nil, "")

	resolved, err := resolvePathWithPolicy(policy, "workspace/notes/todo.md")
	if err != nil {
		t.Fatalf("expected workspace-prefixed relative path to resolve: %v", err)
	}
	if !strings.HasPrefix(resolved, root) {
		t.Fatalf("resolved %q should anchor under %q", resolved, root)
	}
}

func TestResolvePathWithPolicy_RejectsTraversalRelativeToCurrentDir(t *testing.T) {
	workspace := t.TempDir()
	currentDir := t.TempDir()
	policy := NewPathPolicy(workspace, nil, currentDir)

	_, err := resolvePathWithPolicy(policy, "../escape.txt")
	if err == nil {
		t.Fatalf("expected traversal from current dir to be rejected")
	}
	if !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("expected escape error, got %v", err)
	}
}

func TestResolveWritePathWithPolicy_AllowsMissingChildInsideAllowedDir(t *testing.T) {
	primary := tempRoot(t)
	extra := tempRoot(t)
	policy := NewPathPolicy(primary, []string{extra}, "")

	target := filepath.Join(extra, "nested", "child.txt")
	resolved, err := resolveWritePathWithPolicy(policy, target)
	if err != nil {
		t.Fatalf("expected missing nested write target to be allowed: %v", err)
	}
	if !strings.HasPrefix(resolved, extra) {
		t.Fatalf("resolved %q should anchor under %q", resolved, extra)
	}
}

func TestResolveWritePathWithPolicy_RejectsEmptyPath(t *testing.T) {
	policy := SingleDirPolicy(t.TempDir())
	_, err := resolveWritePathWithPolicy(policy, "")
	if err == nil {
		t.Fatalf("expected write to require a path")
	}
}

func TestPolicyRelativePath_HandlesPathsInsideAndOutsidePrimary(t *testing.T) {
	primary := tempRoot(t)
	extra := tempRoot(t)
	policy := NewPathPolicy(primary, []string{extra}, "")

	insidePath := filepath.Join(primary, "notes", "todo.md")
	got := policyRelativePath(policy, insidePath)
	want := filepath.ToSlash(filepath.Join("notes", "todo.md"))
	if got != want {
		t.Fatalf("inside primary: got %q want %q", got, want)
	}

	outsidePath := filepath.Join(extra, "log.txt")
	if got := policyRelativePath(policy, outsidePath); got != outsidePath {
		t.Fatalf("outside primary should remain absolute: got %q want %q", got, outsidePath)
	}
}

// An absolute path is held to the same rule as a relative one: a symlink
// inside an allowed directory must not lead out of every allowed directory.
func TestResolvePathWithPolicy_RejectsAbsoluteSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	root := tempRoot(t)
	outsideDir := tempRoot(t)
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("classified"), 0o644); err != nil {
		t.Fatalf("seed outside: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, "leak")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	policy := SingleDirPolicy(root)

	// The relative form was already refused.
	if _, err := resolvePathWithPolicy(policy, filepath.Join("leak", "secret.txt")); err == nil {
		t.Fatal("relative path through the symlink was accepted")
	}
	got, err := resolvePathWithPolicy(policy, filepath.Join(root, "leak", "secret.txt"))
	if err == nil {
		t.Fatalf("absolute path through the symlink resolved to %s", got)
	}
}

// A symlink from one allowed directory into another stays inside the policy.
func TestResolvePathWithPolicy_AllowsAbsoluteSymlinkBetweenAllowedDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	root := tempRoot(t)
	other := tempRoot(t)
	target := filepath.Join(other, "notes.txt")
	if err := os.WriteFile(target, []byte("notes"), 0o644); err != nil {
		t.Fatalf("seed other: %v", err)
	}
	if err := os.Symlink(other, filepath.Join(root, "shared")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	policy := NewPathPolicy(root, []string{other}, "")

	got, err := resolvePathWithPolicy(policy, filepath.Join(root, "shared", "notes.txt"))
	if err != nil {
		t.Fatalf("symlink into another allowed directory was refused: %v", err)
	}
	if got != target {
		t.Fatalf("resolved = %s, want %s", got, target)
	}
}
