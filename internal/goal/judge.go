// Package goal implements the session-goal judge: a small LLM call that
// decides whether the user's active goal has been satisfied by recent
// assistant activity.
//
// The judge is intentionally narrow and stateless: callers pass the goal
// description and a window of recent chat messages, and the judge returns a
// boolean verdict plus a short reason. Errors and unparseable verdicts are
// treated as "not satisfied" (fail-open: keep working), never as "satisfied",
// so a judge outage cannot accidentally clear a real goal.
package goal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
)

// DefaultRecentMessageWindow is how many trailing chat messages the judge
// inspects by default.
const DefaultRecentMessageWindow = 6

// MaxRecentMessageContentChars bounds the per-message content slice sent to
// the judge so that pathological transcripts cannot inflate the prompt.
const MaxRecentMessageContentChars = 2000

// Verdict is the structured output of a judge call.
type Verdict struct {
	Satisfied bool   `json:"satisfied"`
	Reason    string `json:"reason"`
}

// Judger reports whether a session goal has been satisfied by the recent
// chat history. Implementations must be safe for concurrent use.
type Judger interface {
	Judge(ctx context.Context, goal string, recent []llm.ChatMessage) (Verdict, error)
}

// RouterClientFor is the subset of llm.Router that the judge needs. Using a
// narrow interface keeps judge testable without a full router stub.
type RouterClientFor interface {
	ClientFor(role llm.Role) (llm.Client, llm.TierResolution, error)
}

// LLMJudger calls the configured llm.RoleGoalJudge tier with a small
// structured prompt and parses the JSON verdict.
type LLMJudger struct {
	router RouterClientFor
	role   llm.Role
}

// NewLLMJudger returns a Judger that resolves the goal_judge role on the
// supplied router. The role override is optional and is exposed mostly for
// tests; pass an empty string to use llm.RoleGoalJudge.
func NewLLMJudger(router RouterClientFor, role llm.Role) *LLMJudger {
	if strings.TrimSpace(string(role)) == "" {
		role = llm.RoleGoalJudge
	}
	return &LLMJudger{router: router, role: role}
}

const judgeSystemPrompt = `You are a strict judge. The user has set a goal for an AI assistant working in their session. Decide whether the goal has been clearly satisfied by the recent assistant work shown below. Be conservative: if the work is partial, ambiguous, in-progress, or still failing verification, answer "satisfied": false. Only answer "satisfied": true when the goal is unambiguously met.

Reply with a single JSON object on one line and nothing else, matching: {"satisfied": <true|false>, "reason": "<one short sentence>"}`

// Judge runs the LLM call. It returns Satisfied=false with a non-nil error
// when something went wrong (network/parse/etc). Callers should treat any
// non-nil error as a signal to stop auto-continuing (fail-open).
func (j *LLMJudger) Judge(ctx context.Context, goal string, recent []llm.ChatMessage) (Verdict, error) {
	if strings.TrimSpace(goal) == "" {
		return Verdict{}, errors.New("goal description is empty")
	}
	if j == nil || j.router == nil {
		return Verdict{}, errors.New("judge: router not configured")
	}

	client, _, err := j.router.ClientFor(j.role)
	if err != nil {
		return Verdict{}, fmt.Errorf("judge: resolve client: %w", err)
	}

	userPayload := buildUserPayload(goal, recent)
	resp, err := client.Chat(ctx, []llm.ChatMessage{
		{Role: "system", Content: judgeSystemPrompt},
		{Role: "user", Content: userPayload},
	}, llm.ChatOptions{
		ResponseFormat: &llm.ResponseFormat{Type: llm.ResponseFormatJSONObject},
		ToolChoice:     llm.ToolChoiceNone(),
	})
	if err != nil {
		return Verdict{}, fmt.Errorf("judge: chat: %w", err)
	}

	verdict, parseErr := ParseVerdict(resp.Message.Content)
	if parseErr != nil {
		return Verdict{Satisfied: false, Reason: "unparseable judge output"}, parseErr
	}
	return verdict, nil
}

// ParseVerdict tolerantly extracts a Verdict JSON object from raw text. It
// strips leading/trailing whitespace and code-fence wrappers and looks for
// the first balanced `{...}` block. Returns an error if no valid JSON object
// with a boolean "satisfied" field is found.
func ParseVerdict(raw string) (Verdict, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return Verdict{}, errors.New("judge: empty response")
	}
	// Strip code fence wrappers (```json ... ```).
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	// Find the first `{` and matching last `}` to be tolerant of preamble.
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return Verdict{}, errors.New("judge: no JSON object found in response")
	}
	chunk := text[start : end+1]

	var v Verdict
	if err := json.Unmarshal([]byte(chunk), &v); err != nil {
		return Verdict{}, fmt.Errorf("judge: invalid JSON: %w", err)
	}
	v.Reason = strings.TrimSpace(v.Reason)
	return v, nil
}

func buildUserPayload(goal string, recent []llm.ChatMessage) string {
	window := recent
	if len(window) > DefaultRecentMessageWindow {
		window = window[len(window)-DefaultRecentMessageWindow:]
	}
	var b strings.Builder
	b.WriteString("Goal:\n")
	b.WriteString(strings.TrimSpace(goal))
	b.WriteString("\n\nRecent messages (oldest first):\n")
	if len(window) == 0 {
		b.WriteString("(none)\n")
	}
	for _, m := range window {
		content := m.Content
		if content == "" && len(m.ToolCalls) > 0 {
			content = "<tool_call>"
		}
		content = strings.TrimSpace(content)
		if len(content) > MaxRecentMessageContentChars {
			content = content[:MaxRecentMessageContentChars] + "…"
		}
		b.WriteString("- [")
		b.WriteString(strings.ToLower(strings.TrimSpace(m.Role)))
		b.WriteString("] ")
		b.WriteString(content)
		b.WriteString("\n")
	}
	return b.String()
}
