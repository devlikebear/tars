package tarsserver

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func newSessionAPIHandler(store *session.Store, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	baseWorkspaceDir := ""
	if store != nil {
		baseWorkspaceDir = store.WorkspaceDir()
	}
	resolveStore := func(r *http.Request) (*session.Store, error) {
		if strings.TrimSpace(baseWorkspaceDir) == "" {
			return store, nil
		}
		resolvedStore, _, _, err := resolveSessionStoreForRequest(baseWorkspaceDir, store, r)
		if err != nil {
			return nil, err
		}
		return resolvedStore, nil
	}
	publicUnsupported := func(w http.ResponseWriter) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "single-main-session mode is enabled"})
	}
	resolvePublicMain := func(reqStore *session.Store) (session.Session, error) {
		if reqStore == nil {
			return session.Session{}, fmt.Errorf("session store is not configured")
		}
		mainSession, err := reqStore.EnsureMain()
		if err != nil {
			return session.Session{}, err
		}
		mainSession.ID = "main"
		mainSession.Kind = "main"
		mainSession.Hidden = false
		return mainSession, nil
	}
	resolveInternalMainID := func(reqStore *session.Store) (string, error) {
		if reqStore == nil {
			return "", fmt.Errorf("session store is not configured")
		}
		mainSession, err := reqStore.EnsureMain()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(mainSession.ID), nil
	}
	requireAdmin := func(w http.ResponseWriter, r *http.Request) bool {
		if strings.TrimSpace(serverauth.RoleFromRequest(r)) != serverauth.RoleAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return false
		}
		return true
	}

	mux.HandleFunc("/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		reqStore, err := resolveStore(r)
		if err != nil {
			logger.Error().Err(err).Msg("resolve workspace session store failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve workspace failed"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			mainSession, err := resolvePublicMain(reqStore)
			if err != nil {
				logger.Error().Err(err).Msg("resolve main session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve main session failed"})
				return
			}
			writeJSON(w, http.StatusOK, []session.Session{mainSession})
		case http.MethodPost:
			publicUnsupported(w)
		default:
			requireMethod(w, r)
		}
	})

	mux.HandleFunc("/v1/sessions/search", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		publicUnsupported(w)
	})

	mux.HandleFunc("/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		reqStore, err := resolveStore(r)
		if err != nil {
			logger.Error().Err(err).Msg("resolve workspace session store failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve workspace failed"})
			return
		}
		pathRemainder := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
		pathParts := strings.Split(pathRemainder, "/")
		sessionID := pathParts[0]
		if sessionID == "" {
			http.NotFound(w, r)
			return
		}
		internalMainID, err := resolveInternalMainID(reqStore)
		if err != nil {
			logger.Error().Err(err).Msg("resolve main session failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve main session failed"})
			return
		}
		isPublicMain := strings.EqualFold(strings.TrimSpace(sessionID), "main")

		switch {
		case len(pathParts) == 1:
			switch r.Method {
			case http.MethodGet:
				if !isPublicMain {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
					return
				}
				mainSession, err := resolvePublicMain(reqStore)
				if err != nil {
					logger.Error().Err(err).Msg("resolve main session failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve main session failed"})
					return
				}
				writeJSON(w, http.StatusOK, mainSession)
			case http.MethodDelete:
				publicUnsupported(w)
			default:
				requireMethod(w, r)
			}
		case len(pathParts) == 2 && pathParts[1] == "history":
			if !requireMethod(w, r, http.MethodGet) {
				return
			}
			if !isPublicMain {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			messages, err := session.ReadMessages(reqStore.TranscriptPath(internalMainID))
			if err != nil {
				logger.Error().Err(err).Str("session_id", internalMainID).Msg("read session history failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read session history failed"})
				return
			}
			writeJSON(w, http.StatusOK, messages)
		case len(pathParts) == 2 && pathParts[1] == "export":
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			if !isPublicMain {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			sess, err := reqStore.Get(internalMainID)
			if err != nil {
				logger.Error().Err(err).Str("session_id", internalMainID).Msg("get session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
				return
			}
			sess.ID = "main"
			sess.Kind = "main"
			sess.Hidden = false
			messages, err := session.ReadMessages(reqStore.TranscriptPath(internalMainID))
			if err != nil {
				logger.Error().Err(err).Str("session_id", internalMainID).Msg("read session history failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read session history failed"})
				return
			}

			var b strings.Builder
			fmt.Fprintf(&b, "# Session: %s\n", sess.Title)
			fmt.Fprintf(&b, "Created: %s\n\n", sess.CreatedAt.Format(time.RFC3339))
			for _, msg := range messages {
				fmt.Fprintf(&b, "## %s\n", msg.Timestamp.Format(time.RFC3339))
				fmt.Fprintf(&b, "**%s**: %s\n\n", msg.Role, msg.Content)
			}

			w.Header().Set("Content-Type", "text/markdown")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, b.String())
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/v1/admin/sessions", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r) {
			return
		}
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		reqStore, err := resolveStore(r)
		if err != nil {
			logger.Error().Err(err).Msg("resolve workspace session store failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve workspace failed"})
			return
		}
		includeHidden := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("hidden")), "1") || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("hidden")), "true")
		var sessions []session.Session
		if includeHidden {
			sessions, err = reqStore.ListAll()
		} else {
			sessions, err = reqStore.List()
		}
		if err != nil {
			logger.Error().Err(err).Msg("list sessions failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list sessions failed"})
			return
		}
		writeJSON(w, http.StatusOK, sessions)
	})

	mux.HandleFunc("/v1/admin/sessions/", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r) {
			return
		}
		reqStore, err := resolveStore(r)
		if err != nil {
			logger.Error().Err(err).Msg("resolve workspace session store failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve workspace failed"})
			return
		}
		pathRemainder := strings.TrimPrefix(r.URL.Path, "/v1/admin/sessions/")
		pathParts := strings.Split(pathRemainder, "/")
		sessionID := strings.TrimSpace(pathParts[0])
		if sessionID == "" {
			http.NotFound(w, r)
			return
		}
		switch {
		case len(pathParts) == 1:
			if !requireMethod(w, r, http.MethodGet) {
				return
			}
			sess, err := reqStore.Get(sessionID)
			if err != nil {
				if strings.Contains(err.Error(), "session not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
					return
				}
				logger.Error().Err(err).Str("session_id", sessionID).Msg("get session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
				return
			}
			writeJSON(w, http.StatusOK, sess)
		case len(pathParts) == 2 && pathParts[1] == "history":
			if !requireMethod(w, r, http.MethodGet) {
				return
			}
			if _, err := reqStore.Get(sessionID); err != nil {
				if strings.Contains(err.Error(), "session not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
					return
				}
				logger.Error().Err(err).Str("session_id", sessionID).Msg("get session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
				return
			}
			messages, err := session.ReadMessages(reqStore.TranscriptPath(sessionID))
			if err != nil {
				logger.Error().Err(err).Str("session_id", sessionID).Msg("read session history failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read session history failed"})
				return
			}
			writeJSON(w, http.StatusOK, messages)
		default:
			http.NotFound(w, r)
		}
	})

	return mux
}

func newStatusAPIHandler(workspaceDir string, store *session.Store, mainSessionID string, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}

		reqStore, resolvedWorkspaceDir, _, err := resolveSessionStoreForRequest(workspaceDir, store, r)
		if err != nil {
			logger.Error().Err(err).Msg("resolve workspace session store failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve workspace failed"})
			return
		}
		sessions, err := reqStore.List()
		if err != nil {
			logger.Error().Err(err).Msg("list sessions failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		body := map[string]any{
			"workspace_dir":   resolvedWorkspaceDir,
			"session_count":   len(sessions),
			"main_session_id": publicMainSessionLabel(mainSessionID),
		}
		if role := serverauth.RoleFromRequest(r); role != "" {
			body["auth_role"] = role
		}
		writeJSON(w, http.StatusOK, body)
	})
}

func newHealthzAPIHandler(nowFn func() time.Time, dashboardAuthStatus map[string]any) http.Handler {
	if nowFn == nil {
		nowFn = time.Now
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		body := map[string]any{
			"ok":        true,
			"component": "tars",
			"time":      nowFn().UTC().Format(time.RFC3339),
		}
		if dashboardAuthStatus != nil {
			body["dashboard_auth"] = dashboardAuthStatus
		}
		writeJSON(w, http.StatusOK, body)
	})
}

func newCompactAPIHandler(workspaceDir string, store *session.Store, client llm.Client, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req struct {
			SessionID        string `json:"session_id"`
			KeepRecent       int    `json:"keep_recent"`
			KeepRecentTokens int    `json:"keep_recent_tokens"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}

		sessionID := strings.TrimSpace(req.SessionID)
		if sessionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
			return
		}

		reqStore, resolvedWorkspaceDir, _, err := resolveSessionStoreForRequest(workspaceDir, store, r)
		if err != nil {
			logger.Error().Err(err).Msg("resolve workspace session store failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve workspace failed"})
			return
		}
		if _, err := reqStore.Get(sessionID); err != nil {
			if strings.Contains(err.Error(), "session not found") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			logger.Error().Err(err).Str("session_id", sessionID).Msg("get session failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
			return
		}

		now := time.Now().UTC()
		result, err := compactWithMemoryFlush(resolvedWorkspaceDir, reqStore.TranscriptPath(sessionID), sessionID, req.KeepRecent, req.KeepRecentTokens, client, now)
		if err != nil {
			logger.Error().Err(err).Str("session_id", sessionID).Msg("compact transcript failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "compact failed"})
			return
		}

		message := fmt.Sprintf(
			"compaction complete (session=%s compacted=%d final=%d)",
			sessionID,
			result.CompactedCount,
			result.FinalCount,
		)
		if !result.Compacted {
			message = fmt.Sprintf("compaction skipped (session=%s message_count=%d)", sessionID, result.OriginalCount)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"message":        message,
			"session_id":     sessionID,
			"compacted":      result.Compacted,
			"original_count": result.OriginalCount,
			"final_count":    result.FinalCount,
		})
	})
}
