package tarsserver

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rs/zerolog"
)

func newFilesystemBrowseHandler(logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		dirPath := strings.TrimSpace(r.URL.Query().Get("path"))
		if dirPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot resolve home directory"})
				return
			}
			dirPath = home
		}

		if !filepath.IsAbs(dirPath) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path must be absolute"})
			return
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
			if strings.HasPrefix(name, ".") {
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
	})
}
