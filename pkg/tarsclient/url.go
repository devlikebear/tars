package tarsclient

import (
	"fmt"
	"net/url"
	"strings"
)

// ResolveURL appends rawPath to baseURL while preserving proxy base paths.
// Base query and fragment values are discarded; rawPath may provide its own.
func ResolveURL(baseURL, rawPath string) (string, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = DefaultServerURL
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid server url: %w", err)
	}
	if strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
		return "", fmt.Errorf("invalid server url: missing scheme or host")
	}

	path := strings.TrimSpace(rawPath)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	ref, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	u.Path = strings.TrimRight(u.Path, "/") + ref.Path
	u.RawPath = ""
	u.RawQuery = ref.RawQuery
	u.Fragment = ref.Fragment
	return u.String(), nil
}

func ConsoleURL(baseURL string) (string, error) {
	return ResolveURL(baseURL, "/console")
}
