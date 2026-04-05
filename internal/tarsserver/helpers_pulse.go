package tarsserver

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/pulse"
	"github.com/devlikebear/tars/internal/pulse/autofix"
	"github.com/rs/zerolog"
)

// pulseSetupInputs bundles everything buildPulseRuntime needs. Kept as a
// struct (rather than positional args) because callers pass ~10 values
// and the noise hurts readability at the call site.
type pulseSetupInputs struct {
	Config          config.Config
	WorkspaceDir    string
	LLMClient       llm.Client
	CronStore       *cron.Store
	GatewayRuntime  *gateway.Runtime
	OpsManager      *ops.Manager
	DeliveryCounter *telegramDeliveryCounter
	NotifyEmit      func(ctx context.Context, evt notificationEvent)
	Logger          zerolog.Logger
}

// pulseSetup is the result of wiring up a pulse runtime. A nil Runtime
// means pulse is disabled; the Handler is still usable (it returns
// empty/default responses) so /v1/pulse/* does not 404.
type pulseSetup struct {
	Runtime *pulse.Runtime
	Handler http.Handler
}

// buildPulseRuntime assembles scanner + decider + notify router +
// autofix registry + state + runtime from config and wired dependencies.
//
// Returns pulseSetup{Runtime: nil} when pulse is disabled in config. In
// that case the handler still exists and serves /v1/pulse/status as an
// empty snapshot, which is convenient for frontend polling code that
// doesn't branch on the enabled flag.
func buildPulseRuntime(in pulseSetupInputs) pulseSetup {
	view := pulseConfigViewFromConfig(in.Config)

	if !in.Config.PulseEnabled {
		return pulseSetup{Handler: newPulseAPIHandler(nil, view, in.Logger)}
	}

	interval := parsePulseDuration(in.Config.PulseInterval, time.Minute)
	timeout := parsePulseDuration(in.Config.PulseTimeout, 2*time.Minute)
	deliveryWindow := parsePulseDuration(in.Config.PulseDeliveryFailureWindow, 10*time.Minute)

	thresholds := pulse.Thresholds{
		CronConsecutiveFailures:      in.Config.PulseCronFailureThreshold,
		StuckRunMinutes:              in.Config.PulseStuckRunMinutes,
		DiskUsedPercentWarn:          in.Config.PulseDiskWarnPercent,
		DiskUsedPercentCritical:      in.Config.PulseDiskCriticalPercent,
		DeliveryFailuresWithinWindow: in.Config.PulseDeliveryFailureThreshold,
		DeliveryFailureWindow:        deliveryWindow,
	}

	sources := pulse.ScannerSources{
		Cron:     in.CronStore,
		Gateway:  in.GatewayRuntime,
		Ops:      in.OpsManager,
		Delivery: in.DeliveryCounter,
	}
	scanner := pulse.NewScanner(sources, thresholds)

	// Register Phase 1 autofixes. The config allow-list gates which of
	// these the decider may actually invoke.
	reg := autofix.NewRegistry()
	reg.Register(&autofix.CompressOldLogs{
		LogsDir: filepath.Join(in.WorkspaceDir, "logs"),
	})
	reg.Register(&autofix.CleanupStaleTmp{
		Dirs: []string{
			filepath.Join(in.WorkspaceDir, "tmp"),
			filepath.Join(in.WorkspaceDir, "_shared", "tmp"),
		},
	})
	allowedAutofixes := reg.AllowedIntersection(in.Config.PulseAllowedAutofixes)

	minSeverity, _ := pulse.ParseSeverity(strings.TrimSpace(in.Config.PulseMinSeverity))

	decider := pulse.NewDecider(in.LLMClient, pulse.DeciderPolicy{
		AllowedAutofixes: allowedAutofixes,
		MinSeverity:      minSeverity,
	})

	// Adapter: pulse.NotifyEvent → notificationDispatcher.Emit so pulse
	// notifications land in the same session event stream the rest of
	// the server uses. Pulse itself knows nothing about the broker.
	var notifier pulse.Notifier
	if in.Config.PulseNotifySessionEvents && in.NotifyEmit != nil {
		emit := in.NotifyEmit
		notifier = pulse.NotifierFunc(func(ctx context.Context, evt pulse.NotifyEvent) error {
			emit(ctx, newNotificationEvent(
				evt.Category,
				evt.Severity.String(),
				evt.Title,
				evt.Message,
			))
			return nil
		})
	}
	router := pulse.NewNotifyRouter(notifier, pulse.NotifyConfig{MinSeverity: minSeverity})

	runtime := pulse.NewRuntime(pulse.Config{
		Enabled:     true,
		Interval:    interval,
		Timeout:     timeout,
		ActiveHours: in.Config.PulseActiveHours,
		Timezone:    in.Config.PulseTimezone,
	}, pulse.Dependencies{
		Scanner:   scanner,
		Decider:   decider,
		Router:    router,
		Autofixes: reg,
		State:     pulse.NewState(50),
	})

	return pulseSetup{
		Runtime: runtime,
		Handler: newPulseAPIHandler(runtime, view, in.Logger),
	}
}

// pulseConfigViewFromConfig projects the relevant config fields into
// the read-only view exposed by the HTTP handler.
func pulseConfigViewFromConfig(cfg config.Config) pulseConfigView {
	return pulseConfigView{
		Enabled:              cfg.PulseEnabled,
		IntervalSeconds:      int(parsePulseDuration(cfg.PulseInterval, time.Minute) / time.Second),
		TimeoutSeconds:       int(parsePulseDuration(cfg.PulseTimeout, 2*time.Minute) / time.Second),
		ActiveHours:          cfg.PulseActiveHours,
		Timezone:             cfg.PulseTimezone,
		MinSeverity:          cfg.PulseMinSeverity,
		AllowedAutofixes:     append([]string{}, cfg.PulseAllowedAutofixes...),
		NotifyTelegram:       cfg.PulseNotifyTelegram,
		NotifySessionEvents:  cfg.PulseNotifySessionEvents,
		CronFailureThreshold: cfg.PulseCronFailureThreshold,
		StuckRunMinutes:      cfg.PulseStuckRunMinutes,
		DiskWarnPercent:      cfg.PulseDiskWarnPercent,
		DiskCriticalPercent:  cfg.PulseDiskCriticalPercent,
	}
}

func parsePulseDuration(raw string, fallback time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	return fallback
}
