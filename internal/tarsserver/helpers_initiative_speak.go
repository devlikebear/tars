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
	// exactly the interleaving the claim exists to prevent.
	claim func(sessionID string) (*chatCancelEntry, func(), bool)
}

var _ initiative.Speaker = (*initiativeSpeaker)(nil)

// Speak implements initiative.Speaker.
func (s *initiativeSpeaker) Speak(ctx context.Context, req initiative.SpeakRequest) (initiative.SpeakOutcome, error) {
	if s == nil || s.claim == nil {
		return initiative.SpeakOutcome{Reason: "busy"}, nil
	}
	_, release, ok := s.claim(s.mainSessionID)
	if !ok {
		return initiative.SpeakOutcome{Reason: "busy"}, nil
	}
	defer release()

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

	if err := s.writeMessage(req, text); err != nil {
		s.logger.Warn().Err(err).Str("session_id", s.mainSessionID).Msg("initiative: write speak message failed")
		return initiative.SpeakOutcome{Composed: true, Reason: "write_error"}, err
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
func (s *initiativeSpeaker) writeMessage(req initiative.SpeakRequest, text string) error {
	msg := session.Message{
		Role:      "assistant",
		Content:   text,
		Timestamp: req.Now,
		Initiative: &session.MessageInitiative{
			Intent:  string(req.Intent),
			EntryID: req.EntryID,
		},
	}
	path := s.store.TranscriptPath(s.mainSessionID)
	if err := session.AppendMessage(path, msg); err != nil {
		return err
	}
	if err := s.store.Touch(s.mainSessionID, req.Now); err != nil {
		s.logger.Warn().Err(err).Str("session_id", s.mainSessionID).Msg("initiative: touch session after speak failed")
	}
	return nil
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
