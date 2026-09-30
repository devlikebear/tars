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
	Logger           zerolog.Logger
	Now              func() time.Time
}

type initiativeSetup struct {
	Runtime *initiative.Runtime
	Handler http.Handler
}

// buildInitiativeRuntime wires the initiative loop (tars#997). When it is
// disabled only the status handler exists. User text reaches the System One
// only when its base URL is loopback.
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
		Enabled:      true,
		Mode:         c.Mode,
		Tick:         parsePulseDuration(c.Tick, time.Minute),
		QuietHours:   c.QuietHours,
		Location:     loc,
		DailyCap:     c.DailyCap,
		Cooldown:     parsePulseDuration(c.Cooldown, 45*time.Minute),
		BodyProvider: c.BodyProvider,
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
	if base := strings.TrimSpace(in.Config.Jev.BaseURL); base != "" {
		client := jev.New(jev.Options{
			BaseURL: base,
			APIKey:  in.Config.Jev.APIKey,
			Model:   in.Config.Jev.Model,
			Timeout: time.Duration(in.Config.Jev.TimeoutSeconds) * time.Second,
		})
		deps.SystemOne = client
		deps.IncludeText = client.IsLoopback()
		deps.Backend = initiative.BackendInfo{Configured: true, Loopback: client.IsLoopback(), Host: hostOf(base)}
	}

	observerDeps := sessionObserverDeps{Store: in.SessionStore, WorkspaceDir: in.WorkspaceDir}
	if in.Broker != nil {
		observerDeps.SubscriberCount = in.Broker.subscriberCount
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
