package tarsserver

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/textutil"
)

func (h *telegramCommandHandler) cmdProviders() string {
	if h.providerModels == nil {
		return "SYSTEM > provider metadata unavailable: service is not configured"
	}
	info := h.providerModels.providers()
	lines := []string{
		fmt.Sprintf("SYSTEM > provider=%s model=%s auth_mode=%s supported=%d",
			textutil.ValueOrDash(info.CurrentProvider),
			textutil.ValueOrDash(info.CurrentModel),
			textutil.ValueOrDash(info.AuthMode),
			len(info.Providers),
		),
	}
	for _, item := range info.Providers {
		lines = append(lines, fmt.Sprintf("- %s live_models=%t", strings.TrimSpace(item.ID), item.SupportsLiveModels))
	}
	return strings.Join(lines, "\n")
}

func (h *telegramCommandHandler) cmdModels(ctx context.Context) (string, error) {
	if h.providerModels == nil {
		return "SYSTEM > models unavailable: service is not configured", nil
	}
	info, err := h.providerModels.models(ctx)
	if err != nil {
		return "", err
	}
	lines := []string{
		fmt.Sprintf("SYSTEM > models provider=%s current=%s source=%s stale=%t count=%d",
			textutil.ValueOrDash(info.Provider),
			textutil.ValueOrDash(info.CurrentModel),
			textutil.ValueOrDash(info.Source),
			info.Stale,
			len(info.Models),
		),
	}
	if fetchedAt := strings.TrimSpace(info.FetchedAt); fetchedAt != "" {
		lines = append(lines, fmt.Sprintf("fetched_at=%s expires_at=%s", fetchedAt, textutil.ValueOrDash(info.ExpiresAt)))
	}
	if warning := strings.TrimSpace(info.Warning); warning != "" {
		lines = append(lines, "warning="+warning)
	}
	for _, model := range info.Models {
		lines = append(lines, "- "+strings.TrimSpace(model))
	}
	return strings.Join(lines, "\n"), nil
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
		textutil.ValueOrDash(publicMainSessionLabel(mainSessionID)),
		scope,
	)
}

func (h *telegramCommandHandler) cmdSessions() string {
	return blockInMainSessionMessage()
}

func (h *telegramCommandHandler) cmdSession() string {
	return fmt.Sprintf("SYSTEM > session=%s", publicMainSessionLabel(h.mainSession))
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
