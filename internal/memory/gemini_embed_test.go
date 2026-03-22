package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeminiEmbedder_UsesNativeEmbedContentShape(t *testing.T) {
	var gotAuth string
	var gotPath string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("x-goog-api-key")
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embedding": map[string]any{
				"values": []float64{0.1, 0.2, 0.3},
			},
		})
	}))
	defer server.Close()

	embedder := newGeminiEmbedder(SemanticConfig{
		EmbedBaseURL: server.URL,
		EmbedAPIKey:  "secret",
		EmbedModel:   "gemini-embedding-2-preview",
	}, server.Client())

	vector, err := embedder.Embed(context.Background(), EmbedRequest{
		Text:             "hello world",
		TaskType:         taskTypeRetrievalQuery,
		OutputDimensions: 3,
	})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if gotAuth != "secret" {
		t.Fatalf("expected x-goog-api-key header, got %q", gotAuth)
	}
	if gotPath != "/models/gemini-embedding-2-preview:embedContent" {
		t.Fatalf("expected native embed path, got %q", gotPath)
	}
	if gotBody["taskType"] != taskTypeRetrievalQuery {
		t.Fatalf("expected taskType in request, got %#v", gotBody["taskType"])
	}
	if gotBody["outputDimensionality"] != float64(3) {
		t.Fatalf("expected output dimensionality in request, got %#v", gotBody["outputDimensionality"])
	}
	if len(vector) != 3 {
		t.Fatalf("expected embedding vector, got %#v", vector)
	}
}
