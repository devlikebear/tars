package tarsclient

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/pkg/tarsclient"
)

type chatRequest = tarsclient.ChatRequest

type chatEvent = tarsclient.ChatEvent

type chatResult = tarsclient.ChatResult

type chatClient struct {
	serverURL  string
	apiToken   string
	httpClient *http.Client
}

func (c chatClient) client() *tarsclient.Client {
	return tarsclient.New(tarsclient.Config{
		ServerURL:  c.serverURL,
		APIToken:   c.apiToken,
		HTTPClient: c.httpClient,
	})
}

func (c chatClient) stream(ctx context.Context, req chatRequest, onStatus func(chatEvent), onDelta func(string)) (chatResult, error) {
	result, err := c.client().StreamChat(ctx, req, onStatus, onDelta)
	if err == nil || !shouldRecoverMissingChatSession(req.SessionID, err) {
		return result, err
	}
	retryReq := req
	retryReq.SessionID = c.recoverChatSessionID(ctx, req.SessionID)
	return c.client().StreamChat(ctx, retryReq, onStatus, onDelta)
}

func shouldRecoverMissingChatSession(sessionID string, err error) bool {
	if strings.TrimSpace(sessionID) == "" || err == nil {
		return false
	}
	var apiErr *apiHTTPError
	if !errors.As(err, &apiErr) || apiErr == nil {
		return false
	}
	if apiErr.Status != http.StatusNotFound {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(apiErr.Code), "not_found") &&
		strings.Contains(strings.ToLower(strings.TrimSpace(apiErr.Message)), "session not found") {
		return true
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(apiErr.Body)), "session not found")
}

func (c chatClient) recoverChatSessionID(ctx context.Context, current string) string {
	status, err := c.client().Status(ctx)
	if err != nil {
		return ""
	}
	mainSessionID := strings.TrimSpace(status.MainSessionID)
	if mainSessionID == "" || mainSessionID == strings.TrimSpace(current) {
		return ""
	}
	return mainSessionID
}
