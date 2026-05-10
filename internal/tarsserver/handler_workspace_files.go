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

func shouldHideFilePanelEntry(name string) bool {
	return name == "node_modules"
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

func resolveWorkspaceFilesRootPath(rootDir, rawPath, defaultPath string) (string, string, error) {
	cleanPath, absPath, err := resolveWorkspaceFilesPath(rootDir, rawPath, defaultPath)
	if err != nil {
		return "", "", err
	}
	rootCanonical := canonicalWorkspacePath(rootDir)
	pathCanonical := canonicalWorkspacePath(absPath)
	rootPath, err := filepath.Rel(rootCanonical, pathCanonical)
	if err != nil {
		return "", "", fmt.Errorf("invalid path")
	}
	if rootPath == "." {
		return cleanPath, ".", nil
	}
	if !filepath.IsLocal(rootPath) {
		return "", "", fmt.Errorf("invalid path")
	}
	return cleanPath, rootPath, nil
}

func openWorkspaceFilesRoot(w http.ResponseWriter, rootDir string, notFoundMessage string) (*os.Root, bool) {
	rootFS, err := os.OpenRoot(rootDir)
	if err == nil {
		return rootFS, true
	}
	status := http.StatusInternalServerError
	message := err.Error()
	if os.IsNotExist(err) {
		status = http.StatusNotFound
		message = notFoundMessage
	}
	writeJSON(w, status, map[string]string{"error": message})
	return nil, false
}

func resolveWorkspaceFilesRootEntry(w http.ResponseWriter, rootDir, rawPath, defaultPath, notFoundMessage string) (string, string, *os.Root, os.FileInfo, bool) {
	cleanPath, rootPath, err := resolveWorkspaceFilesRootPath(rootDir, rawPath, defaultPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return "", "", nil, nil, false
	}
	rootFS, ok := openWorkspaceFilesRoot(w, rootDir, notFoundMessage)
	if !ok {
		return "", "", nil, nil, false
	}
	info, err := rootFS.Stat(rootPath)
	if err == nil {
		return cleanPath, rootPath, rootFS, info, true
	}
	_ = rootFS.Close()
	if os.IsNotExist(err) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": notFoundMessage})
		return "", "", nil, nil, false
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	return "", "", nil, nil, false
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
			cleanPath, rootPath, rootFS, info, ok := resolveWorkspaceFilesRootEntry(w, rootDir, relPath, ".", "not found")
			if !ok {
				return
			}
			defer func() { _ = rootFS.Close() }()

			if !info.IsDir() {
				raw, err := rootFS.ReadFile(rootPath)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				kind, mimeType, isBinary := workspaceFileKind(cleanPath, raw)
				payload := map[string]any{
					"path":       cleanPath,
					"name":       filepath.Base(cleanPath),
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

			dir, err := rootFS.Open(rootPath)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			defer func() { _ = dir.Close() }()
			entries, err := dir.ReadDir(-1)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}

			files := make([]fileEntry, 0, len(entries))
			for _, e := range entries {
				name := e.Name()
				if shouldHideFilePanelEntry(name) {
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
			parentPath, parentRootPath, rootFS, info, ok := resolveWorkspaceFilesRootEntry(w, rootDir, req.ParentPath, ".", "parent directory not found")
			if !ok {
				return
			}
			defer func() { _ = rootFS.Close() }()

			if !info.IsDir() {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parent path is not a directory"})
				return
			}
			targetRootPath := filepath.Join(parentRootPath, req.Name)
			if _, err := rootFS.Stat(targetRootPath); err == nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "directory already exists"})
				return
			} else if !os.IsNotExist(err) {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if err := rootFS.Mkdir(targetRootPath, 0o755); err != nil {
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
			cleanPath, rootPath, rootFS, info, ok := resolveWorkspaceFilesRootEntry(w, rootDir, req.Path, "", "directory not found")
			if !ok {
				return
			}
			defer func() { _ = rootFS.Close() }()

			if rootPath == "." {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
				return
			}
			if !info.IsDir() {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is not a directory"})
				return
			}
			parentPath := filepath.Dir(cleanPath)
			targetRootPath := filepath.Join(filepath.Dir(rootPath), req.NewName)
			targetPath := workspaceChildPath(parentPath, req.NewName)
			if filepath.Clean(targetRootPath) != filepath.Clean(rootPath) {
				if _, err := rootFS.Stat(targetRootPath); err == nil {
					writeJSON(w, http.StatusConflict, map[string]string{"error": "directory already exists"})
					return
				} else if !os.IsNotExist(err) {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				if err := rootFS.Rename(rootPath, targetRootPath); err != nil {
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
