package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestTelegramMedia_DownloadAndSave_Success(t *testing.T) {
	content := []byte("telegram-media-content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/bottest-token/getFile":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"file_path": "docs/report.txt",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/file/bottest-token/docs/report.txt":
			_, _ = w.Write(content)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	workspace := t.TempDir()
	downloader := newTelegramMediaDownloader("test-token", workspace)
	httpDownloader, ok := downloader.(*telegramHTTPMediaDownloader)
	if !ok {
		t.Fatalf("expected *telegramHTTPMediaDownloader, got %T", downloader)
	}
	httpDownloader.baseURL = server.URL

	saved, err := httpDownloader.DownloadAndSave(context.Background(), "101", telegramInboundMedia{
		Type:         "document",
		FileID:       "file-1",
		OriginalName: "report.txt",
		MimeType:     "text/plain",
		FileSize:     int64(len(content)),
	})
	if err != nil {
		t.Fatalf("DownloadAndSave: %v", err)
	}
	if !strings.Contains(saved.SavedPath, "/telegram/media/") {
		t.Fatalf("expected telegram media path, got %q", saved.SavedPath)
	}
	data, err := os.ReadFile(saved.SavedPath)
	if err != nil {
		t.Fatalf("read saved media file: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("saved media content mismatch")
	}
}

func TestTelegramMedia_DownloadAndSave_TooLarge(t *testing.T) {
	workspace := t.TempDir()
	downloader := newTelegramMediaDownloader("test-token", workspace)
	httpDownloader, ok := downloader.(*telegramHTTPMediaDownloader)
	if !ok {
		t.Fatalf("expected *telegramHTTPMediaDownloader, got %T", downloader)
	}
	_, err := httpDownloader.DownloadAndSave(context.Background(), "101", telegramInboundMedia{
		Type:     "voice",
		FileID:   "voice-1",
		MimeType: "audio/ogg",
		FileSize: telegramMediaMaxBytes + 1,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "too large") {
		t.Fatalf("expected too-large error, got %v", err)
	}
}
