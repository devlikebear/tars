package tarsserver

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/rs/zerolog"
)

type blockingReadCloser struct {
	payload []byte
	started chan struct{}
	release chan struct{}
	opened  bool
}

func newBlockingReadCloser(payload string) *blockingReadCloser {
	return &blockingReadCloser{
		payload: []byte(payload),
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (b *blockingReadCloser) Read(p []byte) (int, error) {
	if !b.opened {
		b.opened = true
		close(b.started)
		<-b.release
	}
	if len(b.payload) == 0 {
		return 0, io.EOF
	}
	n := copy(p, b.payload)
	b.payload = b.payload[n:]
	return n, nil
}

func (b *blockingReadCloser) Close() error {
	return nil
}

func TestChatAPIHandler_ReturnsOverloadedWhenInflightLimitExceeded(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	logger := zerolog.New(io.Discard)
	client := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{
				Role:    "assistant",
				Content: "ok",
			},
		},
	}

	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		client,
		logger,
		4,
		nil,
		"",
		chatToolingOptions{APIMaxInflightChat: 2},
	)

	blockedA := newBlockingReadCloser(`{"message":"a"}`)
	blockedB := newBlockingReadCloser(`{"message":"b"}`)
	done := make(chan *httptest.ResponseRecorder, 2)
	runBlocked := func(body *blockingReadCloser) {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Body = body
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		done <- rec
	}
	go runBlocked(blockedA)
	go runBlocked(blockedB)

	waitForChannel(t, blockedA.started, "chat blocked request A")
	waitForChannel(t, blockedB.started, "chat blocked request B")

	overflowReq := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"message":"overflow"}`))
	overflowReq.Header.Set("Content-Type", "application/json")
	overflowRec := httptest.NewRecorder()
	handler.ServeHTTP(overflowRec, overflowReq)
	if overflowRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d body=%q", overflowRec.Code, overflowRec.Body.String())
	}
	if !strings.Contains(overflowRec.Body.String(), `"code":"overloaded"`) {
		t.Fatalf("expected overloaded code, got %q", overflowRec.Body.String())
	}

	close(blockedA.release)
	close(blockedB.release)
	for i := 0; i < 2; i++ {
		rec := <-done
		if rec.Code != http.StatusOK {
			t.Fatalf("expected blocked request %d to complete with 200, got %d body=%q", i+1, rec.Code, rec.Body.String())
		}
	}
}

func TestAgentRunsAPIHandler_ReturnsOverloadedWhenInflightLimitExceeded(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	handler := newAgentRunsAPIHandlerWithInflightLimit(runtime, zerolog.New(io.Discard), 2)

	blockedA := newBlockingReadCloser(`{"message":"a"}`)
	blockedB := newBlockingReadCloser(`{"message":"b"}`)
	done := make(chan *httptest.ResponseRecorder, 2)
	runBlocked := func(body *blockingReadCloser) {
		req := httptest.NewRequest(http.MethodPost, "/v1/agent/runs", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Body = body
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		done <- rec
	}
	go runBlocked(blockedA)
	go runBlocked(blockedB)

	waitForChannel(t, blockedA.started, "agent blocked request A")
	waitForChannel(t, blockedB.started, "agent blocked request B")

	overflowReq := httptest.NewRequest(http.MethodPost, "/v1/agent/runs", bytes.NewReader([]byte(`{"message":"overflow"}`)))
	overflowReq.Header.Set("Content-Type", "application/json")
	overflowRec := httptest.NewRecorder()
	handler.ServeHTTP(overflowRec, overflowReq)
	if overflowRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d body=%q", overflowRec.Code, overflowRec.Body.String())
	}
	if !strings.Contains(overflowRec.Body.String(), `"code":"overloaded"`) {
		t.Fatalf("expected overloaded code, got %q", overflowRec.Body.String())
	}

	close(blockedA.release)
	close(blockedB.release)
	for i := 0; i < 2; i++ {
		rec := <-done
		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected blocked request %d to complete with 202, got %d body=%q", i+1, rec.Code, rec.Body.String())
		}
	}
}

func waitForChannel(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}
