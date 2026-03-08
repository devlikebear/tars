package tarsclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/devlikebear/tars/pkg/tarsclient"
)

type notificationMessage = tarsclient.NotificationMessage

type notificationCenter struct {
	mu         sync.RWMutex
	max        int
	filter     string
	items      []notificationMessage
	seenIDs    map[int64]struct{}
	readCursor int64
}

func newNotificationCenter(max int) *notificationCenter {
	if max <= 0 {
		max = 200
	}
	return &notificationCenter{
		max:     max,
		filter:  "all",
		items:   make([]notificationMessage, 0, max),
		seenIDs: map[int64]struct{}{},
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
	if msg.ID > 0 {
		if _, exists := c.seenIDs[msg.ID]; exists {
			return
		}
	}
	c.items = append(c.items, msg)
	if msg.ID > 0 {
		c.seenIDs[msg.ID] = struct{}{}
	}
	if len(c.items) > c.max {
		for _, removed := range c.items[:len(c.items)-c.max] {
			if removed.ID > 0 {
				delete(c.seenIDs, removed.ID)
			}
		}
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
	c.seenIDs = map[int64]struct{}{}
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

func (c *notificationCenter) setReadCursor(lastID int64) {
	if c == nil {
		return
	}
	if lastID < 0 {
		lastID = 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if lastID > c.readCursor {
		c.readCursor = lastID
	}
}

func (c *notificationCenter) unreadCount() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	unread := 0
	for _, item := range c.items {
		if item.ID <= 0 || item.ID > c.readCursor {
			unread++
		}
	}
	return unread
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

func (c eventStreamClient) client() *tarsclient.Client {
	return tarsclient.New(tarsclient.Config{
		ServerURL:  c.serverURL,
		APIToken:   c.apiToken,
		HTTPClient: c.httpClient,
	})
}

func (c eventStreamClient) consume(ctx context.Context, onEvent func(notificationMessage), onError func(error)) {
	c.client().StreamEvents(ctx, onEvent, onError)
}
