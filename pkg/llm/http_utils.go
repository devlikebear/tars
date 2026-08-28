package llm

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/secrets"
	zlog "github.com/rs/zerolog/log"
)

const (
	defaultHTTPTimeout = 30 * time.Second
	sseBufferSize      = 1024
	sseMaxBufferSize   = 1024 * 1024
	maxDebugPayloadLen = 100000
)

func checkHTTPStatus(resp *http.Response, label string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	logLLMResponsePayload(label, resp.StatusCode, string(respBody))

	return newHTTPError(label, resp.StatusCode, string(respBody))
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func createSSEScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, sseBufferSize), sseMaxBufferSize)
	return scanner
}

func logLLMRequestPayload(provider string, payload []byte) {
	if len(payload) == 0 {
		return
	}
	logLLMPayload(provider, "llm request payload", string(payload))
}

func logLLMResponsePayload(provider string, status int, payload string) {
	evt := zlog.Debug().Str("provider", strings.TrimSpace(provider)).Int("status", status).Int("payload_len", len(payload))
	if strings.TrimSpace(payload) != "" {
		evt = evt.Str("payload", truncateForLog(secrets.RedactText(payload), maxDebugPayloadLen))
	}
	evt.Msg("llm response payload")
}

func logLLMStreamPayload(provider, payload string) {
	if strings.TrimSpace(payload) == "" {
		return
	}
	logLLMPayload(provider, "llm stream payload", payload)
}

func logLLMPayload(provider, message, payload string) {
	evt := zlog.Debug().Str("provider", strings.TrimSpace(provider)).Int("payload_len", len(payload))
	if strings.TrimSpace(payload) != "" {
		evt = evt.Str("payload", truncateForLog(secrets.RedactText(payload), maxDebugPayloadLen))
	}
	evt.Msg(strings.TrimSpace(message))
}
