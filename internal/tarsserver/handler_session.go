package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/buildinfo"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/sessionoverride"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

// sessionNotifier is invoked for SSE notifications emitted from session
// handlers (currently only the active-cwd transition). nil is allowed and
// disables notifications.
type sessionNotifier func(context.Context, notificationEvent)

func newSessionAPIHandler(store *session.Store, logger zerolog.Logger) http.Handler {
	return newSessionAPIHandlerWithUsage(store, logger, nil)
}

func newSessionAPIHandlerWithUsage(store *session.Store, logger zerolog.Logger, usageTracker *usage.Tracker) http.Handler {
	return newSessionAPIHandlerWithUsageAndStyleDefaults(store, logger, usageTracker, sessionStyleDefaultsFromConfig(config.Default()))
}

func newSessionAPIHandlerWithUsageAndStyleDefaults(store *session.Store, logger zerolog.Logger, usageTracker *usage.Tracker, styleDefaults sessionStyleValues) http.Handler {
	return newSessionAPIHandlerWithNotifier(store, logger, usageTracker, styleDefaults, nil)
}

func newSessionAPIHandlerWithNotifier(store *session.Store, logger zerolog.Logger, usageTracker *usage.Tracker, styleDefaults sessionStyleValues, notify sessionNotifier) http.Handler {
	return newSessionAPIHandlerFull(store, logger, usageTracker, styleDefaults, notify, nil)
}

func newSessionAPIHandlerFull(store *session.Store, logger zerolog.Logger, usageTracker *usage.Tracker, styleDefaults sessionStyleValues, notify sessionNotifier, overrideService *sessionoverride.Service) http.Handler {
	mux := http.NewServeMux()
	styleDefaults = effectiveSessionStyle(styleDefaults, nil)
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
		internalID := strings.TrimSpace(mainSession.ID)
		mainSession.ID = "main"
		mainSession.Kind = "main"
		mainSession.Hidden = false
		if strings.TrimSpace(mainSession.RootSessionID) == "" || strings.TrimSpace(mainSession.RootSessionID) == internalID {
			mainSession.RootSessionID = "main"
		}
		if strings.TrimSpace(mainSession.ParentSessionID) == internalID {
			mainSession.ParentSessionID = "main"
		}
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
			case http.MethodPatch:
				actualID := sessionID
				if isPublicMain {
					actualID = internalMainID
				}
				var req struct {
					Title string `json:"title"`
				}
				if !decodeJSONBody(w, r, &req) {
					return
				}
				if err := reqStore.SetTitle(actualID, req.Title); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
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
		if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
			return
		}
		reqStore, err := resolveStore(r)
		if err != nil {
			logger.Error().Err(err).Msg("resolve workspace session store failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve workspace failed"})
			return
		}
		switch r.Method {
		case http.MethodGet:
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
		case http.MethodPost:
			var req struct {
				Title string `json:"title,omitempty"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			title := strings.TrimSpace(req.Title)
			if title == "" {
				title = "New Chat"
			}
			sess, err := reqStore.Create(title)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, sess)
		}
	})

	mux.HandleFunc("/v1/admin/tasks", func(w http.ResponseWriter, r *http.Request) {
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
		includeHidden := isTruthyQuery(r.URL.Query().Get("hidden"))
		activeOnly := !isFalsyQuery(r.URL.Query().Get("active"))
		items, err := listGlobalPlanTaskItems(reqStore, includeHidden, activeOnly)
		if err != nil {
			logger.Error().Err(err).Msg("list global tasks failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list tasks failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": items,
			"count": len(items),
		})
	})

	mux.HandleFunc("/v1/admin/plans/archive", func(w http.ResponseWriter, r *http.Request) {
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
		items, err := listPlanArchiveItems(reqStore.WorkspaceDir(), "", parseArchiveLimit(r))
		if err != nil {
			logger.Error().Err(err).Msg("list plan archive failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list plan archive failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": items,
			"count": len(items),
		})
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
		case len(pathParts) == 3 && pathParts[1] == "plans" && pathParts[2] == "archive":
			if !requireMethod(w, r, http.MethodGet) {
				return
			}
			if strings.EqualFold(sessionID, "main") {
				resolvedMainID, err := resolveInternalMainID(reqStore)
				if err != nil {
					logger.Error().Err(err).Msg("resolve main session failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve main session failed"})
					return
				}
				sessionID = resolvedMainID
			}
			if _, err := reqStore.Get(sessionID); err != nil {
				if strings.Contains(err.Error(), "session not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
				return
			}
			items, err := listPlanArchiveItems(reqStore.WorkspaceDir(), sessionID, parseArchiveLimit(r))
			if err != nil {
				logger.Error().Err(err).Str("session_id", sessionID).Msg("list session plan archive failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list plan archive failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"items": items,
				"count": len(items),
			})
		case len(pathParts) == 2 && pathParts[1] == "fork":
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			actualID := sessionID
			if strings.EqualFold(sessionID, "main") {
				resolvedMainID, err := resolveInternalMainID(reqStore)
				if err != nil {
					logger.Error().Err(err).Msg("resolve main session failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve main session failed"})
					return
				}
				actualID = resolvedMainID
			}
			var req struct {
				MessageID  string `json:"message_id"`
				Title      string `json:"title,omitempty"`
				ForkReason string `json:"fork_reason,omitempty"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			child, err := reqStore.ForkFromMessage(actualID, req.MessageID, session.ForkOptions{
				Title:  req.Title,
				Reason: req.ForkReason,
			})
			if err != nil {
				switch {
				case strings.Contains(err.Error(), "is required"):
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				case strings.Contains(err.Error(), "not found"):
					writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				default:
					logger.Error().Err(err).Str("session_id", actualID).Msg("fork session failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "fork session failed"})
				}
				return
			}
			writeJSON(w, http.StatusCreated, child)
		case len(pathParts) == 2 && pathParts[1] == "promotions":
			handleForkPromotions(w, r, reqStore, sessionID, logger)
		case len(pathParts) == 1:
			if !requireMethod(w, r, http.MethodGet, http.MethodPatch, http.MethodDelete) {
				return
			}
			switch r.Method {
			case http.MethodGet:
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
			case http.MethodPatch:
				var req struct {
					Title string `json:"title"`
				}
				if !decodeJSONBody(w, r, &req) {
					return
				}
				if err := reqStore.SetTitle(sessionID, req.Title); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
			case http.MethodDelete:
				if err := reqStore.Delete(sessionID); err != nil {
					logger.Error().Err(err).Str("session_id", sessionID).Msg("delete session failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete session failed"})
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
			}
		case len(pathParts) == 2 && pathParts[1] == "compact":
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			if _, err := reqStore.Get(sessionID); err != nil {
				if strings.Contains(err.Error(), "session not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
				return
			}
			path := reqStore.TranscriptPath(sessionID)
			messages, err := session.ReadMessages(path)
			if err != nil {
				logger.Error().Err(err).Str("session_id", sessionID).Msg("read transcript for compact failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read transcript failed"})
				return
			}
			tokensBefore := session.EstimateTokens(messages)
			// Manual compact uses lower thresholds than auto-compact so it
			// always does meaningful work when the user explicitly requests it.
			now := time.Now().UTC()
			result, err := session.CompactTranscriptWithOptions(path, 5, now, session.CompactOptions{
				KeepRecentTokens:    2000,
				KeepRecentFraction:  0.20,
				PreloadedMessages:   messages,
				PostSummaryMessages: compactionTaskInjectionMessages(compactionTaskInjection(reqStore, sessionID, logger), now),
			})
			if err != nil {
				logger.Error().Err(err).Str("session_id", sessionID).Msg("compact session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "compact session failed"})
				return
			}
			tokensAfter := tokensBefore
			reason := ""
			if result.Compacted {
				after, _ := session.ReadMessages(path)
				tokensAfter = session.EstimateTokens(after)
			} else {
				if len(messages) <= 5 {
					reason = "too few messages to compact"
				} else if tokensBefore <= 2000 {
					reason = "transcript already within token budget"
				} else {
					reason = "no compactable messages found"
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"session_id":      sessionID,
				"compacted":       result.Compacted,
				"original_count":  result.OriginalCount,
				"final_count":     result.FinalCount,
				"compacted_count": result.CompactedCount,
				"tokens_before":   tokensBefore,
				"tokens_after":    tokensAfter,
				"reason":          reason,
			})
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
		case len(pathParts) == 2 && pathParts[1] == "config":
			if !requireMethod(w, r, http.MethodGet, http.MethodPatch) {
				return
			}
			sess, err := reqStore.Get(sessionID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			switch r.Method {
			case http.MethodGet:
				config := sess.ToolConfig
				if config == nil {
					config = &session.SessionToolConfig{}
				}
				writeJSON(w, http.StatusOK, config)
			case http.MethodPatch:
				var config session.SessionToolConfig
				if !decodeJSONBody(w, r, &config) {
					return
				}
				if err := reqStore.SetToolConfig(sessionID, &config); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				recordSessionToolConfigSignal(usageTracker, sessionID, config)
				writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
			}
		case len(pathParts) == 2 && pathParts[1] == "automation-consent":
			if !requireMethod(w, r, http.MethodGet, http.MethodPatch) {
				return
			}
			sess, err := reqStore.Get(sessionID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			switch r.Method {
			case http.MethodGet:
				consent := sess.AutomationConsent
				if consent == nil {
					consent = &session.SessionAutomationConsent{}
				}
				writeJSON(w, http.StatusOK, consent)
			case http.MethodPatch:
				var consent session.SessionAutomationConsent
				if !decodeJSONBody(w, r, &consent) {
					return
				}
				if err := reqStore.SetAutomationConsent(sessionID, &consent); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				updated, err := reqStore.Get(sessionID)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get updated session failed"})
					return
				}
				writeJSON(w, http.StatusOK, updated.AutomationConsent)
			}
		case len(pathParts) == 2 && pathParts[1] == "style":
			if !requireMethod(w, r, http.MethodGet, http.MethodPatch) {
				return
			}
			sess, err := reqStore.Get(sessionID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			switch r.Method {
			case http.MethodGet:
				writeJSON(w, http.StatusOK, buildSessionStyleResponse(styleDefaults, sess))
			case http.MethodPatch:
				var style session.SessionStyleControl
				if !decodeJSONBody(w, r, &style) {
					return
				}
				if err := reqStore.SetStyleControl(sessionID, &style); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				updated, err := reqStore.Get(sessionID)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get updated session failed"})
					return
				}
				writeJSON(w, http.StatusOK, buildSessionStyleResponse(styleDefaults, updated))
			}
		case len(pathParts) == 2 && pathParts[1] == "prompt":
			if !requireMethod(w, r, http.MethodGet, http.MethodPut) {
				return
			}
			sess, err := reqStore.Get(sessionID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			switch r.Method {
			case http.MethodGet:
				writeJSON(w, http.StatusOK, map[string]string{
					"prompt_override": sess.PromptOverride,
				})
			case http.MethodPut:
				var req struct {
					PromptOverride string `json:"prompt_override"`
				}
				if !decodeJSONBody(w, r, &req) {
					return
				}
				if err := reqStore.SetPromptOverride(sessionID, req.PromptOverride); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
			}
		case len(pathParts) == 2 && pathParts[1] == "tasks":
			if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
				return
			}
			if r.Method == http.MethodGet {
				tasks, err := reqStore.GetTasks(sessionID)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, tasks)
				return
			}
			// POST: invoke the tasks aggregator with arbitrary action+params
			// so the console can drive the plan state machine without going
			// through the LLM (e.g. user clicks Approve/Discard/Edit in the
			// TasksPanel). Body shape mirrors the tool's own JSON contract:
			// {"action":"plan_approve"} / {"action":"add","title":"..."} etc.
			var raw json.RawMessage
			if !decodeJSONBody(w, r, &raw) {
				return
			}
			tasksTool := tool.NewTasksTool(reqStore, reqStore.WorkspaceDir(), func() string { return sessionID })
			result, execErr := tasksTool.Execute(context.Background(), raw)
			if execErr != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": execErr.Error()})
				return
			}
			if result.IsError {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimSpace(result.Text())})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(result.Text()))
		case len(pathParts) == 2 && pathParts[1] == "workdirs":
			if !requireMethod(w, r, http.MethodGet, http.MethodPut) {
				return
			}
			sess, err := reqStore.Get(sessionID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			switch r.Method {
			case http.MethodGet:
				workDirs := sess.WorkDirs
				if workDirs == nil {
					workDirs = []string{}
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"work_dirs":   workDirs,
					"current_dir": sess.CurrentDir,
				})
			case http.MethodPut:
				var req struct {
					WorkDirs   []string `json:"work_dirs"`
					CurrentDir string   `json:"current_dir"`
				}
				if !decodeJSONBody(w, r, &req) {
					return
				}
				if err := reqStore.SetWorkDirs(sessionID, req.WorkDirs, req.CurrentDir); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
			}
		case len(pathParts) == 2 && pathParts[1] == "cwd":
			if !requireMethod(w, r, http.MethodGet, http.MethodPut) {
				return
			}
			switch r.Method {
			case http.MethodGet:
				current, err := reqStore.GetCurrentDir(sessionID)
				if err != nil {
					if errors.Is(err, session.ErrSessionNotFound) {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
						return
					}
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				eligible, err := reqStore.EligibleCwds(sessionID)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				if eligible == nil {
					eligible = []string{}
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"current":  current,
					"eligible": eligible,
				})
			case http.MethodPut:
				var req struct {
					Current string `json:"current"`
				}
				if !decodeJSONBody(w, r, &req) {
					return
				}
				if err := reqStore.SetCurrentDir(sessionID, req.Current); err != nil {
					switch {
					case errors.Is(err, session.ErrSessionNotFound):
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
					case errors.Is(err, session.ErrCwdNotEligible):
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					default:
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					}
					return
				}
				// Drop any cached effective-config so the next /effective-config
				// (or chat turn) reloads against the new cwd's settings files.
				if overrideService != nil {
					overrideService.Invalidate(sessionID)
				}
				if notify != nil {
					evt := newNotificationEvent(
						"session",
						"info",
						"Active cwd changed",
						strings.TrimSpace(req.Current),
					)
					evt.SessionID = sessionID
					notify(r.Context(), evt)
				}
				writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
			}
		case len(pathParts) == 2 && pathParts[1] == "effective-config":
			if !requireMethod(w, r, http.MethodGet) {
				return
			}
			if overrideService == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{
					"error": "effective-config service is not configured",
				})
				return
			}
			res, changed, err := overrideService.Resolve(sessionID)
			if err != nil {
				switch {
				case errors.Is(err, session.ErrSessionNotFound):
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				default:
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				}
				return
			}
			if changed && notify != nil {
				evt := newNotificationEvent(
					"session",
					"info",
					"Effective config refreshed",
					"",
				)
				evt.SessionID = sessionID
				notify(r.Context(), evt)
			}
			writeJSON(w, http.StatusOK, res)
		default:
			http.NotFound(w, r)
		}
	})

	return mux
}

func recordSessionToolConfigSignal(tracker *usage.Tracker, sessionID string, config session.SessionToolConfig) {
	if tracker == nil {
		return
	}
	dimensions := map[string]string{
		"tools_custom":  boolDimension(config.ToolsCustom),
		"skills_custom": boolDimension(config.SkillsCustom),
	}
	if len(config.ToolsEnabled) > 0 {
		dimensions["tools_enabled_count"] = fmt.Sprintf("%d", len(config.ToolsEnabled))
	}
	if len(config.ToolsDisabled) > 0 {
		dimensions["tools_disabled_count"] = fmt.Sprintf("%d", len(config.ToolsDisabled))
	}
	if len(config.ToolsAllowGroups) > 0 {
		dimensions["tools_allow_groups_count"] = fmt.Sprintf("%d", len(config.ToolsAllowGroups))
	}
	if len(config.ToolsDenyGroups) > 0 {
		dimensions["tools_deny_groups_count"] = fmt.Sprintf("%d", len(config.ToolsDenyGroups))
	}
	if len(config.SkillsEnabled) > 0 {
		dimensions["skills_enabled_count"] = fmt.Sprintf("%d", len(config.SkillsEnabled))
	}
	if len(config.MCPEnabled) > 0 {
		dimensions["mcp_enabled_count"] = fmt.Sprintf("%d", len(config.MCPEnabled))
	}
	_ = tracker.RecordSignal(usage.SignalEntry{
		Name:       "session.tool_config.updated",
		Source:     "api",
		SessionID:  sessionID,
		Dimensions: dimensions,
	})
}

func boolDimension(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

type globalPlanTaskItem struct {
	Session   session.Session       `json:"session"`
	Plan      *session.Plan         `json:"plan"`
	Contract  *session.TaskContract `json:"contract,omitempty"`
	Tasks     []session.Task        `json:"tasks"`
	Summary   map[string]int        `json:"summary"`
	UpdatedAt string                `json:"updated_at"`
}

func isTruthyQuery(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "1" || value == "true" || value == "yes"
}

func isFalsyQuery(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "0" || value == "false" || value == "no"
}

func listGlobalPlanTaskItems(store *session.Store, includeHidden bool, activeOnly bool) ([]globalPlanTaskItem, error) {
	plans, err := store.ListSessionsWithPlans(includeHidden, activeOnly)
	if err != nil {
		return nil, err
	}
	items := make([]globalPlanTaskItem, 0, len(plans))
	for _, plan := range plans {
		items = append(items, globalPlanTaskItem{
			Session:   plan.Session,
			Plan:      plan.Plan,
			Contract:  plan.Contract,
			Tasks:     plan.Tasks,
			Summary:   plan.Summary,
			UpdatedAt: plan.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	if items == nil {
		return []globalPlanTaskItem{}, nil
	}
	return items, nil
}

const archivedPlanPrefix = "[archived plan]"

type planArchiveItem struct {
	ID         string `json:"id"`
	SessionID  string `json:"session_id,omitempty"`
	Goal       string `json:"goal"`
	ArchivedAt string `json:"archived_at"`
	CreatedAt  string `json:"created_at,omitempty"`
	Summary    string `json:"summary"`
}

func parseArchiveLimit(r *http.Request) int {
	if r == nil {
		return 50
	}
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return 50
	}
	var limit int
	if _, err := fmt.Sscanf(raw, "%d", &limit); err != nil || limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func listPlanArchiveItems(workspaceDir, sessionFilter string, limit int) ([]planArchiveItem, error) {
	noteLimit := limit
	if strings.TrimSpace(sessionFilter) != "" {
		noteLimit = 500
	}
	notes, err := memory.ListMemoryNotesByPrefix(workspaceDir, archivedPlanPrefix, noteLimit)
	if err != nil {
		return nil, err
	}
	items := make([]planArchiveItem, 0, len(notes))
	for idx, note := range notes {
		item, ok := parsePlanArchiveNote(note, idx)
		if !ok {
			continue
		}
		if strings.TrimSpace(sessionFilter) != "" && item.SessionID != strings.TrimSpace(sessionFilter) {
			continue
		}
		items = append(items, item)
		if len(items) >= limit {
			break
		}
	}
	if items == nil {
		return []planArchiveItem{}, nil
	}
	return items, nil
}

func parsePlanArchiveNote(note memory.MemoryNote, idx int) (planArchiveItem, bool) {
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(note.Text), archivedPlanPrefix))
	sessionID := ""
	if firstLine, rest, ok := strings.Cut(text, "\n"); ok {
		fields := strings.Fields(strings.TrimSpace(firstLine))
		metadataOnly := len(fields) > 0
		for _, field := range fields {
			if strings.HasPrefix(field, "session=") {
				sessionID = strings.TrimSpace(strings.TrimPrefix(field, "session="))
				continue
			}
			metadataOnly = false
		}
		if metadataOnly {
			text = strings.TrimSpace(rest)
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return planArchiveItem{}, false
	}
	goal, createdAt := parseArchivedPlanGoal(text)
	if goal == "" {
		goal = "Archived plan"
	}
	return planArchiveItem{
		ID:         fmt.Sprintf("%s-%d", note.Timestamp.UTC().Format(time.RFC3339Nano), idx),
		SessionID:  sessionID,
		Goal:       goal,
		ArchivedAt: note.Timestamp.UTC().Format(time.RFC3339),
		CreatedAt:  createdAt,
		Summary:    text,
	}, true
}

func parseArchivedPlanGoal(summary string) (string, string) {
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Plan:") {
			continue
		}
		goal := strings.TrimSpace(strings.TrimPrefix(line, "Plan:"))
		createdAt := ""
		if before, after, ok := strings.Cut(goal, " (created: "); ok {
			goal = strings.TrimSpace(before)
			createdAt = strings.TrimSuffix(strings.TrimSpace(after), ")")
		}
		return goal, createdAt
	}
	return "", ""
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
			"version":         buildinfo.Version,
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

func newHealthzAPIHandler(nowFn func() time.Time, dashboardAuthStatus map[string]any, needsSetupFn func() bool) http.Handler {
	if nowFn == nil {
		nowFn = time.Now
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		needsSetup := false
		if needsSetupFn != nil {
			needsSetup = needsSetupFn()
		}
		body := map[string]any{
			"ok":          true,
			"component":   "tars",
			"time":        nowFn().UTC().Format(time.RFC3339),
			"needs_setup": needsSetup,
		}
		if dashboardAuthStatus != nil {
			body["dashboard_auth"] = dashboardAuthStatus
		}
		writeJSON(w, http.StatusOK, body)
	})
}

func newCompactAPIHandler(workspaceDir string, store *session.Store, router llm.Router, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req struct {
			SessionID        string `json:"session_id"`
			KeepRecent       int    `json:"keep_recent"`
			KeepRecentTokens int    `json:"keep_recent_tokens"`
			Instructions     string `json:"instructions"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}

		sessionID := strings.TrimSpace(req.SessionID)
		if sessionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
			return
		}
		publicSessionID := sessionID

		reqStore, resolvedWorkspaceDir, _, err := resolveSessionStoreForRequest(workspaceDir, store, r)
		if err != nil {
			logger.Error().Err(err).Msg("resolve workspace session store failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve workspace failed"})
			return
		}
		if strings.EqualFold(sessionID, "main") {
			resolvedMainID, err := resolveMainSessionID(reqStore, "")
			if err != nil {
				logger.Error().Err(err).Msg("resolve main session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve main session failed"})
				return
			}
			sessionID = resolvedMainID
			publicSessionID = publicMainSessionLabel(resolvedMainID)
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
		keepRecentFraction := 0.0
		if req.KeepRecent <= 0 && req.KeepRecentTokens <= 0 {
			keepRecentFraction = session.DefaultKeepRecentFraction
		}
		result, _, err := compactWithMemoryFlush(
			resolvedWorkspaceDir,
			reqStore.TranscriptPath(sessionID),
			sessionID,
			req.KeepRecent,
			chatCompactionOptions{
				KeepRecentTokens:   req.KeepRecentTokens,
				KeepRecentFraction: keepRecentFraction,
				LLMMode:            "auto",
				LLMTimeoutSeconds:  defaultChatToolingOptions().Compaction.LLMTimeoutSeconds,
			},
			req.Instructions,
			router,
			now,
			nil,
			compactionTaskInjection(reqStore, sessionID, logger),
		)
		if err != nil {
			logger.Error().Err(err).Str("session_id", sessionID).Msg("compact transcript failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "compact failed"})
			return
		}
		displaySessionID := publicSessionID
		if strings.TrimSpace(displaySessionID) == "" {
			displaySessionID = sessionID
		}

		message := fmt.Sprintf(
			"compaction complete (session=%s compacted=%d final=%d)",
			displaySessionID,
			result.CompactedCount,
			result.FinalCount,
		)
		if !result.Compacted {
			message = fmt.Sprintf("compaction skipped (session=%s message_count=%d)", displaySessionID, result.OriginalCount)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"message":        message,
			"session_id":     publicSessionID,
			"compacted":      result.Compacted,
			"original_count": result.OriginalCount,
			"final_count":    result.FinalCount,
		})
	})
}
