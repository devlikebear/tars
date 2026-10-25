package focusprobe

import (
	"context"
	"os"
)

// WorkTreeState is a fingerprint of dir's working tree, used to prove a
// fix turn changed nothing (tars dogfooding fix #2): the server decides by
// git state, not by the model's report. It combines HEAD with a tree hash
// computed over a disposable index (read-tree HEAD, then add -A, then
// write-tree), so a new commit, an edited tracked file or a new untracked
// file all change the value, a gitignored file does not, and the
// repository's real index and HEAD are never touched — the disposable
// index is a temp file pointed to by GIT_INDEX_FILE, removed before
// return. Two calls return the same string exactly when nothing changed in
// between (same HEAD, same tree); either call failing (dir not a git
// repository, no commits yet) returns an error, which callers must treat
// as "unknown", not as "unchanged".
func WorkTreeState(ctx context.Context, dir string) (string, error) {
	head, err := RunGit(ctx, GitTimeout, dir, nil, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", err
	}
	index, err := os.CreateTemp("", "tars-focus-worktree-index-*")
	if err != nil {
		return "", err
	}
	indexPath := index.Name()
	closeErr := index.Close()
	defer func() { _ = os.Remove(indexPath) }()
	if closeErr != nil {
		return "", closeErr
	}
	env := []string{"GIT_INDEX_FILE=" + indexPath}
	if _, err := RunGit(ctx, GitTimeout, dir, env, "read-tree", "HEAD"); err != nil {
		return "", err
	}
	if _, err := RunGit(ctx, GitTimeout, dir, env, "add", "-A"); err != nil {
		return "", err
	}
	tree, err := RunGit(ctx, GitTimeout, dir, env, "write-tree")
	if err != nil {
		return "", err
	}
	return head + ":" + tree, nil
}
