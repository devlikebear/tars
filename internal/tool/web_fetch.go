package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type webFetchResponse struct {
	URL       string `json:"url,omitempty"`
	Content   string `json:"content,omitempty"`
	Bytes     int    `json:"bytes,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Message   string `json:"message,omitempty"`
}

type WebFetchOptions struct {
	Enabled              bool
	AllowPrivateHosts    bool
	PrivateHostAllowlist []string
	HTTPClient           *http.Client
}

const (
	defaultWebFetchMaxChars = 12000
	maxWebFetchMaxChars     = 50000
	maxWebFetchRedirects    = 5
)

func NewWebFetchTool(enabled bool) Tool {
	return NewWebFetchToolWithOptions(WebFetchOptions{
		Enabled:           enabled,
		AllowPrivateHosts: false,
		HTTPClient:        &http.Client{Timeout: 15 * time.Second},
	})
}

func NewWebFetchToolWithOptions(opts WebFetchOptions) Tool {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	allowlist := normalizeHostAllowlist(opts.PrivateHostAllowlist)

	return Tool{
		Name:        "web_fetch",
		Description: "Fetch a URL and return extracted text content (with SSRF protection).",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "url":{"type":"string"},
    "max_chars":{"type":"integer","minimum":1,"maximum":50000,"default":12000}
  },
  "required":["url"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if !opts.Enabled {
				return jsonTextResult(webFetchResponse{Message: "web_fetch is disabled"}, true), nil
			}
			var input struct {
				URL      string `json:"url"`
				MaxChars *int   `json:"max_chars,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(webFetchResponse{Message: fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			rawURL := strings.TrimSpace(input.URL)
			if rawURL == "" {
				return jsonTextResult(webFetchResponse{Message: "url is required"}, true), nil
			}
			maxChars := defaultWebFetchMaxChars
			if input.MaxChars != nil {
				maxChars = *input.MaxChars
			}
			if maxChars <= 0 {
				maxChars = defaultWebFetchMaxChars
			}
			if maxChars > maxWebFetchMaxChars {
				maxChars = maxWebFetchMaxChars
			}

			finalURL, resp, err := fetchWithSSRFGuard(ctx, httpClient, rawURL, opts.AllowPrivateHosts, allowlist)
			if err != nil {
				return jsonTextResult(webFetchResponse{Message: err.Error()}, true), nil
			}
			defer resp.Body.Close()

			raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxWebFetchMaxChars*4)))
			if err != nil {
				return jsonTextResult(webFetchResponse{Message: fmt.Sprintf("read response failed: %v", err)}, true), nil
			}
			text := string(raw)
			if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "html") {
				text = htmlToText(text)
			}
			text = strings.Join(strings.Fields(text), " ")
			truncated := false
			if len(text) > maxChars {
				text = text[:maxChars]
				truncated = true
			}
			return jsonTextResult(webFetchResponse{URL: finalURL, Content: text, Bytes: len(raw), Truncated: truncated}, false), nil
		},
	}
}

func newWebFetchToolWithHTTP(enabled bool, httpClient *http.Client) Tool {
	return NewWebFetchToolWithOptions(WebFetchOptions{
		Enabled:           enabled,
		AllowPrivateHosts: true,
		HTTPClient:        httpClient,
	})
}

func fetchWithSSRFGuard(
	ctx context.Context,
	httpClient *http.Client,
	rawURL string,
	allowPrivateHosts bool,
	allowlist map[string]struct{},
) (string, *http.Response, error) {
	current := strings.TrimSpace(rawURL)
	for redirectCount := 0; redirectCount <= maxWebFetchRedirects; redirectCount++ {
		target, err := url.Parse(current)
		if err != nil {
			return "", nil, fmt.Errorf("invalid url: %w", err)
		}
		if err := validateFetchURL(ctx, target, allowPrivateHosts, allowlist); err != nil {
			return "", nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return "", nil, fmt.Errorf("build request failed: %w", err)
		}
		client := cloneHTTPClientWithoutRedirect(httpClient)
		resp, err := client.Do(req)
		if err != nil {
			return "", nil, fmt.Errorf("fetch failed: %w", err)
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := strings.TrimSpace(resp.Header.Get("Location"))
			if location == "" {
				resp.Body.Close()
				return "", nil, fmt.Errorf("redirect status %d without location", resp.StatusCode)
			}
			nextURL, err := target.Parse(location)
			resp.Body.Close()
			if err != nil {
				return "", nil, fmt.Errorf("invalid redirect location: %w", err)
			}
			current = nextURL.String()
			if redirectCount == maxWebFetchRedirects {
				return "", nil, fmt.Errorf("too many redirects")
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return "", nil, fmt.Errorf("web_fetch status %d", resp.StatusCode)
		}
		return target.String(), resp, nil
	}
	return "", nil, fmt.Errorf("too many redirects")
}

func validateFetchURL(ctx context.Context, target *url.URL, allowPrivateHosts bool, allowlist map[string]struct{}) error {
	scheme := strings.ToLower(strings.TrimSpace(target.Scheme))
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported url scheme: %s", target.Scheme)
	}
	host := strings.TrimSpace(target.Hostname())
	if host == "" {
		return fmt.Errorf("url host is required")
	}
	if _, ok := allowlist[strings.ToLower(host)]; ok {
		return nil
	}
	if allowPrivateHosts {
		return nil
	}

	if ip := net.ParseIP(host); ip != nil {
		if isPrivateOrLocalIP(ip) {
			return fmt.Errorf("private host is blocked: %s", host)
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("resolve host failed: %w", err)
	}
	for _, ip := range ips {
		if isPrivateOrLocalIP(ip) {
			return fmt.Errorf("private host is blocked: %s", host)
		}
	}
	return nil
}

func isPrivateOrLocalIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	v := strings.ToLower(ip.String())
	if strings.HasPrefix(v, "fc") || strings.HasPrefix(v, "fd") || strings.HasPrefix(v, "fe80:") {
		return true
	}
	return false
}

func normalizeHostAllowlist(items []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item))
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}

func cloneHTTPClientWithoutRedirect(base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{Timeout: 15 * time.Second}
	}
	clone := *base
	clone.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &clone
}

var htmlTagRE = regexp.MustCompile(`(?s)<[^>]*>`)

func htmlToText(html string) string {
	cleaned := htmlTagRE.ReplaceAllString(html, " ")
	cleaned = strings.ReplaceAll(cleaned, "&nbsp;", " ")
	cleaned = strings.ReplaceAll(cleaned, "&amp;", "&")
	return cleaned
}
