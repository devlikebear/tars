package tarsserver

import (
	"context"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/apihandlers"
	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/keepawake"
	"github.com/rs/zerolog"
)

// Keeping the desk awake for a paired phone (docs/decisions/mobile-remote.md,
// "A device that sleeps is unreachable"). A phone reaches this desktop only
// while it is awake; a laptop left on the charger otherwise sleeps, and the
// phone's inbox shows it as not answering.
//
// The owner turns it on in the Phones card (off unless set). It holds only
// while a phone is paired, and only on AC power: on macOS the assertion is
// caffeinate -s, which the system honors only when plugged in. The display
// still sleeps. Go decides from two facts, never a model; the loop checks
// every keepAwakeInterval and at once when the choice changes, and releases
// on shutdown.

const keepAwakeInterval = 30 * time.Second

// keepAwakeHolder is keepawake.Holder, narrowed for tests.
type keepAwakeHolder interface {
	Supported() bool
	Held() bool
	Hold() error
	Release()
}

type deskKeepAwake struct {
	store  *consoleauth.Store
	holder keepAwakeHolder
	// paired reports whether an active phone is paired.
	paired func() (bool, error)
	logger zerolog.Logger

	mu sync.Mutex
	// failed is set when the last Hold did not start.
	failed bool
}

func newDeskKeepAwake(workspaceDir string, logger zerolog.Logger) *deskKeepAwake {
	store := consoleauth.NewStore(workspaceDir)
	return &deskKeepAwake{
		store:  store,
		holder: keepawake.New(),
		paired: func() (bool, error) { return phoneDevicePaired(store) },
		logger: logger,
	}
}

// keepAwakeWanted is the whole decision.
func keepAwakeWanted(enabled, paired bool) bool {
	return enabled && paired
}

// ApplyKeepAwake holds or releases to match the owner's choice and the
// paired phones now.
func (k *deskKeepAwake) ApplyKeepAwake() {
	if k == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	enabled, err := k.store.KeepAwake()
	if err != nil {
		k.logger.Debug().Err(err).Msg("keep awake: read choice")
		return
	}
	paired, err := k.paired()
	if err != nil {
		k.logger.Debug().Err(err).Msg("keep awake: list devices")
		return
	}
	want := keepAwakeWanted(enabled, paired) && k.holder.Supported()
	switch {
	case want && !k.holder.Held():
		if err := k.holder.Hold(); err != nil {
			if !k.failed {
				k.logger.Warn().Err(err).Msg("keep awake: could not hold")
			}
			k.failed = true
			return
		}
		k.failed = false
		k.logger.Info().Msg("keep awake: holding while plugged in, for a paired phone")
	case !want && k.holder.Held():
		k.holder.Release()
		k.failed = false
		k.logger.Info().Msg("keep awake: released")
	case !want:
		k.failed = false
	}
}

// KeepAwakeStatus is what the Phones card shows.
func (k *deskKeepAwake) KeepAwakeStatus() (apihandlers.KeepAwakeStatus, error) {
	enabled, err := k.store.KeepAwake()
	if err != nil {
		return apihandlers.KeepAwakeStatus{}, err
	}
	paired, err := k.paired()
	if err != nil {
		return apihandlers.KeepAwakeStatus{}, err
	}
	k.mu.Lock()
	failed := k.failed
	k.mu.Unlock()
	status := apihandlers.KeepAwakeStatus{Enabled: enabled, Supported: k.holder.Supported(), Active: k.holder.Held()}
	if enabled && !status.Active {
		switch {
		case !status.Supported:
			status.Reason = "unsupported"
		case !paired:
			status.Reason = "no_phone"
		case failed:
			status.Reason = "failed"
		}
	}
	return status, nil
}

// start applies now and every keepAwakeInterval until ctx ends, then
// releases.
func (k *deskKeepAwake) start(ctx context.Context) {
	if k == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(keepAwakeInterval)
		defer ticker.Stop()
		k.ApplyKeepAwake()
		for {
			select {
			case <-ctx.Done():
				k.mu.Lock()
				k.holder.Release()
				k.mu.Unlock()
				return
			case <-ticker.C:
				k.ApplyKeepAwake()
			}
		}
	}()
}
