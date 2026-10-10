package tarsserver

import (
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

const (
	maxInitiativeContextBytes = 600

	initiativeContextOpen  = "<initiative-context>"
	initiativeContextClose = "</initiative-context>"
)

// initiativeResumeProviderKinds names the provider kinds that resume an
// upstream session (ResumeSessionID) instead of replaying the full
// transcript. A message TARS appended on its own (an initiative-authored
// assistant message, tars#1220) never reaches these providers unless this
// package puts it in the next turn's hidden context — the model otherwise
// sees only the user's reply, with no idea what it replies to.
var initiativeResumeProviderKinds = map[string]bool{
	"claude-code-cli": true,
	"antigravity-cli": true,
}

// mostRecentUnansweredInitiativeMessage walks the transcript from the end
// and returns the most recent initiative-authored assistant message, as
// long as the user has not already spoken since (a "user" message found
// first stops the walk with no message — the user's own reply already
// followed it in the transcript, which resuming providers do see). nil,
// nil when there is no such message or the transcript is empty/missing.
func mostRecentUnansweredInitiativeMessage(path string) (*session.Message, error) {
	msgs, err := session.ReadMessages(path)
	if err != nil {
		return nil, err
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role == "user" {
			return nil, nil
		}
		if m.Role == "assistant" && m.Initiative != nil {
			return &msgs[i], nil
		}
	}
	return nil, nil
}

// appendInitiativeContext puts a one-time hidden note after the user's
// message, right alongside appendConsoleContext/appendReviewNotes: "you
// spoke first a moment ago, the user's message below may be a reply to
// that." It never repeats on a later turn, because by then the user's own
// reply sits in the transcript between the initiative message and this
// one — mostRecentUnansweredInitiativeMessage stops there. A slash command
// keeps its arguments clean, like the other two hidden blocks.
func appendInitiativeContext(message string, note *session.Message) string {
	if note == nil || strings.HasPrefix(strings.TrimSpace(message), "/") {
		return message
	}
	text := clipBytes(strings.TrimSpace(note.Content), maxInitiativeContextBytes)
	if text == "" {
		return message
	}
	body := fmt.Sprintf(
		"You spoke first, unprompted, at %s — before the user's message below. It may be a reply to what you said, or unrelated; read it either way.\nWhat you said: %q",
		note.Timestamp.UTC().Format("2006-01-02T15:04Z"), text,
	)
	return strings.TrimRight(message, "\n") + "\n\n" + initiativeContextOpen + "\n" + body + "\n" + initiativeContextClose
}

// maybeAppendInitiativeContext decides whether this turn's provider
// resumes an upstream session (tars#1220) and, if so, looks up and appends
// the hidden note. providerKind is the resolved tier's Kind for this turn
// (TierResolution.Provider — only claude-code-cli/antigravity-cli resume;
// every other provider, including an empty/unresolved kind, already gets
// the full transcript and needs nothing here). upstreamSessionID is the
// session's stored UpstreamSessionID — empty means there is no upstream
// session yet to backfill context for, regardless of provider kind (a
// first turn, or a provider that has never resumed).
func maybeAppendInitiativeContext(message, transcriptPath, providerKind, upstreamSessionID string, logger zerolog.Logger) string {
	if upstreamSessionID == "" || !initiativeResumeProviderKinds[providerKind] {
		return message
	}
	note, err := mostRecentUnansweredInitiativeMessage(transcriptPath)
	if err != nil {
		logger.Warn().Err(err).Msg("initiative: read transcript for hidden context failed")
		return message
	}
	return appendInitiativeContext(message, note)
}
