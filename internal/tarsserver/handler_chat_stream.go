package tarsserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/rs/zerolog"
)

type chatStreamWriter struct {
	w         http.ResponseWriter
	flusher   http.Flusher
	sessionID string
	logger    zerolog.Logger
}

func newChatStreamWriter(w http.ResponseWriter, sessionID string, logger zerolog.Logger) *chatStreamWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	return &chatStreamWriter{
		w:         w,
		flusher:   flusher,
		sessionID: sessionID,
		logger:    logger,
	}
}

func (s *chatStreamWriter) send(data any) {
	if s == nil {
		return
	}
	jsonData, _ := json.Marshal(data)
	_, _ = fmt.Fprintf(s.w, "data: %s\n\n", jsonData)
	if evt, ok := data.(map[string]string); ok {
		s.logger.Debug().Str("event_type", evt["type"]).Msg("chat sse event")
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

func (s *chatStreamWriter) status(phase, message, toolName, toolCallID, toolArgsPreview, toolResultPreview string) {
	payload := map[string]string{
		"type":       "status",
		"phase":      phase,
		"message":    message,
		"session_id": s.sessionID,
	}
	if strings.TrimSpace(toolName) != "" {
		payload["tool_name"] = strings.TrimSpace(toolName)
	}
	if strings.TrimSpace(toolCallID) != "" {
		payload["tool_call_id"] = strings.TrimSpace(toolCallID)
	}
	if strings.TrimSpace(toolArgsPreview) != "" {
		payload["tool_args_preview"] = strings.TrimSpace(toolArgsPreview)
	}
	if strings.TrimSpace(toolResultPreview) != "" {
		payload["tool_result_preview"] = strings.TrimSpace(toolResultPreview)
	}
	s.send(payload)
}

func (s *chatStreamWriter) skillSelected(name, reason string) {
	payload := map[string]string{
		"type":       "status",
		"phase":      "skill_selected",
		"message":    "using skill " + strings.TrimSpace(name),
		"session_id": s.sessionID,
		"skill_name": strings.TrimSpace(name),
	}
	if strings.TrimSpace(reason) != "" {
		payload["skill_reason"] = strings.TrimSpace(reason)
	}
	s.send(payload)
}

func (s *chatStreamWriter) delta(text string) {
	s.send(map[string]string{"type": "delta", "text": text})
}

func (s *chatStreamWriter) error(err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	s.send(map[string]string{"type": "error", "error": msg})
}

func (s *chatStreamWriter) done(usage llm.Usage) {
	s.send(map[string]any{
		"type":       "done",
		"session_id": s.sessionID,
		"usage": map[string]int{
			"input_tokens":       usage.InputTokens,
			"output_tokens":      usage.OutputTokens,
			"cached_tokens":      usage.CachedTokens,
			"cache_read_tokens":  usage.CacheReadTokens,
			"cache_write_tokens": usage.CacheWriteTokens,
		},
	})
}
