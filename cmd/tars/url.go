package main

import (
	"fmt"
	"net/url"
	"strings"
)

const defaultServerURL = "http://127.0.0.1:43180"

func resolveURL(baseURL, path string) (string, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = defaultServerURL
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
