package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type webSearchResponse struct {
	Query   string            `json:"query,omitempty"`
	Count   int               `json:"count"`
	Results []webSearchResult `json:"results,omitempty"`
	Message string            `json:"message,omitempty"`
}

type webSearchResult struct {
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
}

func NewWebSearchTool(enabled bool, apiKey string) Tool {
	return newWebSearchToolWithHTTP("https://api.search.brave.com/res/v1/web/search", enabled, apiKey, &http.Client{Timeout: 15 * time.Second})
}

func newWebSearchToolWithHTTP(baseURL string, enabled bool, apiKey string, httpClient *http.Client) Tool {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return Tool{
		Name:        "web_search",
		Description: "Search the web and return result snippets.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "query":{"type":"string"},
    "count":{"type":"integer","minimum":1,"maximum":10,"default":5}
  },
  "required":["query"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if !enabled {
				return jsonTextResult(webSearchResponse{Message: "web_search is disabled"}, true), nil
			}
			if strings.TrimSpace(apiKey) == "" {
				return jsonTextResult(webSearchResponse{Message: "web_search api key is required"}, true), nil
			}
			var input struct {
				Query string `json:"query"`
				Count *int   `json:"count,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(webSearchResponse{Message: fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			query := strings.TrimSpace(input.Query)
			if query == "" {
				return jsonTextResult(webSearchResponse{Message: "query is required"}, true), nil
			}
			count := 5
			if input.Count != nil {
				count = *input.Count
			}
			if count <= 0 {
				count = 5
			}
			if count > 10 {
				count = 10
			}
			u, err := url.Parse(strings.TrimSpace(baseURL))
			if err != nil {
				return jsonTextResult(webSearchResponse{Message: fmt.Sprintf("invalid web_search base url: %v", err)}, true), nil
			}
			q := u.Query()
			q.Set("q", query)
			q.Set("count", fmt.Sprintf("%d", count))
			u.RawQuery = q.Encode()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if err != nil {
				return jsonTextResult(webSearchResponse{Message: fmt.Sprintf("search request build failed: %v", err)}, true), nil
			}
			req.Header.Set("Accept", "application/json")
			req.Header.Set("X-Subscription-Token", strings.TrimSpace(apiKey))
			resp, err := httpClient.Do(req)
			if err != nil {
				return jsonTextResult(webSearchResponse{Message: fmt.Sprintf("search request failed: %v", err)}, true), nil
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return jsonTextResult(webSearchResponse{Message: fmt.Sprintf("web_search status %d", resp.StatusCode)}, true), nil
			}
			var payload struct {
				Web struct {
					Results []struct {
						Title       string `json:"title"`
						URL         string `json:"url"`
						Description string `json:"description"`
					} `json:"results"`
				} `json:"web"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				return jsonTextResult(webSearchResponse{Message: fmt.Sprintf("decode search response failed: %v", err)}, true), nil
			}
			results := make([]webSearchResult, 0, len(payload.Web.Results))
			for _, r := range payload.Web.Results {
				results = append(results, webSearchResult{Title: r.Title, URL: r.URL, Description: r.Description})
			}
			return jsonTextResult(webSearchResponse{Query: query, Count: len(results), Results: results}, false), nil
		},
	}
}
