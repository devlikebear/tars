package memory

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func openAIEmbeddingServer(t *testing.T, vector []float64, onRequest func(*http.Request, map[string]any)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if onRequest != nil {
			onRequest(r, body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"index": 0, "embedding": vector}},
		})
	}))
}

func TestOpenAIEmbedder_UsesEmbeddingsEndpointShape(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody map[string]any
	server := openAIEmbeddingServer(t, []float64{0.6, 0.8}, func(r *http.Request, body map[string]any) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotBody = body
	})
	defer server.Close()

	embedder := newOpenAIEmbedder(SemanticConfig{
		EmbedBaseURL: server.URL + "/v1/",
		EmbedAPIKey:  "secret",
		EmbedModel:   "kakaocorp/kanana-2-1.3b-embedding",
	}, server.Client())

	vector, err := embedder.Embed(context.Background(), EmbedRequest{
		Text:             "  안녕하세요  ",
		TaskType:         taskTypeRetrievalQuery,
		Title:            "ignored",
		OutputDimensions: 2,
	})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if gotPath != "/v1/embeddings" {
		t.Fatalf("expected /v1/embeddings, got %q", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("expected bearer auth, got %q", gotAuth)
	}
	if gotBody["model"] != "kakaocorp/kanana-2-1.3b-embedding" {
		t.Fatalf("expected model in request, got %#v", gotBody["model"])
	}
	if gotBody["input"] != "안녕하세요" {
		t.Fatalf("expected trimmed input, got %#v", gotBody["input"])
	}
	for _, key := range []string{"taskType", "title", "dimensions"} {
		if _, ok := gotBody[key]; ok {
			t.Fatalf("expected no %q field in OpenAI-compatible request, got %#v", key, gotBody)
		}
	}
	if len(vector) != 2 || vector[0] != 0.6 || vector[1] != 0.8 {
		t.Fatalf("unexpected vector %#v", vector)
	}
}

func TestOpenAIEmbedder_OmitsAuthorizationWithoutKey(t *testing.T) {
	gotAuth := "unset"
	server := openAIEmbeddingServer(t, []float64{1}, func(r *http.Request, _ map[string]any) {
		gotAuth = r.Header.Get("Authorization")
	})
	defer server.Close()

	embedder := newOpenAIEmbedder(SemanticConfig{EmbedBaseURL: server.URL, EmbedModel: "local"}, server.Client())
	if _, err := embedder.Embed(context.Background(), EmbedRequest{Text: "x", OutputDimensions: 1}); err != nil {
		t.Fatalf("embed: %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("expected no Authorization header for a keyless local server, got %q", gotAuth)
	}
}

func TestOpenAIEmbedder_TruncatesAndRenormalizesLongerVectors(t *testing.T) {
	server := openAIEmbeddingServer(t, []float64{3, 4, 12}, nil)
	defer server.Close()

	embedder := newOpenAIEmbedder(SemanticConfig{EmbedBaseURL: server.URL, EmbedModel: "m"}, server.Client())
	vector, err := embedder.Embed(context.Background(), EmbedRequest{Text: "x", OutputDimensions: 2})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vector) != 2 || math.Abs(vector[0]-0.6) > 1e-9 || math.Abs(vector[1]-0.8) > 1e-9 {
		t.Fatalf("expected truncated unit vector [0.6 0.8], got %#v", vector)
	}
}

func TestOpenAIEmbedder_RejectsShorterVectors(t *testing.T) {
	server := openAIEmbeddingServer(t, []float64{0.1, 0.2}, nil)
	defer server.Close()

	embedder := newOpenAIEmbedder(SemanticConfig{EmbedBaseURL: server.URL, EmbedModel: "m"}, server.Client())
	_, err := embedder.Embed(context.Background(), EmbedRequest{Text: "x", OutputDimensions: 768})
	if err == nil || !strings.Contains(err.Error(), "2 dimensions") {
		t.Fatalf("expected dimension mismatch error, got %v", err)
	}
}

func TestOpenAIEmbedder_ReportsServerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "model not found"}})
	}))
	defer server.Close()

	embedder := newOpenAIEmbedder(SemanticConfig{EmbedBaseURL: server.URL, EmbedModel: "m"}, server.Client())
	_, err := embedder.Embed(context.Background(), EmbedRequest{Text: "x", OutputDimensions: 2})
	if err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("expected server error message, got %v", err)
	}
}

func TestOpenAIEmbedder_RejectsEmptyAndUndecodableResponses(t *testing.T) {
	for name, body := range map[string]string{
		"empty":   `{"data":[]}`,
		"garbage": `not json`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			embedder := newOpenAIEmbedder(SemanticConfig{EmbedBaseURL: server.URL, EmbedModel: "m"}, server.Client())
			if _, err := embedder.Embed(context.Background(), EmbedRequest{Text: "x", OutputDimensions: 2}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestValidateSemanticConfig_OpenAIDoesNotRequireAPIKey(t *testing.T) {
	cfg := SemanticConfig{
		Enabled:         true,
		EmbedProvider:   "OpenAI",
		EmbedBaseURL:    "http://127.0.0.1:8000/v1",
		EmbedModel:      "kakaocorp/kanana-2-1.3b-embedding",
		EmbedDimensions: 1024,
	}
	if err := ValidateSemanticConfig(cfg); err != nil {
		t.Fatalf("expected keyless openai config to validate, got %v", err)
	}
	svc := NewService(t.TempDir(), ServiceOptions{Config: cfg})
	if !svc.Enabled() {
		t.Fatal("expected keyless openai semantic service to be enabled")
	}
}

func TestValidateSemanticConfig_GeminiStillRequiresAPIKey(t *testing.T) {
	err := ValidateSemanticConfig(SemanticConfig{
		Enabled:         true,
		EmbedProvider:   "gemini",
		EmbedBaseURL:    "https://generativelanguage.googleapis.com/v1beta",
		EmbedModel:      "gemini-embedding-2-preview",
		EmbedDimensions: 768,
	})
	if err == nil || !strings.Contains(err.Error(), "memory_embed_api_key") {
		t.Fatalf("expected missing api key error, got %v", err)
	}
}

func TestValidateSemanticConfig_LocalDoesNotRequireAPIKey(t *testing.T) {
	if err := ValidateSemanticConfig(SemanticConfig{
		Enabled:         true,
		EmbedProvider:   "local",
		EmbedBaseURL:    "http://127.0.0.1:43181/v1",
		EmbedModel:      "kakaocorp/kanana-2-1.3b-embedding",
		EmbedDimensions: 1024,
	}); err != nil {
		t.Fatalf("expected keyless local config to validate, got %v", err)
	}
}
