package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

// openAIEmbedder calls an OpenAI-compatible POST {base_url}/embeddings
// endpoint: OpenAI itself, or a local server (vLLM, Ollama, LM Studio,
// sentence-transformers behind a small shim) serving a model such as
// kakaocorp/kanana-2-1.3b-embedding. The API key is optional because local
// servers usually have none.
//
// TaskType and Title have no OpenAI equivalent and are not sent. The
// `dimensions` parameter is not sent either: servers reject it for models
// they do not know to be Matryoshka-trained, so a longer vector is cut to
// OutputDimensions here and re-normalized instead.
type openAIEmbedder struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

func newOpenAIEmbedder(cfg SemanticConfig, client *http.Client) Embedder {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &openAIEmbedder{
		baseURL: strings.TrimRight(strings.TrimSpace(cfg.EmbedBaseURL), "/"),
		model:   strings.TrimSpace(cfg.EmbedModel),
		apiKey:  strings.TrimSpace(cfg.EmbedAPIKey),
		client:  client,
	}
}

func (o *openAIEmbedder) Embed(ctx context.Context, req EmbedRequest) ([]float64, error) {
	if o == nil || o.client == nil {
		return nil, fmt.Errorf("openai embedder is not configured")
	}
	encoded, err := json.Marshal(struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}{
		Model: o.model,
		Input: strings.TrimSpace(req.Text),
	})
	if err != nil {
		return nil, fmt.Errorf("encode openai embedding request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/embeddings", bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("build openai embedding request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("run openai embedding request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var parsed struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("openai embedding error: HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("decode openai embedding response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openai embedding error: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(parsed.Error.Message))
	}
	if len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("openai embedding response was empty")
	}
	return fitEmbeddingDimensions(parsed.Data[0].Embedding, req.OutputDimensions)
}

// fitEmbeddingDimensions keeps the first dims values and scales them back to
// unit length, which is how Matryoshka embeddings (OpenAI text-embedding-3,
// kanana-2 embeddings) are meant to be shortened. A vector shorter than dims
// is a configuration error: memory_embed_dimensions asks for more than the
// model produces.
func fitEmbeddingDimensions(vector []float64, dims int) ([]float64, error) {
	if dims <= 0 || len(vector) == dims {
		return vector, nil
	}
	if len(vector) < dims {
		return nil, fmt.Errorf("embedding model returned %d dimensions but memory_embed_dimensions is %d", len(vector), dims)
	}
	out := make([]float64, dims)
	copy(out, vector[:dims])
	var norm float64
	for _, v := range out {
		norm += v * v
	}
	if norm == 0 {
		return out, nil
	}
	norm = math.Sqrt(norm)
	for i := range out {
		out[i] /= norm
	}
	return out, nil
}
