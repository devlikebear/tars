package tarsserver

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"
)

type fileEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

const (
	workspaceTextPreviewMaxBytes  = 100 * 1024
	workspaceImagePreviewMaxBytes = 8 * 1024 * 1024
)

func workspaceFileKind(name string, raw []byte) (kind string, mimeType string, isBinary bool) {
	ext := strings.ToLower(filepath.Ext(name))
	mimeType = strings.TrimSpace(mime.TypeByExtension(ext))
	if mimeType == "" {
		mimeType = http.DetectContentType(raw)
	}
	if strings.Contains(mimeType, ";") {
		mimeType = strings.TrimSpace(strings.SplitN(mimeType, ";", 2)[0])
	}

	switch ext {
	case ".md", ".markdown", ".mdx":
		return "markdown", firstNonEmpty(mimeType, "text/markdown"), false
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico", ".svg":
		return "image", firstNonEmpty(mimeType, "image/*"), false
	}

	if strings.HasPrefix(mimeType, "image/") {
		return "image", mimeType, false
	}

	if bytes.IndexByte(raw, 0) >= 0 || !utf8.Valid(raw) {
		return "binary", firstNonEmpty(mimeType, "application/octet-stream"), true
	}

	if strings.HasPrefix(mimeType, "text/") ||
		mimeType == "application/json" ||
		mimeType == "application/xml" ||
		strings.HasSuffix(mimeType, "+json") ||
		strings.HasSuffix(mimeType, "+xml") ||
		mimeType == "image/svg+xml" ||
		mimeType == "" {
		return "text", firstNonEmpty(mimeType, "text/plain"), false
	}

	return "binary", firstNonEmpty(mimeType, "application/octet-stream"), true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func truncateWorkspacePreviewText(value string, limit int) (string, bool) {
	if limit <= 0 || len(value) <= limit {
		return value, false
	}
	end := 0
	for idx := range value {
		if idx > limit {
			break
		}
		end = idx
	}
	if end == 0 && len(value) > limit {
		return "", true
	}
	return value[:end], true
}

func workspacePathWithinRoot(root, candidate string) bool {
	rootClean := canonicalWorkspacePath(filepath.Clean(root))
	candidateClean := canonicalWorkspacePath(filepath.Clean(candidate))
	if candidateClean == rootClean {
		return true
	}
	return strings.HasPrefix(candidateClean, rootClean+string(filepath.Separator))
}

func canonicalWorkspacePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}

	current := filepath.Clean(path)
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			out := filepath.Clean(resolved)
			for i := len(suffix) - 1; i >= 0; i-- {
				out = filepath.Join(out, suffix[i])
			}
			return filepath.Clean(out)
		}
		if !os.IsNotExist(err) {
			return filepath.Clean(path)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(path)
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func resolveWorkspaceFilesRoot(workspaceDir string, rootQuery string) string {
	rootDir := strings.TrimSpace(rootQuery)
	if rootDir == "" {
		rootDir = filepath.Join(workspaceDir, "artifacts")
	} else if rootDir == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			rootDir = home
		}
	}
	if !filepath.IsAbs(rootDir) {
		if absRoot, err := filepath.Abs(rootDir); err == nil {
			rootDir = absRoot
		}
	}
	cleanRoot := filepath.Clean(rootDir)
	if cleanRoot != "/" {
		cleanRoot = strings.TrimRight(cleanRoot, "/")
	}
	return cleanRoot
}

func resolveWorkspaceFilesPath(rootDir, rawPath, defaultPath string) (string, string, error) {
	candidate := strings.TrimSpace(rawPath)
	if candidate == "" {
		candidate = defaultPath
	}
	if strings.TrimSpace(candidate) == "" {
		return "", "", fmt.Errorf("path is required")
	}
	cleanPath := filepath.Clean(candidate)
	absPath := cleanPath
	if !filepath.IsAbs(cleanPath) {
		absPath = filepath.Join(rootDir, cleanPath)
	}
	if !workspacePathWithinRoot(rootDir, absPath) {
		return "", "", fmt.Errorf("invalid path")
	}
	return cleanPath, absPath, nil
}

func validateWorkspaceDirectoryName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("name is required")
	}
	if trimmed == "." || trimmed == ".." {
		return fmt.Errorf("invalid directory name")
	}
	if strings.ContainsAny(trimmed, `/\`) {
		return fmt.Errorf("directory name cannot contain path separators")
	}
	return nil
}

func workspaceChildPath(parentPath, childName string) string {
	parentClean := filepath.Clean(strings.TrimSpace(parentPath))
	if parentClean == "." || parentClean == "" {
		return filepath.ToSlash(childName)
	}
	return filepath.ToSlash(filepath.Join(parentClean, childName))
}

func newWorkspaceFilesHandler(workspaceDir string, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rootDir := resolveWorkspaceFilesRoot(workspaceDir, r.URL.Query().Get("root"))

		switch r.Method {
		case http.MethodGet:
			relPath := strings.TrimSpace(r.URL.Query().Get("path"))
			cleanPath, absPath, err := resolveWorkspaceFilesPath(rootDir, relPath, ".")
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
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
				raw, err := os.ReadFile(absPath)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				kind, mimeType, isBinary := workspaceFileKind(absPath, raw)
				payload := map[string]any{
					"path":       relPath,
					"name":       filepath.Base(absPath),
					"size":       info.Size(),
					"updated_at": info.ModTime().UTC().Format(time.RFC3339),
					"kind":       kind,
					"mime_type":  mimeType,
					"is_binary":  isBinary,
				}

				switch kind {
				case "markdown", "text":
					content, truncated := truncateWorkspacePreviewText(string(raw), workspaceTextPreviewMaxBytes)
					if truncated {
						content += "\n... (truncated)"
					}
					payload["encoding"] = "utf-8"
					payload["content"] = content
					if truncated {
						payload["truncated"] = true
						payload["message"] = fmt.Sprintf("Preview truncated to %d bytes.", workspaceTextPreviewMaxBytes)
					}
				case "image":
					if len(raw) > workspaceImagePreviewMaxBytes {
						payload["truncated"] = true
						payload["message"] = fmt.Sprintf("Image preview is limited to %d bytes.", workspaceImagePreviewMaxBytes)
					} else {
						payload["encoding"] = "base64"
						payload["content_base64"] = base64.StdEncoding.EncodeToString(raw)
					}
				default:
					payload["message"] = "Binary file preview is not available."
				}

				writeJSON(w, http.StatusOK, payload)
				return
			}

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
					Path:  filepath.Join(cleanPath, name),
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
				"path":  cleanPath,
				"files": files,
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
			parentPath, parentAbs, err := resolveWorkspaceFilesPath(rootDir, req.ParentPath, ".")
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			info, err := os.Stat(parentAbs)
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
			targetAbs := filepath.Join(parentAbs, req.Name)
			if !workspacePathWithinRoot(rootDir, targetAbs) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
				return
			}
			if _, err := os.Stat(targetAbs); err == nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "directory already exists"})
				return
			} else if !os.IsNotExist(err) {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if err := os.Mkdir(targetAbs, 0o755); err != nil {
				if os.IsExist(err) {
					writeJSON(w, http.StatusConflict, map[string]string{"error": "directory already exists"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{
				"path":   workspaceChildPath(parentPath, req.Name),
				"name":   req.Name,
				"is_dir": true,
			})
		case http.MethodPatch:
			var req struct {
				Path    string `json:"path"`
				NewName string `json:"new_name"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			if err := validateWorkspaceDirectoryName(req.NewName); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			cleanPath, absPath, err := resolveWorkspaceFilesPath(rootDir, req.Path, "")
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			info, err := os.Stat(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "directory not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if !info.IsDir() {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is not a directory"})
				return
			}
			parentPath := filepath.Dir(cleanPath)
			targetAbs := filepath.Join(filepath.Dir(absPath), req.NewName)
			if !workspacePathWithinRoot(rootDir, targetAbs) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
				return
			}
			targetPath := workspaceChildPath(parentPath, req.NewName)
			if filepath.Clean(targetAbs) != filepath.Clean(absPath) {
				if _, err := os.Stat(targetAbs); err == nil {
					writeJSON(w, http.StatusConflict, map[string]string{"error": "directory already exists"})
					return
				} else if !os.IsNotExist(err) {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				if err := os.Rename(absPath, targetAbs); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"path":   targetPath,
				"name":   req.NewName,
				"is_dir": true,
			})
		default:
			writeMethodNotAllowed(w)
		}
	})

	return mux
}
