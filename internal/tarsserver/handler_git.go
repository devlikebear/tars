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
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func newGitAPIHandler(workspaceDir string, store *session.Store, logger zerolog.Logger) http.Handler {
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
