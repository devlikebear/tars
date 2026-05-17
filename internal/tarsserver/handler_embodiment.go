package tarsserver

import (
	"context"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/embodiment"
	"github.com/rs/zerolog"
)

func newEmbodimentAPIHandler(runtime *agentruntime.Runtime, ingress embodimentIngress, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	handle := func(w http.ResponseWriter, r *http.Request) {
		handleEmbodimentPercept(w, r, runtime, ingress, logger)
	}
	mux.HandleFunc("/v1/embodiment/percept/", handle)
	mux.HandleFunc("/v1/embodiment/percepts/", handle)
	return mux
}

func handleEmbodimentPercept(w http.ResponseWriter, r *http.Request, runtime *agentruntime.Runtime, ingress embodimentIngress, logger zerolog.Logger) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if ingress == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "embodiment subsystem is not configured"})
		return
	}
	provider := embodimentProviderFromPath(r.URL.Path)
	if provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return
	}
	payload, ok := decodeMapBody(w, r, false)
	if !ok {
		return
	}
	text := extractInboundText(payload)
	if runtime != nil && text != "" {
		if _, err := runtime.InboundWebhook(provider, strings.TrimSpace(asString(payload["thread_id"])), text, payload); err != nil {
			logger.Debug().Err(err).Str("provider", provider).Msg("embodiment percept channel persistence skipped")
		}
	}
	result, err := ingress.IngestPayload(r.Context(), provider, payload)
	if err != nil {
		logger.Error().Err(err).Str("provider", provider).Msg("embodiment percept ingest failed")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, embodimentIngestResponse(result))
}

func maybeIngestEmbodimentPayload(ctx context.Context, ingress embodimentIngress, provider string, payload map[string]any, logger zerolog.Logger) {
	if ingress == nil {
		return
	}
	known := ingress.KnownProvider(provider)
	if !embodiment.LooksLikePerceptPayload(provider, payload, known) {
		return
	}
	if _, err := ingress.IngestPayload(ctx, provider, payload); err != nil {
		logger.Warn().Err(err).Str("provider", strings.TrimSpace(provider)).Msg("embodiment percept ingest skipped")
	}
}

func embodimentProviderFromPath(path string) string {
	for _, prefix := range []string{"/v1/embodiment/percept/", "/v1/embodiment/percepts/"} {
		if strings.HasPrefix(path, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(path, prefix))
		}
	}
	return ""
}

func embodimentIngestResponse(result embodiment.IngestResult) map[string]any {
	out := map[string]any{
		"percept_id": result.Percept.ID,
		"provider":   result.Percept.Provider,
		"decision":   result.Decision,
		"cognition":  result.CognitionResult,
	}
	return out
}
