package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type notificationMessage struct {
	Type      string `json:"type"`
	Category  string `json:"category"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type notificationCenter struct {
	mu     sync.RWMutex
	max    int
	filter string
	items  []notificationMessage
}

func newNotificationCenter(max int) *notificationCenter {
	if max <= 0 {
		max = 200
	}
	return &notificationCenter{
		max:    max,
		filter: "all",
		items:  make([]notificationMessage, 0, max),
	}
}

func (c *notificationCenter) add(msg notificationMessage) {
	if c == nil {
		return
	}
	msg.Type = strings.TrimSpace(msg.Type)
	if msg.Type == "" {
		msg.Type = "notification"
	}
	if msg.Type == "keepalive" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = append(c.items, msg)
	if len(c.items) > c.max {
		c.items = c.items[len(c.items)-c.max:]
	}
}

func (c *notificationCenter) clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = c.items[:0]
}

func (c *notificationCenter) setFilter(filter string) error {
	if c == nil {
		return errors.New("notification center is not initialized")
	}
	next := strings.ToLower(strings.TrimSpace(filter))
	switch next {
	case "all", "cron", "heartbeat", "error":
	default:
		return fmt.Errorf("filter must be one of: all|cron|heartbeat|error")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.filter = next
	return nil
}

func (c *notificationCenter) filtered() []notificationMessage {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	filter := strings.TrimSpace(c.filter)
	out := make([]notificationMessage, 0, len(c.items))
	for _, item := range c.items {
		if filter == "" || filter == "all" {
			out = append(out, item)
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.Category), filter) {
			out = append(out, item)
		}
	}
	return out
}

func (c *notificationCenter) filterName() string {
	if c == nil {
		return "all"
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	name := strings.TrimSpace(c.filter)
	if name == "" {
		return "all"
	}
	return name
}

type eventStreamClient struct {
	serverURL  string
	apiToken   string
	httpClient *http.Client
}

func newEventStreamClient(runtime runtimeClient) eventStreamClient {
	return eventStreamClient{
		serverURL:  runtime.serverURL,
		apiToken:   runtime.apiToken,
		httpClient: runtime.httpClient,
	}
}

func (c eventStreamClient) consume(ctx context.Context, onEvent func(notificationMessage), onError func(error)) {
	backoff := 500 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.consumeOnce(ctx, onEvent)
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

func (c eventStreamClient) consumeOnce(ctx context.Context, onEvent func(notificationMessage)) error {
	endpoint, err := resolveURL(c.serverURL, "/v1/events/stream")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if token := strings.TrimSpace(c.apiToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		code, message, ok := parseAPIErrorPayload(body)
		if ok {
			return &apiHTTPError{
				Method:   http.MethodGet,
				Endpoint: endpoint,
				Status:   resp.StatusCode,
				Code:     code,
				Message:  message,
				Body:     strings.TrimSpace(string(body)),
			}
		}
		return &apiHTTPError{
			Method:   http.MethodGet,
			Endpoint: endpoint,
			Status:   resp.StatusCode,
			Body:     strings.TrimSpace(string(body)),
		}
	}
	if err := scanSSELines(resp.Body, func(payload []byte) error {
		var evt notificationMessage
		if err := json.Unmarshal(payload, &evt); err != nil {
			return fmt.Errorf("decode event: %w", err)
		}
		if onEvent != nil {
			onEvent(evt)
		}
		return nil
	}); err != nil {
		return err
	}
	return io.EOF
}

func isEventStreamPermanentError(err error) bool {
	var apiErr *apiHTTPError
	if !errors.As(err, &apiErr) || apiErr == nil {
		return false
	}
	return apiErr.Status >= 400 && apiErr.Status < 500 && apiErr.Status != http.StatusTooManyRequests
}
