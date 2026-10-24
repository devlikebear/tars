package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestGeminiImageClient(t *testing.T, url, model string) *GeminiNativeClient {
	t.Helper()
	client, err := NewGeminiNativeClient(url+"/v1beta", "gemini-key", model)
	if err != nil {
		t.Fatalf("new gemini client: %v", err)
	}
	return client
}

func geminiImageResponse(mime string, data []byte) string {
	// thoughtSignature rides beside the image on Gemini 3 models.
	return fmt.Sprintf(`{"candidates":[{"content":{"role":"model","parts":[{"inlineData":{"mimeType":%q,"data":%q},"thoughtSignature":"c2ln"}]},"finishReason":"STOP"}],
		"usageMetadata":{"promptTokenCount":17,"candidatesTokenCount":1120}}`, mime, base64.StdEncoding.EncodeToString(data))
}

func TestGeminiNativeGenerateImage_SendsImageRequestAndDecodesImage(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-3.1-flash-image:generateContent" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "gemini-key" {
			t.Errorf("api key header = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(geminiImageResponse("image/jpeg", fakeJPEG)))
	}))
	defer srv.Close()

	got, err := newTestGeminiImageClient(t, srv.URL, "gemini-3.1-flash-image").GenerateImage(context.Background(), ImageRequest{
		Prompt:     "a red apple",
		References: []ContentBlock{{Type: "image", MediaType: "image/jpeg", Data: "cmVm"}},
		Aspect:     ImageAspectPortrait,
	})
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if string(got.Data) != string(fakeJPEG) || got.MediaType != "image/jpeg" {
		t.Fatalf("result = %+v", got)
	}

	cfg := captured["generationConfig"].(map[string]any)
	if mods := cfg["responseModalities"].([]any); len(mods) != 1 || mods[0] != "IMAGE" {
		t.Fatalf("responseModalities = %v", mods)
	}
	img := cfg["imageConfig"].(map[string]any)
	if img["aspectRatio"] != "2:3" || img["imageSize"] != "1K" {
		t.Fatalf("imageConfig = %v", img)
	}
	parts := captured["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	ref := parts[0].(map[string]any)["inlineData"].(map[string]any)
	if ref["mimeType"] != "image/jpeg" || ref["data"] != "cmVm" {
		t.Fatalf("reference part = %v", parts[0])
	}
	if parts[1].(map[string]any)["text"] != "a red apple" {
		t.Fatalf("prompt part = %v", parts[1])
	}
}

func TestGeminiNativeGenerateImage_SendsNoImageSizeToGemini25(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		_, _ = w.Write([]byte(geminiImageResponse("image/png", []byte("\x89PNG"))))
	}))
	defer srv.Close()

	got, err := newTestGeminiImageClient(t, srv.URL, "gemini-2.5-flash-image").GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	img := captured["generationConfig"].(map[string]any)["imageConfig"].(map[string]any)
	if _, sent := img["imageSize"]; sent || img["aspectRatio"] != "1:1" {
		t.Fatalf("imageConfig = %v", img)
	}
	if got.MediaType != "image/png" {
		t.Fatalf("media type = %q", got.MediaType)
	}
}

func TestGeminiNativeGenerateImage_ReadsAResponseLargerThanTheChatLimit(t *testing.T) {
	big := append(append([]byte{}, fakeJPEG...), make([]byte, 11*1024*1024)...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(geminiImageResponse("image/jpeg", big)))
	}))
	defer srv.Close()

	got, err := newTestGeminiImageClient(t, srv.URL, "gemini-3.1-flash-image").GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if len(got.Data) != len(big) {
		t.Fatalf("got %d bytes, want %d", len(got.Data), len(big))
	}
}

func TestGeminiNativeGenerateImage_Refusals(t *testing.T) {
	cases := map[string]struct {
		body   string
		reason string
	}{
		"prohibited content": {
			body:   `{"candidates":[{"content":{},"finishReason":"PROHIBITED_CONTENT","index":0}]}`,
			reason: "PROHIBITED_CONTENT",
		},
		"text only": {
			body:   `{"candidates":[{"content":{"parts":[{"text":"I can't draw that."}]},"finishReason":"STOP"}]}`,
			reason: "held no image",
		},
		"no candidates": {
			body:   `{"candidates":[]}`,
			reason: "no candidates",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			_, err := newTestGeminiImageClient(t, srv.URL, "gemini-3.1-flash-image").GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
			if !errors.Is(err, ErrImageRefused) || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("err = %v, want ErrImageRefused with %q", err, tc.reason)
			}
		})
	}
}

func TestGeminiNativeGenerateImage_HTTPErrors(t *testing.T) {
	cases := map[string]struct {
		status  int
		message string
		billing bool
	}{
		"free tier key": {
			status:  http.StatusTooManyRequests,
			message: "You exceeded your current quota, please check your plan and billing details.\n* Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_free_tier_requests, limit: 0, model: gemini-3.1-flash-image",
			billing: true,
		},
		"ordinary rate limit": {
			status:  http.StatusTooManyRequests,
			message: "Resource has been exhausted (e.g. check quota).",
		},
		"bad request": {
			status:  http.StatusBadRequest,
			message: "Invalid aspect ratio",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintf(w, `{"error":{"code":%d,"message":%q,"status":"X"}}`, tc.status, tc.message)
			}))
			defer srv.Close()

			_, err := newTestGeminiImageClient(t, srv.URL, "gemini-3.1-flash-image").GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
			var perr *ProviderError
			if !errors.As(err, &perr) || perr.StatusCode != tc.status || perr.Message != tc.message {
				t.Fatalf("err = %v", err)
			}
			if errors.Is(err, ErrImageBillingRequired) != tc.billing {
				t.Fatalf("billing = %v, want %v", errors.Is(err, ErrImageBillingRequired), tc.billing)
			}
		})
	}
}

func TestGeminiNativeGenerateImage_BadResponses(t *testing.T) {
	cases := map[string]string{
		"not json":   `{"candidates":`,
		"not base64": `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/jpeg","data":"%%%"}}]},"finishReason":"STOP"}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			_, err := newTestGeminiImageClient(t, srv.URL, "gemini-3.1-flash-image").GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
			var perr *ProviderError
			if !errors.As(err, &perr) || perr.Operation != "parse" {
				t.Fatalf("err = %v, want a parse error", err)
			}
		})
	}
}

func TestGeminiNativeGenerateImage_TransportAndValidationErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing listens any more

	client := newTestGeminiImageClient(t, url, "gemini-3.1-flash-image")
	var perr *ProviderError
	if _, err := client.GenerateImage(context.Background(), ImageRequest{Prompt: "p"}); !errors.As(err, &perr) || perr.Operation != "request" {
		t.Fatalf("closed server: err = %v, want a request error", err)
	}
	if _, err := client.GenerateImage(context.Background(), ImageRequest{Prompt: "p", Aspect: "panorama"}); err == nil {
		t.Fatal("want an error for an unknown aspect")
	}
}
