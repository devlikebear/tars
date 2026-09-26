package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// A shadow repository holds the snapshots of one work-tree root. It lives in
// TARS's data directory, never inside the root, and its config and attributes
// turn off everything that would rewrite file bytes on the way in or out:
// EOL conversion, clean/smudge filters (LFS), ident, and working-tree
// encodings. So a checkpoint records exactly the bytes on disk, and the
// user's own .git — index, stash, HEAD, refs, objects, hooks — is never
// touched.
type shadowRepo struct {
	root   string // absolute work-tree root
	key    string
	gitDir string
}

// shadowKey names a root's shadow. Windows paths compare case-insensitively.
func shadowKey(root string) string {
	key := filepath.Clean(root)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

const shadowRootMarker = "tars-root"

// shadowConfig is written to every shadow's local config. It overrides the
// user's global config wherever that could change what gets stored or run.
var shadowConfig = [][2]string{
	{"core.bare", "false"},
	{"core.autocrlf", "false"},
	{"core.safecrlf", "false"},
	{"core.fsmonitor", "false"},
	{"core.untrackedCache", "false"},
	{"core.longpaths", "true"},
	{"core.quotePath", "false"},
	{"core.fscache", "true"},
	{"gc.auto", "0"},
	{"commit.gpgSign", "false"},
	{"color.ui", "false"},
	{"diff.noprefix", "false"},
	{"diff.mnemonicPrefix", "false"},
	{"diff.relative", "false"},
	{"diff.renames", "true"},
	{"user.name", checkpointIdentity},
	{"user.email", checkpointEmail},
}

// shadowAttributes has the highest precedence of any attributes source, so
// the root's own .gitattributes cannot turn conversions back on.
const shadowAttributes = "* -text -filter -ident -working-tree-encoding\n"

// defaultExcludes keeps well-known dependency and cache folders out of
// snapshots when the root's .gitignore does not already do so, which is the
// common case for folders that are not git repositories.
var defaultExcludes = []string{
	"node_modules/",
	".venv/",
	"venv/",
	"__pycache__/",
	".tox/",
	".mypy_cache/",
	".pytest_cache/",
	".gradle/",
	".next/",
	".nuxt/",
	".svelte-kit/",
	".turbo/",
	".DS_Store",
	"Thumbs.db",
	"desktop.ini",
}

// shadow returns root's shadow repository, creating it on first use.
func (s *Store) shadow(ctx context.Context, root string) (*shadowRepo, error) {
	key := shadowKey(root)
	sh := &shadowRepo{root: root, key: key, gitDir: filepath.Join(s.dir, "shadow", key+".git")}
	if recorded, err := readMarker(sh.gitDir); err == nil {
		if !samePath(recorded, root) {
			return nil, fmt.Errorf("checkpoint: shadow %s belongs to %q, not %q", key, recorded, root)
		}
		return sh, nil
	}
	if err := s.initShadow(ctx, sh); err != nil {
		_ = removeAllWritable(sh.gitDir)
		return nil, err
	}
	return sh, nil
}

func (s *Store) initShadow(ctx context.Context, sh *shadowRepo) error {
	if err := os.MkdirAll(filepath.Dir(sh.gitDir), 0o755); err != nil {
		return fmt.Errorf("checkpoint: create shadow dir: %w", err)
	}
	if _, _, err := s.git.run(ctx, gitCall{dir: filepath.Dir(sh.gitDir), args: []string{"init", "--bare", "--quiet", sh.gitDir}}); err != nil {
		return err
	}
	noHooks := filepath.Join(sh.gitDir, "no-hooks")
	if err := os.MkdirAll(noHooks, 0o755); err != nil {
		return fmt.Errorf("checkpoint: create hooks dir: %w", err)
	}
	settings := append([][2]string{
		{"core.worktree", sh.root},
		{"core.hooksPath", noHooks},
	}, shadowConfig...)
	for _, kv := range settings {
		call := gitCall{gitDir: sh.gitDir, dir: filepath.Dir(sh.gitDir), args: []string{"config", kv[0], kv[1]}}
		if _, _, err := s.git.run(ctx, call); err != nil {
			return err
		}
	}
	info := filepath.Join(sh.gitDir, "info")
	if err := os.MkdirAll(info, 0o755); err != nil {
		return fmt.Errorf("checkpoint: create info dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(info, "attributes"), []byte(shadowAttributes), 0o644); err != nil {
		return fmt.Errorf("checkpoint: write attributes: %w", err)
	}
	// The marker goes last: a shadow without it is incomplete and is rebuilt.
	return os.WriteFile(filepath.Join(sh.gitDir, shadowRootMarker), []byte(sh.root+"\n"), 0o644)
}

// readMarker returns the root a complete shadow belongs to.
func readMarker(gitDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(gitDir, shadowRootMarker))
	if err != nil {
		return "", fmt.Errorf("checkpoint: shadow %s: %w", filepath.Base(gitDir), err)
	}
	return strings.TrimSpace(string(data)), nil
}

// call builds a git call against the shadow, run from its root.
func (sh *shadowRepo) call(args ...string) gitCall {
	return gitCall{gitDir: sh.gitDir, workTree: sh.root, dir: sh.root, args: args}
}

// writeExcludes rewrites info/exclude for the next snapshot: the defaults,
// the root repository's own info/exclude, TARS's data directory when it sits
// inside the root, and the paths this snapshot must leave out.
func (s *Store) writeExcludes(sh *shadowRepo, extra []string) error {
	var b strings.Builder
	b.WriteString("# Written by TARS before each checkpoint. Do not edit.\n")
	for _, pattern := range defaultExcludes {
		b.WriteString(pattern)
		b.WriteByte('\n')
	}
	if own, err := os.ReadFile(filepath.Join(sh.root, ".git", "info", "exclude")); err == nil {
		b.Write(own)
		if len(own) > 0 && own[len(own)-1] != '\n' {
			b.WriteByte('\n')
		}
	}
	if rel, ok := relativeInside(sh.root, s.dir); ok {
		b.WriteString(excludeLiteral(rel) + "/\n")
	}
	for _, path := range extra {
		b.WriteString(excludeLiteral(strings.TrimSuffix(path, "/")))
		if strings.HasSuffix(path, "/") {
			b.WriteByte('/')
		}
		b.WriteByte('\n')
	}
	return os.WriteFile(filepath.Join(sh.gitDir, "info", "exclude"), []byte(b.String()), 0o644)
}

// excludeLiteral turns a root-relative slash path into an anchored gitignore
// pattern that matches only that path.
func excludeLiteral(path string) string {
	var b strings.Builder
	b.WriteByte('/')
	for _, r := range path {
		switch r {
		case '\\', '*', '?', '[':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	out := b.String()
	// Trailing spaces are dropped from patterns unless escaped.
	trimmed := strings.TrimRight(out, " ")
	if n := len(out) - len(trimmed); n > 0 {
		out = trimmed + strings.Repeat("\\ ", n)
	}
	return out
}

// removeStaleLock drops an index.lock left by a killed or crashed git. Only
// TARS writes a shadow, and it holds the root's lock while it does, so a lock
// file seen here is always stale.
func (sh *shadowRepo) removeStaleLock() {
	_ = os.Remove(filepath.Join(sh.gitDir, "index.lock"))
}

// relativeInside reports path as a slash path relative to root when it lies
// strictly inside root.
func relativeInside(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// removeAllWritable deletes a tree that git may have made read-only. Git for
// Windows writes objects without the write bit, and os.RemoveAll cannot
// delete read-only files there.
func removeAllWritable(path string) error {
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			_ = os.Chmod(p, 0o600)
		}
		return nil
	})
	if err := os.RemoveAll(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
