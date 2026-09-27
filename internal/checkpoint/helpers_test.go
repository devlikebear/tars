package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/git"
)

func requireGit(t *testing.T) string {
	t.Helper()
	exe, err := git.Executable()
	if err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	return exe
}

func newTestStore(t *testing.T, opts Options) *Store {
	t.Helper()
	requireGit(t)
	s, err := Open(filepath.Join(t.TempDir(), "checkpoints"), opts)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return s
}

// userGit runs git in a test repository the way a user's own git would,
// with an isolated config so the developer's settings cannot leak in.
func userGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(requireGit(t), args...)
	cmd.Dir = dir
	cmd.Env = userGitEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func userGitEnv(t *testing.T) []string {
	t.Helper()
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(kv), "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+global,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.test",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.test",
	)
}

func initUserRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	userGit(t, repo, "init", "-q", "-b", "main")
	return repo
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fingerprint hashes every file under dir, keyed by relative path.
func fingerprint(t *testing.T, dir string) map[string]string {
	t.Helper()
	fp := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		sum := sha256.Sum256(data)
		fp[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("fingerprint %s: %v", dir, err)
	}
	return fp
}

// runTurn records one turn around edit.
func runTurn(t *testing.T, s *Store, sessionID, turnID, cwd string, edit func()) Entry {
	t.Helper()
	ctx := context.Background()
	turn, err := s.BeginTurn(ctx, sessionID, turnID, cwd, "turn "+turnID)
	if err != nil {
		t.Fatalf("begin %s: %v", turnID, err)
	}
	if edit != nil {
		edit()
	}
	entry, err := turn.End(ctx)
	if err != nil {
		t.Fatalf("end %s: %v", turnID, err)
	}
	return entry
}

func mustDiff(t *testing.T, s *Store, sessionID, turnID string, scope Scope) DiffResult {
	t.Helper()
	result, err := s.Diff(context.Background(), sessionID, turnID, scope, "")
	if err != nil {
		t.Fatalf("diff %s %s: %v", turnID, scope, err)
	}
	return result
}

func filesByPath(files []FileDiff) map[string]FileDiff {
	m := map[string]FileDiff{}
	for _, f := range files {
		m[f.Path] = f
	}
	return m
}

func shadowBlob(t *testing.T, s *Store, e Entry, commit, path string) []byte {
	t.Helper()
	sh, err := s.openShadow(e.Shadow)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := s.git.run(context.Background(), sh.call("cat-file", "blob", commit+":"+path))
	if err != nil {
		t.Fatalf("cat-file %s: %v", path, err)
	}
	return out
}

func shadowRefs(t *testing.T, s *Store, e Entry) []string {
	t.Helper()
	sh, err := s.openShadow(e.Shadow)
	if err != nil {
		return nil
	}
	refs, err := s.refsUnder(context.Background(), sh, "refs/tars/checkpoints/")
	if err != nil {
		t.Fatal(err)
	}
	return refs
}
