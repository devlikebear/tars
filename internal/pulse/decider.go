package pulse

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
)

// PulseDecideToolName is the fixed tool name the decider LLM must call.
// It must match the tool registered on the pulse tool Registry in
// internal/tool. Keep this constant in sync with that registration.
const PulseDecideToolName = "pulse_decide"

// pulseDecideSchema is the JSON Schema handed to the LLM. It enforces the
// shape of an acceptable Decision. Keeping the schema here (next to the
// parser) ensures the two stay in lockstep.
var pulseDecideSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "action": {
      "type": "string",
      "enum": ["ignore", "notify", "autofix"],
      "description": "what pulse should do in response to the observed signals"
    },
    "severity": {
      "type": "string",
      "enum": ["info", "warn", "error", "critical"],
      "description": "how urgent this situation is"
    },
    "title": {
      "type": "string",
      "description": "short user-facing headline; required for notify"
    },
    "summary": {
      "type": "string",
      "description": "1-3 sentence explanation suitable for the user"
    },
    "details": {
      "type": "object",
      "additionalProperties": true,
      "description": "optional structured context"
    },
    "autofix_name": {
      "type": "string",
      "description": "required when action=autofix; must be one of the allowed autofixes"
    }
  },
  "required": ["action", "severity"],
  "additionalProperties": false
}`)

// PulseDecideToolSchema returns the llm.ToolSchema the decider passes to
// the model. Callers that wire pulse into an HTTP handler reuse this to
// register the same schema on their pulse-scoped tool Registry.
func PulseDecideToolSchema() llm.ToolSchema {
	return llm.ToolSchema{
		Type: "function",
		Function: llm.ToolFunctionSchema{
			Name:        PulseDecideToolName,
			Description: "classify the current pulse tick into ignore, notify, or autofix",
			Parameters:  pulseDecideSchema,
		},
	}
}

// DeciderPolicy carries the runtime-configured knobs the decider shows to
// the LLM so it can reason about what autofixes are allowed and what the
// minimum severity floor is.
type DeciderPolicy struct {
	AllowedAutofixes []string
	MinSeverity      Severity
}

// Decider turns a set of signals into a Decision by calling the LLM. It
// does not perform any side-effects; notification and autofix execution
// happen downstream.
//
// Decider resolves its client from the router via RolePulseDecider on
// every call, so operators can retarget pulse's model at runtime by
// editing llm_role_pulse_decider in config.
type Decider struct {
	router llm.Router
	policy DeciderPolicy
}

// NewDecider constructs a Decider bound to an llm.Router and a policy.
// The router's RolePulseDecider mapping decides which tier (typically
// light) serves the classification calls.
func NewDecider(router llm.Router, policy DeciderPolicy) *Decider {
	return &Decider{router: router, policy: policy}
}

// Decide calls the LLM with a signal summary and returns the parsed
// Decision. Errors from the LLM or malformed tool calls propagate up so
// the caller can record them on the TickOutcome; the caller is expected
// to treat decider errors as non-fatal (the tick becomes an ignore).
func (d *Decider) Decide(ctx context.Context, signals []Signal) (Decision, error) {
	if d == nil || d.router == nil {
		return Decision{}, fmt.Errorf("decider not configured")
	}
	if len(signals) == 0 {
		return Decision{}, fmt.Errorf("decide called with no signals")
	}

	client, resolution, err := d.router.ClientFor(llm.RolePulseDecider)
	if err != nil {
		return Decision{}, fmt.Errorf("pulse router resolve: %w", err)
	}

	prompt := buildDeciderPrompt(signals, d.policy)
	if deciderRepliesInText(resolution.Provider) {
		return d.decideInText(ctx, client, prompt)
	}
	messages := []llm.ChatMessage{
		{Role: "system", Content: pulseSystemPrompt},
		{Role: "user", Content: prompt + "\nCall pulse_decide exactly once with your classification."},
	}
	opts := llm.ChatOptions{
		Tools:      []llm.ToolSchema{PulseDecideToolSchema()},
		ToolChoice: llm.ToolChoiceRequired(),
	}
	if strings.EqualFold(strings.TrimSpace(resolution.Provider), "openai-codex") {
		opts.OnDelta = func(string) {}
	}
	resp, err := client.Chat(ctx, messages, opts)
	if err != nil {
		return Decision{}, fmt.Errorf("pulse llm chat: %w", err)
	}
	return parseDecideResponse(resp.Message, d.policy)
}

// deciderRepliesInText reports provider kinds that cannot be handed the
// pulse_decide tool: CLI-backed providers run their own tool loop and never
// see TARS tool schemas, so a tool-call request to them always came back
// without one. They are asked for the same fields as one JSON object.
func deciderRepliesInText(kind string) bool {
	_, declared := llm.CapabilitiesFor(kind)
	return declared && !llm.SupportsCapability(kind, llm.CapToolCalling)
}

// decideInText asks for the classification as a JSON object in the reply
// text. The call is decision-only: none of the provider's own tools, one
// turn, no project settings, which also keeps a CLI call small.
func (d *Decider) decideInText(ctx context.Context, client llm.Client, prompt string) (Decision, error) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: pulseTextSystemPrompt},
		{Role: "user", Content: prompt + "\nReply with the JSON object only."},
	}
	resp, err := client.Chat(ctx, messages, llm.ChatOptions{
		ToolChoice:               llm.ToolChoiceNone(),
		ResponseFormat:           &llm.ResponseFormat{Type: llm.ResponseFormatJSONObject},
		ClaudeCodePermissionMode: "plan",
		ClaudeCodeHarness:        &llm.ClaudeCodeHarnessOptions{Tools: []string{}, SafeMode: true, StrictMCP: true, DisableChrome: true, MaxTurns: 1},
	})
	if err != nil {
		return Decision{}, fmt.Errorf("pulse llm chat: %w", err)
	}
	object, ok := firstJSONObject(resp.Message.Content)
	if !ok {
		return Decision{}, fmt.Errorf("decider reply had no JSON object")
	}
	return parseDecisionArguments(object, d.policy)
}

// firstJSONObject returns the first complete JSON object in text, so a
// reply wrapped in a code fence or a sentence still parses.
func firstJSONObject(text string) (string, bool) {
	start := strings.IndexByte(text, '{')
	for start >= 0 {
		var raw json.RawMessage
		if err := json.NewDecoder(strings.NewReader(text[start:])).Decode(&raw); err == nil {
			return string(raw), true
		}
		next := strings.IndexByte(text[start+1:], '{')
		if next < 0 {
			break
		}
		start += next + 1
	}
	return "", false
}

const pulseActions = `
  - "ignore"  : the situation is benign, transient, or already resolving
  - "notify"  : the user should be alerted, but no automatic action is appropriate
  - "autofix" : an allowed autofix can safely resolve the issue
`

const pulseRules = `Rules:
  - Always choose the least invasive action that addresses the signals.
  - Only select "autofix" when the named autofix is in the allowed list.
  - For stalled_chat signals, select "autofix" only when signal details include can_auto_resume=true.
  - Set severity conservatively; prefer "warn" unless signals clearly exceed it.`

// pulseTextSystemPrompt is pulseSystemPrompt for providers that cannot be
// given the pulse_decide tool: the same classification and fields, as JSON.
const pulseTextSystemPrompt = `You are the pulse watchdog classifier for the TARS system.
You receive a bundle of signals from cron, agent runtime, disk, and telegram delivery.
Your ONLY job is to reply with one JSON object choosing one of three actions:
` + pulseActions + `
Fields: "action" (ignore|notify|autofix) and "severity" (info|warn|error|critical) are required.
"title" (short headline, required for notify), "summary" (1-3 sentences) and
"autofix_name" (required for autofix, one of the allowed autofixes) are optional.

` + pulseRules + `
  - Do not use any tools. Reply with the JSON object and nothing else.`

// pulseSystemPrompt is intentionally terse. The LLM here is a classifier,
// not a general assistant: its only job is to pick one of three actions.
const pulseSystemPrompt = `You are the pulse watchdog classifier for the TARS system.
You receive a bundle of signals from cron, agent runtime, disk, and telegram delivery.
Your ONLY job is to call the pulse_decide tool with one of three actions:
` + pulseActions + `
` + pulseRules + `
  - Do not call any other tools. Do not include freeform commentary.`

func buildDeciderPrompt(signals []Signal, policy DeciderPolicy) string {
	var b strings.Builder
	b.WriteString("## Signals\n")
	for i, s := range signals {
		fmt.Fprintf(&b, "%d. [%s / %s] %s\n", i+1, s.Kind, s.Severity, s.Summary)
		if len(s.Details) > 0 {
			if raw, err := json.Marshal(s.Details); err == nil {
				fmt.Fprintf(&b, "   details: %s\n", raw)
			}
		}
	}
	b.WriteString("\n## Policy\n")
	if len(policy.AllowedAutofixes) == 0 {
		b.WriteString("allowed_autofixes: (none — autofix is disabled)\n")
	} else {
		fmt.Fprintf(&b, "allowed_autofixes: %s\n", strings.Join(policy.AllowedAutofixes, ", "))
	}
	fmt.Fprintf(&b, "min_severity: %s\n", policy.MinSeverity)
	return b.String()
}

func parseDecideResponse(msg llm.ChatMessage, policy DeciderPolicy) (Decision, error) {
	if len(msg.ToolCalls) == 0 {
		return Decision{}, fmt.Errorf("decider response had no tool calls")
	}
	// Use the first pulse_decide call; ignore any extras.
	var call *llm.ToolCall
	for i := range msg.ToolCalls {
		if msg.ToolCalls[i].Name == PulseDecideToolName {
			call = &msg.ToolCalls[i]
			break
		}
	}
	if call == nil {
		return Decision{}, fmt.Errorf("decider did not call %s", PulseDecideToolName)
	}
	return parseDecisionArguments(call.Arguments, policy)
}

// parseDecisionArguments validates one classification, whether it arrived
// as pulse_decide arguments or as the JSON object of a text reply.
func parseDecisionArguments(arguments string, policy DeciderPolicy) (Decision, error) {
	var raw struct {
		Action      string         `json:"action"`
		Severity    string         `json:"severity"`
		Title       string         `json:"title"`
		Summary     string         `json:"summary"`
		Details     map[string]any `json:"details"`
		AutofixName string         `json:"autofix_name"`
	}
	if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
		return Decision{}, fmt.Errorf("parse pulse_decide arguments: %w", err)
	}
	action, err := ParseAction(raw.Action)
	if err != nil {
		return Decision{}, fmt.Errorf("invalid action: %w", err)
	}
	sev, err := ParseSeverity(raw.Severity)
	if err != nil {
		return Decision{}, fmt.Errorf("invalid severity: %w", err)
	}
	if action == ActionAutofix {
		if raw.AutofixName == "" {
			return Decision{}, fmt.Errorf("autofix action requires autofix_name")
		}
		if !slices.Contains(policy.AllowedAutofixes, raw.AutofixName) {
			return Decision{}, fmt.Errorf("autofix %q is not in the allowed list", raw.AutofixName)
		}
	}
	if action == ActionNotify && strings.TrimSpace(raw.Title) == "" {
		return Decision{}, fmt.Errorf("notify action requires a title")
	}
	return Decision{
		Action:      action,
		Severity:    sev,
		Title:       raw.Title,
		Summary:     raw.Summary,
		Details:     raw.Details,
		AutofixName: raw.AutofixName,
	}, nil
}
