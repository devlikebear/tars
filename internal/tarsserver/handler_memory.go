package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/sysprompt"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/rs/zerolog"
)

type managedMemoryAsset struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Editable  bool   `json:"editable"`
	SizeBytes int64  `json:"size_bytes"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func newMemoryAPIHandler(workspaceDir string, backend memory.Backend, logger zerolog.Logger) http.Handler {
	if backend == nil {
		backend = memory.NewFileBackend(workspaceDir, nil)
	}
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/memory/assets", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		items, err := listManagedMemoryAssets(workspaceDir)
		if err != nil {
			logger.Error().Err(err).Msg("list memory assets failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list memory assets failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"count": len(items),
			"items": items,
		})
	})

	mux.HandleFunc("/v1/memory/file", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet, http.MethodPut) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			relPath := strings.TrimSpace(r.URL.Query().Get("path"))
			absPath, kind, err := resolveManagedMemoryPath(workspaceDir, relPath)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			raw, err := os.ReadFile(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "memory file not found"})
					return
				}
				logger.Error().Err(err).Str("path", relPath).Msg("read memory file failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read memory file failed"})
				return
			}
			stat, _ := os.Stat(absPath)
			payload := map[string]any{
				"path":     filepath.ToSlash(relPath),
				"kind":     kind,
				"editable": true,
				"content":  string(raw),
			}
			if stat != nil {
				payload["size_bytes"] = stat.Size()
				payload["updated_at"] = stat.ModTime().UTC().Format(time.RFC3339)
			}
			writeJSON(w, http.StatusOK, payload)
		case http.MethodPut:
			var req struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			absPath, kind, err := resolveManagedMemoryPath(workspaceDir, req.Path)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
				logger.Error().Err(err).Str("path", req.Path).Msg("ensure memory file dir failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "prepare memory file failed"})
				return
			}
			if err := os.WriteFile(absPath, []byte(req.Content), 0o644); err != nil {
				logger.Error().Err(err).Str("path", req.Path).Msg("write memory file failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write memory file failed"})
				return
			}
			stat, _ := os.Stat(absPath)
			payload := map[string]any{
				"path":     filepath.ToSlash(strings.TrimSpace(req.Path)),
				"kind":     kind,
				"editable": true,
				"saved":    true,
			}
			if stat != nil {
				payload["size_bytes"] = stat.Size()
				payload["updated_at"] = stat.ModTime().UTC().Format(time.RFC3339)
			}
			writeJSON(w, http.StatusOK, payload)
		}
	})

	mux.HandleFunc("/v1/memory/search", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var raw json.RawMessage
		if !decodeJSONBody(w, r, &raw) {
			return
		}
		result, err := tool.NewMemorySearchTool(workspaceDir, backend).Execute(context.Background(), raw)
		if err != nil {
			logger.Error().Err(err).Msg("memory search failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "memory search failed"})
			return
		}
		if result.IsError {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimSpace(result.Text())})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(result.Text()))
	})

	mux.HandleFunc("/v1/workspace/sysprompt/files", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		items, err := sysprompt.List(workspaceDir, parseSyspromptScope(r.URL.Query().Get("scope")))
		if err != nil {
			logger.Error().Err(err).Msg("list sysprompt files failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list sysprompt files failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"count": len(items),
			"items": items,
		})
	})

	mux.HandleFunc("/v1/workspace/sysprompt/file", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet, http.MethodPut) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			item, err := sysprompt.Get(
				workspaceDir,
				parseSyspromptScope(r.URL.Query().Get("scope")),
				strings.TrimSpace(r.URL.Query().Get("path")),
			)
			if err != nil {
				if os.IsNotExist(err) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown sysprompt file"})
					return
				}
				logger.Error().Err(err).Msg("read sysprompt file failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read sysprompt file failed"})
				return
			}
			writeJSON(w, http.StatusOK, item)
		case http.MethodPut:
			var req struct {
				Scope   string `json:"scope"`
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			item, err := sysprompt.Save(
				workspaceDir,
				parseSyspromptScope(req.Scope),
				strings.TrimSpace(req.Path),
				req.Content,
			)
			if err != nil {
				if os.IsNotExist(err) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown sysprompt file"})
					return
				}
				logger.Error().Err(err).Msg("write sysprompt file failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write sysprompt file failed"})
				return
			}
			writeJSON(w, http.StatusOK, item)
		}
	})

	mux.HandleFunc("/v1/memory/kb/graph", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		graph, err := backend.KnowledgeGraph(context.Background())
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
			items, err := backend.ListKnowledgeNotes(context.Background(), memory.KnowledgeListOptions{
				Query: strings.TrimSpace(r.URL.Query().Get("query")),
				Kind:  strings.TrimSpace(r.URL.Query().Get("kind")),
				Tag:   strings.TrimSpace(r.URL.Query().Get("tag")),
				Limit: limit,
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
			item, err := backend.ApplyKnowledgePatch(context.Background(), patch)
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
			item, err := backend.GetKnowledgeNote(context.Background(), slug)
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
			item, err := backend.ApplyKnowledgePatch(context.Background(), patch)
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
			if err := backend.DeleteKnowledgeNote(context.Background(), slug); err != nil {
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

func listManagedMemoryAssets(workspaceDir string) ([]managedMemoryAsset, error) {
	paths := []string{"MEMORY.md", filepath.ToSlash(filepath.Join("memory", "experiences.jsonl"))}

	dailyPaths, err := filepath.Glob(filepath.Join(workspaceDir, "memory", "*.md"))
	if err != nil {
		return nil, err
	}
	for _, path := range dailyPaths {
		paths = append(paths, filepath.ToSlash(strings.TrimPrefix(path, workspaceDir+string(filepath.Separator))))
	}

	indexPaths, err := filepath.Glob(filepath.Join(workspaceDir, "memory", "index", "*"))
	if err != nil {
		return nil, err
	}
	for _, path := range indexPaths {
		info, statErr := os.Stat(path)
		if statErr != nil || info.IsDir() {
			continue
		}
		paths = append(paths, filepath.ToSlash(strings.TrimPrefix(path, workspaceDir+string(filepath.Separator))))
	}

	rawPaths, err := filepath.Glob(filepath.Join(workspaceDir, "memory", "raw", "*"))
	if err != nil {
		return nil, err
	}
	for _, path := range rawPaths {
		info, statErr := os.Stat(path)
		if statErr != nil || info.IsDir() {
			continue
		}
		paths = append(paths, filepath.ToSlash(strings.TrimPrefix(path, workspaceDir+string(filepath.Separator))))
	}

	seen := map[string]struct{}{}
	items := make([]managedMemoryAsset, 0, len(paths))
	for _, relPath := range paths {
		relPath = filepath.ToSlash(strings.TrimSpace(relPath))
		if relPath == "" {
			continue
		}
		if _, exists := seen[relPath]; exists {
			continue
		}
		seen[relPath] = struct{}{}
		absPath, kind, err := resolveManagedMemoryPath(workspaceDir, relPath)
		if err != nil {
			continue
		}
		stat, err := os.Stat(absPath)
		if err != nil || stat.IsDir() {
			continue
		}
		items = append(items, managedMemoryAsset{
			Path:      relPath,
			Kind:      kind,
			Editable:  true,
			SizeBytes: stat.Size(),
			UpdatedAt: stat.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].UpdatedAt == items[j].UpdatedAt {
			return items[i].Path < items[j].Path
		}
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	return items, nil
}

func resolveManagedMemoryPath(workspaceDir, relPath string) (string, string, error) {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if relPath == "" {
		return "", "", http.ErrMissingFile
	}
	if strings.HasPrefix(relPath, "/") || strings.Contains(relPath, "..") {
		return "", "", os.ErrPermission
	}
	switch {
	case relPath == "MEMORY.md":
		return filepath.Join(workspaceDir, "MEMORY.md"), "long_term_memory", nil
	case relPath == "memory/experiences.jsonl":
		return filepath.Join(workspaceDir, filepath.FromSlash(relPath)), "experience_log", nil
	case strings.HasPrefix(relPath, "memory/wiki/"):
		return "", "", os.ErrPermission
	case strings.HasPrefix(relPath, "memory/index/"):
		return filepath.Join(workspaceDir, filepath.FromSlash(relPath)), "semantic_index", nil
	case strings.HasPrefix(relPath, "memory/raw/"):
		return filepath.Join(workspaceDir, filepath.FromSlash(relPath)), "semantic_raw", nil
	case strings.HasPrefix(relPath, "memory/") && strings.HasSuffix(relPath, ".md"):
		base := filepath.Base(relPath)
		if strings.Count(base, "-") == 2 && len(strings.TrimSuffix(base, ".md")) == len("2006-01-02") {
			return filepath.Join(workspaceDir, filepath.FromSlash(relPath)), "daily_memory", nil
		}
	}
	return "", "", os.ErrNotExist
}

func parseSyspromptScope(raw string) sysprompt.Scope {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ""
	case string(sysprompt.ScopeAgent):
		return sysprompt.ScopeAgent
	case string(sysprompt.ScopeWorkspace):
		return sysprompt.ScopeWorkspace
	default:
		return ""
	}
}
