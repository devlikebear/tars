package tarsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	serverURL     string
	apiToken      string
	adminAPIToken string
	httpClient    *http.Client
}

func New(cfg Config) *Client {
	return &Client{
		serverURL:     strings.TrimSpace(cfg.ServerURL),
		apiToken:      strings.TrimSpace(cfg.APIToken),
		adminAPIToken: strings.TrimSpace(cfg.AdminAPIToken),
		httpClient:    cfg.HTTPClient,
	}
}

func (c *Client) Do(ctx context.Context, method, path string, body any, out any) error {
	_, err := c.doJSON(ctx, method, path, body, false, out)
	return err
}

func (c *Client) StreamSSE(ctx context.Context, method, path string, body any, onData func([]byte) error) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	endpoint, err := c.resolve(path)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	if strings.TrimSpace(method) == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token := strings.TrimSpace(c.apiToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClientOrDefault().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		code, message, ok := parseAPIErrorPayload(bodyBytes)
		if ok {
			return &APIError{
				Method:   method,
				Endpoint: endpoint,
				Status:   resp.StatusCode,
				Code:     code,
				Message:  message,
				Body:     strings.TrimSpace(string(bodyBytes)),
			}
		}
		return &APIError{
			Method:   method,
			Endpoint: endpoint,
			Status:   resp.StatusCode,
			Body:     strings.TrimSpace(string(bodyBytes)),
		}
	}
	return ScanSSELines(resp.Body, onData)
}

func (c *Client) StreamChat(ctx context.Context, req ChatRequest, onStatus func(ChatEvent), onDelta func(string)) (ChatResult, error) {
	if strings.TrimSpace(req.Message) == "" {
		return ChatResult{}, fmt.Errorf("message is required")
	}
	result := ChatResult{SessionID: strings.TrimSpace(req.SessionID)}
	err := c.StreamSSE(ctx, http.MethodPost, "/v1/chat", req, func(payload []byte) error {
		var evt ChatEvent
		if err := json.Unmarshal(payload, &evt); err != nil {
			return fmt.Errorf("decode sse event: %w", err)
		}
		switch evt.Type {
		case "status":
			if onStatus != nil {
				onStatus(evt)
			}
		case "delta":
			result.Assistant += evt.Text
			if onDelta != nil && evt.Text != "" {
				onDelta(evt.Text)
			}
		case "error":
			if strings.TrimSpace(evt.Error) == "" {
				return fmt.Errorf("chat stream error")
			}
			return errors.New(strings.TrimSpace(evt.Error))
		case "done":
			if strings.TrimSpace(evt.SessionID) != "" {
				result.SessionID = strings.TrimSpace(evt.SessionID)
			}
		}
		return nil
	})
	if err != nil {
		return ChatResult{}, err
	}
	return result, nil
}

func (c *Client) StreamEvents(ctx context.Context, onEvent func(NotificationMessage), onError func(error)) {
	backoff := 500 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.streamEventsOnce(ctx, onEvent)
		if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if onError != nil {
			onError(err)
		}
		if isEventStreamPermanentError(err) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 5*time.Second {
			backoff *= 2
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
		}
	}
}

func (c *Client) streamEventsOnce(ctx context.Context, onEvent func(NotificationMessage)) error {
	err := c.StreamSSE(ctx, http.MethodGet, "/v1/events/stream", nil, func(payload []byte) error {
		var evt NotificationMessage
		if err := json.Unmarshal(payload, &evt); err != nil {
			return fmt.Errorf("decode event: %w", err)
		}
		if onEvent != nil {
			onEvent(evt)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return io.EOF
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, admin bool, out any) (string, error) {
	text, err := c.doText(ctx, method, path, body, admin)
	if err != nil {
		return "", err
	}
	if out == nil || len(text) == 0 {
		return text, nil
	}
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return text, nil
}

func (c *Client) doText(ctx context.Context, method, path string, body any, admin bool) (string, error) {
	if c == nil {
		return "", fmt.Errorf("client is nil")
	}
	endpoint, err := c.resolve(path)
	if err != nil {
		return "", err
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return "", err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	token := strings.TrimSpace(c.apiToken)
	if admin {
		token = strings.TrimSpace(c.adminAPIToken)
		if token == "" {
			token = strings.TrimSpace(c.apiToken)
		}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClientOrDefault().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	text, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code, message, ok := parseAPIErrorPayload(text)
		if ok {
			return "", &APIError{
				Method:   method,
				Endpoint: endpoint,
				Status:   resp.StatusCode,
				Code:     code,
				Message:  message,
				Body:     strings.TrimSpace(string(text)),
			}
		}
		return "", &APIError{
			Method:   method,
			Endpoint: endpoint,
			Status:   resp.StatusCode,
			Body:     strings.TrimSpace(string(text)),
		}
	}
	return string(text), nil
}

func (c *Client) resolve(path string) (string, error) {
	return resolveURL(c.serverURL, path)
}

func (c *Client) httpClientOrDefault() *http.Client {
	if c != nil && c.httpClient != nil {
		return c.httpClient
	}
	return http.DefaultClient
}

func resolveURL(baseURL, path string) (string, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = DefaultServerURL
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid server url: %w", err)
	}
	rawPath := strings.TrimSpace(path)
	if rawPath == "" {
		rawPath = "/"
	}
	if !strings.HasPrefix(rawPath, "/") {
		rawPath = "/" + rawPath
	}
	ref, err := url.Parse(rawPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + ref.Path
	u.RawQuery = ref.RawQuery
	return u.String(), nil
}

func parseAPIErrorPayload(payload []byte) (code, message string, ok bool) {
	var body struct {
		Code    string `json:"code"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return "", "", false
	}
	code = strings.TrimSpace(body.Code)
	message = strings.TrimSpace(body.Error)
	if message == "" {
		message = strings.TrimSpace(body.Message)
	}
	if message == "" {
		message = code
	}
	if code == "" && message == "" {
		return "", "", false
	}
	return code, message, true
}

func isEventStreamPermanentError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr == nil {
		return false
	}
	return apiErr.Status >= 400 && apiErr.Status < 500 && apiErr.Status != http.StatusTooManyRequests
}
