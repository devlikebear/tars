package tarsclient

import (
	"fmt"
	"strings"
)

func cmdGateway(c commandContext) (bool, string, error) {
	if c.fields[0] != "/gateway" {
		return false, c.session, nil
	}
	action := "status"
	if len(c.fields) > 1 {
		action = strings.TrimSpace(c.fields[1])
	}
	switch action {
	case "status":
		status, err := c.runtime.gatewayStatus(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > gateway enabled=%t version=%d runs_total=%d runs_active=%d agents=%d watch=%t persistence=%t runs_store=%t channels_store=%t restored_runs=%d restored_channels=%d reload_version=%d",
			status.Enabled,
			status.Version,
			status.RunsTotal,
			status.RunsActive,
			status.AgentsCount,
			status.AgentsWatchEnabled,
			status.PersistenceEnabled,
			status.RunsPersistenceEnabled,
			status.ChannelsPersistenceEnabled,
			status.RunsRestored,
			status.ChannelsRestored,
			status.AgentsReloadVersion,
		)
		if strings.TrimSpace(status.LastRestoreError) != "" {
			fmt.Fprintf(c.stdout, " restore_error=%s", strings.TrimSpace(status.LastRestoreError))
		}
		fmt.Fprintln(c.stdout)
		return true, c.session, nil
	case "reload":
		status, err := c.runtime.gatewayReload(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > gateway enabled=%t version=%d\n", status.Enabled, status.Version)
		return true, c.session, nil
	case "restart":
		status, err := c.runtime.gatewayRestart(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > gateway enabled=%t version=%d\n", status.Enabled, status.Version)
		return true, c.session, nil
	case "summary":
		report, err := c.runtime.gatewayReportSummary(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > gateway summary runs_total=%d runs_active=%d channels_total=%d messages_total=%d archive=%t\n",
			report.RunsTotal, report.RunsActive, report.ChannelsTotal, report.MessagesTotal, report.ArchiveEnabled)
		return true, c.session, nil
	case "runs":
		limit := 50
		if len(c.fields) > 2 {
			n, err := parseOptionalLimit(c.fields[2], 50)
			if err != nil {
				return true, c.session, fmt.Errorf("usage: /gateway runs [limit]")
			}
			limit = n
		}
		report, err := c.runtime.gatewayReportRuns(c.ctx, limit)
		if err != nil {
			return true, c.session, err
		}
		if len(report.Runs) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no gateway runs)")
			return true, c.session, nil
		}
		fmt.Fprintln(c.stdout, "SYSTEM > gateway runs")
		for _, run := range report.Runs {
			fmt.Fprintf(c.stdout, "- %s status=%s agent=%s session=%s\n", run.RunID, run.Status, run.Agent, run.SessionID)
		}
		return true, c.session, nil
	case "channels":
		limit := 50
		if len(c.fields) > 2 {
			n, err := parseOptionalLimit(c.fields[2], 50)
			if err != nil {
				return true, c.session, fmt.Errorf("usage: /gateway channels [limit]")
			}
			limit = n
		}
		report, err := c.runtime.gatewayReportChannels(c.ctx, limit)
		if err != nil {
			return true, c.session, err
		}
		if len(report.Messages) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no channel messages)")
			return true, c.session, nil
		}
		fmt.Fprintln(c.stdout, "SYSTEM > gateway channel messages")
		for channelID, messages := range report.Messages {
			fmt.Fprintf(c.stdout, "- %s messages=%d\n", channelID, len(messages))
		}
		return true, c.session, nil
	case "report":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /gateway report {summary|runs [limit]|channels [limit]}")
		}
		fwd := c
		fwd.fields = append([]string{"/gateway"}, c.fields[2:]...)
		return cmdGateway(fwd)
	default:
		return true, c.session, fmt.Errorf("usage: /gateway {status|reload|restart|summary|runs [limit]|channels [limit]}")
	}
}

func cmdChannels(c commandContext) (bool, string, error) {
	if c.fields[0] != "/channels" {
		return false, c.session, nil
	}
	status, err := c.runtime.gatewayStatus(c.ctx)
	if err != nil {
		return true, c.session, err
	}
	fmt.Fprintf(c.stdout, "SYSTEM > channels_local=%t channels_webhook=%t channels_telegram=%t\n",
		status.ChannelsLocalEnabled,
		status.ChannelsWebhookEnabled,
		status.ChannelsTelegramEnabled,
	)
	return true, c.session, nil
}
