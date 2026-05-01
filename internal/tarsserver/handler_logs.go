package tarsserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
)

const (
	defaultLogsLineCount = 100
	maxLogsLineCount     = 500
	maxLogsScanLines     = 5000
	logTailChunkSize     = 32 * 1024
)

type logFileOption struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	SizeBytes int64  `json:"size_bytes"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type logLineView struct {
	Raw       string         `json:"raw"`
	Level     string         `json:"level,omitempty"`
	Component string         `json:"component,omitempty"`
	Message   string         `json:"message,omitempty"`
	Time      string         `json:"time,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

type logsAPIResponse struct {
	Files          []logFileOption `json:"files"`
	SelectedFile   string          `json:"selected_file"`
	Lines          []logLineView   `json:"lines"`
	Count          int             `json:"count"`
	LinesRequested int             `json:"lines_requested"`
	Level          string          `json:"level"`
	Component      string          `json:"component"`
}

func newLogsAPIHandler(workspaceDir string, runtimeLogPath string, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/admin/logs", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}

		files := buildLogFileOptions(workspaceDir, runtimeLogPath)
		fileID := strings.TrimSpace(r.URL.Query().Get("file"))
		if fileID == "" {
			fileID = "runtime"
		}
		selected, ok := findLogFileOption(files, fileID)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown log file"})
			return
		}

		lineCount, ok := parseLogsLineCount(w, r)
		if !ok {
			return
		}
		level, ok := parseLogsLevel(w, r)
		if !ok {
			return
		}
		component := strings.TrimSpace(r.URL.Query().Get("component"))

		rawLines, err := tailLogFileLines(selected.Path, scanLineCount(lineCount, level, component))
		if err != nil {
			logger.Error().Err(err).Str("file_id", fileID).Str("path", selected.Path).Msg("read log file failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read log file failed"})
			return
		}

		lines := filterLogLines(rawLines, level, component, lineCount)
		writeJSON(w, http.StatusOK, logsAPIResponse{
			Files:          files,
			SelectedFile:   selected.ID,
			Lines:          lines,
			Count:          len(lines),
			LinesRequested: lineCount,
			Level:          level,
			Component:      component,
		})
	})
	return mux
}

func buildLogFileOptions(workspaceDir string, runtimeLogPath string) []logFileOption {
	seen := map[string]struct{}{}
	var options []logFileOption
	add := func(id, label, path string) {
		id = strings.TrimSpace(id)
		label = strings.TrimSpace(label)
		path = normalizeRuntimeLogFilePath(path)
		if id == "" || path == "" {
			return
		}
		key := canonicalLogPathKey(path)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		options = append(options, logFileOptionFromPath(id, label, path))
	}

	add("runtime", "Runtime log", runtimeLogPath)
	if strings.TrimSpace(workspaceDir) != "" {
		add("workspace", "Workspace tars.log", filepath.Join(workspaceDir, "logs", "tars.log"))
		for _, opt := range discoverWorkspaceLogFileOptions(workspaceDir, seen) {
			options = append(options, opt)
		}
	}
	add("debug", "Debug log", ".logs/tars-debug.log")

	return options
}

func discoverWorkspaceLogFileOptions(workspaceDir string, seen map[string]struct{}) []logFileOption {
	logDir := filepath.Join(workspaceDir, "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".log") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	var options []logFileOption
	for _, name := range names {
		path := filepath.Join(logDir, name)
		key := canonicalLogPathKey(path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		id := "workspace:" + name
		options = append(options, logFileOptionFromPath(id, "workspace/logs/"+name, path))
	}
	return options
}

func logFileOptionFromPath(id, label, path string) logFileOption {
	opt := logFileOption{
		ID:    id,
		Label: label,
		Path:  path,
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return opt
	}
	opt.Exists = true
	opt.SizeBytes = info.Size()
	opt.UpdatedAt = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
	return opt
}

func canonicalLogPathKey(path string) string {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

func findLogFileOption(options []logFileOption, id string) (logFileOption, bool) {
	for _, option := range options {
		if option.ID == id {
			return option, true
		}
	}
	return logFileOption{}, false
}

func parseLogsLineCount(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("lines"))
	if raw == "" {
		return defaultLogsLineCount, true
	}
	lines, err := strconv.Atoi(raw)
	if err != nil || lines <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lines must be a positive integer"})
		return 0, false
	}
	if lines > maxLogsLineCount {
		lines = maxLogsLineCount
	}
	return lines, true
}

func parseLogsLevel(w http.ResponseWriter, r *http.Request) (string, bool) {
	level := normalizeLogLevelFilter(r.URL.Query().Get("level"))
	switch level {
	case "all", "trace", "debug", "info", "warn", "error":
		return level, true
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "level must be ALL, TRACE, DEBUG, INFO, WARN, or ERROR"})
		return "", false
	}
}

func normalizeLogLevelFilter(level string) string {
	switch strings.TrimSpace(strings.ToLower(level)) {
	case "", "all":
		return "all"
	case "warning":
		return "warn"
	default:
		return strings.TrimSpace(strings.ToLower(level))
	}
}

func scanLineCount(requested int, level string, component string) int {
	if requested <= 0 {
		requested = defaultLogsLineCount
	}
	if level != "all" || strings.TrimSpace(component) != "" {
		requested *= 10
	}
	if requested > maxLogsScanLines {
		return maxLogsScanLines
	}
	return requested
}

func tailLogFileLines(path string, limit int) ([]string, error) {
	if strings.TrimSpace(path) == "" || limit <= 0 {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if stat.Size() == 0 {
		return nil, nil
	}

	var data []byte
	var offset = stat.Size()
	newlines := 0
	for offset > 0 && newlines <= limit {
		readSize := int64(logTailChunkSize)
		if offset < readSize {
			readSize = offset
		}
		offset -= readSize
		chunk := make([]byte, readSize)
		if _, err := file.ReadAt(chunk, offset); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		newlines += bytes.Count(chunk, []byte{'\n'})
		data = append(chunk, data...)
	}

	parts := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(parts) <= limit {
		return parts, nil
	}
	return parts[len(parts)-limit:], nil
}

func filterLogLines(rawLines []string, level string, component string, limit int) []logLineView {
	component = strings.TrimSpace(strings.ToLower(component))
	level = normalizeLogLevelFilter(level)
	if limit <= 0 {
		limit = defaultLogsLineCount
	}

	var filtered []logLineView
	for _, raw := range rawLines {
		line := parseLogLine(raw)
		if line.Raw == "" {
			continue
		}
		if level != "all" && line.Level != level {
			continue
		}
		if component != "" && !strings.Contains(strings.ToLower(line.Component), component) {
			continue
		}
		filtered = append(filtered, line)
	}
	if len(filtered) <= limit {
		return filtered
	}
	return filtered[len(filtered)-limit:]
}

func parseLogLine(raw string) logLineView {
	raw = strings.TrimRight(raw, "\r")
	if strings.TrimSpace(raw) == "" {
		return logLineView{}
	}
	line := logLineView{Raw: raw}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return line
	}

	line.Level = normalizeLogLevelFilter(stringField(payload, "level"))
	if line.Level == "all" {
		line.Level = ""
	}
	line.Component = stringField(payload, "component")
	line.Message = firstStringField(payload, "message", "msg")
	line.Time = stringField(payload, "time")
	fields := map[string]any{}
	for key, value := range payload {
		switch key {
		case "level", "component", "message", "msg", "time":
			continue
		default:
			fields[key] = value
		}
	}
	if len(fields) > 0 {
		line.Fields = fields
	}
	return line
}

func stringField(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func firstStringField(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringField(payload, key); value != "" {
			return value
		}
	}
	return ""
}
