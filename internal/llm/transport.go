package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type jsonRequestSpec struct {
	Provider string
	URL      string
	Headers  map[string]string
	Body     any
}

func (s jsonRequestSpec) buildRequest(ctx context.Context) (*http.Request, error) {
	body, err := json.Marshal(s.Body)
	if err != nil {
		return nil, newProviderError(strings.TrimSpace(s.Provider), "parse", fmt.Errorf("marshal request: %w", err))
	}
	logLLMRequestPayload(s.Provider, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(s.URL), bytes.NewReader(body))
	if err != nil {
		return nil, newProviderError(strings.TrimSpace(s.Provider), "request", fmt.Errorf("create request: %w", err))
	}
	for key, value := range s.Headers {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}
	return req, nil
}

func transportHTTPClient(base *http.Client, streaming bool) *http.Client {
	if !streaming {
		return base
	}
	if base == nil {
		return &http.Client{}
	}
	return &http.Client{Transport: base.Transport}
}

func doJSONRequest(spec jsonRequestSpec, httpClient *http.Client, streaming bool) (*http.Response, error) {
	req, err := spec.buildRequest(context.Background())
	if err != nil {
		return nil, err
	}
	return doPreparedRequest(req, strings.TrimSpace(spec.Provider), transportHTTPClient(httpClient, streaming))
}

func doPreparedRequest(req *http.Request, provider string, httpClient *http.Client) (*http.Response, error) {
	resp, err := doRawPreparedRequest(req, provider, httpClient)
	if err != nil {
		return nil, err
	}
	if err := checkHTTPStatus(resp, provider); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	return resp, nil
}

func doRawPreparedRequest(req *http.Request, provider string, httpClient *http.Client) (*http.Response, error) {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, newProviderError(provider, "request", fmt.Errorf("request %s: %w", provider, err))
	}
	return resp, nil
}
