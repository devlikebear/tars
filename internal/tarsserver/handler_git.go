package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	gitrepo "github.com/devlikebear/tars/internal/git"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

var errGitMutationRootOutsideSession = errors.New("git mutation root is outside the session workspace")

func newGitAPIHandler(workspaceDir string, store *session.Store, manager *ops.Manager, logger zerolog.Logger) http.Handler {
	client := gitrepo.NewClient()
	mux := http.NewServeMux()

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		status, err := gitStatusForRequest(r.Context(), client, workspaceDir, store, r)
		if err != nil {
			logger.Warn().Err(err).Msg("git status failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, status)
	})

	mux.HandleFunc("/diff", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		diff, err := gitDiffForRequest(r.Context(), client, workspaceDir, store, r)
		if err != nil {
			writeGitAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, diff)
	})

	mux.HandleFunc("/log", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
		log, err := gitLogForRequest(r.Context(), client, workspaceDir, store, r, limit)
		if err != nil {
			writeGitAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, log)
	})

	mux.HandleFunc("/branches", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		branches, err := gitBranchesForRequest(r.Context(), client, workspaceDir, store, r)
		if err != nil {
			writeGitAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, branches)
	})

	mux.HandleFunc("/commit", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		hash := strings.TrimSpace(r.URL.Query().Get("hash"))
		if hash == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hash is required"})
			return
		}
		detail, err := gitCommitForRequest(r.Context(), client, workspaceDir, store, r, hash)
		if err != nil {
			writeGitAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	})

	mux.HandleFunc("/worktrees", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		trees, err := gitWorktreesForRequest(r.Context(), client, workspaceDir, store, r)
		if err != nil {
			writeGitAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, trees)
	})

	mux.HandleFunc("/mutations", func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ops manager is not configured"})
			return
		}
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req struct {
			SessionID    string `json:"session_id"`
			Root         string `json:"root"`
			Action       string `json:"action"`
			Path         string `json:"path"`
			Branch       string `json:"branch"`
			Message      string `json:"message"`
			Hash         string `json:"hash"`
			WorktreePath string `json:"worktree_path"`
			NewBranch    string `json:"new_branch"`
			Reason       string `json:"reason"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		sessionID := strings.TrimSpace(req.SessionID)
		if !sessionAllowsApprovedGitMutation(store, sessionID) {
			root := strings.TrimSpace(req.Root)
			_, _ = manager.RecordAutomationAudit(ops.AutomationAuditEntry{
				Actor:     "git",
				Action:    "git." + strings.TrimSpace(req.Action),
				Reason:    "session has not enabled approved git mutations",
				SessionID: sessionID,
				CWD:       root,
				Result:    "blocked",
			})
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "session has not enabled approved git mutations"})
			return
		}
		root, err := gitMutationRootForRequest(r.Context(), client, workspaceDir, store, sessionID, req.Root)
		if err != nil {
			if errors.Is(err, errGitMutationRootOutsideSession) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
				return
			}
			writeGitAPIError(w, err)
			return
		}
		plan, err := manager.CreateGitMutationApproval(r.Context(), ops.GitMutationPlan{
			SessionID:    sessionID,
			Root:         root,
			Action:       strings.TrimSpace(req.Action),
			Path:         strings.TrimSpace(req.Path),
			Branch:       strings.TrimSpace(req.Branch),
			Message:      strings.TrimSpace(req.Message),
			Hash:         strings.TrimSpace(req.Hash),
			WorktreePath: strings.TrimSpace(req.WorktreePath),
			NewBranch:    strings.TrimSpace(req.NewBranch),
			Reason:       strings.TrimSpace(req.Reason),
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, plan)
	})

	return http.StripPrefix("/v1/git", mux)
}

func writeGitAPIError(w http.ResponseWriter, err error) {
	if errors.Is(err, gitrepo.ErrNotRepository) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not a git repository"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func gitStatusForRequest(ctx context.Context, client *gitrepo.Client, workspaceDir string, store *session.Store, r *http.Request) (gitrepo.Status, error) {
	targets := gitTargetDirs(workspaceDir, store, r)
	for _, target := range targets {
		status, err := client.Status(ctx, target)
		if err == nil {
			return status, nil
		}
		if !errors.Is(err, gitrepo.ErrNotRepository) {
			return gitrepo.Status{}, err
		}
	}
	return gitrepo.Status{IsGit: false, Root: firstGitTarget(targets), Files: []gitrepo.StatusFile{}}, nil
}

func gitDiffForRequest(ctx context.Context, client *gitrepo.Client, workspaceDir string, store *session.Store, r *http.Request) (gitrepo.Diff, error) {
	targets := gitTargetDirs(workspaceDir, store, r)
	var lastErr error = gitrepo.ErrNotRepository
	for _, target := range targets {
		diff, err := client.Diff(ctx, gitrepo.DiffOptions{
			StartDir: target,
			Path:     r.URL.Query().Get("path"),
			Staged:   queryBool(r.URL.Query().Get("staged")),
			Hash:     strings.TrimSpace(r.URL.Query().Get("hash")),
		})
		if err == nil {
			return diff, nil
		}
		lastErr = err
		if !errors.Is(err, gitrepo.ErrNotRepository) {
			return gitrepo.Diff{}, err
		}
	}
	return gitrepo.Diff{}, lastErr
}

func gitCommitForRequest(ctx context.Context, client *gitrepo.Client, workspaceDir string, store *session.Store, r *http.Request, hash string) (gitrepo.CommitDetail, error) {
	targets := gitTargetDirs(workspaceDir, store, r)
	var lastErr error = gitrepo.ErrNotRepository
	for _, target := range targets {
		detail, err := client.CommitDetail(ctx, target, hash)
		if err == nil {
			return detail, nil
		}
		lastErr = err
		if !errors.Is(err, gitrepo.ErrNotRepository) {
			return gitrepo.CommitDetail{}, err
		}
	}
	return gitrepo.CommitDetail{}, lastErr
}

func gitWorktreesForRequest(ctx context.Context, client *gitrepo.Client, workspaceDir string, store *session.Store, r *http.Request) (gitrepo.Worktrees, error) {
	targets := gitTargetDirs(workspaceDir, store, r)
	var lastErr error = gitrepo.ErrNotRepository
	for _, target := range targets {
		trees, err := client.Worktrees(ctx, target)
		if err == nil {
			return trees, nil
		}
		lastErr = err
		if !errors.Is(err, gitrepo.ErrNotRepository) {
			return gitrepo.Worktrees{}, err
		}
	}
	return gitrepo.Worktrees{}, lastErr
}

func gitLogForRequest(ctx context.Context, client *gitrepo.Client, workspaceDir string, store *session.Store, r *http.Request, limit int) (gitrepo.Log, error) {
	targets := gitTargetDirs(workspaceDir, store, r)
	var lastErr error = gitrepo.ErrNotRepository
	for _, target := range targets {
		log, err := client.Log(ctx, target, limit)
		if err == nil {
			return log, nil
		}
		lastErr = err
		if !errors.Is(err, gitrepo.ErrNotRepository) {
			return gitrepo.Log{}, err
		}
	}
	return gitrepo.Log{}, lastErr
}

func gitBranchesForRequest(ctx context.Context, client *gitrepo.Client, workspaceDir string, store *session.Store, r *http.Request) (gitrepo.Branches, error) {
	targets := gitTargetDirs(workspaceDir, store, r)
	var lastErr error = gitrepo.ErrNotRepository
	for _, target := range targets {
		branches, err := client.Branches(ctx, target)
		if err == nil {
			return branches, nil
		}
		lastErr = err
		if !errors.Is(err, gitrepo.ErrNotRepository) {
			return gitrepo.Branches{}, err
		}
	}
	return gitrepo.Branches{}, lastErr
}

func gitMutationRootForRequest(ctx context.Context, client *gitrepo.Client, workspaceDir string, store *session.Store, sessionID string, root string) (string, error) {
	targets := make([]string, 0, 4)
	requested := strings.TrimSpace(root)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if abs, err := filepath.Abs(value); err == nil {
			value = abs
		}
		value = filepath.Clean(value)
		for _, existing := range targets {
			if existing == value {
				return
			}
		}
		targets = append(targets, value)
	}
	if store != nil && strings.TrimSpace(sessionID) != "" {
		if sess, err := store.Get(strings.TrimSpace(sessionID)); err == nil {
			add(sess.CurrentDir)
			for _, dir := range sess.WorkDirs {
				add(dir)
			}
		}
	}
	add(workspaceDir)

	var lastErr error = gitrepo.ErrNotRepository
	allowedRoots := make([]string, 0, len(targets))
	for _, target := range targets {
		candidateRoot, err := client.RepositoryRoot(ctx, target)
		if err == nil {
			allowedRoots = appendUniquePath(allowedRoots, candidateRoot)
			continue
		}
		lastErr = err
		if !errors.Is(err, gitrepo.ErrNotRepository) {
			return "", err
		}
	}

	if requested != "" {
		requestedRoot, err := client.RepositoryRoot(ctx, requested)
		if err != nil {
			return "", err
		}
		for _, allowed := range allowedRoots {
			if filepath.Clean(requestedRoot) == filepath.Clean(allowed) {
				return requestedRoot, nil
			}
		}
		return "", errGitMutationRootOutsideSession
	}

	if len(allowedRoots) > 0 {
		return allowedRoots[0], nil
	}
	return "", lastErr
}

func appendUniquePath(items []string, value string) []string {
	value = filepath.Clean(strings.TrimSpace(value))
	if value == "" {
		return items
	}
	for _, existing := range items {
		if filepath.Clean(existing) == value {
			return items
		}
	}
	return append(items, value)
}

func gitTargetDirs(workspaceDir string, store *session.Store, r *http.Request) []string {
	query := r.URL.Query()
	targets := make([]string, 0, 4)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if value == "~" {
			if home, err := os.UserHomeDir(); err == nil {
				value = home
			}
		}
		if abs, err := filepath.Abs(value); err == nil {
			value = abs
		}
		value = filepath.Clean(value)
		for _, existing := range targets {
			if existing == value {
				return
			}
		}
		targets = append(targets, value)
	}

	if root := strings.TrimSpace(query.Get("root")); root != "" {
		add(root)
		return targets
	}

	if store != nil {
		if sessionID := strings.TrimSpace(query.Get("session_id")); sessionID != "" {
			if sess, err := store.Get(sessionID); err == nil {
				add(sess.CurrentDir)
				for _, dir := range sess.WorkDirs {
					add(dir)
				}
			}
		}
	}
	add(workspaceDir)
	return targets
}

func sessionAllowsApprovedGitMutation(store *session.Store, sessionID string) bool {
	if store == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	sess, err := store.Get(sessionID)
	if err != nil || sess.AutomationConsent == nil {
		return false
	}
	return sess.AutomationConsent.GitMutations
}

func firstGitTarget(targets []string) string {
	if len(targets) == 0 {
		return ""
	}
	return targets[0]
}

func queryBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on", "staged":
		return true
	default:
		return false
	}
}
