package tarsserver

import (
	"context"
	"time"

	"github.com/devlikebear/tars/internal/heartbeat"
	"github.com/rs/zerolog"
)

type heartbeatTickerManager struct {
	run      func(ctx context.Context) (heartbeat.RunResult, error)
	interval time.Duration
	logger   zerolog.Logger
}

func newHeartbeatTickerManager(
	run func(ctx context.Context) (heartbeat.RunResult, error),
	interval time.Duration,
	logger zerolog.Logger,
) *heartbeatTickerManager {
	if run == nil || interval <= 0 {
		return nil
	}
	return &heartbeatTickerManager{
		run:      run,
		interval: interval,
		logger:   logger,
	}
}

func (m *heartbeatTickerManager) Start(ctx context.Context) error {
	if m == nil || m.run == nil {
		return nil
	}
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	m.logger.Info().Dur("interval", m.interval).Msg("heartbeat ticker started")
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := m.run(ctx); err != nil {
				m.logger.Debug().Err(err).Msg("heartbeat tick failed")
			}
		}
	}
}
