// Package critic implements the session-scoped critic reviewer: a small LLM
// call that inspects a freshly-proposed or just-completed plan and either
// accepts it or returns concrete improvement feedback. The chat handler
// drives it as an OnTurnEnd hook bounded by SessionCritic.MaxIterations so
// each plan transition gets at most N review rounds.
//
// Like the goal judge it is intentionally narrow: callers pass the trigger
// kind (plan_proposed | plan_completed), the plan and tasks snapshot, and a
// trailing chat window; the reviewer returns a structured verdict. Errors
// and unparseable verdicts are treated as "acceptable=false, fall-open" —
// the caller still emits the SSE event but does not inject feedback, so a
// reviewer outage cannot deadlock the chat loop.
package critic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
)

// Trigger kinds match the SessionCritic.LastTrigger field and the SSE
// payload's "trigger" key. New trigger kinds must update both the reviewer
// prompt switch and the hook's plan-transition detector.
const (
	TriggerPlanProposed  = "plan_proposed"
	TriggerPlanCompleted = "plan_completed"
	// TriggerAssistantTurn fires on every assistant turn that did not also
	// cross a plan transition. Enables critic review for sessions (worker,
	// subagent, or main without an active plan) where there is no plan to
	// attach to. The reviewer inspects the last assistant response for
	// general quality issues.
	TriggerAssistantTurn = "assistant_turn"
)

// DefaultRecentMessageWindow is how many trailing chat messages the reviewer
// inspects by default. Kept small to bound prompt cost.
const DefaultRecentMessageWindow = 6

// MaxRecentMessageContentChars bounds each message body included in the
// reviewer prompt so pathological tool outputs cannot inflate cost.
const MaxRecentMessageContentChars = 2000

// Verdict is the structured output of a reviewer call.
type Verdict struct {
	Acceptable bool   `json:"acceptable"`
	Feedback   string `json:"feedback"`
	Reason     string `json:"reason"`
}

// Reviewer evaluates a plan-transition event. Implementations must be safe
// for concurrent use.
type Reviewer interface {
	Review(ctx context.Context, trigger string, plan *session.Plan, tasks []session.Task, recent []llm.ChatMessage) (Verdict, error)
}

// RouterClientFor is the subset of llm.Router that the reviewer needs.
type RouterClientFor interface {
	ClientFor(role llm.Role) (llm.Client, llm.TierResolution, error)
}

// LLMReviewer resolves the configured RoleCritic tier and runs a structured
// JSON-output chat call.
type LLMReviewer struct {
	router RouterClientFor
	role   llm.Role
}

// NewLLMReviewer returns a Reviewer that resolves the critic role on the
// supplied router. Pass an empty role to use llm.RoleCritic.
func NewLLMReviewer(router RouterClientFor, role llm.Role) *LLMReviewer {
	if strings.TrimSpace(string(role)) == "" {
		role = llm.RoleCritic
	}
	return &LLMReviewer{router: router, role: role}
}

const criticSystemPromptPlanProposed = `You are a critical reviewer. The assistant has just PROPOSED a plan for the user's session. Your job is to find concrete gaps, ambiguities, missing edge cases, or risks BEFORE execution begins. Be specific — generic praise or vague concerns are unhelpful. Prefer pointing out at most 3 high-impact issues over an exhaustive list. If the plan looks solid and ready to execute as-is, mark it acceptable.

Reply with a single JSON object on one line and nothing else, matching: {"acceptable": <true|false>, "feedback": "<bulleted concrete improvements; empty string when acceptable>", "reason": "<one short sentence>"}`

const criticSystemPromptPlanCompleted = `You are a critical reviewer. The assistant claims a plan has been COMPLETED. Verify against the plan goal and constraints: were all stated outcomes actually achieved? Any missing tests, follow-ups, verification gaps, or signs of regression? Be specific. If the completion is clean, mark it acceptable.

Reply with a single JSON object on one line and nothing else, matching: {"acceptable": <true|false>, "feedback": "<bulleted concrete improvements; empty string when acceptable>", "reason": "<one short sentence>"}`

const criticSystemPromptAssistantTurn = `You are a critical reviewer. The assistant has just finished a turn responding to the user. Your job is to spot concrete quality issues a fresh pair of eyes would catch: logic errors, gaps in the user's actual request, unstated assumptions, missing verification, factual mistakes, or a response that misses what the user asked. Be specific — generic praise or vague concerns are not useful. Skip stylistic nitpicks. If the response is solid and addresses the user's request as-is, mark it acceptable.

Reply with a single JSON object on one line and nothing else, matching: {"acceptable": <true|false>, "feedback": "<bulleted concrete improvements; empty string when acceptable>", "reason": "<one short sentence>"}`

// Review runs the LLM call. Non-nil error → caller should treat as fail-open
// (skip injection this turn, no state mutation). Plan may be nil only when
// trigger == TriggerAssistantTurn; plan triggers still require a non-nil plan.
func (r *LLMReviewer) Review(ctx context.Context, trigger string, plan *session.Plan, tasks []session.Task, recent []llm.ChatMessage) (Verdict, error) {
	if r == nil || r.router == nil {
		return Verdict{}, errors.New("critic: router not configured")
	}

	var sysPrompt string
	switch trigger {
	case TriggerPlanProposed:
		if plan == nil {
			return Verdict{}, errors.New("critic: plan is nil for plan_proposed trigger")
		}
		sysPrompt = criticSystemPromptPlanProposed
	case TriggerPlanCompleted:
		if plan == nil {
			return Verdict{}, errors.New("critic: plan is nil for plan_completed trigger")
		}
		sysPrompt = criticSystemPromptPlanCompleted
	case TriggerAssistantTurn:
		sysPrompt = criticSystemPromptAssistantTurn
	default:
		return Verdict{}, fmt.Errorf("critic: unknown trigger %q", trigger)
	}

	client, _, err := r.router.ClientFor(r.role)
	if err != nil {
		return Verdict{}, fmt.Errorf("critic: resolve client: %w", err)
	}

	userPayload := buildUserPayload(trigger, plan, tasks, recent)
	resp, err := client.Chat(ctx, []llm.ChatMessage{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: userPayload},
	}, llm.ChatOptions{
		ResponseFormat: &llm.ResponseFormat{Type: llm.ResponseFormatJSONObject},
		ToolChoice:     llm.ToolChoiceNone(),
	})
	if err != nil {
		return Verdict{}, fmt.Errorf("critic: chat: %w", err)
	}

	verdict, parseErr := ParseVerdict(resp.Message.Content)
	if parseErr != nil {
		return Verdict{Acceptable: false, Reason: "unparseable critic output"}, parseErr
	}
	return verdict, nil
}

// ParseVerdict tolerantly extracts a Verdict JSON object from raw text.
// Mirrors goal.ParseVerdict so reviewers and judges share the same lenient
// tolerance to code-fence wrappers and preamble text.
func ParseVerdict(raw string) (Verdict, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return Verdict{}, errors.New("critic: empty response")
	}
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return Verdict{}, errors.New("critic: no JSON object found in response")
	}
	chunk := text[start : end+1]

	var v Verdict
	if err := json.Unmarshal([]byte(chunk), &v); err != nil {
		return Verdict{}, fmt.Errorf("critic: invalid JSON: %w", err)
	}
	v.Feedback = strings.TrimSpace(v.Feedback)
	v.Reason = strings.TrimSpace(v.Reason)
	return v, nil
}

func buildUserPayload(trigger string, plan *session.Plan, tasks []session.Task, recent []llm.ChatMessage) string {
	var b strings.Builder
	b.WriteString("Trigger: ")
	b.WriteString(trigger)
	b.WriteString("\n\n")
	if plan != nil {
		b.WriteString("Plan goal:\n")
		b.WriteString(strings.TrimSpace(plan.Goal))
		b.WriteString("\n")
		if constraints := strings.TrimSpace(plan.Constraints); constraints != "" {
			b.WriteString("Constraints: ")
			b.WriteString(constraints)
			b.WriteString("\n")
		}
		b.WriteString("Plan status: ")
		b.WriteString(strings.TrimSpace(plan.Status))
		b.WriteString("\n\n")
	}

	if len(tasks) > 0 {
		b.WriteString("Tasks:\n")
		for _, t := range tasks {
			title := strings.TrimSpace(t.Title)
			if title == "" {
				title = "(untitled)"
			}
			fmt.Fprintf(&b, "- [%s] %s: %s\n", strings.TrimSpace(t.Status), t.ID, title)
		}
		b.WriteString("\n")
	}

	window := recent
	if len(window) > DefaultRecentMessageWindow {
		window = window[len(window)-DefaultRecentMessageWindow:]
	}
	if len(window) > 0 {
		b.WriteString("Recent messages (oldest first):\n")
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
	}
	return b.String()
}
