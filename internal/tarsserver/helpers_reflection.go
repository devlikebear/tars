package tarsserver

import (
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/pulse"
	"github.com/devlikebear/tars/internal/reflection"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// reflectionHealthFromSetup returns a pulse.ReflectionHealthSource view
// onto the reflection runtime's state, or nil when reflection is
// disabled. Pulse uses this to emit the reflection_failure signal when
// nightly runs keep failing.
func reflectionHealthFromSetup(setup reflectionSetup) pulse.ReflectionHealthSource {
	if setup.Runtime == nil {
		return nil
	}
	return setup.Runtime.State()
}

// reflectionSetupInputs bundles the dependencies buildReflectionRuntime
// needs. Packaged as a struct for readability at the call site.
type reflectionSetupInputs struct {
	Config       config.Config
	WorkspaceDir string
	Router       llm.Router
	SessionStore *session.Store
	Logger       zerolog.Logger
}

// reflectionSetup is the output of buildReflectionRuntime. A nil Runtime
// means reflection is disabled; Handler is still usable so
// /v1/reflection/* does not 404 (it returns empty status / 503).
type reflectionSetup struct {
	Runtime *reflection.Runtime
	Handler http.Handler
}

// buildReflectionRuntime wires the reflection pipeline from config.
// Returns an empty setup (nil Runtime, disabled handler) when
// reflection is disabled so the rest of server bootstrap code never
// has to branch on enable state.
func buildReflectionRuntime(in reflectionSetupInputs) reflectionSetup {
	view := reflectionConfigViewFromConfig(in.Config)

	if !in.Config.ReflectionEnabled {
		return reflectionSetup{Handler: newReflectionAPIHandler(nil, view, in.Logger)}
	}

	tickInterval := parseDurationWithFallback(in.Config.ReflectionTickInterval, 5*time.Minute)
	emptyAge := parseDurationWithFallback(in.Config.ReflectionEmptySessionAge, 24*time.Hour)

	cfg := reflection.Config{
		Enabled:             true,
		SleepWindow:         in.Config.ReflectionSleepWindow,
		Timezone:            in.Config.ReflectionTimezone,
		TickInterval:        tickInterval,
		EmptySessionAge:     emptyAge,
		MemoryLookbackHours: in.Config.ReflectionMemoryLookbackHours,
		MaxTurnsPerSession:  in.Config.ReflectionMaxTurnsPerSession,
	}

	jobs := []reflection.Job{
		&reflection.MemoryJob{
			WorkspaceDir:       in.WorkspaceDir,
			Backend:            memory.NewFileBackend(in.WorkspaceDir, nil),
			Sessions:           in.SessionStore,
			Router:             in.Router,
			Lookback:           cfg.EffectiveMemoryLookback(),
			MaxTurnsPerSession: cfg.EffectiveMaxTurnsPerSession(),
		},
		&reflection.KBCleanupJob{
			Sessions:        in.SessionStore,
			EmptySessionAge: cfg.EffectiveEmptySessionAge(),
		},
	}

	state := reflection.NewState(0)
	runtime := reflection.NewRuntime(cfg, jobs, state)

	return reflectionSetup{
		Runtime: runtime,
		Handler: newReflectionAPIHandler(runtime, view, in.Logger),
	}
}

func reflectionConfigViewFromConfig(cfg config.Config) reflectionConfigView {
	return reflectionConfigView{
		Enabled:                cfg.ReflectionEnabled,
		SleepWindow:            cfg.ReflectionSleepWindow,
		Timezone:               cfg.ReflectionTimezone,
		TickIntervalSeconds:    int(parseDurationWithFallback(cfg.ReflectionTickInterval, 5*time.Minute) / time.Second),
		EmptySessionAgeSeconds: int64(parseDurationWithFallback(cfg.ReflectionEmptySessionAge, 24*time.Hour) / time.Second),
		MemoryLookbackHours:    cfg.ReflectionMemoryLookbackHours,
		MaxTurnsPerSession:     cfg.ReflectionMaxTurnsPerSession,
	}
}

func parseDurationWithFallback(raw string, fallback time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	return fallback
}
