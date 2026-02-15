package llm

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultHTTPTimeout = 30 * time.Second
	sseBufferSize      = 1024
	sseMaxBufferSize   = 1024 * 1024
)

func checkHTTPStatus(resp *http.Response, label string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

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
