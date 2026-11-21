package easyproject

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Creator makes projects: Start answers at once with a project that is
// creating, and the scaffold, git and the pipeline follow in the
// background (a cold scaffold took 25 s when measured).
type Creator struct {
	Store      *Store
	BaseDir    func() (string, error)
	Scaffolder Scaffolder
	Open       Opener
	Now        func() time.Time
	// OnFailure is told why a project failed (for the server log); the
	// project itself carries only an error code.
	OnFailure func(p Project, err error)

	wg sync.WaitGroup
}

// Start checks the idea and the computer, makes the project's folder,
// records the project as creating, and finishes it in the background.
// ErrIdeaRequired, ErrIdeaTooLong and ErrNodeMissing are answered before
// anything is made.
func (c *Creator) Start(ctx context.Context, idea string) (Project, error) {
	idea, err := CleanIdea(idea)
	if err != nil {
		return Project{}, err
	}
	if err := c.Scaffolder.Ready(); err != nil {
		return Project{}, err
	}
	base, err := c.BaseDir()
	if err != nil {
		return Project{}, err
	}
	// The folder is recorded as sessions see it, with links resolved (on
	// macOS /var is /private/var), so the project and its pipeline name the
	// same path.
	if err := os.MkdirAll(base, 0o755); err != nil {
		return Project{}, err
	}
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}
	folder, err := NewFolder(base, idea)
	if err != nil {
		return Project{}, err
	}
	now := c.Now().UTC()
	p := Project{
		ID: newID(), Name: NameOf(idea), Idea: idea, Folder: folder,
		Status: StatusCreating, CreatedAt: now, UpdatedAt: now,
	}
	if err := c.Store.Put(p); err != nil {
		_ = os.Remove(folder)
		return Project{}, err
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.finish(context.WithoutCancel(ctx), p, base)
	}()
	return p, nil
}

// Wait blocks until every creation started so far has ended (tests).
func (c *Creator) Wait() { c.wg.Wait() }

func (c *Creator) finish(ctx context.Context, p Project, base string) {
	code, err := c.build(ctx, p)
	if err == nil {
		return
	}
	p.Status, p.ErrorCode, p.UpdatedAt = StatusFailed, code, c.Now().UTC()
	// The folder is ours and holds nothing of the user's yet: a half-made
	// project is removed, so a retry starts clean under the same name.
	if inside(base, p.Folder) {
		_ = os.RemoveAll(p.Folder)
	}
	if c.OnFailure != nil {
		c.OnFailure(p, err)
	}
	_ = c.Store.Put(p)
}

// build scaffolds, starts git and opens the pipeline; on success it stores
// the project ready. It answers the error code of the step that failed.
func (c *Creator) build(ctx context.Context, p Project) (string, error) {
	if err := c.Scaffolder.Scaffold(ctx, p.Folder); err != nil {
		if errors.Is(err, ErrNodeMissing) {
			return ErrorNodeMissing, err
		}
		return ErrorScaffoldFailed, err
	}
	if err := InitGit(ctx, p.Folder); err != nil {
		return ErrorGitFailed, err
	}
	sessionID, err := c.Open(ctx, p.Folder, p.Idea)
	if err != nil {
		return ErrorPipelineFailed, err
	}
	p.Status, p.SessionID, p.UpdatedAt = StatusReady, sessionID, c.Now().UTC()
	if err := c.Store.Put(p); err != nil {
		return ErrorPipelineFailed, err
	}
	return "", nil
}

// inside reports whether path is a folder strictly below base, so that a
// failed project's cleanup can never reach anything else.
func inside(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}
