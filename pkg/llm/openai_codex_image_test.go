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

	"github.com/devlikebear/tars/internal/auth"
)

// fakeJPEG is enough for http.DetectContentType to call it image/jpeg.
var fakeJPEG = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("jpeg body")...)

func codexImageSSE(events ...string) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString("data: " + e + "\n\n")
	}
	return b.String()
}

// codexImageStream mirrors the event order the ChatGPT backend sent in the
// 2026-10 spike, keepalive included.
func codexImageStream(result string) string {
	return codexImageSSE(
		`{"type":"response.created","response":{"status":"in_progress"}}`,
		`{"type":"response.image_generation_call.in_progress","item_id":"ig_1"}`,
		`{"type":"keepalive"}`,
		`{"type":"response.image_generation_call.generating","item_id":"ig_1"}`,
		`{"type":"response.image_generation_call.completed","item_id":"ig_1"}`,
		fmt.Sprintf(`{"type":"response.output_item.done","item":{"id":"ig_1","type":"image_generation_call","status":"completed","action":"generate","output_format":"jpeg","quality":"low","size":"1024x1536","revised_prompt":"a red apple, drawn","result":%q}}`, result),
		`{"type":"response.output_item.done","item":{"type":"message","content":[{"type":"output_text","text":""}]}}`,
		`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":10,"output_tokens":5}}}`,
	)
}

func newTestCodexImageClient(t *testing.T, url string, refresher codexCredentialRefresher) *OpenAICodexClient {
	t.Helper()
	if refresher == nil {
		refresher = func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			t.Fatal("refresh should not be called")
			return auth.CodexCredential{}, nil
		}
	}
	client, err := newOpenAICodexClientWithConfig(
		url, "gpt-5.6-luna", "oauth", "openai-codex", "", DefaultClientConfig(),
		func() (auth.CodexCredential, error) {
			return auth.CodexCredential{AccessToken: "token-1", RefreshToken: "refresh-1", AccountID: "acc-1"}, nil
		},
		refresher,
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}
	return client
}

func TestOpenAICodexGenerateImage_SendsToolRequestAndDecodesImage(t *testing.T) {
	var body map[string]any
	var gotAuth, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		if r.URL.Path != "/codex/responses" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(codexImageStream(base64.StdEncoding.EncodeToString(fakeJPEG))))
	}))
	defer srv.Close()

	got, err := newTestCodexImageClient(t, srv.URL, nil).GenerateImage(context.Background(), ImageRequest{
		Prompt:     "a red apple",
		References: []ContentBlock{{Type: "image", MediaType: "image/png", Data: "cmVm"}},
		Aspect:     ImageAspectPortrait,
	})
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if string(got.Data) != string(fakeJPEG) || got.MediaType != "image/jpeg" || got.RevisedPrompt != "a red apple, drawn" {
		t.Fatalf("result = %+v", got)
	}
	if gotAuth != "Bearer token-1" || gotAccept != "text/event-stream" {
		t.Fatalf("headers: auth=%q accept=%q", gotAuth, gotAccept)
	}

	if body["model"] != "gpt-5.6-luna" || body["stream"] != true || body["store"] != false {
		t.Fatalf("model/stream/store = %v/%v/%v", body["model"], body["stream"], body["store"])
	}
	if !strings.Contains(body["instructions"].(string), "2:3") || !strings.Contains(body["instructions"].(string), "do not call it again") {
		t.Fatalf("instructions = %q", body["instructions"])
	}
	tools := body["tools"].([]any)
	tool := tools[0].(map[string]any)
	if len(tools) != 1 || tool["type"] != "image_generation" || tool["output_format"] != "jpeg" {
		t.Fatalf("tools = %v", tools)
	}
	if _, sent := tool["size"]; sent {
		t.Fatal("size is ignored by the backend and must not be sent")
	}
	if choice := body["tool_choice"].(map[string]any); choice["type"] != "image_generation" {
		t.Fatalf("tool_choice = %v", choice)
	}
	content := body["input"].([]any)[0].(map[string]any)["content"].([]any)
	if text := content[0].(map[string]any); text["type"] != "input_text" || text["text"] != "a red apple" {
		t.Fatalf("content[0] = %v", text)
	}
	if img := content[1].(map[string]any); img["type"] != "input_image" || img["image_url"] != "data:image/png;base64,cmVm" {
		t.Fatalf("content[1] = %v", img)
	}
}

func TestOpenAICodexGenerateImage_ReadsAResultLineLargerThanTheChatScanner(t *testing.T) {
	big := append(append([]byte{}, fakeJPEG...), make([]byte, 2*1024*1024)...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(codexImageStream(base64.StdEncoding.EncodeToString(big))))
	}))
	defer srv.Close()

	got, err := newTestCodexImageClient(t, srv.URL, nil).GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if len(got.Data) != len(big) {
		t.Fatalf("got %d bytes, want %d", len(got.Data), len(big))
	}
}

func TestOpenAICodexGenerateImage_RefreshesOnUnauthorized(t *testing.T) {
	var auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("Authorization"))
		if len(auths) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(codexImageStream(base64.StdEncoding.EncodeToString(fakeJPEG))))
	}))
	defer srv.Close()

	refreshed := 0
	client := newTestCodexImageClient(t, srv.URL, func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
		refreshed++
		return auth.CodexCredential{AccessToken: "token-2", RefreshToken: "refresh-2", AccountID: "acc-1"}, nil
	})
	if _, err := client.GenerateImage(context.Background(), ImageRequest{Prompt: "p"}); err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if refreshed != 1 || len(auths) != 2 || auths[1] != "Bearer token-2" {
		t.Fatalf("refreshed=%d auths=%v", refreshed, auths)
	}
}

func TestOpenAICodexGenerateImage_Refusals(t *testing.T) {
	cases := map[string]struct {
		stream string
		reason string
	}{
		"moderation block": {
			stream: codexImageSSE(
				`{"type":"response.output_item.done","item":{"type":"image_generation_call","status":"failed","error":{"type":"image_generation_user_error","code":"moderation_blocked","message":"blocked"}}}`,
				// The backend would let the routing model try again; the
				// first failure has to end the call.
				fmt.Sprintf(`{"type":"response.output_item.done","item":{"type":"image_generation_call","status":"completed","result":%q}}`, base64.StdEncoding.EncodeToString(fakeJPEG)),
			),
			reason: "moderation_blocked",
		},
		"completed without an image": {
			stream: codexImageSSE(`{"type":"response.completed","response":{"status":"completed"}}`),
			reason: "without an image",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.stream))
			}))
			defer srv.Close()

			_, err := newTestCodexImageClient(t, srv.URL, nil).GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
			if !errors.Is(err, ErrImageRefused) || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("err = %v, want ErrImageRefused with %q", err, tc.reason)
			}
		})
	}
}

func TestOpenAICodexGenerateImage_StreamFailures(t *testing.T) {
	cases := map[string]string{
		"truncated":       codexImageSSE(`{"type":"response.created"}`),
		"response.failed": codexImageSSE(`{"type":"response.failed","response":{"error":{"message":"server exploded"}}}`),
	}
	for name, stream := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(stream))
			}))
			defer srv.Close()

			_, err := newTestCodexImageClient(t, srv.URL, nil).GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
			if err == nil || errors.Is(err, ErrImageRefused) {
				t.Fatalf("err = %v, want a non-refusal failure", err)
			}
		})
	}
}

func TestOpenAICodexGenerateImage_HTTPErrorKeepsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"The 'gpt-5.3-codex-spark' model is not supported when using Codex with a ChatGPT account."}`))
	}))
	defer srv.Close()

	_, err := newTestCodexImageClient(t, srv.URL, nil).GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
	var perr *ProviderError
	if !errors.As(err, &perr) || perr.StatusCode != http.StatusBadRequest || !strings.Contains(perr.Message, "not supported") {
		t.Fatalf("err = %v", err)
	}
}

func TestImageRequestValidate(t *testing.T) {
	cases := map[string]ImageRequest{
		"empty prompt":     {Prompt: "  "},
		"non-image ref":    {Prompt: "p", References: []ContentBlock{{Type: "text", Text: "x"}}},
		"ref without data": {Prompt: "p", References: []ContentBlock{{Type: "image", MediaType: "image/png"}}},
		"unknown aspect":   {Prompt: "p", Aspect: "panorama"},
		"too many refs":    {Prompt: "p", References: make([]ContentBlock, maxImageReferences+1)},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := req.validate("test"); err == nil {
				t.Fatal("want an error")
			}
		})
	}
	if aspect, err := (ImageRequest{Prompt: "p"}).validate("test"); err != nil || aspect != ImageAspectSquare {
		t.Fatalf("default aspect = %q, %v", aspect, err)
	}
}

func TestOpenAICodexGenerateImage_BadResults(t *testing.T) {
	cases := map[string]string{
		"not base64":   "%%%",
		"not an image": base64.StdEncoding.EncodeToString([]byte("plain text, not a picture")),
		"bad event":    "",
	}
	for name, result := range cases {
		t.Run(name, func(t *testing.T) {
			stream := codexImageStream(result)
			if name == "bad event" {
				stream = "data: {not json\n\n"
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(stream))
			}))
			defer srv.Close()

			_, err := newTestCodexImageClient(t, srv.URL, nil).GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
			var perr *ProviderError
			if !errors.As(err, &perr) || perr.Operation != "parse" {
				t.Fatalf("err = %v, want a parse error", err)
			}
		})
	}
}

func TestOpenAICodexGenerateImage_RefreshFailureIsAnAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := newTestCodexImageClient(t, srv.URL, func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
		return auth.CodexCredential{}, errors.New("refresh token revoked")
	})
	_, err := client.GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
	var perr *ProviderError
	if !errors.As(err, &perr) || perr.Operation != "auth" || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("err = %v, want the refresh failure", err)
	}
}

func TestOpenAICodexGenerateImage_RejectsAnInvalidRequestBeforeSending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("an invalid request must not reach the backend")
	}))
	defer srv.Close()

	if _, err := newTestCodexImageClient(t, srv.URL, nil).GenerateImage(context.Background(), ImageRequest{}); err == nil {
		t.Fatal("want an error for an empty prompt")
	}
}
