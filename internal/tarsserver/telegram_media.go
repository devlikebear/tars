package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const telegramMediaMaxBytes = 20 * 1024 * 1024

type telegramInboundMedia struct {
	Type         string
	FileID       string
	OriginalName string
	MimeType     string
	FileSize     int64
}

type telegramSavedMedia struct {
	Type         string
	SavedPath    string
	MimeType     string
	Size         int64
	OriginalName string
}

type telegramMediaDownloader interface {
	DownloadAndSave(ctx context.Context, chatID string, media telegramInboundMedia) (telegramSavedMedia, error)
}

type telegramMediaDownloadFunc func(ctx context.Context, chatID string, media telegramInboundMedia) (telegramSavedMedia, error)

func (f telegramMediaDownloadFunc) DownloadAndSave(ctx context.Context, chatID string, media telegramInboundMedia) (telegramSavedMedia, error) {
	if f == nil {
		return telegramSavedMedia{}, fmt.Errorf("telegram media downloader is not configured")
	}
	return f(ctx, chatID, media)
}

type telegramHTTPMediaDownloader struct {
	botToken     string
	workspaceDir string
	baseURL      string
	client       *http.Client
}

func newTelegramMediaDownloader(botToken, workspaceDir string) telegramMediaDownloader {
	trimmedToken := strings.TrimSpace(botToken)
	trimmedWorkspace := strings.TrimSpace(workspaceDir)
	if trimmedToken == "" || trimmedWorkspace == "" {
		return nil
	}
	return &telegramHTTPMediaDownloader{
		botToken:     trimmedToken,
		workspaceDir: trimmedWorkspace,
		baseURL:      "https://api.telegram.org",
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (d *telegramHTTPMediaDownloader) DownloadAndSave(ctx context.Context, chatID string, media telegramInboundMedia) (telegramSavedMedia, error) {
	if d == nil || strings.TrimSpace(d.botToken) == "" {
		return telegramSavedMedia{}, fmt.Errorf("telegram media downloader is not configured")
	}
	if strings.TrimSpace(media.FileID) == "" {
		return telegramSavedMedia{}, fmt.Errorf("telegram file_id is required")
	}
	if media.FileSize > telegramMediaMaxBytes {
		return telegramSavedMedia{}, fmt.Errorf("media file too large")
	}

	filePath, err := d.getFilePath(ctx, media.FileID)
	if err != nil {
		return telegramSavedMedia{}, err
	}
	data, err := d.downloadFile(ctx, filePath)
	if err != nil {
		return telegramSavedMedia{}, err
	}
	if int64(len(data)) > telegramMediaMaxBytes {
		return telegramSavedMedia{}, fmt.Errorf("media file too large")
	}
	savedPath, originalName, err := d.writeFile(chatID, media, filePath, data)
	if err != nil {
		return telegramSavedMedia{}, err
	}
	mimeType := strings.TrimSpace(media.MimeType)
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(savedPath)))
	}
	return telegramSavedMedia{
		Type:         strings.TrimSpace(media.Type),
		SavedPath:    savedPath,
		MimeType:     strings.TrimSpace(mimeType),
		Size:         int64(len(data)),
		OriginalName: originalName,
	}, nil
}

func (d *telegramHTTPMediaDownloader) getFilePath(ctx context.Context, fileID string) (string, error) {
	body, err := json.Marshal(map[string]any{"file_id": strings.TrimSpace(fileID)})
	if err != nil {
		return "", fmt.Errorf("encode getFile request: %w", err)
	}
	endpoint := strings.TrimRight(d.baseURL, "/") + "/bot" + d.botToken + "/getFile"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build getFile request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("telegram getFile request failed: %w", err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("telegram getFile status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(bytes.NewReader(payload)).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode getFile response: %w", err)
	}
	if !parsed.OK {
		return "", fmt.Errorf("telegram getFile failed: %s", strings.TrimSpace(parsed.Description))
	}
	filePath := strings.TrimSpace(parsed.Result.FilePath)
	if filePath == "" {
		return "", fmt.Errorf("telegram getFile response missing file_path")
	}
	return filePath, nil
}

func (d *telegramHTTPMediaDownloader) downloadFile(ctx context.Context, filePath string) ([]byte, error) {
	endpoint := strings.TrimRight(d.baseURL, "/") + "/file/bot" + d.botToken + "/" + strings.TrimLeft(strings.TrimSpace(filePath), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build telegram file download request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram file download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("telegram file download status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	limited := io.LimitReader(resp.Body, telegramMediaMaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read telegram file content: %w", err)
	}
	if int64(len(data)) > telegramMediaMaxBytes {
		return nil, fmt.Errorf("media file too large")
	}
	return data, nil
}

func (d *telegramHTTPMediaDownloader) writeFile(chatID string, media telegramInboundMedia, filePath string, data []byte) (savedPath string, originalName string, err error) {
	day := time.Now().UTC().Format("20060102")
	chatPath := "chat_" + sanitizeTelegramPathToken(chatID)
	if strings.TrimSpace(chatPath) == "chat_" {
		chatPath = "chat_unknown"
	}
	dir := filepath.Join(d.workspaceDir, "telegram", "media", day, chatPath)
	if err := ensureDir(dir); err != nil {
		return "", "", err
	}
	originalName = strings.TrimSpace(media.OriginalName)
	if originalName == "" {
		originalName = strings.TrimSpace(filepath.Base(filePath))
	}
	originalName = sanitizeTelegramFilename(originalName)
	if originalName == "" {
		originalName = defaultTelegramMediaFilename(media)
	}
	filename := fmt.Sprintf("%d_%s", time.Now().UTC().Unix(), originalName)
	path := filepath.Join(dir, filename)
	if err := writeFile(path, data); err != nil {
		return "", "", err
	}
	return path, originalName, nil
}

var telegramFilenameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeTelegramFilename(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, string(filepath.Separator), "_")
	trimmed = telegramFilenameSanitizer.ReplaceAllString(trimmed, "_")
	trimmed = strings.Trim(trimmed, "._-")
	if trimmed == "" {
		return ""
	}
	return trimmed
}

func sanitizeTelegramPathToken(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = telegramFilenameSanitizer.ReplaceAllString(trimmed, "_")
	return strings.Trim(trimmed, "._-")
}

func defaultTelegramMediaFilename(media telegramInboundMedia) string {
	kind := strings.TrimSpace(strings.ToLower(media.Type))
	if kind == "" {
		kind = "file"
	}
	ext := ""
	if mimeType := strings.TrimSpace(media.MimeType); mimeType != "" {
		if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
			ext = exts[0]
		}
	}
	if ext == "" {
		switch kind {
		case "photo":
			ext = ".jpg"
		case "voice":
			ext = ".ogg"
		default:
			ext = ".bin"
		}
	}
	return kind + ext
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create telegram media directory: %w", err)
	}
	return nil
}

func writeFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write telegram media file: %w", err)
	}
	return nil
}
