package tarsserver

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/rs/zerolog"
)

func newMemoryAPIHandler(workspaceDir string, semantic *memory.Service, logger zerolog.Logger) http.Handler {
	store := memory.NewKnowledgeStore(workspaceDir, semantic)
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/memory/kb/graph", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		graph, err := store.Graph()
		if err != nil {
			logger.Error().Err(err).Msg("get knowledge graph failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get knowledge graph failed"})
			return
		}
		writeJSON(w, http.StatusOK, graph)
	})

	mux.HandleFunc("/v1/memory/kb/notes", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
			items, err := store.List(memory.KnowledgeListOptions{
				Query:     strings.TrimSpace(r.URL.Query().Get("query")),
				Kind:      strings.TrimSpace(r.URL.Query().Get("kind")),
				Tag:       strings.TrimSpace(r.URL.Query().Get("tag")),
				ProjectID: strings.TrimSpace(r.URL.Query().Get("project_id")),
				Limit:     limit,
			})
			if err != nil {
				logger.Error().Err(err).Msg("list knowledge notes failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list knowledge notes failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"count": len(items),
				"items": items,
			})
		case http.MethodPost:
			patch, ok := decodeKnowledgePatchRequest(w, r)
			if !ok {
				return
			}
			patch.UpdatedAt = time.Now().UTC()
			item, err := store.ApplyPatch(patch)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "required") || strings.Contains(strings.ToLower(err.Error()), "not found") {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				logger.Error().Err(err).Msg("upsert knowledge note failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "upsert knowledge note failed"})
				return
			}
			writeJSON(w, http.StatusOK, item)
		}
	})

	mux.HandleFunc("/v1/memory/kb/notes/", func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/memory/kb/notes/"))
		if slug == "" {
			http.NotFound(w, r)
			return
		}
		if !requireMethod(w, r, http.MethodGet, http.MethodPatch, http.MethodDelete) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			item, err := store.Get(slug)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "knowledge note not found"})
				return
			}
			writeJSON(w, http.StatusOK, item)
		case http.MethodPatch:
			patch, ok := decodeKnowledgePatchRequest(w, r)
			if !ok {
				return
			}
			patch.Slug = slug
			patch.UpdatedAt = time.Now().UTC()
			item, err := store.ApplyPatch(patch)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
					return
				}
				if strings.Contains(strings.ToLower(err.Error()), "required") {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				logger.Error().Err(err).Msg("patch knowledge note failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "patch knowledge note failed"})
				return
			}
			writeJSON(w, http.StatusOK, item)
		case http.MethodDelete:
			if err := store.Delete(slug); err != nil {
				logger.Error().Err(err).Msg("delete knowledge note failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete knowledge note failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "slug": slug})
		}
	})

	return mux
}

func decodeKnowledgePatchRequest(w http.ResponseWriter, r *http.Request) (memory.KnowledgeNotePatch, bool) {
	var req struct {
		Slug          string                  `json:"slug,omitempty"`
		Title         *string                 `json:"title,omitempty"`
		Kind          *string                 `json:"kind,omitempty"`
		Summary       *string                 `json:"summary,omitempty"`
		Body          *string                 `json:"body,omitempty"`
		Tags          *[]string               `json:"tags,omitempty"`
		Aliases       *[]string               `json:"aliases,omitempty"`
		Links         *[]memory.KnowledgeLink `json:"links,omitempty"`
		ProjectID     *string                 `json:"project_id,omitempty"`
		SourceSession *string                 `json:"source_session,omitempty"`
	}
	if !decodeJSONBody(w, r, &req) {
		return memory.KnowledgeNotePatch{}, false
	}
	return memory.KnowledgeNotePatch{
		Slug:          strings.TrimSpace(req.Slug),
		Title:         trimOptionalString(req.Title),
		Kind:          trimOptionalString(req.Kind),
		Summary:       trimOptionalString(req.Summary),
		Body:          trimOptionalString(req.Body),
		Tags:          trimOptionalStringSlice(req.Tags),
		Aliases:       trimOptionalStringSlice(req.Aliases),
		Links:         req.Links,
		ProjectID:     trimOptionalString(req.ProjectID),
		SourceSession: trimOptionalString(req.SourceSession),
	}, true
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func trimOptionalStringSlice(value *[]string) *[]string {
	if value == nil {
		return nil
	}
	out := make([]string, 0, len(*value))
	for _, item := range *value {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return &out
}
