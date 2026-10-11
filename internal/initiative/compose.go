package initiative

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/devlikebear/tars/pkg/llm"
)

const (
	maxSpeechLines      = 3
	maxSpeechLineRunes  = 200
	maxSpeechTotalRunes = 480
	maxSpeechIdentity   = 1200
	maxSpeechProfile    = 600
	maxComposeMessages  = 5
	composeTextWindow   = 6 * time.Hour
)

// ErrSpeechEmpty and ErrSpeechToolAttempt let a caller (the tarsserver
// Speaker implementation) map a Compose failure to a fixed delivery reason
// without parsing an error string. Any other error from Compose is a
// transport/provider failure (compose_error).
var (
	ErrSpeechEmpty       = errors.New("initiative: speech composer returned no text")
	ErrSpeechToolAttempt = errors.New("initiative: speech composer attempted tools")
)

// ComposeInput is the context for one speak attempt (tars#1220). It never
// carries the ledger entry id or ChatOptions session fields — Compose is a
// one-shot, tool-free call that is never resumed and never persisted
// (unlike chat turns), matching the isolation computer_use/initiative's
// text-signal backend already use (llm.DecisionOnlyChatOptions).
type ComposeInput struct {
	Intent   Intent
	Now      time.Time
	Location *time.Location
	// Identity is IDENTITY.md's content (tone), always included — it names
	// no user secrets, only how TARS talks.
	Identity string
	// SendsText mirrors SpeakRequest.SendsText: whether Profile/RecentUser
	// may be used at all, following the same same-provider rule as the
	// text-signal backend (tars#1219 §3) applied to the speak role.
	SendsText  bool
	Profile    string
	RecentUser []UserMessage
}

// speechIntentGuidance names what to say for each Speak intent. Greet and
// check_in are the only intents this package ever composes for (tars#1220
// scope); any other intent is a programming error in the caller.
var speechIntentGuidance = map[Intent]string{
	IntentGreet:   "This is a greeting: the user just arrived after being away for a while (or it is a special day). Welcome them back briefly and warmly. Do not ask what they want yet.",
	IntentCheckIn: "This is a check-in: the user has been in a long session, been away a long time, or seemed strained, and has not been checked on recently. Ask, briefly and gently, how they are doing or whether they need anything — do not assume what is wrong.",
}

const speechSystemPrompt = `You are TARS, speaking first and unprompted, to someone you already know well. Write what you would actually say — 1 to 3 short lines, plain text only.

Rules:
- No markdown, no code fences, no bullet points, no quotation marks around the whole message.
- Never call a tool, never ask to run a command, never output JSON.
- Do not repeat verbatim or quote back anything given to you as context; use it only to decide tone and content.
- If no recent conversation or profile is given, speak from the intent and time of day alone, in simple, generic words — do not invent specifics you were not given.
- Stay in the voice described below.`

// buildSpeechMessages renders the system+user messages for one Compose
// call. Pure — no I/O, no randomness — so every branch is table-tested.
func buildSpeechMessages(in ComposeInput) []llm.ChatMessage {
	loc := in.Location
	if loc == nil {
		loc = time.UTC
	}
	now := in.Now.In(loc)
	var sys strings.Builder
	sys.WriteString(speechSystemPrompt)
	if identity := truncateRunes(strings.TrimSpace(in.Identity), maxSpeechIdentity); identity != "" {
		sys.WriteString("\n\nYour voice (from IDENTITY.md):\n")
		sys.WriteString(identity)
	}

	var user strings.Builder
	user.WriteString("intent: ")
	user.WriteString(string(in.Intent))
	user.WriteString("\n")
	user.WriteString(speechIntentGuidance[in.Intent])
	user.WriteString("\nnow: ")
	user.WriteString(now.Format("Mon 2006-01-02 15:04"))
	user.WriteString("\n")

	if in.SendsText {
		var recent []UserMessage
		for _, m := range in.RecentUser {
			if in.Now.Sub(m.At) <= composeTextWindow && strings.TrimSpace(m.Text) != "" {
				recent = append(recent, m)
			}
		}
		if len(recent) > maxComposeMessages {
			recent = recent[len(recent)-maxComposeMessages:]
		}
		if len(recent) > 0 {
			user.WriteString("recent_conversation (newest last, for tone/context only — do not quote):\n")
			for _, m := range recent {
				fmt.Fprintf(&user, "- %q (%s)\n", truncateRunes(strings.TrimSpace(m.Text), maxMessageRunes), ago(in.Now.Sub(m.At)))
			}
		}
		if profile := truncateRunes(strings.TrimSpace(in.Profile), maxSpeechProfile); profile != "" {
			user.WriteString("profile:\n")
			user.WriteString(profile)
			user.WriteString("\n")
		}
	} else {
		user.WriteString("(no recent conversation or profile is available this time — speak from the intent and time alone)\n")
	}

	return []llm.ChatMessage{
		{Role: "system", Content: sys.String()},
		{Role: "user", Content: user.String()},
	}
}

// SpeechComposer calls the configured speak-role LLM to write one
// greet/check_in utterance (System 2). It never resumes or persists an
// upstream session — each attempt is a fresh, isolated, tool-free call.
type SpeechComposer struct {
	Client llm.Client
	// Model is the resolved tier's model name, kept only for callers that
	// want to label usage; it is not sent on the call explicitly (the
	// Client already targets it).
	Model string
}

// Compose asks the composer for one utterance. It never returns partial or
// placeholder text: an empty response is ErrSpeechEmpty, an attempted tool
// call is ErrSpeechToolAttempt, and any other failure is the underlying
// transport/provider error — the caller classifies these into its own
// fixed delivery-reason codes without inspecting error text (tars#1003:
// error messages on a same-provider call can echo the request, which may
// hold the user's own words).
func (c *SpeechComposer) Compose(ctx context.Context, in ComposeInput) (string, error) {
	if c == nil || c.Client == nil {
		return "", fmt.Errorf("initiative: speech composer is not configured")
	}
	opts := llm.DecisionOnlyChatOptions()
	// Unlike the text-signal backend (strict JSON booleans), speech is
	// free-form text — the shared isolation (no tools, Claude Code empty
	// harness tool list + plan mode, strict MCP, no Chrome) stays, but the
	// JSON response-format constraint does not apply here.
	opts.ResponseFormat = nil
	resp, err := c.Client.Chat(ctx, buildSpeechMessages(in), opts)
	if err != nil {
		return "", err
	}
	if llm.AttemptedTools(resp) {
		return "", ErrSpeechToolAttempt
	}
	text := sanitizeSpeechResponse(resp.Message.Content)
	if text == "" {
		return "", ErrSpeechEmpty
	}
	return text, nil
}

// sanitizeSpeechResponse strips one leading/trailing code fence and any
// control characters, keeps at most maxSpeechLines non-empty lines (each
// capped at maxSpeechLineRunes), and caps the whole result at
// maxSpeechTotalRunes. Never returns markdown or raw control bytes,
// regardless of what the model produced.
func sanitizeSpeechResponse(raw string) string {
	cleaned := unwrapSingleCodeFence(raw)
	cleaned = strings.Map(dropSpeechControlRunes, cleaned)
	lines := strings.Split(cleaned, "\n")
	kept := make([]string, 0, maxSpeechLines)
	for _, line := range lines {
		line = strings.TrimSpace(trimSpeechMarkup(line))
		if line == "" {
			continue
		}
		kept = append(kept, truncateRunes(line, maxSpeechLineRunes))
		if len(kept) >= maxSpeechLines {
			break
		}
	}
	return truncateRunes(strings.Join(kept, "\n"), maxSpeechTotalRunes)
}

// trimSpeechMarkup strips the leading list/quote markup a model sometimes
// adds despite being told not to ("- ", "* ", a single pair of wrapping
// quotes), so a line reads as plain prose.
func trimSpeechMarkup(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "- ")
	line = strings.TrimPrefix(line, "* ")
	if len(line) >= 2 && line[0] == '"' && line[len(line)-1] == '"' {
		line = line[1 : len(line)-1]
	}
	return line
}

func dropSpeechControlRunes(r rune) rune {
	if r == '\n' || r == '\t' {
		return r
	}
	if unicode.IsControl(r) {
		return -1
	}
	return r
}
