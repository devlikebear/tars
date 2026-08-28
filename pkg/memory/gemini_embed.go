package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type geminiEmbedder struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

func newGeminiEmbedder(cfg SemanticConfig, client *http.Client) Embedder {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &geminiEmbedder{
		baseURL: strings.TrimRight(strings.TrimSpace(cfg.EmbedBaseURL), "/"),
		model:   strings.TrimSpace(cfg.EmbedModel),
		apiKey:  strings.TrimSpace(cfg.EmbedAPIKey),
		client:  client,
	}
}

func (g *geminiEmbedder) Embed(ctx context.Context, req EmbedRequest) ([]float64, error) {
	if g == nil || g.client == nil {
		return nil, fmt.Errorf("gemini embedder is not configured")
	}
	payload := struct {
		Model   string `json:"model"`
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		TaskType             string `json:"taskType,omitempty"`
		Title                string `json:"title,omitempty"`
		OutputDimensionality int    `json:"outputDimensionality,omitempty"`
	}{
		Model:                normalizeGeminiModelName(g.model),
		TaskType:             strings.TrimSpace(req.TaskType),
		Title:                strings.TrimSpace(req.Title),
		OutputDimensionality: req.OutputDimensions,
	}
	payload.Content.Parts = []struct {
		Text string `json:"text"`
	}{{Text: strings.TrimSpace(req.Text)}}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode gemini embedding request: %w", err)
	}
	endpoint := fmt.Sprintf("%s/%s:embedContent", g.baseURL, normalizeGeminiModelName(g.model))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("build gemini embedding request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", g.apiKey)

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("run gemini embedding request: %w", err)
	}
	defer resp.Body.Close()

	var parsed struct {
		Embedding struct {
			Values []float64 `json:"values"`
		} `json:"embedding"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode gemini embedding response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gemini embedding error: %s", strings.TrimSpace(parsed.Error.Message))
	}
	if len(parsed.Embedding.Values) == 0 {
		return nil, fmt.Errorf("gemini embedding response was empty")
	}
	return parsed.Embedding.Values, nil
}

func normalizeGeminiModelName(model string) string {
	model = strings.TrimSpace(model)
	if strings.HasPrefix(model, "models/") {
		return model
	}
	return "models/" + model
}
