package tarsserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestTelegramUpdatePoller_FetchUpdates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/bottest-token/getUpdates" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": []map[string]any{
				{
					"update_id": 5,
					"message": map[string]any{
						"text": "hello",
						"chat": map[string]any{
							"id":   101,
							"type": "private",
						},
						"from": map[string]any{
							"id":       11,
							"username": "alice",
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	poller := newTelegramUpdatePoller("test-token", zerolog.New(io.Discard), func(context.Context, telegramUpdate) {})
	if poller == nil {
		t.Fatal("expected poller")
	}
	poller.baseURL = server.URL
	updates, err := poller.fetchUpdates(context.Background(), 0)
	if err != nil {
		t.Fatalf("fetchUpdates: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if updates[0].UpdateID != 5 {
		t.Fatalf("expected update_id=5, got %+v", updates[0])
	}
	if updates[0].Message == nil || updates[0].Message.Chat.IDString() != "101" {
		t.Fatalf("unexpected update payload: %+v", updates[0])
	}
}

func TestTelegramUpdatePoller_FetchUpdates_UsesPersistedOffset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/bottest-token/getUpdates" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got := int64(body["offset"].(float64)); got != 8 {
			t.Fatalf("expected offset=8, got %+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": []map[string]any{
				{"update_id": 8},
			},
		})
	}))
	defer server.Close()

	var saved int64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poller := newTelegramUpdatePoller("test-token", zerolog.New(io.Discard), func(context.Context, telegramUpdate) {
		cancel()
	})
	if poller == nil {
		t.Fatal("expected poller")
	}
	poller.baseURL = server.URL
	poller.withOffsetStore(
		func() int64 { return 7 },
		func(v int64) error {
			saved = v
			return nil
		},
	)
	done := make(chan struct{})
	go func() {
		poller.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("poller did not stop in time")
	}
	if saved != 8 {
		t.Fatalf("expected saved last_update_id=8, got %d", saved)
	}
}
