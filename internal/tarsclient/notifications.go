package tarsclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/devlikebear/tarsncase/pkg/tarsclient"
)

type notificationMessage = tarsclient.NotificationMessage

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
