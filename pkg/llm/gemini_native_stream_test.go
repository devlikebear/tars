package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// geminiStreamTestTimeout stands in for the 30s default HTTPTimeout so the
// tests can stall past it quickly.
const geminiStreamTestTimeout = 200 * time.Millisecond

// newGeminiStreamTestClient serves the model preflight and hands every
// streamGenerateContent request to stream.
func newGeminiStreamTestClient(t *testing.T, stream http.HandlerFunc) *GeminiNativeClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1beta/models/gemini-test" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"models/gemini-test","supportedGenerationMethods":["generateContent"]}`))
			return
		}
		if r.URL.Path != "/v1beta/models/gemini-test:streamGenerateContent" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
			return
		}
		stream(w, r)
	}))
	t.Cleanup(srv.Close)

	config := DefaultClientConfig()
	config.HTTPTimeout = geminiStreamTestTimeout
	client, err := newGeminiNativeClientWithConfig(srv.URL+"/v1beta", "gemini-key", "gemini-test", config)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func chatGeminiStream(t *testing.T, client *GeminiNativeClient) (ChatResponse, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return client.Chat(ctx, []ChatMessage{{Role: "user", Content: "write the scene"}}, ChatOptions{
		OnDelta: func(string) {},
		Tools: []ToolSchema{{
			Type:     "function",
			Function: ToolFunctionSchema{Name: "write_scene", Parameters: json.RawMessage(`{"type":"object"}`)},
		}},
	})
}

func writeSSE(t *testing.T, w http.ResponseWriter, data string) {
	t.Helper()
	if _, err := w.Write([]byte("data: " + data + "\n\n")); err != nil {
		t.Errorf("write sse: %v", err)
	}
	w.(http.Flusher).Flush()
}

func sleepPastTimeout(r *http.Request) {
	select {
	case <-time.After(3 * geminiStreamTestTimeout):
	case <-r.Context().Done():
	}
}

const geminiWriteSceneChunk = `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"write_scene","args":{"text":"long"}},"thoughtSignature":"c2ln"}]},"finishReason":"STOP"}]}`

func TestGeminiNativeStreaming_OutlivesHTTPTimeout(t *testing.T) {
	tests := []struct {
		name   string
		stream http.HandlerFunc
	}{
		{
			name: "headers delayed past HTTPTimeout",
			stream: func(w http.ResponseWriter, r *http.Request) {
				sleepPastTimeout(r)
				w.Header().Set("Content-Type", "text/event-stream")
				writeSSE(t, w, geminiWriteSceneChunk)
			},
		},
		{
			name: "body stalls past HTTPTimeout after headers",
			stream: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				sleepPastTimeout(r)
				writeSSE(t, w, geminiWriteSceneChunk)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := chatGeminiStream(t, newGeminiStreamTestClient(t, tt.stream))
			if err != nil {
				t.Fatalf("chat: %v", err)
			}
			if len(resp.Message.ToolCalls) != 1 {
				t.Fatalf("expected one tool call, got %+v", resp.Message.ToolCalls)
			}
			call := resp.Message.ToolCalls[0]
			if call.Name != "write_scene" || call.Arguments != `{"text":"long"}` {
				t.Fatalf("unexpected tool call: %+v", call)
			}
			if call.ThoughtSignature != base64.StdEncoding.EncodeToString([]byte("sig")) {
				t.Fatalf("thought signature lost: %q", call.ThoughtSignature)
			}
			if resp.StopReason != "tool_calls" {
				t.Fatalf("expected tool_calls stop reason, got %q", resp.StopReason)
			}
		})
	}
}

func TestGeminiNativeStreaming_StillHonorsContextCancel(t *testing.T) {
	client := newGeminiStreamTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	ctx, cancel := context.WithTimeout(context.Background(), geminiStreamTestTimeout)
	defer cancel()
	_, err := client.Chat(ctx, []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{OnDelta: func(string) {}})
	if err == nil {
		t.Fatal("expected an error once the context expires")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline error, got %v", err)
	}
}

func TestGeminiNativeStreaming_TruncatedStreamIsAnError(t *testing.T) {
	tests := []struct {
		name   string
		stream http.HandlerFunc
	}{
		{
			name: "connection aborted mid-line",
			stream: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				writeSSE(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hel"}]}}]}`)
				_, _ = w.Write([]byte(`data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"write_`))
				w.(http.Flusher).Flush()
				panic(http.ErrAbortHandler)
			},
		},
		{
			name: "line longer than the scanner buffer",
			stream: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				big := strings.Repeat("x", 2*1024*1024)
				// The client gives up partway through, so the write error is expected.
				_, _ = w.Write([]byte(`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"` + big + `"}]}}]}` + "\n\n"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := chatGeminiStream(t, newGeminiStreamTestClient(t, tt.stream))
			if err == nil {
				t.Fatal("expected an error for a truncated stream")
			}
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) || providerErr.Operation != "stream" {
				t.Fatalf("expected a gemini-native stream ProviderError, got %T %v", err, err)
			}
		})
	}
}

func TestGeminiNativeStreaming_ToolCallIDs(t *testing.T) {
	tests := []struct {
		name    string
		chunks  []string
		wantIDs []string
		wantSig []string
	}{
		{
			name: "id-less calls in separate chunks are both kept",
			chunks: []string{
				`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"write_scene","args":{"text":"a"}},"thoughtSignature":"c2ln"}]}}]}`,
				`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"read_scene","args":{"id":"b"}}}]},"finishReason":"STOP"}]}`,
			},
			wantIDs: []string{"tool_call_0", "tool_call_1"},
			wantSig: []string{"c2ln", ""},
		},
		{
			name: "identical id-less calls in separate chunks are both kept",
			chunks: []string{
				`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"write_scene","args":{"text":"a"}}}]}}]}`,
				`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"write_scene","args":{"text":"a"}}}]},"finishReason":"STOP"}]}`,
			},
			wantIDs: []string{"tool_call_0", "tool_call_1"},
			wantSig: []string{"", ""},
		},
		{
			name: "id-less call after text in the same chunk",
			chunks: []string{
				`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"},{"functionCall":{"name":"write_scene","args":{"text":"a"}}}]},"finishReason":"STOP"}]}`,
			},
			wantIDs: []string{"tool_call_0"},
			wantSig: []string{""},
		},
		{
			name: "server-provided duplicate id is deduped",
			chunks: []string{
				`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call-1","name":"write_scene","args":{"text":"a"}},"thoughtSignature":"c2ln"}]}}]}`,
				`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call-1","name":"write_scene","args":{"text":"a"}}}]}}]}`,
				`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call-2","name":"read_scene","args":{}}}]},"finishReason":"STOP"}]}`,
			},
			wantIDs: []string{"call-1", "call-2"},
			wantSig: []string{"c2ln", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newGeminiStreamTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for _, chunk := range tt.chunks {
					writeSSE(t, w, chunk)
				}
			})
			resp, err := chatGeminiStream(t, client)
			if err != nil {
				t.Fatalf("chat: %v", err)
			}
			if len(resp.Message.ToolCalls) != len(tt.wantIDs) {
				t.Fatalf("expected %d tool calls, got %+v", len(tt.wantIDs), resp.Message.ToolCalls)
			}
			for i, call := range resp.Message.ToolCalls {
				if call.ID != tt.wantIDs[i] {
					t.Errorf("call %d: id = %q, want %q", i, call.ID, tt.wantIDs[i])
				}
				if call.ThoughtSignature != tt.wantSig[i] {
					t.Errorf("call %d: thought signature = %q, want %q", i, call.ThoughtSignature, tt.wantSig[i])
				}
			}
		})
	}
}
