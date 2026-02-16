package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

const (
	defaultWebFetchMaxChars = 12000
	maxWebFetchMaxChars     = 50000
)

func NewWebFetchTool(enabled bool) Tool {
	return newWebFetchToolWithHTTP(enabled, &http.Client{Timeout: 15 * time.Second})
}

func newWebFetchToolWithHTTP(enabled bool, httpClient *http.Client) Tool {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return Tool{
		Name:        "web_fetch",
		Description: "Fetch a URL and return extracted text content.",
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
			if !enabled {
				return jsonTextResult(webFetchResponse{Message: "web_fetch is disabled"}, true), nil
			}
			var input struct {
				URL      string `json:"url"`
				MaxChars *int   `json:"max_chars,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(webFetchResponse{Message: fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			url := strings.TrimSpace(input.URL)
			if url == "" {
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

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return jsonTextResult(webFetchResponse{Message: fmt.Sprintf("invalid url: %v", err)}, true), nil
			}
			resp, err := httpClient.Do(req)
			if err != nil {
				return jsonTextResult(webFetchResponse{Message: fmt.Sprintf("fetch failed: %v", err)}, true), nil
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return jsonTextResult(webFetchResponse{Message: fmt.Sprintf("web_fetch status %d", resp.StatusCode)}, true), nil
			}
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
			return jsonTextResult(webFetchResponse{URL: url, Content: text, Bytes: len(raw), Truncated: truncated}, false), nil
		},
	}
}

var htmlTagRE = regexp.MustCompile(`(?s)<[^>]*>`)

func htmlToText(html string) string {
	cleaned := htmlTagRE.ReplaceAllString(html, " ")
	cleaned = strings.ReplaceAll(cleaned, "&nbsp;", " ")
	cleaned = strings.ReplaceAll(cleaned, "&amp;", "&")
	return cleaned
}
