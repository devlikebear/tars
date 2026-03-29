package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type webSearchResponse struct {
	Provider string            `json:"provider,omitempty"`
	Query    string            `json:"query,omitempty"`
	Count    int               `json:"count"`
	Cached   bool              `json:"cached,omitempty"`
	Results  []webSearchResult `json:"results,omitempty"`
	Message  string            `json:"message,omitempty"`
}

type webSearchResult struct {
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
}

type WebSearchOptions struct {
	Enabled           bool
	Provider          string
	BraveAPIKey       string
	BraveBaseURL      string
	PerplexityAPIKey  string
	PerplexityModel   string
	PerplexityBaseURL string
	CacheTTL          time.Duration
	HTTPClient        *http.Client
}

type webSearchCacheEntry struct {
	Expires time.Time
	Value   webSearchResponse
}

func NewWebSearchTool(enabled bool, apiKey string) Tool {
	return NewWebSearchToolWithOptions(WebSearchOptions{
		Enabled:      enabled,
		Provider:     "brave",
		BraveAPIKey:  apiKey,
		CacheTTL:     60 * time.Second,
		BraveBaseURL: "https://api.search.brave.com/res/v1/web/search",
		HTTPClient:   &http.Client{Timeout: 15 * time.Second},
	})
}

func NewWebSearchToolWithOptions(opts WebSearchOptions) Tool {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	providerDefault := strings.TrimSpace(strings.ToLower(opts.Provider))
	if providerDefault == "" {
		providerDefault = "brave"
	}
	braveBase := strings.TrimSpace(opts.BraveBaseURL)
	if braveBase == "" {
		braveBase = "https://api.search.brave.com/res/v1/web/search"
	}
	perplexityURL := strings.TrimSpace(opts.PerplexityBaseURL)
	if perplexityURL == "" {
		perplexityURL = "https://api.perplexity.ai/chat/completions"
	}
	perplexityModel := strings.TrimSpace(opts.PerplexityModel)
	if perplexityModel == "" {
		perplexityModel = "sonar"
	}
	cacheTTL := opts.CacheTTL
	if cacheTTL < 0 {
		cacheTTL = 0
	}
	cache := map[string]webSearchCacheEntry{}
	var cacheMu sync.Mutex

	searchBrave := func(ctx context.Context, query string, count int) (webSearchResponse, error) {
		if strings.TrimSpace(opts.BraveAPIKey) == "" {
			return webSearchResponse{}, fmt.Errorf("web_search brave api key is required")
		}
		u, err := url.Parse(braveBase)
		if err != nil {
			return webSearchResponse{}, fmt.Errorf("invalid brave base url: %w", err)
		}
		q := u.Query()
		q.Set("q", query)
		q.Set("count", fmt.Sprintf("%d", count))
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return webSearchResponse{}, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Subscription-Token", strings.TrimSpace(opts.BraveAPIKey))
		resp, err := httpClient.Do(req)
		if err != nil {
			return webSearchResponse{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return webSearchResponse{}, fmt.Errorf("brave status %d", resp.StatusCode)
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
			return webSearchResponse{}, fmt.Errorf("decode brave response failed: %w", err)
		}
		results := make([]webSearchResult, 0, len(payload.Web.Results))
		for _, item := range payload.Web.Results {
			results = append(results, webSearchResult{
				Title:       strings.TrimSpace(item.Title),
				URL:         strings.TrimSpace(item.URL),
				Description: strings.TrimSpace(item.Description),
			})
		}
		return webSearchResponse{Provider: "brave", Query: query, Count: len(results), Results: results}, nil
	}

	searchPerplexity := func(ctx context.Context, query string, count int) (webSearchResponse, error) {
		if strings.TrimSpace(opts.PerplexityAPIKey) == "" {
			return webSearchResponse{}, fmt.Errorf("web_search perplexity api key is required")
		}
		payload := map[string]any{
			"model":    perplexityModel,
			"messages": []map[string]string{{"role": "user", "content": query}},
			"stream":   false,
		}
		raw, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, perplexityURL, strings.NewReader(string(raw)))
		if err != nil {
			return webSearchResponse{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(opts.PerplexityAPIKey))
		resp, err := httpClient.Do(req)
		if err != nil {
			return webSearchResponse{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return webSearchResponse{}, fmt.Errorf("perplexity status %d", resp.StatusCode)
		}
		var result struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Citations []string `json:"citations"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return webSearchResponse{}, fmt.Errorf("decode perplexity response failed: %w", err)
		}
		answer := ""
		if len(result.Choices) > 0 {
			answer = strings.TrimSpace(result.Choices[0].Message.Content)
		}
		results := make([]webSearchResult, 0)
		for _, citation := range result.Citations {
			if strings.TrimSpace(citation) == "" {
				continue
			}
			results = append(results, webSearchResult{
				Title:       "Perplexity Citation",
				URL:         strings.TrimSpace(citation),
				Description: answer,
			})
			if len(results) >= count {
				break
			}
		}
		if len(results) == 0 {
			results = append(results, webSearchResult{Title: "Perplexity Answer", Description: answer})
		}
		return webSearchResponse{Provider: "perplexity", Query: query, Count: len(results), Results: results}, nil
	}

	return Tool{
		Name:        "web_search",
		Description: "Search the web (provider: brave/perplexity) and return result snippets.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "query":{"type":"string"},
    "count":{"type":"integer","minimum":1,"maximum":10,"default":5},
    "provider":{"type":"string","enum":["brave","perplexity"]}
  },
  "required":["query"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if !opts.Enabled {
				return JSONTextResult(webSearchResponse{Message: "web_search is disabled"}, true), nil
			}
			var input struct {
				Query    string `json:"query"`
				Count    *int   `json:"count,omitempty"`
				Provider string `json:"provider,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(webSearchResponse{Message: fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			query := strings.TrimSpace(input.Query)
			if query == "" {
				return JSONTextResult(webSearchResponse{Message: "query is required"}, true), nil
			}
			count := 5
			if input.Count != nil {
				count = *input.Count
			}
			if count < 1 {
				count = 1
			}
			if count > 10 {
				count = 10
			}
			provider := strings.TrimSpace(strings.ToLower(input.Provider))
			if provider == "" {
				provider = providerDefault
			}
			cacheKey := provider + "|" + strings.ToLower(query) + "|" + fmt.Sprintf("%d", count)
			if cacheTTL > 0 {
				cacheMu.Lock()
				cached, ok := cache[cacheKey]
				if ok && time.Now().Before(cached.Expires) {
					payload := cached.Value
					payload.Cached = true
					cacheMu.Unlock()
					return JSONTextResult(payload, false), nil
				}
				cacheMu.Unlock()
			}

			var payload webSearchResponse
			var err error
			switch provider {
			case "brave":
				payload, err = searchBrave(ctx, query, count)
			case "perplexity":
				payload, err = searchPerplexity(ctx, query, count)
			default:
				err = fmt.Errorf("provider must be one of: brave|perplexity")
			}
			if err != nil {
				return JSONTextResult(webSearchResponse{Provider: provider, Query: query, Message: err.Error()}, true), nil
			}
			if cacheTTL > 0 {
				cacheMu.Lock()
				cache[cacheKey] = webSearchCacheEntry{Expires: time.Now().Add(cacheTTL), Value: payload}
				cacheMu.Unlock()
			}
			return JSONTextResult(payload, false), nil
		},
	}
}

func newWebSearchToolWithHTTP(baseURL string, enabled bool, apiKey string, httpClient *http.Client) Tool {
	return NewWebSearchToolWithOptions(WebSearchOptions{
		Enabled:      enabled,
		Provider:     "brave",
		BraveAPIKey:  apiKey,
		BraveBaseURL: baseURL,
		CacheTTL:     0,
		HTTPClient:   httpClient,
	})
}
