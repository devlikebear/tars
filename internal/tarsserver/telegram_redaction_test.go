package tarsserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSanitizeTelegramLogText_RedactsBotToken(t *testing.T) {
	raw := `telegram request failed: Post "https://api.telegram.org/bot12345:ABC/sendMessage": EOF`
	redacted := sanitizeTelegramLogText(raw, "12345:ABC")
	if strings.Contains(redacted, "12345:ABC") {
		t.Fatalf("expected token to be redacted, got %q", redacted)
	}
	if !strings.Contains(redacted, "/bot[REDACTED]/sendMessage") {
		t.Fatalf("expected bot path to be redacted, got %q", redacted)
	}
}

func TestSanitizeTelegramLogText_RedactsURLEncodedToken(t *testing.T) {
	raw := `telegram relay failed: GET "/relay?token=12345%3AABC&chat_id=1"`
	redacted := sanitizeTelegramLogText(raw, "12345:ABC")
	if strings.Contains(strings.ToLower(redacted), "12345%3aabc") {
		t.Fatalf("expected encoded token to be redacted, got %q", redacted)
	}
	if !strings.Contains(redacted, "token=[REDACTED]") {
		t.Fatalf("expected encoded token marker, got %q", redacted)
	}
}

func TestSanitizeTelegramLogText_RedactsJSONEscapedBotPath(t *testing.T) {
	raw := `{"description":"Post \"https:\/\/api.telegram.org\/bot12345:ABC\/sendMessage\": EOF"}`
	redacted := sanitizeTelegramLogText(raw, "12345:ABC")
	if strings.Contains(redacted, "12345:ABC") {
		t.Fatalf("expected token to be redacted, got %q", redacted)
	}
	if !strings.Contains(redacted, `\/bot[REDACTED]\/sendMessage`) {
		t.Fatalf("expected escaped bot path to be redacted, got %q", redacted)
	}
}

func TestTelegramUpdatePoller_FetchUpdates_RedactsTokenInError(t *testing.T) {
	poller := newTelegramUpdatePoller("secret-token", zerolog.New(io.Discard), func(context.Context, telegramUpdate) {})
	if poller == nil {
		t.Fatal("expected poller")
	}
	poller.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf(`Post "%s": read tcp 127.0.0.1:1->127.0.0.1:2: connection reset by peer`, req.URL.String())
		}),
	}
	_, err := poller.fetchUpdates(context.Background(), 0)
	if err == nil {
		t.Fatalf("expected fetchUpdates error")
	}
	message := err.Error()
	if strings.Contains(message, "secret-token") {
		t.Fatalf("expected token to be redacted, got %q", message)
	}
	if !strings.Contains(message, "[REDACTED]") {
		t.Fatalf("expected redaction marker in %q", message)
	}
}
