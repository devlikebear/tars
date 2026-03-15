package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	zlog "github.com/rs/zerolog/log"
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
	_, resp, err := executeJSONChatRequest(context.Background(), spec, httpClient, streaming)
	return resp, err
}

func executeJSONChatRequest(ctx context.Context, spec jsonRequestSpec, httpClient *http.Client, streaming bool) (*http.Request, *http.Response, error) {
	req, err := spec.buildRequest(ctx)
	if err != nil {
		return nil, nil, err
	}
	provider := strings.TrimSpace(spec.Provider)
	resp, err := doPreparedRequest(req, provider, transportHTTPClient(httpClient, streaming))
	if err != nil {
		return nil, nil, err
	}
	zlog.Debug().Str("provider", provider).Int("status", resp.StatusCode).Msg("llm response received")
	return req, resp, nil
}

func logChatRequestStart(provider, model, url string, messageCount int, streaming bool, toolCount int, toolChoice string) {
	zlog.Debug().
		Str("provider", strings.TrimSpace(provider)).
		Str("model", strings.TrimSpace(model)).
		Str("url", strings.TrimSpace(url)).
		Int("message_count", messageCount).
		Bool("stream", streaming).
		Int("tool_count", toolCount).
		Str("tool_choice", strings.TrimSpace(toolChoice)).
		Msg("llm request start")
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
