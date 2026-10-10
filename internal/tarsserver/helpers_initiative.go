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
	Store           *session.Store
	WorkspaceDir    string
	SubscriberCount func() int
	ChatBusy        func() bool
	TelegramPaired  func() bool
	BodyAvailable   func() bool
}

// sessionInitiativeObserver builds an initiative.Observation from every
// visible session's transcript, USER.md and live server state. The user may
// be working in any session, so activity is not read from the main session
// alone. Files are re-read only when they change.
type sessionInitiativeObserver struct {
	deps        sessionObserverDeps
	mu          sync.Mutex
	connectedAt time.Time
	transcripts map[string]*fileCache[[]initiative.UserMessage]
	profile     fileCache[string]
}

func newSessionInitiativeObserver(deps sessionObserverDeps) *sessionInitiativeObserver {
	return &sessionInitiativeObserver{deps: deps, transcripts: map[string]*fileCache[[]initiative.UserMessage]{}}
}

func (o *sessionInitiativeObserver) Observe(_ context.Context, now time.Time) (initiative.Observation, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	obs := initiative.Observation{Now: now}
	if o.deps.SubscriberCount != nil && o.deps.SubscriberCount() > 0 {
		if o.connectedAt.IsZero() {
			o.connectedAt = now
		}
		obs.ConsoleConnectedAt = o.connectedAt
	} else {
		o.connectedAt = time.Time{}
	}
	if o.deps.ChatBusy != nil {
		obs.ChatBusy = o.deps.ChatBusy()
	}
	if o.deps.TelegramPaired != nil {
		obs.TelegramPaired = o.deps.TelegramPaired()
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
	for _, sess := range sessions {
		if sess.Hidden || strings.TrimSpace(sess.Kind) == "worker" {
			continue
		}
		if sess.UpdatedAt.After(newestActivity) {
			newestActivity = sess.UpdatedAt
		}
		if now.Sub(sess.UpdatedAt) > observerSessionHorizon {
			continue
		}
		path := o.deps.Store.TranscriptPath(sess.ID)
		seen[path] = true
		cache := o.transcripts[path]
		if cache == nil {
			cache = &fileCache[[]initiative.UserMessage]{}
			o.transcripts[path] = cache
		}
		msgs, err := cache.load(path, readUserMessages)
		if err != nil {
			return err
		}
		all = append(all, msgs...)
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

// readUserMessages keeps what the user actually typed: synthetic pulse
// resumes are written with the user role but are not the user speaking.
func readUserMessages(path string) ([]initiative.UserMessage, error) {
	msgs, err := session.ReadMessages(path)
	if err != nil {
		return nil, err
	}
	var out []initiative.UserMessage
	for _, m := range msgs {
		if m.Role != "user" || strings.HasPrefix(strings.TrimSpace(m.Content), "[PULSE") {
			continue
		}
		out = append(out, initiative.UserMessage{At: m.Timestamp, Text: m.Content})
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
	Config           config.Config
	WorkspaceDir     string
	SessionStore     *session.Store
	Broker           *eventBroker
	Activity         *runtimeActivity
	TelegramPairings *telegramPairingStore
	Embodiment       *embodiment.Subsystem
	// Router resolves the "llm" text-signal backend (RoleInitiative) and
	// the chat provider it is compared against (RoleChatMain). nil in
	// setup-only mode, in which case the llm backend is simply unavailable.
	Router llm.Router
	Logger zerolog.Logger
	Now    func() time.Time
}

type initiativeSetup struct {
	Runtime *initiative.Runtime
	Handler http.Handler
}

// buildInitiativeRuntime wires the initiative loop (tars#997). When it is
// disabled only the status handler exists. User text reaches the
// text-signal backend only when initiative.PlanText says so: for jev, only
// a loopback base URL; for llm (the default), only when the initiative
// role resolves to the same provider pool alias chat uses (tars#1219).
func buildInitiativeRuntime(in initiativeSetupInputs) initiativeSetup {
	c := in.Config.Initiative
	if !c.Enabled {
		return initiativeSetup{Handler: newInitiativeAPIHandler(nil)}
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
		Enabled:        true,
		Mode:           c.Mode,
		Tick:           parsePulseDuration(c.Tick, time.Minute),
		QuietHours:     c.QuietHours,
		Location:       loc,
		DailyCap:       c.DailyCap,
		Cooldown:       parsePulseDuration(c.Cooldown, 45*time.Minute),
		BodyProvider:   c.BodyProvider,
		DailyTextCalls: c.DailyTextCalls,
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

	observerDeps := sessionObserverDeps{Store: in.SessionStore, WorkspaceDir: in.WorkspaceDir}
	if in.Broker != nil {
		observerDeps.SubscriberCount = in.Broker.SubscriberCount
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
	}
	if in.Embodiment != nil && cfg.BodyProvider != "" {
		body := in.Embodiment
		provider := cfg.BodyProvider
		deps.Body = embodimentBodyActor{subsystem: body}
		observerDeps.BodyAvailable = func() bool { return body.Status().Enabled && body.KnownProvider(provider) }
	}
	deps.Observer = newSessionInitiativeObserver(observerDeps)

	runtime := initiative.NewRuntime(cfg, deps)
	return initiativeSetup{Runtime: runtime, Handler: newInitiativeAPIHandler(runtime)}
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
