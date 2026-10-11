package initiative

import (
	"context"
	"time"
)

// Delivery outcomes recorded on an Entry in live mode (tars#1220).
// Unreachable and skipped never call Speak at all — the runtime decides
// those itself, before any network call.
const (
	DeliveryDelivered   = "delivered"
	DeliveryUnreachable = "unreachable"
	DeliverySkipped     = "skipped"
	DeliveryError       = "error"
)

// Delivery reason codes. Never user or assistant text (tars#1003's "the
// ledger never holds words" rule applies here too) — just a fixed label.
const (
	deliveryReasonNoConsole        = "no_console"
	deliveryReasonBackoff          = "backoff"
	deliveryReasonDailySpeakCap    = "daily_speak_cap"
	deliveryReasonSpeakUnavailable = "speak_unavailable"
	deliveryReasonBusy             = "busy"
	deliveryReasonComposeError     = "compose_error"
	deliveryReasonDelivered        = "delivered"
	// deliveryReasonUserSpoke is Superseded's other reason (tars#1220
	// review): Compose finished, the claim was taken, but the main
	// session's message count had already grown while Compose was in
	// flight (claim-less) — the user spoke first, so the composed text is
	// discarded unwritten. deliveryReasonBusy covers Superseded's other
	// case (the claim itself could not be taken: a real turn started
	// instead), reusing the same code pre-compose's "busy" already used
	// for "no compose call was made" — the meaning (the main session's
	// claim was unavailable) is the same either side of Compose.
	deliveryReasonUserSpoke = "user_spoke"
	// deliveryReasonReadError is the pre-compose counterpart to
	// write_error (tars#1220 review f2): the main session's transcript
	// could not even be read to establish the message-count baseline the
	// Superseded check needs, so Compose is never called at all —
	// Composed stays false, unlike write_error, which always implies a
	// compose call happened (the after-claim recheck or the final
	// AppendMessage failing, both only reachable once Compose has
	// returned).
	deliveryReasonReadError = "read_error"
)

// Speaker composes and delivers one greet/check_in utterance: a tool-free
// LLM call (System 2) producing 1-3 lines, then handing them to wherever
// they reach the user — a real assistant message in the main session plus
// a companion event for CASE (tars#1220). Implemented in tarsserver, which
// owns the session store, the chat claim registry and the notification
// dispatcher; this package only decides whether and when to call it and
// records the outcome, never the words themselves (tars#1003).
type Speaker interface {
	Speak(ctx context.Context, req SpeakRequest) (SpeakOutcome, error)
	// NotifyBodyOnly sends a message-less companion event (an expression
	// only, no bubble line) alongside a body_only tick's existing
	// BodyActor expression, so CASE's face can react even when there is no
	// line to show. Best-effort: the runtime logs a failure but does not
	// retry or record it on the Entry.
	NotifyBodyOnly(ctx context.Context) error
}

// SpeakRequest is what the speak composer needs for one attempt.
type SpeakRequest struct {
	Intent  Intent
	EntryID string
	Now     time.Time
	// SendsText mirrors the text-signal backend's same-provider rule
	// (tars#1219 §3) but for the speak role specifically: recent user
	// messages and the profile are meaningful context for the composer
	// only when the speak role resolves to the same provider pool alias as
	// chat (that text already went to that exact credential/endpoint
	// during the turn that produced it). The runtime only forwards
	// RecentUser/Profile unconditionally; whether to use them is the
	// implementation's call based on this flag.
	SendsText  bool
	RecentUser []UserMessage
	Profile    string
}

// SpeakOutcome is what Speak managed to do.
type SpeakOutcome struct {
	// Delivered is true only once a real assistant message has been
	// written to the main session (the companion event itself is
	// best-effort on top of that and never un-delivers).
	Delivered bool
	// Composed reports whether the speak-composer LLM was actually
	// invoked this attempt (success or failure) — what counts toward the
	// daily speak-call cap and the speak backoff schedule, independent of
	// Delivered.
	Composed bool
	// Superseded is true when Compose succeeded but the composed text was
	// deliberately discarded unwritten, never an error (tars#1220 review):
	// either the main session's claim could not be taken (a real turn
	// started while composing, Reason "busy") or it could, but the
	// session's message count had already grown since before Compose
	// started (the user spoke first, Reason "user_spoke"). The runtime
	// (deliverLocked/applyLocked) must count this attempt toward
	// DailySpeakCalls like any other Composed call, but never toward the
	// speak backoff schedule (SpeakFailCount) or cooldown/daily-cap pacing
	// (spoken()) — Compose ran, but nothing was actually said.
	Superseded bool
	// Reason explains the outcome when it is not delivered: busy (the
	// main session's claim was unavailable, either before Compose — no
	// compose call was made — or after, with Superseded true), "user_spoke"
	// (Superseded true, see above), speak_unavailable (no composer
	// configured, or the provider cannot do a tool-free call — no compose
	// call), compose_error, compose_empty, compose_tool_attempt (a compose
	// call was made and failed), read_error (the main session's transcript
	// could not be read to establish the Superseded check's baseline — no
	// compose call was made), or write_error (a compose call was made, but
	// either the after-claim recheck read or the final append failed).
	Reason string
}

// backoffDuration returns how long to wait before the next attempt after
// failCount consecutive failures (0 means never attempted/last succeeded,
// so there is no wait). It starts at defaultBackoffStart and doubles each
// additional failure, capped at defaultBackoffMax.
func backoffDuration(failCount int) time.Duration {
	if failCount <= 0 {
		return 0
	}
	d := defaultBackoffStart
	for i := 1; i < failCount; i++ {
		d *= 2
		if d >= defaultBackoffMax {
			return defaultBackoffMax
		}
	}
	return d
}
