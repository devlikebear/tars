package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/embodiment"
	"github.com/devlikebear/tars/internal/initiative"
	"github.com/devlikebear/tars/internal/jev"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/rs/zerolog"
)

const (
	observerUserLookback = 12 * time.Hour
	observerMaxUser      = 50
	// observerSessionHorizon bounds which sessions are read at all. The user
	// can be away longer; LastUserAt then falls back to session activity.
	observerSessionHorizon = 7 * 24 * time.Hour
)

// sessionObserverDeps are the live sources the initiative observer reads.
// Every function is optional; a missing one reads as false or zero.
type sessionObserverDeps struct {
	Store        *session.Store
	WorkspaceDir string
	// MainSessionID identifies the one session reply tracking (tars#1221)
	// reads for Observation.MainUserAt — a delivered greet/check_in is
	// only ever written there (helpers_initiative_speak.go), so only a
	// reply in that session counts.
	MainSessionID string
	// Presence returns the console presence tracker's last recorded state
	// (tars#1249: POST /v1/console/presence) — replacing the old
	// subscriber-count signal, which read "something is still subscribed
	// to /v1/events/stream" (a desktop tray subscription, or a tab left
	// open for hours with nobody there) rather than "a person is actually
	// at the console".
	Presence       func() initiative.PresenceState
	ChatBusy       func() bool
	TelegramPaired func() bool
	// TelegramReady reports whether a live-mode speak can actually be
	// delivered over Telegram right now (tars#1221 P2c): a sender is
	// configured AND exactly one default paired chat resolves. Stricter
	// than TelegramPaired, which also counts several paired chats with
	// none chosen as reachable (that is enough for GoSignals.Reachable,
	// but not enough to pick a send target).
	TelegramReady func() bool
	// TelegramReplySession resolves the session a Telegram reply to the
	// default paired chat lands in, when session_telegram_scope is
	// "per-user" (tars#1221 P2c requirement 5) — "" when the scope is
	// "main" (replies already land in the main session) or no single
	// default chat resolves.
	TelegramReplySession func() string
	BodyAvailable        func() bool
}

// sessionInitiativeObserver builds an initiative.Observation from every
// visible session's transcript, USER.md and live server state. The user may
// be working in any session, so activity is not read from the main session
// alone. Files are re-read only when they change.
type sessionInitiativeObserver struct {
	deps        sessionObserverDeps
	mu          sync.Mutex
	transcripts map[string]*fileCache[sessionActivitySnapshot]
	profile     fileCache[string]
}

func newSessionInitiativeObserver(deps sessionObserverDeps) *sessionInitiativeObserver {
	return &sessionInitiativeObserver{deps: deps, transcripts: map[string]*fileCache[sessionActivitySnapshot]{}}
}

func (o *sessionInitiativeObserver) Observe(_ context.Context, now time.Time) (initiative.Observation, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	obs := initiative.Observation{Now: now}
	if o.deps.Presence != nil {
		state := o.deps.Presence()
		obs.ConsolePresentAt = state.LastAt
		obs.ConsoleArrivedAt = state.ArrivedAt
	}
	if o.deps.ChatBusy != nil {
		obs.ChatBusy = o.deps.ChatBusy()
	}
	if o.deps.TelegramPaired != nil {
		obs.TelegramPaired = o.deps.TelegramPaired()
	}
	if o.deps.TelegramReady != nil {
		obs.TelegramReady = o.deps.TelegramReady()
	}
	if o.deps.BodyAvailable != nil {
		obs.BodyAvailable = o.deps.BodyAvailable()
	}
	if o.deps.Store != nil {
		if err := o.readSessions(now, &obs); err != nil {
			return obs, err
		}
	}
	if o.deps.WorkspaceDir != "" {
		if profile, err := o.profile.load(filepath.Join(o.deps.WorkspaceDir, "USER.md"), readTextFile); err == nil {
			obs.Profile = profile
		}
	}
	return obs, nil
}

func (o *sessionInitiativeObserver) readSessions(now time.Time, obs *initiative.Observation) error {
	sessions, err := o.deps.Store.List()
	if err != nil {
		return err
	}
	var all []initiative.UserMessage
	var newestActivity time.Time
	seen := map[string]bool{}
	// telegramReplySessionID is the per-user session a Telegram reply to
	// the default paired chat lands in (tars#1221 P2c requirement 5) —
	// "" when session_telegram_scope is "main" or no single default chat
	// resolves. Resolved once per tick, not per session.
	telegramReplySessionID := ""
	if o.deps.TelegramReplySession != nil {
		telegramReplySessionID = o.deps.TelegramReplySession()
	}
	for _, sess := range sessions {
		if sess.Hidden || strings.TrimSpace(sess.Kind) == "worker" {
			continue
		}
		if now.Sub(sess.UpdatedAt) > observerSessionHorizon {
			continue
		}
		path := o.deps.Store.TranscriptPath(sess.ID)
		seen[path] = true
		cache := o.transcripts[path]
		if cache == nil {
			cache = &fileCache[sessionActivitySnapshot]{}
			o.transcripts[path] = cache
		}
		activity, err := cache.load(path, readSessionActivity)
		if err != nil {
			return err
		}
		all = append(all, activity.UserMessages...)
		if activity.LastActivityAt.After(newestActivity) {
			newestActivity = activity.LastActivityAt
		}
		if n := len(activity.UserMessages); n > 0 {
			ts := activity.UserMessages[n-1].At
			// Both branches take the max rather than assign outright:
			// which session is visited first in this unordered List()
			// loop is not guaranteed, and under session_telegram_scope:
			// per-user the two session ids are different sessions whose
			// newest message could come from either side.
			if o.deps.MainSessionID != "" && sess.ID == o.deps.MainSessionID && ts.After(obs.MainUserAt) {
				obs.MainUserAt = ts
			}
			if telegramReplySessionID != "" && sess.ID == telegramReplySessionID &&
				telegramReplySessionID != o.deps.MainSessionID && ts.After(obs.MainUserAt) {
				obs.MainUserAt = ts
			}
		}
	}
	for path := range o.transcripts {
		if !seen[path] {
			delete(o.transcripts, path)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].At.Before(all[j].At) })
	if n := len(all); n > 0 {
		obs.LastUserAt = all[n-1].At
	} else {
		// Nobody spoke within the horizon: the newest session activity is an
		// upper bound on the last user message, which errs toward "recent".
		obs.LastUserAt = newestActivity
	}
	for _, m := range all {
		if now.Sub(m.At) <= observerUserLookback {
			obs.RecentUser = append(obs.RecentUser, m)
		}
	}
	if len(obs.RecentUser) > observerMaxUser {
		obs.RecentUser = obs.RecentUser[len(obs.RecentUser)-observerMaxUser:]
	}
	return nil
}

// sessionActivitySnapshot is what one transcript file read contributes to
// the observer: the user's own messages, and the newest timestamp counted
// as "activity" for the no-user-message-in-horizon fallback (tars#1220).
type sessionActivitySnapshot struct {
	UserMessages []initiative.UserMessage
	// LastActivityAt excludes initiative-authored assistant messages (see
	// readSessionActivity) — TARS speaking first must never look like the
	// user being active to the very runtime deciding whether to speak
	// again, or its own greeting would read as "busy typing" or "just
	// arrived" on the next tick.
	LastActivityAt time.Time
}

// readSessionActivity reads one transcript once for both the user's own
// messages and the newest non-initiative activity timestamp used as
// LastUserAt's fallback upper bound. An initiative-authored assistant
// message (Message.Initiative != nil, tars#1220) counts toward neither: it
// is not something the user said, and it must not look like fresh activity
// either — its own greeting must never read as "busy typing" or "just
// arrived" to the next tick.
//
// A role "user" message the server started on its own (Message.Origin,
// tars#1249 — the focus driver's kickoff/gate/fix prompts, "focus_qa",
// "pulse"; a synthetic pulse auto-resume prompt predating Origin is still
// read the same way through its "[PULSE" content prefix, kept for
// transcripts written before this field) is not something a person typed
// either, so it is excluded from UserMessages the same as an
// initiative-authored reply. serverTurn also carries forward across that
// turn's own assistant/tool reply messages up to (not including) the next
// role "user" message, so the turn's own reply does not advance
// LastActivityAt — otherwise a focus pipeline running unattended for days
// would still look like someone is at the desk through this fallback
// alone. It deliberately does not carry forward onto role "system"
// messages: those are standalone notices unrelated to any turn — cron's
// own `[CRON]`/`[REMINDER]` note (appendCronSessionMessage) chief among
// them — appended independently of whichever user message happens to
// precede them in the file, so a cron note landing in a session right
// after a focus-origin turn (no human message in between) would otherwise
// be excluded from this fallback for a reason that has nothing to do with
// the note itself. A transcript with no Origin on any message (written
// before this field existed) reads every role "user" message as a
// person's, same as before.
func readSessionActivity(path string) (sessionActivitySnapshot, error) {
	msgs, err := session.ReadMessages(path)
	if err != nil {
		return sessionActivitySnapshot{}, err
	}
	var out sessionActivitySnapshot
	serverTurn := false
	for _, m := range msgs {
		if m.Role == "user" {
			serverTurn = strings.TrimSpace(m.Origin) != "" || strings.HasPrefix(strings.TrimSpace(m.Content), "[PULSE")
			if !serverTurn {
				out.UserMessages = append(out.UserMessages, initiative.UserMessage{At: m.Timestamp, Text: m.Content})
			}
		}
		if m.Initiative != nil || (serverTurn && m.Role != "system") {
			continue
		}
		if m.Timestamp.After(out.LastActivityAt) {
			out.LastActivityAt = m.Timestamp
		}
	}
	return out, nil
}

func readTextFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	return string(raw), err
}

// fileCache re-reads a file only when its size or mtime changes. A missing
// file yields the zero value without error.
type fileCache[T any] struct {
	path  string
	size  int64
	mtime time.Time
	value T
}

func (c *fileCache[T]) load(path string, read func(string) (T, error)) (T, error) {
	var zero T
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		c.path, c.value = "", zero
		return zero, nil
	}
	if err != nil {
		return zero, err
	}
	if path == c.path && info.Size() == c.size && info.ModTime().Equal(c.mtime) {
		return c.value, nil
	}
	value, err := read(path)
	if err != nil {
		return zero, err
	}
	c.path, c.size, c.mtime, c.value = path, info.Size(), info.ModTime(), value
	return value, nil
}

type embodimentBodyActor struct{ subsystem *embodiment.Subsystem }

func (a embodimentBodyActor) Express(ctx context.Context, provider, emotion string) (string, error) {
	res := a.subsystem.Act(ctx, provider, embodiment.BodyAction{
		Kind:    embodiment.ActionExpress,
		Payload: map[string]any{"emotion": emotion},
	})
	switch {
	case res.Delivered:
		return "delivered", nil
	case res.Error != "":
		return "error", errors.New(res.Error)
	default:
		return "dropped:" + res.Reason, nil
	}
}

type initiativeSetupInputs struct {
	Config       config.Config
	WorkspaceDir string
	SessionStore *session.Store
	// Presence is the console presence tracker (tars#1249: POST
	// /v1/console/presence) the observer reads for arrival/"at the
	// console right now" instead of the SSE broker's subscriber count.
	// nil (never expected outside a test) means the observer never sees
	// console presence at all.
	Presence         *initiative.PresenceStore
	Activity         *runtimeActivity
	TelegramPairings *telegramPairingStore
	Embodiment       *embodiment.Subsystem
	// Router resolves the "llm" text-signal backend (RoleInitiative), the
	// live-mode speak composer (RoleInitiativeSpeak), and the chat provider
	// both are compared against (RoleChatMain). nil in setup-only mode, in
	// which case neither backend is usable.
	Router llm.Router
	// MainSessionID is where a delivered speak writes its assistant
	// message (tars#1220). Resolved before buildInitiativeRuntime runs
	// (main_serve_api.go), so it is always available here.
	MainSessionID string
	// TelegramSender performs a live-mode speak's Telegram delivery
	// (tars#1221 P2c) — the same sender channels.telegram's other paths
	// use (counting wrapper included). nil (Telegram not configured)
	// means the telegram channel is never picked regardless of
	// Config.Initiative.Telegram or pairing state.
	TelegramSender telegramSender
	// Notify publishes the companion event CASE's bubble reacts to.
	// Available before buildInitiativeRuntime runs too; only the chat
	// cancel registry (claim) is bound later, once the chat handler
	// exists — see initiativeSetup.Speaker.
	Notify func(context.Context, notificationEvent)
	Logger zerolog.Logger
	Now    func() time.Time
}

type initiativeSetup struct {
	Runtime *initiative.Runtime
	Handler http.Handler
	// Speaker is non-nil whenever live mode has a usable speak backend
	// (Config.Enabled and a resolvable RoleInitiativeSpeak client); its
	// claim field still needs the chat cancel registry, bound by the
	// caller once newChatAPIHandlerWithRuntimeConfig creates one (the same
	// late-binding chatWorktrees.running already uses, since that registry
	// does not exist yet when buildInitiativeRuntime runs).
	Speaker *initiativeSpeaker
}

// buildInitiativeRuntime wires the initiative loop (tars#997). When it is
// disabled only the status handler exists. User text reaches the
// text-signal backend only when initiative.PlanText says so: for jev, only
// a loopback base URL; for llm (the default), only when the initiative
// role resolves to the same provider pool alias chat uses (tars#1219).
func buildInitiativeRuntime(in initiativeSetupInputs) initiativeSetup {
	c := in.Config.Initiative
	if !c.Enabled {
		return initiativeSetup{Handler: newInitiativeAPIHandler(nil, in.Presence, in.Now)}
	}
	logger := in.Logger.With().Str("component", "initiative").Logger()
	loc := time.Local
	if tz := strings.TrimSpace(c.Timezone); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		} else {
			logger.Warn().Str("timezone", tz).Msg("initiative: unknown timezone, using server local")
		}
	}
	cfg := initiative.Config{
		Enabled:         true,
		Mode:            c.Mode,
		Tick:            parsePulseDuration(c.Tick, time.Minute),
		QuietHours:      c.QuietHours,
		Location:        loc,
		DailyCap:        c.DailyCap,
		Cooldown:        parsePulseDuration(c.Cooldown, 45*time.Minute),
		BodyProvider:    c.BodyProvider,
		DailyTextCalls:  c.DailyTextCalls,
		DailySpeakCalls: c.DailySpeakCalls,
		ReplyWindow:     parsePulseDuration(c.ReplyWindow, 30*time.Minute),
		Telegram:        c.Telegram,
		Thresholds: initiative.Thresholds{
			QuietRequested: c.QuietRequestedThreshold,
			UserStrained:   c.UserStrainedThreshold,
			SpecialDay:     c.SpecialDayThreshold,
		},
	}
	deps := initiative.Dependencies{
		Ledger: initiative.OpenLedger(filepath.Join(in.WorkspaceDir, "_shared", "initiative", "ledger.jsonl"), 0),
		Logger: logger,
		Now:    in.Now,
	}
	plan, systemOne, textLLM, textLLMModel := resolveInitiativeTextBackend(in.Config, in.Router)
	deps.SystemOne = systemOne
	deps.TextLLM = textLLM
	deps.TextLLMModel = textLLMModel
	deps.IncludeText = plan.SendsText
	deps.Backend = initiativeBackendInfoFromPlan(plan)

	// telegramReplyResolver resolves the per-user session a Telegram reply
	// to the default paired chat lands in (tars#1221 P2c requirement 5) —
	// nil when session_telegram_scope is not "per-user" or pairing is
	// unavailable, so both the speaker (dual-write) and the observer
	// (reply tracking) share one definition of "which session".
	telegramReplyResolver := telegramReplySessionResolver(in.Config, in.TelegramPairings)

	var speaker *initiativeSpeaker
	speakPlan, speakClient, speakModel := resolveInitiativeSpeakBackend(in.Config, in.Router)
	deps.SpeakSendsText = speakPlan.SendsText
	deps.SpeakBackend = initiativeBackendInfoFromPlan(speakPlan)
	if speakClient != nil {
		speaker = &initiativeSpeaker{
			workspaceDir:         in.WorkspaceDir,
			mainSessionID:        strings.TrimSpace(in.MainSessionID),
			store:                in.SessionStore,
			composer:             &initiative.SpeechComposer{Client: speakClient, Model: speakModel},
			sendsText:            speakPlan.SendsText,
			location:             loc,
			notify:               in.Notify,
			logger:               logger,
			telegram:             in.TelegramSender,
			telegramReplySession: telegramReplyResolver,
		}
		if in.TelegramPairings != nil {
			pairings := in.TelegramPairings
			speaker.telegramDefaultChatID = pairings.resolveDefaultChatID
		}
		deps.Speaker = speaker
	}

	observerDeps := sessionObserverDeps{Store: in.SessionStore, WorkspaceDir: in.WorkspaceDir, MainSessionID: strings.TrimSpace(in.MainSessionID)}
	if in.Presence != nil {
		observerDeps.Presence = in.Presence.Current
	}
	if in.Activity != nil {
		observerDeps.ChatBusy = in.Activity.isChatBusy
	}
	if in.TelegramPairings != nil {
		pairings := in.TelegramPairings
		observerDeps.TelegramPaired = func() bool {
			id, err := pairings.resolveDefaultChatID()
			return err != nil || id != "" // several paired chats still means reachable
		}
		// TelegramReady additionally requires channels.telegram.enabled
		// (review f1): a sender and a prior pairing can both still exist
		// on disk after the user turns the channel off, and neither
		// should resurrect it for initiative's proactive delivery.
		if in.TelegramSender != nil && in.Config.ChannelsTelegramEnabled {
			observerDeps.TelegramReady = func() bool {
				id, err := pairings.resolveDefaultChatID()
				return err == nil && id != ""
			}
		}
	}
	if telegramReplyResolver != nil {
		observerDeps.TelegramReplySession = func() string {
			id, err := telegramReplyResolver()
			if err != nil {
				return ""
			}
			return id
		}
	}
	if in.Embodiment != nil && cfg.BodyProvider != "" {
		body := in.Embodiment
		provider := cfg.BodyProvider
		deps.Body = embodimentBodyActor{subsystem: body}
		observerDeps.BodyAvailable = func() bool { return body.Status().Enabled && body.KnownProvider(provider) }
	}
	deps.Observer = newSessionInitiativeObserver(observerDeps)

	runtime := initiative.NewRuntime(cfg, deps)
	return initiativeSetup{Runtime: runtime, Handler: newInitiativeAPIHandler(runtime, in.Presence, in.Now), Speaker: speaker}
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

// resolveInitiativeTextBackend resolves the configured text-signal backend
// (initiative.backend: llm | jev, default llm) into a BackendPlan plus the
// client the runtime should use (at most one of the SystemOne/llm.Client
// return values is non-nil). It never performs network I/O itself — only
// config lookups and llm.Router.ClientFor, which returns an
// already-constructed, already usage-tracked client.
func resolveInitiativeTextBackend(cfg config.Config, router llm.Router) (initiative.BackendPlan, initiative.SystemOne, llm.Client, string) {
	backend := strings.ToLower(strings.TrimSpace(cfg.Initiative.Backend))
	if backend == "jev" {
		return resolveInitiativeJevBackend(cfg)
	}
	return resolveInitiativeLLMBackend(cfg, router)
}

func resolveInitiativeJevBackend(cfg config.Config) (initiative.BackendPlan, initiative.SystemOne, llm.Client, string) {
	base := strings.TrimSpace(cfg.Jev.BaseURL)
	if base == "" {
		return initiative.PlanText(initiative.PlanInput{Backend: "jev"}), nil, nil, ""
	}
	client := jev.New(jev.Options{
		BaseURL: base,
		APIKey:  cfg.Jev.APIKey,
		Model:   cfg.Jev.Model,
		Timeout: time.Duration(cfg.Jev.TimeoutSeconds) * time.Second,
	})
	plan := initiative.PlanText(initiative.PlanInput{
		Backend: "jev", JevConfigured: true, JevLoopback: client.IsLoopback(), JevHost: hostOf(base), JevModel: cfg.Jev.Model,
	})
	return plan, client, nil, ""
}

func resolveInitiativeLLMBackend(cfg config.Config, router llm.Router) (initiative.BackendPlan, initiative.SystemOne, llm.Client, string) {
	in := initiative.PlanInput{Backend: "llm", LLMChatProviderAlias: chatProviderAlias(cfg)}
	if router == nil {
		return initiative.PlanText(in), nil, nil, ""
	}
	tier := strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleInitiative)])
	if tier == "" {
		tier = "light"
	}
	resolved, err := config.ResolveLLMTier(&cfg, tier)
	if err != nil {
		return initiative.PlanText(in), nil, nil, ""
	}
	in.LLMResolved = true
	in.LLMKind = resolved.Kind
	in.LLMProviderAlias = resolved.ProviderAlias
	in.LLMModel = resolved.Model
	in.LLMTier = tier
	in.LLMSupportsDecisionOnly = llm.SupportsDecisionOnly(resolved.Kind)
	plan := initiative.PlanText(in)
	if !plan.Usable {
		return plan, nil, nil, ""
	}
	client, _, err := router.ClientFor(llm.RoleInitiative)
	if err != nil {
		plan.Usable, plan.SendsText, plan.Reason = false, false, "llm_unavailable"
		return plan, nil, nil, ""
	}
	return plan, nil, client, resolved.Model
}

// chatProviderAlias resolves the provider pool alias behind the chat
// role's tier (RoleChatMain if mapped, else the default tier). Matching
// provider *alias* — not just kind, and not kind+base_url — is
// deliberately the strictest check available: two provider pool entries of
// the same kind (even the same base URL) can hold different credentials,
// and reusing an alias is the one signal that always means "the exact same
// credential/endpoint the user's words already went to during the chat
// turn that produced them."
func chatProviderAlias(cfg config.Config) string {
	tier := strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleChatMain)])
	if tier == "" {
		tier = strings.TrimSpace(cfg.LLMDefaultTier)
	}
	if tier == "" {
		return ""
	}
	resolved, err := config.ResolveLLMTier(&cfg, tier)
	if err != nil {
		return ""
	}
	return resolved.ProviderAlias
}

// initiativeBackendInfoFromPlan carries a BackendPlan into the runtime's
// status shape. Loopback/Host remain jev-specific (network reachability);
// the llm backend reports through Kind/Provider/Model/Tier instead.
func initiativeBackendInfoFromPlan(plan initiative.BackendPlan) initiative.BackendInfo {
	info := initiative.BackendInfo{
		Backend:    plan.Backend,
		Configured: plan.Usable,
		Kind:       plan.Kind,
		Provider:   plan.Provider,
		Model:      plan.Model,
		Tier:       plan.Tier,
		SendsText:  plan.SendsText,
		TextReason: plan.Reason,
	}
	if plan.Backend == "jev" {
		info.Loopback = plan.SendsText
		info.Host = plan.Provider
	}
	return info
}

// telegramReplySessionResolver returns a resolver for the session a
// Telegram reply to the default paired chat lands in (tars#1221 P2c
// requirement 5), or nil when that question never applies: pairing is
// unavailable, or session_telegram_scope (telegram_inbound_session.go's
// normalizeTelegramSessionScope) is not "per-user" — under the default
// "main" scope a Telegram reply already lands in the main session, the
// same session reply tracking already follows. Under "per-user", the
// resolver finds the Telegram user bound to that one default chat
// (resolveDefaultUserID, the same single-chat rule resolveDefaultChatID
// uses) and returns their session id — "" with a nil error when no
// default chat is paired yet, the same multiple-paired-chats error
// resolveDefaultChatID returns otherwise.
func telegramReplySessionResolver(cfg config.Config, pairings *telegramPairingStore) func() (string, error) {
	if pairings == nil {
		return nil
	}
	if normalizeTelegramSessionScope(cfg.SessionTelegramScope) != "per-user" {
		return nil
	}
	return func() (string, error) {
		userID, err := pairings.resolveDefaultUserID()
		if err != nil || userID == 0 {
			return "", err
		}
		return strings.TrimSpace(pairings.sessionID(userID)), nil
	}
}
