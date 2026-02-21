package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTelegramSender_SendChatAction_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/bottest-token/sendChatAction" {
			http.NotFound(w, r)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["chat_id"] != "101" {
			t.Fatalf("unexpected chat_id payload: %+v", payload)
		}
		if payload["action"] != "typing" {
			t.Fatalf("unexpected action payload: %+v", payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": true,
		})
	}))
	defer server.Close()

	sender := newTelegramSender("test-token")
	httpSender, ok := sender.(*telegramHTTPSender)
	if !ok {
		t.Fatalf("expected *telegramHTTPSender, got %T", sender)
	}
	httpSender.baseURL = server.URL
	if err := httpSender.SendChatAction(context.Background(), telegramChatActionRequest{
		ChatID:   "101",
		ThreadID: "",
		Action:   "typing",
	}); err != nil {
		t.Fatalf("SendChatAction: %v", err)
	}
}
