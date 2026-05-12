package tarsserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
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
	switch evt := data.(type) {
	case map[string]string:
		s.logger.Debug().Str("event_type", evt["type"]).Msg("chat sse event")
	case map[string]any:
		if eventType, ok := evt["type"].(string); ok {
			s.logger.Debug().Str("event_type", eventType).Msg("chat sse event")
		}
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

func (s *chatStreamWriter) status(phase, message, toolName, toolCallID, toolArgsPreview, toolResultPreview string, toolIsError ...bool) {
	payload := map[string]any{
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
	if len(toolIsError) > 0 && toolIsError[0] {
		payload["tool_is_error"] = true
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

func (s *chatStreamWriter) commandSelected(name, reason string) {
	payload := map[string]string{
		"type":         "status",
		"phase":        "command_selected",
		"message":      "using command " + strings.TrimSpace(name),
		"session_id":   s.sessionID,
		"command_name": strings.TrimSpace(name),
	}
	if strings.TrimSpace(reason) != "" {
		payload["command_reason"] = strings.TrimSpace(reason)
	}
	s.send(payload)
}

func (s *chatStreamWriter) delta(text string) {
	s.send(map[string]string{"type": "delta", "text": text})
}

// reasoning forwards provider-native reasoning / thinking deltas so the
// console can render the chain-of-thought in a separate panel from the
// final assistant content.
func (s *chatStreamWriter) reasoning(text string) {
	s.send(map[string]string{"type": "reasoning_delta", "text": text})
}

func (s *chatStreamWriter) error(err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	s.send(map[string]string{"type": "error", "error": msg})
}

// toolOutputLine forwards a single line streamed by a running tool (e.g.
// exec stdout/stderr). Non-empty lines are tagged with the tool_call_id
// the agent loop bound at invocation time so the console can group lines
// under the right tool call entry.
func (s *chatStreamWriter) toolOutputLine(toolCallID, stream, text string) {
	if s == nil {
		return
	}
	s.send(map[string]string{
		"type":         "tool_output_line",
		"session_id":   s.sessionID,
		"tool_call_id": strings.TrimSpace(toolCallID),
		"stream":       strings.TrimSpace(stream),
		"text":         text,
	})
}

// EmitToolLine implements tool.LineEmitter so the chat handler can plug
// the stream writer directly into the agent loop via context.
func (s *chatStreamWriter) EmitToolLine(toolCallID, stream, text string) {
	s.toolOutputLine(toolCallID, stream, text)
}

func (s *chatStreamWriter) memoryRecall(count int) {
	s.send(map[string]any{
		"type":         "memory_recall",
		"session_id":   s.sessionID,
		"memory_count": count,
	})
}

// tasksChanged broadcasts the current plan/task counts so the console can
// keep the chat pulse-bar Tasks badge live without re-fetching after every
// tool call.
func (s *chatStreamWriter) tasksChanged(st session.SessionTasks) {
	summary := session.TaskSummary(st.Tasks)
	payload := map[string]any{
		"type":             "tasks_changed",
		"session_id":       s.sessionID,
		"task_total":       summary["total"],
		"task_pending":     summary["pending"],
		"task_in_progress": summary["in_progress"],
		"task_completed":   summary["completed"],
		"task_cancelled":   summary["cancelled"],
	}
	if st.Plan != nil {
		payload["plan_goal"] = strings.TrimSpace(st.Plan.Goal)
	}
	s.send(payload)
}

// goalEvent broadcasts a session-goal state change (auto-continue tick,
// satisfied, exhausted, or cleared) so the console can update the goal chip
// without polling the admin API.
func (s *chatStreamWriter) goalEvent(phase, reason string, goal *session.SessionGoal) {
	payload := map[string]any{
		"type":       "goal_event",
		"session_id": s.sessionID,
		"phase":      phase,
	}
	if strings.TrimSpace(reason) != "" {
		payload["reason"] = reason
	}
	if goal != nil {
		payload["goal"] = goal
	}
	s.send(payload)
}

func (s *chatStreamWriter) cancelled() {
	s.send(map[string]string{
		"type":       "cancelled",
		"session_id": s.sessionID,
	})
}

func (s *chatStreamWriter) contextInfo(info map[string]any) {
	info["type"] = "context_info"
	info["session_id"] = s.sessionID
	s.send(info)
}

func (s *chatStreamWriter) compactionApplied(info map[string]any) {
	info["type"] = "compaction_applied"
	info["session_id"] = s.sessionID
	s.send(info)
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
