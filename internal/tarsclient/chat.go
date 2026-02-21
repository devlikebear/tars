package tarsclient

import (
	"context"
	"net/http"

	"github.com/devlikebear/tarsncase/pkg/tarsclient"
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
	return c.client().StreamChat(ctx, req, onStatus, onDelta)
}
