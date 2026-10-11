package tarsserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/initiative"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/rs/zerolog"
)

// companionExpressionMaxLen is the console's own cap on a companion event's
// bubble line (lib/companion.ts COMPANION_SERVER_LINE_MAX_LEN); this
// package clips to it too so the console never has to.
const companionExpressionMaxLen = 140

// companionEventCategory mirrors internal/notification's own unexported
// constant of the same name (#1192/#1204): the category that marks an
// event as CASE's live face/bubble rather than a general notification.
// Only the string value matters — notification.Dispatcher.Emit compares
// categories by string, not by a shared symbol — so duplicating the
// literal here needs no new export from that package.
const companionEventCategory = "companion"

// initiativeSpeakExpression maps a Speak intent to one of CASE's faces
// (lib/companion.ts COMPANION_EXPRESSIONS). Greet is the dedicated
// "greeting" face; check_in reads as a caring check-in, the same face an
// idle body_only expression already uses (runtime.go's idleEmotion).
func initiativeSpeakExpression(intent initiative.Intent) string {
	if intent == initiative.IntentGreet {
		return "greeting"
	}
	return "happy"
}

// resolveInitiativeSpeakTier picks the tier the live-mode speak composer
// (RoleInitiativeSpeak) uses: an explicit role mapping first, else chat's
// own tier (RoleChatMain if mapped, else the default tier) — deliberately
// not light, unlike every other automation role, so an unprompted word
// sounds like the same voice the user already talks to and has a real
// chance of sharing chat's provider alias (tars#1220).
func resolveInitiativeSpeakTier(cfg config.Config) string {
	if tier := strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleInitiativeSpeak)]); tier != "" {
		return tier
	}
	if tier := strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleChatMain)]); tier != "" {
		return tier
	}
	return strings.TrimSpace(cfg.LLMDefaultTier)
}

// resolveInitiativeSpeakBackend resolves the speak role into a BackendPlan
// plus the client to use, reusing the same PlanText rules tars#1219 already
// established for the text-signal role's llm backend (same-provider-alias
// gates whether recent user text/profile go into the compose prompt). nil
// client means live mode can never speak (deliveryReasonSpeakUnavailable).
func resolveInitiativeSpeakBackend(cfg config.Config, router llm.Router) (initiative.BackendPlan, llm.Client, string) {
	in := initiative.PlanInput{Backend: "llm", LLMChatProviderAlias: chatProviderAlias(cfg)}
	if router == nil {
		return initiative.PlanText(in), nil, ""
	}
	tier := resolveInitiativeSpeakTier(cfg)
	if tier == "" {
		return initiative.PlanText(in), nil, ""
	}
	resolved, err := config.ResolveLLMTier(&cfg, tier)
	if err != nil {
		return initiative.PlanText(in), nil, ""
	}
	in.LLMResolved = true
	in.LLMKind = resolved.Kind
	in.LLMProviderAlias = resolved.ProviderAlias
	in.LLMModel = resolved.Model
	in.LLMTier = tier
	in.LLMSupportsDecisionOnly = llm.SupportsDecisionOnly(resolved.Kind)
	plan := initiative.PlanText(in)
	if !plan.Usable {
		return plan, nil, ""
	}
	client, _, err := router.ClientForTier(llm.Tier(tier))
	if err != nil {
		plan.Usable, plan.SendsText, plan.Reason = false, false, "llm_unavailable"
		return plan, nil, ""
	}
	return plan, client, resolved.Model
}

// initiativeSpeaker implements initiative.Speaker: it composes one
// greet/check_in utterance (via its Composer), writes it as a real
// assistant message to the main session, and sends CASE a companion event
// (tars#1220). claim is late-bound after the chat cancel registry exists —
// the same pattern chatWorktrees.running uses — since buildInitiativeRuntime
// runs before the chat handler is built.
type initiativeSpeaker struct {
	workspaceDir  string
	mainSessionID string
	store         *session.Store
	composer      *initiative.SpeechComposer
	sendsText     bool
	location      *time.Location
	notify        func(context.Context, notificationEvent)
	logger        zerolog.Logger

	// claim takes the named session for the duration of one write, so a
	// real chat turn starting concurrently cannot race the message being
	// appended. nil (before the chat handler binds it, or when there is no
	// chat handler at all — setup-only mode) always reports busy: writing
	// to the main session without a working claim mechanism would risk
	// exactly the interleaving the claim exists to prevent. Only held
	// around the write itself (see Speak) — never around Compose, which is
	// the slow step (a real LLM call, potentially tens of seconds on
	// claude-code-cli) and must not block a user's own message to the main
	// session for its duration (tars#1220 review).
	claim func(sessionID string) (*chatCancelEntry, func(), bool)

	// telegram sends the composed text to Telegram (tars#1221 P2c). nil
	// means Telegram is not configured at all (channels.telegram has no
	// bot token) — Speak must then never be asked for ChannelTelegram in
	// the first place (the runtime only picks that channel when
	// Observation.TelegramReady, which already requires a sender).
	telegram telegramSender
	// telegramDefaultChatID re-resolves the default paired chat at send
	// time — never cached from the tick that decided to speak, since
	// pairing can change between decide and send. nil alongside a nil
	// telegram.
	telegramDefaultChatID func() (string, error)
	// telegramReplySession resolves the session a Telegram reply to that
	// default chat lands in, when session_telegram_scope is "per-user"
	// (tars#1221 P2c requirement 5) — nil under the default "main" scope,
	// where that session is mainSessionID already. Used to also write the
	// composed message there, tagged the same way as the main session's
	// copy, so the model answering that reply knows what it said.
	telegramReplySession func() (string, error)
}

var _ initiative.Speaker = (*initiativeSpeaker)(nil)

// Speak implements initiative.Speaker. The main session's claim is taken
// only around the write, not around Compose: claim → Compose → write would
// hold the claim for the whole LLM call, and on a slow backend that could
// block a user's own message for tens of seconds behind an invisible
// background job (tars#1220 review). Instead: read the transcript's
// message count, Compose with no claim held, then claim and re-read the
// count. A claim that cannot be taken (a real turn started while composing)
// or a count that grew (the user's own message landed first) both discard
// the composed text without writing it — SpeakOutcome.Superseded, not an
// error: Compose still counts toward the daily speak-call cap and backoff
// schedule's call count (a real LLM call happened), but the runtime must
// not treat either as a compose failure (no backoff increase) or as
// cooldown/daily-cap pacing (nothing was actually said).
func (s *initiativeSpeaker) Speak(ctx context.Context, req initiative.SpeakRequest) (initiative.SpeakOutcome, error) {
	if s == nil || s.claim == nil {
		return initiative.SpeakOutcome{Reason: "busy"}, nil
	}
	path := s.store.TranscriptPath(s.mainSessionID)
	before, err := session.ReadMessages(path)
	if err != nil {
		// No compose call has happened yet — read_error, not write_error:
		// the latter is documented (initiative.SpeakOutcome.Reason) to
		// always mean a compose call was made (tars#1220 review f2).
		s.logger.Warn().Err(err).Str("session_id", s.mainSessionID).Msg("initiative: read transcript before speak failed")
		return initiative.SpeakOutcome{Reason: "read_error"}, err
	}

	text, err := s.composer.Compose(ctx, initiative.ComposeInput{
		Intent:     req.Intent,
		Now:        req.Now,
		Location:   s.location,
		Identity:   s.readIdentity(),
		SendsText:  req.SendsText,
		Profile:    req.Profile,
		RecentUser: req.RecentUser,
	})
	if err != nil {
		return initiative.SpeakOutcome{Composed: true, Reason: composeFailureReason(err)}, err
	}

	_, release, ok := s.claim(s.mainSessionID)
	if !ok {
		return initiative.SpeakOutcome{Composed: true, Superseded: true, Reason: "busy"}, nil
	}
	defer release()

	after, err := session.ReadMessages(path)
	if err != nil {
		s.logger.Warn().Err(err).Str("session_id", s.mainSessionID).Msg("initiative: read transcript after speak compose failed")
		return initiative.SpeakOutcome{Composed: true, Reason: "write_error"}, err
	}
	if len(after) > len(before) {
		// The user (or something else) already wrote to the main session
		// while Compose was in flight — never append a stale greeting
		// behind their own, already-sent message.
		return initiative.SpeakOutcome{Composed: true, Superseded: true, Reason: "user_spoke"}, nil
	}

	if err := s.writeMessage(s.mainSessionID, req, text); err != nil {
		s.logger.Warn().Err(err).Str("session_id", s.mainSessionID).Msg("initiative: write speak message failed")
		return initiative.SpeakOutcome{Composed: true, Reason: "write_error"}, err
	}

	// The main session's write has succeeded — this attempt is delivered
	// from here on regardless of what the channel step below does
	// (tars#1221 P2c requirement 3): a channel send failure only ever
	// sets ChannelError, never Reason, never Delivered=false.
	if req.Channel == initiative.ChannelTelegram {
		s.writeTelegramReplySessionCopy(req, text)
		channelErr := s.sendTelegram(ctx, text)
		return initiative.SpeakOutcome{Composed: true, Delivered: true, ChannelError: channelErr}, nil
	}

	s.sendCompanionEvent(ctx, req.Intent, text)
	return initiative.SpeakOutcome{Composed: true, Delivered: true}, nil
}

// NotifyBodyOnly implements initiative.Speaker.
func (s *initiativeSpeaker) NotifyBodyOnly(ctx context.Context) error {
	if s == nil || s.notify == nil {
		return nil
	}
	s.notify(ctx, notificationEvent{
		Category:   companionEventCategory,
		Severity:   "info",
		Expression: "happy",
	})
	return nil
}

// composeFailureReason classifies a Compose error into one of the fixed
// delivery reason codes (tars#1003: never the error text itself, which on
// a same-provider call can echo the request).
func composeFailureReason(err error) string {
	switch {
	case errors.Is(err, initiative.ErrSpeechEmpty):
		return "compose_empty"
	case errors.Is(err, initiative.ErrSpeechToolAttempt):
		return "compose_tool_attempt"
	default:
		return "compose_error"
	}
}

// writeMessage appends the composed text as a real role:"assistant"
// message to the main session's transcript (never a synthetic user
// message), tagged with the Initiative metadata the ledger entry
// correlates to, then touches the session so it sorts/retains like any
// other fresh activity.
//
// Only a failed AppendMessage is reported as an error — the message was
// never persisted, so write_error is the true outcome. A failed Touch
// afterward (session index I/O, the session deleted concurrently, …) is
// logged and swallowed instead of returned: the message is already real
// and sitting in the transcript at that point, so reporting it as a
// failed write would make the runtime record delivery: error for a speak
// that actually succeeded, skip the companion event for it, and — since a
// non-delivered entry never starts the cooldown — go on to compose a
// second message on the very next eligible tick. handler_chat_execution.go's
// persistChatResult treats the same AppendMessage/Touch pair the same way,
// for the same reason.
func (s *initiativeSpeaker) writeMessage(sessionID string, req initiative.SpeakRequest, text string) error {
	msg := session.Message{
		Role:      "assistant",
		Content:   text,
		Timestamp: req.Now,
		Initiative: &session.MessageInitiative{
			Intent:  string(req.Intent),
			EntryID: req.EntryID,
		},
	}
	path := s.store.TranscriptPath(sessionID)
	if err := session.AppendMessage(path, msg); err != nil {
		return err
	}
	if err := s.store.Touch(sessionID, req.Now); err != nil {
		s.logger.Warn().Err(err).Str("session_id", sessionID).Msg("initiative: touch session after speak failed")
	}
	return nil
}

// writeTelegramReplySessionCopy also writes the composed message into the
// Telegram per-user reply session (tars#1221 P2c requirement 5), when one
// resolves and differs from the main session: under
// session_telegram_scope: per-user, a reply to this speak lands in that
// session, not the main one, and telegram_inbound_process.go always
// resends the whole transcript (never a resumed upstream session), so
// that session's own history needs the same assistant message too —
// otherwise the model answering the reply has no idea what it is
// replying to. Best effort: a failure here is logged and never changes
// the outcome (the message already exists at least once, in the main
// session) or Delivered/ChannelError.
func (s *initiativeSpeaker) writeTelegramReplySessionCopy(req initiative.SpeakRequest, text string) {
	if s.telegramReplySession == nil {
		return
	}
	sessionID, err := s.telegramReplySession()
	if err != nil || sessionID == "" || sessionID == s.mainSessionID {
		return
	}
	if err := s.writeMessage(sessionID, req, text); err != nil {
		s.logger.Warn().Err(err).Str("session_id", sessionID).Msg("initiative: write telegram reply-session copy failed")
	}
}

// sendTelegram sends the composed text to Telegram's default paired chat
// as plain text (tars#1221 P2c) — no parse_mode, so the console's/CLI's
// markdown-ish formatting never risks a parse error silently dropping the
// message. Called only after the main session's write has already
// succeeded, so a failure here never un-delivers: it returns a fixed
// channel-error reason code and nothing else (tars#1003's rule — never
// the underlying error text, the chat id, or the bot token, even though
// the sender already strips the token from its own errors).
func (s *initiativeSpeaker) sendTelegram(ctx context.Context, text string) string {
	if s.telegram == nil || s.telegramDefaultChatID == nil {
		return "send_failed"
	}
	chatID, err := s.telegramDefaultChatID()
	if err != nil || strings.TrimSpace(chatID) == "" {
		s.logger.Warn().Str("session_id", s.mainSessionID).Msg("initiative: telegram default chat unavailable at send time")
		return "send_failed"
	}
	if _, err := s.telegram.Send(ctx, telegramSendRequest{ChatID: chatID, Text: text}); err != nil {
		s.logger.Warn().Str("session_id", s.mainSessionID).Msg("initiative: telegram delivery failed")
		return "send_failed"
	}
	return ""
}

// sendCompanionEvent sends CASE's category "companion" event (#1192's wire
// format) for a delivered speak: the expression for the intent, the
// composed text's first line (clipped to the console's own bubble-line
// cap), and the main session id so clicking the line navigates there.
// Best-effort — notify.go's Emit has no error return, and a failure here
// never un-delivers a message already written to the transcript.
func (s *initiativeSpeaker) sendCompanionEvent(ctx context.Context, intent initiative.Intent, text string) {
	if s.notify == nil {
		return
	}
	s.notify(ctx, notificationEvent{
		Category:   companionEventCategory,
		Severity:   "info",
		Message:    clipBytes(firstLine(text), companionExpressionMaxLen),
		Expression: initiativeSpeakExpression(intent),
		SessionID:  s.mainSessionID,
	})
}

// firstLine returns the text up to its first newline, or the whole text
// when it has none.
func firstLine(text string) string {
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return text[:i]
	}
	return text
}

// readIdentity reads IDENTITY.md fresh on every call: live-mode speak
// attempts are capped (initiative.daily_speak_calls, default 12/day), so
// this is not a hot path worth caching.
func (s *initiativeSpeaker) readIdentity() string {
	if strings.TrimSpace(s.workspaceDir) == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(s.workspaceDir, "IDENTITY.md"))
	if err != nil {
		return ""
	}
	return string(raw)
}
