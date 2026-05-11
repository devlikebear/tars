package tarsserver

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rs/zerolog"
)

// assertFilesystemBrowsePathShape validates that a raw filesystem-browse path
// query is either empty (falls back to home) or an absolute path. This is the
// documented entrypoint sanitizer for the local directory enumeration surface;
// the os.Stat/os.ReadDir calls in newFilesystemBrowseHandler always run on a
// path that has been through this check.
func assertFilesystemBrowsePathShape(raw string) error {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return nil
	}
	if !filepath.IsAbs(candidate) {
		return fmt.Errorf("path must be absolute")
	}
	return nil
}

func newFilesystemBrowseHandler(logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			rawPath := r.URL.Query().Get("path")
			if err := assertFilesystemBrowsePathShape(rawPath); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			dirPath := strings.TrimSpace(rawPath)
			if dirPath == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot resolve home directory"})
					return
				}
				dirPath = home
			}

			info, err := os.Stat(dirPath)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "path not found"})
				return
			}
			if !info.IsDir() {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is not a directory"})
				return
			}

			entries, err := os.ReadDir(dirPath)
			if err != nil {
				logger.Warn().Err(err).Str("path", dirPath).Msg("cannot read directory")
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "cannot read directory"})
				return
			}

			type dirEntry struct {
				Name  string `json:"name"`
				IsDir bool   `json:"is_dir"`
				IsGit bool   `json:"is_git,omitempty"`
			}

			dirs := make([]dirEntry, 0)
			for _, entry := range entries {
				name := entry.Name()
				if shouldHideFilePanelEntry(name) {
					continue
				}
				if !entry.IsDir() {
					continue
				}
				e := dirEntry{Name: name, IsDir: true}
				if _, err := os.Stat(filepath.Join(dirPath, name, ".git")); err == nil {
					e.IsGit = true
				}
				dirs = append(dirs, e)
			}
			sort.Slice(dirs, func(i, j int) bool {
				return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
			})

			parent := filepath.Dir(dirPath)
			if parent == dirPath {
				parent = ""
			}

			writeJSON(w, http.StatusOK, map[string]any{
				"path":    dirPath,
				"parent":  parent,
				"entries": dirs,
			})
		case http.MethodPost:
			var req struct {
				ParentPath string `json:"parent_path"`
				Name       string `json:"name"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			if err := validateWorkspaceDirectoryName(req.Name); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			parentPath := strings.TrimSpace(req.ParentPath)
			if !filepath.IsAbs(parentPath) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parent_path must be absolute"})
				return
			}
			info, err := os.Stat(parentPath)
			if err != nil {
				if os.IsNotExist(err) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "parent directory not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if !info.IsDir() {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parent path is not a directory"})
				return
			}
			targetPath := filepath.Join(parentPath, req.Name)
			if _, err := os.Stat(targetPath); err == nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "directory already exists"})
				return
			} else if !os.IsNotExist(err) {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if err := os.Mkdir(targetPath, 0o755); err != nil {
				if os.IsExist(err) {
					writeJSON(w, http.StatusConflict, map[string]string{"error": "directory already exists"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{
				"path":   targetPath,
				"name":   req.Name,
				"is_dir": true,
			})
		default:
			writeMethodNotAllowed(w)
		}
	})
}
