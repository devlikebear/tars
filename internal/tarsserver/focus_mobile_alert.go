package tarsserver

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/remoteaccess"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// Focus alerts for a paired phone (docs/decisions/mobile-remote.md §7): when
// a focus pipeline starts waiting on its developer — a gate opens, a
// decision or finding card arrives, an unattended turn queues a tool call —
// the desktop sends one Telegram message with a link to that pipeline.
//
// Who decides: Go, from the pipeline files and the ops queue
// (focuspipeline.WaitingOf). No model is asked whether something is worth a
// ping.
//
// When it sends: only while a phone is paired (an active focus device), the
// Telegram channel is enabled, and exactly one Telegram chat is paired. Each
// thing that waits is announced once; a pipeline left waiting is not
// announced again every tick. The first tick after start only records what
// already waits, so a restart sends nothing.
//
// The link: Telegram makes only http(s) and tg:// links tappable, so the
// message carries the desktop's own Tailscale HTTPS address
// (remoteaccess.Status.URL) with the console's focus path, which the phone
// reaches because it is on the tailnet. The app claims that link when it
// exists; until then it opens the console. With no Tailscale serve address
// the message goes without a link.

const (
	focusAlertInterval = 30 * time.Second
	// focusAlertURLTTL is how long the Tailscale address is reused before
	// `tailscale status` runs again.
	focusAlertURLTTL = 10 * time.Minute
	// focusAlertGoalRunes caps the goal line of a message.
	focusAlertGoalRunes = 120
)

var errFocusAlertNoChat = errors.New("no single paired telegram chat")

type focusAlertState struct {
	wait      focuspipeline.Waiting
	approvals []string
}

type focusMobileAlerts struct {
	pipelines *focuspipeline.Store
	// devices reports whether a phone is paired.
	devices func() (bool, error)
	// queued lists unattended turns' tool calls waiting in the ops queue.
	queued func() ([]chatQueuedApproval, error)
	// send delivers one message; nil when Telegram is not set up.
	send func(ctx context.Context, text string) error
	// baseURL is the address a phone reaches this desktop at; "" when none.
	baseURL func(ctx context.Context) string
	logger  zerolog.Logger

	mu     sync.Mutex
	seen   map[string]focusAlertState
	primed bool
}

// newFocusMobileAlerts builds the alerts. Messages can go out only when the
// Telegram channel is enabled and has a bot token; the paired phone and the
// single paired chat are checked at each send.
func newFocusMobileAlerts(cfg config.Config, sessions *session.Store, unattended *unattendedPermissions, sender telegramSender, pairings *telegramPairingStore, logger zerolog.Logger) *focusMobileAlerts {
	a := &focusMobileAlerts{
		pipelines: focusStoreFor(sessions),
		logger:    logger,
	}
	store := consoleauth.NewStore(cfg.WorkspaceDir)
	a.devices = func() (bool, error) { return phoneDevicePaired(store) }
	if unattended != nil {
		a.queued = unattended.queuedApprovals
	}
	if cfg.ChannelsTelegramEnabled && sender != nil && pairings != nil {
		a.send = func(ctx context.Context, text string) error {
			chatID, err := pairings.resolveDefaultChatID()
			if err != nil || strings.TrimSpace(chatID) == "" {
				return errFocusAlertNoChat
			}
			_, err = sender.Send(ctx, telegramSendRequest{ChatID: chatID, Text: text})
			return err
		}
	}
	return a
}

// startFocusAlerts starts the alerts with the desktop's Tailscale address as
// the link base, read through the same runner and target as remote access.
func startFocusAlerts(ctx context.Context, runtime *serveAPIRuntime) {
	if runtime == nil || runtime.focusAlerts == nil {
		return
	}
	port := effectiveRemoteAccessPort(runtime.cfg.RemoteAccessTailscaleServeHTTPSPort)
	opts := remoteaccess.Options{Runner: remoteAccessRunner(runtime), HTTPSPort: port, TargetURL: remoteAccessTargetURL(runtime)}
	runtime.focusAlerts.baseURL = cachedRemoteURL(func(ctx context.Context) string {
		detectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		status, err := remoteaccess.Detect(detectCtx, opts)
		if err != nil {
			return ""
		}
		return status.URL()
	}, focusAlertURLTTL, time.Now)
	runtime.focusAlerts.start(ctx)
}

// phoneDevicePaired reports whether store has an active focus device.
func phoneDevicePaired(store *consoleauth.Store) (bool, error) {
	devices, err := store.ListDevices()
	if err != nil {
		return false, err
	}
	for _, d := range devices {
		if d.Active() && d.Role == consoleauth.RoleFocus {
			return true, nil
		}
	}
	return false, nil
}

// start runs a tick every focusAlertInterval until ctx ends.
func (a *focusMobileAlerts) start(ctx context.Context) {
	if a == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(focusAlertInterval)
		defer ticker.Stop()
		a.tick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.tick(ctx)
			}
		}
	}()
}

// tick compares what waits now with what waited at the last tick and sends
// a message for each pipeline with something new.
func (a *focusMobileAlerts) tick(ctx context.Context) {
	if a == nil || a.pipelines == nil {
		return
	}
	pipelines, err := a.pipelines.List()
	if err != nil {
		a.logger.Debug().Err(err).Msg("focus alerts: list pipelines")
		return
	}
	queued := map[string][]chatQueuedApproval{}
	if a.queued != nil {
		if list, err := a.queued(); err == nil {
			for _, q := range list {
				queued[q.SessionID] = append(queued[q.SessionID], q)
			}
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	enabled := a.primed && a.enabled()
	next := make(map[string]focusAlertState, len(pipelines))
	for _, p := range pipelines {
		state := focusAlertState{wait: focuspipeline.WaitingOf(p)}
		var approvalLines []string
		prev := a.seen[p.SessionID]
		for _, sessionID := range []string{p.SessionID, p.QASessionID} {
			for _, q := range queued[sessionID] {
				state.approvals = append(state.approvals, q.ApprovalID)
				if !slices.Contains(prev.approvals, q.ApprovalID) {
					approvalLines = append(approvalLines, "Tool call waits for approval: "+q.ToolName)
				}
			}
		}
		next[p.SessionID] = state
		lines := append(state.wait.NewSince(prev.wait), approvalLines...)
		if !enabled || len(lines) == 0 {
			continue
		}
		if err := a.send(ctx, focusAlertMessage(p, lines, a.link(ctx, p.SessionID))); err != nil {
			a.logger.Warn().Err(err).Str("session_id", p.SessionID).Msg("focus alerts: send")
		}
	}
	a.seen = next
	a.primed = true
}

// enabled reports whether a message may go out now.
func (a *focusMobileAlerts) enabled() bool {
	if a.send == nil || a.devices == nil {
		return false
	}
	paired, err := a.devices()
	return err == nil && paired
}

// link is the pipeline's address on this desktop for the phone, "" when the
// desktop has no Tailscale serve address.
func (a *focusMobileAlerts) link(ctx context.Context, sessionID string) string {
	if a.baseURL == nil {
		return ""
	}
	base := strings.TrimRight(a.baseURL(ctx), "/")
	if base == "" {
		return ""
	}
	return base + "/console/focus/" + url.PathEscape(sessionID)
}

// focusAlertMessage is the text of one alert: who is waiting, on what, and
// where to answer. Plain text, no parse mode.
func focusAlertMessage(p focuspipeline.Pipeline, lines []string, link string) string {
	head := "TARS · waiting on you"
	if link != "" {
		if u, err := url.Parse(link); err == nil && u.Hostname() != "" {
			head = "TARS on " + strings.SplitN(u.Hostname(), ".", 2)[0] + " · waiting on you"
		}
	}
	var b strings.Builder
	b.WriteString(head)
	if goal := truncateRunes(strings.TrimSpace(p.Goal), focusAlertGoalRunes); goal != "" {
		b.WriteString("\n" + goal)
	}
	for _, line := range lines {
		b.WriteString("\n• " + line)
	}
	if link != "" {
		b.WriteString("\n" + link)
	}
	return b.String()
}

// cachedRemoteURL wraps detect so `tailscale status` runs at most once per
// ttl. A failed or empty answer is cached too: the next try waits as long.
func cachedRemoteURL(detect func(ctx context.Context) string, ttl time.Duration, now func() time.Time) func(ctx context.Context) string {
	var mu sync.Mutex
	var value string
	var at time.Time
	return func(ctx context.Context) string {
		mu.Lock()
		defer mu.Unlock()
		if !at.IsZero() && now().Sub(at) < ttl {
			return value
		}
		value, at = detect(ctx), now()
		return value
	}
}
