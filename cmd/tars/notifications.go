package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	serverURL   string
	apiToken    string
	workspaceID string
	httpClient  *http.Client
}

func newEventStreamClient(runtime runtimeClient) eventStreamClient {
	return eventStreamClient{
		serverURL:   runtime.serverURL,
		apiToken:    runtime.apiToken,
		workspaceID: runtime.workspaceID,
		httpClient:  runtime.httpClient,
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
	endpoint, err := resolveEventsEndpoint(c.serverURL)
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
	if ws := strings.TrimSpace(c.workspaceID); ws != "" {
		req.Header.Set("Tars-Workspace-Id", ws)
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
		return fmt.Errorf("events stream status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var evt notificationMessage
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			return fmt.Errorf("decode event: %w", err)
		}
		if onEvent != nil {
			onEvent(evt)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

func resolveEventsEndpoint(serverURL string) (string, error) {
	base := strings.TrimSpace(serverURL)
	if base == "" {
		base = "http://127.0.0.1:43180"
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid server url: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/events/stream"
	return u.String(), nil
}
