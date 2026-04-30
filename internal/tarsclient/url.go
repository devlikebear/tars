package tarsclient

import (
	protocol "github.com/devlikebear/tars/pkg/tarsclient"
)

func resolveURL(baseURL, path string) (string, error) {
	return protocol.ResolveURL(baseURL, path)
}
