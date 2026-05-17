package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/embodiment"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

type recordingEmbodimentIngress struct {
	mu        sync.Mutex
	provider  string
	payload   map[string]any
	known     map[string]bool
	result    embodiment.IngestResult
	callCount int
}

func (r *recordingEmbodimentIngress) KnownProvider(provider string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.known[strings.TrimSpace(strings.ToLower(provider))]
}

func (r *recordingEmbodimentIngress) IngestPayload(_ context.Context, provider string, payload map[string]any) (embodiment.IngestResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.provider = provider
	r.payload = payload
	r.callCount++
	if r.result.Percept.ID == "" {
		r.result.Percept.ID = "percept_1"
	}
	return r.result, nil
}

func (r *recordingEmbodimentIngress) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

func TestWebhookInboundEmbodimentPerceptPersistsAndIngests(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	ingress := &recordingEmbodimentIngress{
		known: map[string]bool{"stackchan": true},
		result: embodiment.IngestResult{
			Decision:        embodiment.GateDecision{Trigger: true, Mode: embodiment.GateModeDirective},
			CognitionResult: embodiment.CognitionResult{Triggered: true, RunID: "run_1"},
		},
	}
	h := newChannelsAPIHandlerWithEmbodiment(runtime, ingress, zerolog.New(io.Discard))

	payload, _ := json.Marshal(map[string]any{
		"source":    "stackchan",
		"summary":   "The owner asked for status.",
		"text":      "The owner asked for status.",
		"identity":  "owner",
		"audio_ref": "obs.wav",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/channels/webhook/inbound/stackchan", bytes.NewReader(payload))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ingress.calls() != 1 || ingress.provider != "stackchan" {
		t.Fatalf("expected one stackchan ingress, got provider=%q calls=%d", ingress.provider, ingress.calls())
	}
	messages, err := runtime.MessageRead("stackchan", 10)
	if err != nil {
		t.Fatalf("read channel: %v", err)
	}
	if len(messages) != 1 || messages[0].Text != "The owner asked for status." {
		t.Fatalf("expected persisted channel message, got %+v", messages)
	}
}

func TestWebhookInboundNonEmbodimentPayloadKeepsExistingBehavior(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	ingress := &recordingEmbodimentIngress{known: map[string]bool{"stackchan": true}}
	h := newChannelsAPIHandlerWithEmbodiment(runtime, ingress, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/channels/webhook/inbound/general", strings.NewReader(`{"text":"plain webhook"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ingress.calls() != 0 {
		t.Fatalf("expected no embodiment ingress for plain webhook")
	}
}

func TestEmbodimentPerceptAPIIngestsAndPersists(t *testing.T) {
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:                true,
		WorkspaceDir:           t.TempDir(),
		ChannelsWebhookEnabled: true,
	})
	t.Cleanup(func() {
		if err := runtime.Close(context.Background()); err != nil {
			t.Fatalf("close runtime: %v", err)
		}
	})
	ingress := &recordingEmbodimentIngress{known: map[string]bool{"host": true}}
	h := newEmbodimentAPIHandler(runtime, ingress, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/embodiment/percept/host", strings.NewReader(`{
		"x-embodiment": true,
		"modality": "audio",
		"owner": "owner",
		"summary": "Owner asked a question."
	}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ingress.calls() != 1 || ingress.provider != "host" {
		t.Fatalf("expected host ingress, got provider=%q calls=%d", ingress.provider, ingress.calls())
	}
	messages, err := runtime.MessageRead("host", 10)
	if err != nil {
		t.Fatalf("read channel: %v", err)
	}
	if len(messages) != 1 || messages[0].Text != "Owner asked a question." {
		t.Fatalf("expected dedicated endpoint to persist channel message, got %+v", messages)
	}
}

func TestEmbodimentPerceptAPITriggersAgentRuntimeForOwnerVoice(t *testing.T) {
	store := session.NewStore(t.TempDir())
	sess, err := store.Create("body")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:                true,
		WorkspaceDir:           t.TempDir(),
		SessionStore:           store,
		ChannelsWebhookEnabled: true,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			if !strings.Contains(prompt, "Owner asked a question.") {
				t.Fatalf("prompt missing percept summary: %q", prompt)
			}
			return "ack body", nil
		},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := runtime.Close(ctx); err != nil {
			t.Fatalf("close runtime: %v", err)
		}
	})
	subsystem := embodiment.NewWithOptions(config.EmbodimentConfig{
		Enabled: true,
		Providers: []config.EmbodimentProviderConfig{{
			Name:               "host",
			Enabled:            true,
			Transport:          "webhook",
			Capabilities:       []string{"hearing", "speech"},
			MinTriggerInterval: "0s",
		}},
	}, zerolog.New(io.Discard), embodiment.Options{
		Runtime:          runtime,
		DefaultSessionID: sess.ID,
	})
	h := newEmbodimentAPIHandler(runtime, subsystem, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/embodiment/percept/host", strings.NewReader(`{
		"x-embodiment": true,
		"modality": "audio",
		"owner": "owner",
		"summary": "Owner asked a question."
	}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Cognition embodiment.CognitionResult `json:"cognition"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Cognition.Triggered || body.Cognition.RunID == "" {
		t.Fatalf("expected triggered cognition, got %+v", body.Cognition)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	final, err := runtime.Wait(waitCtx, body.Cognition.RunID)
	if err != nil {
		t.Fatalf("wait run: %v", err)
	}
	if final.Status != agentruntime.RunStatusCompleted || final.Response != "ack body" || final.SessionID != sess.ID {
		t.Fatalf("unexpected final run: %+v", final)
	}
}
