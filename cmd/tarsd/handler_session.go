package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/rs/zerolog"
)

func newSessionAPIHandler(store *session.Store, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			sessions, err := store.List()
			if err != nil {
				logger.Error().Err(err).Msg("list sessions failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list sessions failed"})
				return
			}
			writeJSON(w, http.StatusOK, sessions)
		case http.MethodPost:
			var req struct {
				Title string `json:"title"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
			title := strings.TrimSpace(req.Title)
			if title == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
				return
			}
			sess, err := store.Create(title)
			if err != nil {
				logger.Error().Err(err).Msg("create session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create session failed"})
				return
			}
			writeJSON(w, http.StatusOK, sess)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/sessions/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		sessions, err := store.List()
		if err != nil {
			logger.Error().Err(err).Msg("search sessions failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "search sessions failed"})
			return
		}

		query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		results := make([]session.Session, 0, len(sessions))
		for _, sess := range sessions {
			if strings.Contains(strings.ToLower(sess.Title), query) {
				results = append(results, sess)
			}
		}

		writeJSON(w, http.StatusOK, results)
	})

	mux.HandleFunc("/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		pathRemainder := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
		pathParts := strings.Split(pathRemainder, "/")
		sessionID := pathParts[0]
		if sessionID == "" {
			http.NotFound(w, r)
			return
		}

		switch {
		case len(pathParts) == 1:
			switch r.Method {
			case http.MethodGet:
				sess, err := store.Get(sessionID)
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
			case http.MethodDelete:
				if err := store.Delete(sessionID); err != nil {
					logger.Error().Err(err).Str("session_id", sessionID).Msg("delete session failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete session failed"})
					return
				}
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		case len(pathParts) == 2 && pathParts[1] == "history":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if _, err := store.Get(sessionID); err != nil {
				if strings.Contains(err.Error(), "session not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
					return
				}
				logger.Error().Err(err).Str("session_id", sessionID).Msg("get session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
				return
			}
			messages, err := session.ReadMessages(store.TranscriptPath(sessionID))
			if err != nil {
				logger.Error().Err(err).Str("session_id", sessionID).Msg("read session history failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read session history failed"})
				return
			}
			writeJSON(w, http.StatusOK, messages)
		case len(pathParts) == 2 && pathParts[1] == "export":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			sess, err := store.Get(sessionID)
			if err != nil {
				if strings.Contains(err.Error(), "session not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
					return
				}
				logger.Error().Err(err).Str("session_id", sessionID).Msg("get session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
				return
			}

			messages, err := session.ReadMessages(store.TranscriptPath(sessionID))
			if err != nil {
				logger.Error().Err(err).Str("session_id", sessionID).Msg("read session history failed")
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

	return mux
}

func newStatusAPIHandler(workspaceDir string, store *session.Store, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		sessions, err := store.List()
		if err != nil {
			logger.Error().Err(err).Msg("list sessions failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"workspace_dir": workspaceDir,
			"session_count": len(sessions),
		})
	})
}

func newCompactAPIHandler(workspaceDir string, store *session.Store, client llm.Client, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			SessionID        string `json:"session_id"`
			KeepRecent       int    `json:"keep_recent"`
			KeepRecentTokens int    `json:"keep_recent_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		sessionID := strings.TrimSpace(req.SessionID)
		if sessionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
			return
		}

		if _, err := store.Get(sessionID); err != nil {
			if strings.Contains(err.Error(), "session not found") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			logger.Error().Err(err).Str("session_id", sessionID).Msg("get session failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
			return
		}

		now := time.Now().UTC()
		result, err := compactWithMemoryFlush(workspaceDir, store.TranscriptPath(sessionID), sessionID, req.KeepRecent, req.KeepRecentTokens, client, now)
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
