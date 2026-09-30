package tarsserver

import (
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
)

// chatTurnText cuts the text a turn streams at the tool calls it makes, so
// the turn can be saved in the order it was shown: text, tools, text,
// tools, reply. Without it a reopened turn showed every tool card first and
// all of its text after them, and a native provider's text before a tool
// call was not saved at all (only the last iteration's reply is returned).
//
// Deltas and tool reports can come from the provider's reader goroutine, so
// it is locked. A nil *chatTurnText records nothing.
type chatTurnText struct {
	mu      sync.Mutex
	pending strings.Builder
}

func newChatTurnText() *chatTurnText { return &chatTurnText{} }

// write adds streamed reply text.
func (t *chatTurnText) write(text string) {
	if t == nil || text == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending.WriteString(text)
}

// take returns the text streamed since the last take: at a tool call, the
// text said before it; at the end of the turn, the reply.
func (t *chatTurnText) take() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	text := t.pending.String()
	t.pending.Reset()
	return text
}

// chatTurnMessages lays out a finished turn for the transcript. When the
// turn said something before one of its tool calls, the messages follow the
// stream: each piece of text as an interim assistant message ahead of the
// tools it led to, then the reply — the text streamed after the last tool,
// passed as trailing. Otherwise, or when the streamed text does not end the
// way the provider's reply does (a provider that did not stream it), the
// layout is the one transcripts have always had: every tool, then the reply.
func chatTurnMessages(chatResp llm.ChatResponse, toolCalls []ToolCallRecord, trailing string, now time.Time) []session.Message {
	reply := session.Message{
		Role:      "assistant",
		Content:   chatResp.Message.Content,
		Timestamp: now,
		// Persisted so a transcript replayed after a restart can hand the
		// signed reasoning blocks back to the provider instead of rebuilding
		// a turn the provider will not recognize.
		ReasoningBlocks: toSessionReasoningBlocks(chatResp.Message.ReasoningBlocks),
	}
	trailing = strings.TrimSpace(trailing)
	interleave := streamedTextEndsReply(chatResp.Message.Content, toolCalls, trailing)

	out := make([]session.Message, 0, 2*len(toolCalls)+1)
	for _, tc := range toolCalls {
		if text := strings.TrimSpace(tc.textBefore); interleave && text != "" {
			out = append(out, session.Message{Role: "assistant", Content: text, Timestamp: now, Interim: true})
		}
		out = append(out, toolCallMessage(tc, now))
	}
	if interleave {
		reply.Content = trailing
	}
	return append(out, reply)
}

// streamedTextEndsReply reports whether the turn said something before a
// tool call and the reply ends with the last text it streamed — the text
// after the last tool, or, for a turn that ended on a tool, the text before
// it (claude-code-cli joins every text block into the reply). A reply the
// provider did not stream ends with neither, and keeps the old layout
// rather than being replaced by text that is not it.
func streamedTextEndsReply(reply string, toolCalls []ToolCallRecord, trailing string) bool {
	last := ""
	for _, tc := range toolCalls {
		if text := strings.TrimSpace(tc.textBefore); text != "" {
			last = text
		}
	}
	if last == "" {
		return false
	}
	if trailing != "" {
		last = trailing
	}
	reply = strings.TrimSpace(reply)
	return reply == "" || strings.HasSuffix(reply, last)
}

func toolCallMessage(tc ToolCallRecord, now time.Time) session.Message {
	return session.Message{
		Role:        "tool",
		Content:     tc.ToolResult,
		Timestamp:   now,
		ToolName:    tc.ToolName,
		ToolCallID:  tc.ToolCallID,
		ToolArgs:    tc.ToolArgs,
		ToolIsError: tc.ToolIsError,
	}
}

// joinInterimText puts a turn's interim text ahead of its reply, the way a
// transcript written before interim messages held it: in one reply.
func joinInterimText(interim []string, reply string) string {
	if len(interim) == 0 {
		return reply
	}
	parts := append([]string(nil), interim...)
	if strings.TrimSpace(reply) != "" {
		parts = append(parts, reply)
	}
	return strings.Join(parts, "\n\n")
}
