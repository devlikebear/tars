package tarsclient

import (
	"net/http"

	"github.com/devlikebear/tarsncase/pkg/tarsclient"
)

type runtimeClient struct {
	serverURL     string
	apiToken      string
	adminAPIToken string
	httpClient    *http.Client
}

func (c runtimeClient) client() *tarsclient.Client {
	return tarsclient.New(tarsclient.Config{
		ServerURL:     c.serverURL,
		APIToken:      c.apiToken,
		AdminAPIToken: c.adminAPIToken,
		HTTPClient:    c.httpClient,
	})
}
