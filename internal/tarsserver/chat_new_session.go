package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/sessionworktree"
)

// A new chat in a folder, isolated or not, in one call.
//
// POST /v1/admin/sessions takes two optional fields besides title:
//
//	{"title": "...", "cwd": "/abs/or/~/path", "isolate": true}
//
// With cwd the session starts working in that folder (it becomes its only
// registered work dir and the active cwd); with isolate it also moves into a
// worktree of its own before the first turn, as the header's ⑂ chip would.
// The request fails before anything is created when the folder does not
// exist or isolate names a folder outside a git repository, and a worktree
// that cannot be made removes the session again, so a caller never gets half
// of what it asked for. Without cwd and isolate the request goes to the
// session handler as it always has.
//
// Choosing an arbitrary folder is what PUT .../workdirs does, which only the
// admin token may; so does a create that carries one, even though a plain
// create is open to a browser user session.
//
// GET /v1/admin/session-folders lists the folders recent sessions worked in
// (what the console offers first), and ?path= checks one folder the way a
// create would, reporting its repository.

const (
	// worktreeReasonNewChat marks a worktree asked for when the chat began.
	worktreeReasonNewChat = "new_chat"
	// recentFolderLimit is how many recent folders the picker offers.
	recentFolderLimit = 8
)

var (
	errFolderRequired      = errors.New("a folder is required")
	errFolderNotAbsolute   = errors.New("folder must be an absolute path")
	errFolderNotFound      = errors.New("folder not found")
	errFolderNotDirectory  = errors.New("path is not a folder")
	errFolderNotRepository = errors.New("folder is not in a git repository, so it cannot be isolated")
	errIsolateNeedsFolder  = errors.New("isolate needs a cwd")
)

// chatFolder is a folder a chat can start in.
type chatFolder struct {
	Path string `json:"path"`
	// RepoRoot is the top level of the git repository the folder is in;
	// empty outside one. Only such folders can be isolated.
	RepoRoot   string     `json:"repo_root,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// resolveChatFolder checks a folder someone typed or picked: "~" means the
// home folder, anything else must be absolute, exist, and be a folder. The
// result has symlinks resolved, as the session store keeps them.
func resolveChatFolder(ctx context.Context, raw string) (chatFolder, error) {
	dir := strings.TrimSpace(raw)
	if dir == "" {
		return chatFolder{}, errFolderRequired
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return chatFolder{}, err
		}
		dir = filepath.Join(home, dir[1:])
	}
	if err := assertFilesystemBrowsePathShape(dir); err != nil {
		return chatFolder{}, errFolderNotAbsolute
	}
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil {
		return chatFolder{}, errFolderNotFound
	}
	if !info.IsDir() {
		return chatFolder{}, errFolderNotDirectory
	}
	dir = resolvedDir(dir)
	folder := chatFolder{Path: dir}
	if root, err := sessionworktree.RepoRoot(ctx, dir); err == nil {
		folder.RepoRoot = root
	}
	return folder, nil
}

func folderErrorStatus(err error) int {
	switch {
	case errors.Is(err, errFolderNotFound):
		return http.StatusNotFound
	case errors.Is(err, errFolderRequired), errors.Is(err, errFolderNotAbsolute),
		errors.Is(err, errFolderNotDirectory), errors.Is(err, errFolderNotRepository),
		errors.Is(err, errIsolateNeedsFolder):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

type newSessionRequest struct {
	Title   string `json:"title,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Isolate bool   `json:"isolate,omitempty"`
}

// withSessionCreateIn handles POST /v1/admin/sessions when it names a folder
// and passes every other request to next.
func withSessionCreateIn(next http.Handler, c *chatWorktrees) http.Handler {
	if c == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/admin/sessions" {
			next.ServeHTTP(w, r)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, defaultJSONBodyLimitBytes))
		if err != nil {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body too large"})
			return
		}
		var req newSessionRequest
		if len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, &req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
		}
		if strings.TrimSpace(req.Cwd) == "" && !req.Isolate {
			r.Body = io.NopCloser(bytes.NewReader(raw))
			next.ServeHTTP(w, r)
			return
		}
		if serverauth.RoleFromRequest(r) != serverauth.RoleAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "starting a chat in a folder needs the admin token"})
			return
		}
		sess, err := c.createIn(r.Context(), req)
		if err != nil {
			writeJSON(w, folderErrorStatus(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, sess)
	})
}

// createIn makes a session working in req.Cwd, isolated when asked, or
// nothing at all.
func (c *chatWorktrees) createIn(ctx context.Context, req newSessionRequest) (session.Session, error) {
	if strings.TrimSpace(req.Cwd) == "" {
		return session.Session{}, errIsolateNeedsFolder
	}
	folder, err := resolveChatFolder(ctx, req.Cwd)
	if err != nil {
		return session.Session{}, err
	}
	if req.Isolate && folder.RepoRoot == "" {
		return session.Session{}, errFolderNotRepository
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "New Chat"
	}
	created, err := c.store.Create(title)
	if err != nil {
		return session.Session{}, err
	}
	undo := func(cause error) (session.Session, error) {
		_ = c.store.Delete(created.ID)
		return session.Session{}, cause
	}
	if err := c.store.SetWorkDirs(created.ID, []string{folder.Path}, folder.Path); err != nil {
		return undo(err)
	}
	if req.Isolate {
		current, err := c.store.Get(created.ID)
		if err != nil {
			return undo(err)
		}
		if _, err := c.isolate(ctx, current, worktreeReasonNewChat, ""); err != nil {
			return undo(err)
		}
	}
	return c.store.Get(created.ID)
}

// sessionArtifactFolder reports whether cwd is still sess's own artifact
// folder, where every new session starts; such a session works in no project.
func sessionArtifactFolder(store *session.Store, sess session.Session, cwd string) bool {
	return sameDir(cwd, filepath.Join(store.WorkspaceDir(), "artifacts", sess.ID))
}

// sessionSourceFolder is the project folder a session works in: the folder
// its worktree was made from when isolated, otherwise its active cwd.
func sessionSourceFolder(sess session.Session) string {
	if sess.Worktree != nil && strings.TrimSpace(sess.Worktree.SourceDir) != "" {
		return strings.TrimSpace(sess.Worktree.SourceDir)
	}
	return strings.TrimSpace(sess.CurrentDir)
}

// recentFolders lists the folders visible sessions work in, most recently
// used first, without duplicates or folders that no longer exist.
func recentFolders(list []session.Session, inArtifacts func(session.Session) bool) []chatFolder {
	sorted := slices.Clone(list)
	slices.SortStableFunc(sorted, func(a, b session.Session) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	seen := map[string]bool{}
	out := []chatFolder{}
	for _, s := range sorted {
		if s.Hidden || (s.Kind != "" && s.Kind != "main") {
			continue
		}
		dir := sessionSourceFolder(s)
		if dir == "" || seen[dir] || inArtifacts(s) {
			continue
		}
		seen[dir] = true
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		used := s.UpdatedAt
		out = append(out, chatFolder{Path: dir, LastUsedAt: &used})
		if len(out) == recentFolderLimit {
			break
		}
	}
	return out
}

// newSessionFoldersHandler serves GET /v1/admin/session-folders.
func newSessionFoldersHandler(c *chatWorktrees) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}
		if path, ok := r.URL.Query()["path"]; ok {
			folder, err := resolveChatFolder(r.Context(), strings.Join(path, ""))
			if err != nil {
				writeJSON(w, folderErrorStatus(err), map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"folder": folder})
			return
		}
		list, err := c.store.List()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		recent := recentFolders(list, func(s session.Session) bool {
			return s.Worktree == nil && sessionArtifactFolder(c.store, s, s.CurrentDir)
		})
		for i := range recent {
			if root, err := sessionworktree.RepoRoot(r.Context(), recent[i].Path); err == nil {
				recent[i].RepoRoot = root
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"recent": recent})
	})
}
