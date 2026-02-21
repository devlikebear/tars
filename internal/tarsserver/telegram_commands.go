package tarsserver

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/rs/zerolog"
)

const telegramMaxMessageLength = 4096

type telegramCommandExecutor interface {
	Execute(ctx context.Context, line, currentSessionID string) (handled bool, result string, nextSessionID string, err error)
}

type telegramCommandExecFunc func(ctx context.Context, line, currentSessionID string) (handled bool, result string, nextSessionID string, err error)

func (f telegramCommandExecFunc) Execute(ctx context.Context, line, currentSessionID string) (bool, string, string, error) {
	if f == nil {
		return false, "", "", nil
	}
	return f(ctx, line, currentSessionID)
}

type telegramCommandHandlerOptions struct {
	Store        *session.Store
	CronResolver *workspaceCronStoreResolver
	Runtime      *gateway.Runtime
	MainSession  string
	SessionScope string
	Logger       zerolog.Logger
}

type telegramCommandHandler struct {
	store        *session.Store
	cronResolver *workspaceCronStoreResolver
	runtime      *gateway.Runtime
	mainSession  string
	sessionScope string
	logger       zerolog.Logger
}

func newTelegramCommandHandler(opts telegramCommandHandlerOptions) *telegramCommandHandler {
	return &telegramCommandHandler{
		store:        opts.Store,
		cronResolver: opts.CronResolver,
		runtime:      opts.Runtime,
		mainSession:  strings.TrimSpace(opts.MainSession),
		sessionScope: normalizeTelegramSessionScope(opts.SessionScope),
		logger:       opts.Logger,
	}
}

func (h *telegramCommandHandler) Execute(ctx context.Context, line, currentSessionID string) (bool, string, string, error) {
	if h == nil {
		return false, "", "", nil
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return false, "", "", nil
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false, "", "", nil
	}
	command := strings.TrimSpace(fields[0])

	switch command {
	case "/help":
		return true, telegramHelpText(), "", nil
	case "/sessions":
		return true, h.cmdSessions(), "", nil
	case "/status":
		return true, h.cmdStatus(), "", nil
	case "/health":
		return true, "SYSTEM > ok=true component=tars", "", nil
	case "/cron":
		result, err := h.cmdCron(ctx, fields)
		return true, result, "", err
	case "/gateway":
		result, err := h.cmdGateway(fields)
		return true, result, "", err
	case "/channels":
		result, err := h.cmdChannels()
		return true, result, "", err
	case "/new":
		if h.sessionScope == "main" {
			return true, blockInMainSessionMessage(), "", nil
		}
		nextSessionID, result, err := h.cmdNew(trimmed)
		return true, result, nextSessionID, err
	case "/resume":
		nextSessionID, result, err := h.cmdResume(fields)
		return true, result, nextSessionID, err
	case "/reload":
		return true, blockedCommandMessage("admin-only command. use tars tui or admin api token."), "", nil
	case "/notify", "/trace", "/quit":
		return true, blockedCommandMessage("tui-only command."), "", nil
	case "/telegram":
		return true, blockedCommandMessage("admin-only command. use tars tui or admin api token."), "", nil
	default:
		return false, "", "", nil
	}
}

func (h *telegramCommandHandler) cmdStatus() string {
	if h.store == nil {
		return "SYSTEM > status unavailable: session store is not configured"
	}
	sessions, err := h.store.List()
	if err != nil {
		return "SYSTEM > status unavailable: list sessions failed"
	}
	mainSessionID := strings.TrimSpace(h.mainSession)
	scope := normalizeTelegramSessionScope(h.sessionScope)
	return fmt.Sprintf(
		"SYSTEM > sessions=%d main_session=%s session_scope=%s",
		len(sessions),
		valueOrDash(mainSessionID),
		scope,
	)
}

func (h *telegramCommandHandler) cmdSessions() string {
	if h.store == nil {
		return "SYSTEM > sessions unavailable: session store is not configured"
	}
	sessions, err := listSessionsOrdered(h.store)
	if err != nil {
		return "SYSTEM > sessions unavailable: list sessions failed"
	}
	if len(sessions) == 0 {
		return "SYSTEM > (no sessions)"
	}
	lines := []string{"SYSTEM > sessions"}
	for _, item := range sessions {
		lines = append(lines, fmt.Sprintf("- %s %s", strings.TrimSpace(item.ID), strings.TrimSpace(item.Title)))
	}
	return strings.Join(lines, "\n")
}

func (h *telegramCommandHandler) cmdCron(ctx context.Context, fields []string) (string, error) {
	if h.cronResolver == nil {
		return "", fmt.Errorf("cron resolver is not configured")
	}
	store, err := h.cronResolver.Resolve(defaultWorkspaceID)
	if err != nil {
		return "", err
	}
	if store == nil {
		return "", fmt.Errorf("cron store is not configured")
	}
	if len(fields) == 1 || strings.EqualFold(strings.TrimSpace(fields[1]), "list") {
		jobs, err := store.List()
		if err != nil {
			return "", err
		}
		if len(jobs) == 0 {
			return "SYSTEM > (no cron jobs)", nil
		}
		lines := []string{"SYSTEM > cron jobs"}
		for _, job := range jobs {
			lines = append(lines, fmt.Sprintf("- %s name=%s schedule=%s enabled=%t", job.ID, job.Name, job.Schedule, job.Enabled))
		}
		return strings.Join(lines, "\n"), nil
	}
	if strings.EqualFold(strings.TrimSpace(fields[1]), "runs") {
		if len(fields) < 3 {
			return "", fmt.Errorf("usage: /cron runs {job_id} [limit]")
		}
		jobID := strings.TrimSpace(fields[2])
		limit := 20
		if len(fields) > 3 {
			n, err := strconv.Atoi(strings.TrimSpace(fields[3]))
			if err != nil || n <= 0 {
				return "", fmt.Errorf("usage: /cron runs {job_id} [limit]")
			}
			limit = n
		}
		if _, err := store.Get(jobID); err != nil {
			return "", err
		}
		runs, err := store.ListRuns(jobID, limit)
		if err != nil {
			return "", err
		}
		if len(runs) == 0 {
			return "SYSTEM > (no cron runs)", nil
		}
		lines := []string{"SYSTEM > cron runs"}
		for _, run := range runs {
			ranAt := run.RanAt.UTC().Format(time.RFC3339)
			if strings.TrimSpace(run.Error) != "" {
				lines = append(lines, fmt.Sprintf("- %s error=%s", ranAt, strings.TrimSpace(run.Error)))
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s response=%s", ranAt, strings.TrimSpace(run.Response)))
		}
		return strings.Join(lines, "\n"), nil
	}
	return "", fmt.Errorf("usage: /cron {list|runs {job_id} [limit]}")
}

func (h *telegramCommandHandler) cmdGateway(fields []string) (string, error) {
	action := "status"
	if len(fields) > 1 {
		action = strings.TrimSpace(fields[1])
	}
	switch action {
	case "", "status":
		if h.runtime == nil {
			return "", fmt.Errorf("gateway runtime is not configured")
		}
		status := h.runtime.Status()
		return fmt.Sprintf(
			"SYSTEM > gateway enabled=%t version=%d runs_total=%d runs_active=%d agents=%d",
			status.Enabled,
			status.Version,
			status.RunsTotal,
			status.RunsActive,
			status.AgentsCount,
		), nil
	case "reload", "restart":
		return blockedCommandMessage("admin-only command. use tars tui or admin api token."), nil
	default:
		return "", fmt.Errorf("usage: /gateway {status}")
	}
}

func (h *telegramCommandHandler) cmdChannels() (string, error) {
	if h.runtime == nil {
		return "", fmt.Errorf("gateway runtime is not configured")
	}
	status := h.runtime.Status()
	return fmt.Sprintf(
		"SYSTEM > channels_local=%t channels_webhook=%t channels_telegram=%t",
		status.ChannelsLocal,
		status.ChannelsWebhook,
		status.ChannelsTelegram,
	), nil
}

func (h *telegramCommandHandler) cmdNew(line string) (string, string, error) {
	if h.store == nil {
		return "", "", fmt.Errorf("session store is not configured")
	}
	title := strings.TrimSpace(strings.TrimPrefix(line, "/new"))
	if title == "" {
		title = "chat"
	}
	created, err := h.store.Create(title)
	if err != nil {
		return "", "", err
	}
	return created.ID, fmt.Sprintf("SYSTEM > created session %s (%s)", created.ID, created.Title), nil
}

func (h *telegramCommandHandler) cmdResume(fields []string) (string, string, error) {
	if h.store == nil {
		return "", "", fmt.Errorf("session store is not configured")
	}
	if len(fields) < 2 || strings.TrimSpace(fields[1]) == "" {
		return "", "SYSTEM > usage: /resume {id|number|latest|main}", nil
	}
	arg := strings.TrimSpace(fields[1])
	if strings.EqualFold(arg, "main") {
		mainSessionID := strings.TrimSpace(h.mainSession)
		if mainSessionID == "" {
			return "", "", fmt.Errorf("main session is not configured")
		}
		if _, err := h.store.Get(mainSessionID); err != nil {
			return "", "", err
		}
		if h.sessionScope == "main" {
			return "", fmt.Sprintf("SYSTEM > using main session=%s", mainSessionID), nil
		}
		return mainSessionID, fmt.Sprintf("SYSTEM > resumed session=%s", mainSessionID), nil
	}
	if h.sessionScope == "main" {
		return "", blockInMainSessionMessage(), nil
	}
	if strings.EqualFold(arg, "latest") {
		latest, err := h.store.Latest()
		if err != nil {
			return "", "", err
		}
		return latest.ID, fmt.Sprintf("SYSTEM > resumed session=%s", latest.ID), nil
	}
	if idx, err := strconv.Atoi(arg); err == nil {
		sessions, err := listSessionsOrdered(h.store)
		if err != nil {
			return "", "", err
		}
		if idx <= 0 || idx > len(sessions) {
			return "", "", fmt.Errorf("resume target out of range: %d", idx)
		}
		next := strings.TrimSpace(sessions[idx-1].ID)
		return next, fmt.Sprintf("SYSTEM > resumed session=%s", next), nil
	}
	if _, err := h.store.Get(arg); err != nil {
		return "", "", err
	}
	next := strings.TrimSpace(arg)
	return next, fmt.Sprintf("SYSTEM > resumed session=%s", next), nil
}

func listSessionsOrdered(store *session.Store) ([]session.Session, error) {
	sessions, err := store.List()
	if err != nil {
		return nil, err
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
		}
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

func telegramHelpText() string {
	return strings.TrimSpace(`SYSTEM > telegram commands
/help
/sessions
/status
/health
/cron {list|runs {job_id} [limit]}
/gateway status
/channels
/new [title]        (per-user scope only)
/resume main        (all scopes)
/resume {id|latest} (per-user scope only)`)
}

func blockedCommandMessage(reason string) string {
	msg := strings.TrimSpace(reason)
	if msg == "" {
		msg = "command is not supported on telegram."
	}
	return "SYSTEM > " + msg
}

func blockInMainSessionMessage() string {
	return blockedCommandMessage("main session mode does not support session switching. use per-user mode.")
}

func valueOrDash(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "-"
	}
	return v
}

func splitTelegramMessage(text string, maxLen int) []string {
	body := strings.TrimSpace(text)
	if body == "" {
		return []string{"done."}
	}
	if maxLen <= 0 {
		maxLen = telegramMaxMessageLength
	}
	if len(body) <= maxLen {
		return []string{body}
	}

	lines := strings.SplitAfter(body, "\n")
	chunks := make([]string, 0, len(lines))
	var buf strings.Builder
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		chunks = append(chunks, buf.String())
		buf.Reset()
	}
	for _, line := range lines {
		if len(line) > maxLen {
			flush()
			start := 0
			for start < len(line) {
				end := start + maxLen
				if end > len(line) {
					end = len(line)
				}
				chunks = append(chunks, line[start:end])
				start = end
			}
			continue
		}
		if buf.Len()+len(line) > maxLen {
			flush()
		}
		buf.WriteString(line)
	}
	flush()
	if len(chunks) <= 1 {
		return chunks
	}
	last := chunks[len(chunks)-1]
	prev := chunks[len(chunks)-2]
	if len(last) < maxLen/8 && len(prev)+len(last) <= maxLen {
		chunks[len(chunks)-2] = prev + last
		chunks = chunks[:len(chunks)-1]
	}
	return chunks
}
