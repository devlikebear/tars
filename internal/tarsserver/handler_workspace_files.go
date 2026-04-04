package tarsserver

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

type fileEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func newWorkspaceFilesHandler(workspaceDir string, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		relPath := strings.TrimSpace(r.URL.Query().Get("path"))
		if relPath == "" {
			relPath = "."
		}

		// Prevent path traversal
		cleanRoot := strings.TrimRight(filepath.Clean(workspaceDir), "/")
		absPath := filepath.Join(cleanRoot, filepath.Clean(relPath))
		if !strings.HasPrefix(absPath, cleanRoot) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
			return
		}

		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if !info.IsDir() {
			// Read file content
			raw, err := os.ReadFile(absPath)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			// Limit to 100KB for preview
			content := string(raw)
			if len(content) > 100*1024 {
				content = content[:100*1024] + "\n... (truncated)"
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"path":       relPath,
				"name":       filepath.Base(absPath),
				"size":       info.Size(),
				"updated_at": info.ModTime().UTC().Format(time.RFC3339),
				"content":    content,
			})
			return
		}

		// List directory
		entries, err := os.ReadDir(absPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		files := make([]fileEntry, 0, len(entries))
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				continue
			}
			fe := fileEntry{
				Name:  name,
				Path:  filepath.Join(relPath, name),
				IsDir: e.IsDir(),
			}
			if info, err := e.Info(); err == nil {
				fe.Size = info.Size()
				fe.UpdatedAt = info.ModTime().UTC().Format(time.RFC3339)
			}
			files = append(files, fe)
		}
		sort.Slice(files, func(i, j int) bool {
			if files[i].IsDir != files[j].IsDir {
				return files[i].IsDir
			}
			return files[i].Name < files[j].Name
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"path":  relPath,
			"files": files,
		})
	})

	return mux
}
