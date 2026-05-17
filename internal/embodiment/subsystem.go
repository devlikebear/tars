package embodiment

import (
	"context"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

type Status struct {
	Enabled   bool
	Providers []ProviderDescriptor
}

type Subsystem struct {
	cfg      config.EmbodimentConfig
	logger   zerolog.Logger
	registry *Registry

	mu      sync.Mutex
	running bool
}

func New(cfg config.EmbodimentConfig, logger zerolog.Logger) *Subsystem {
	subsystem := &Subsystem{
		cfg:      cfg,
		logger:   logger,
		registry: NewRegistry(),
	}
	for _, provider := range cfg.Providers {
		desc := descriptorFromConfig(provider)
		if err := subsystem.registry.Register(desc); err != nil {
			subsystem.logger.Warn().Err(err).Str("provider", strings.TrimSpace(provider.Name)).Msg("skip embodiment provider")
		}
	}
	return subsystem
}

func (s *Subsystem) Start(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.cfg.Enabled || s.registry.Empty() {
		s.running = false
		return nil
	}
	s.running = true
	s.logger.Info().Int("providers", len(s.registry.Enabled())).Msg("embodiment subsystem enabled")
	return nil
}

func (s *Subsystem) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
}

func (s *Subsystem) Status() Status {
	if s == nil {
		return Status{}
	}
	providers := s.registry.Enabled()
	return Status{
		Enabled:   s.cfg.Enabled && len(providers) > 0,
		Providers: providers,
	}
}

func descriptorFromConfig(provider config.EmbodimentProviderConfig) ProviderDescriptor {
	capabilities := make([]Capability, 0, len(provider.Capabilities))
	for _, capability := range provider.Capabilities {
		capabilities = append(capabilities, Capability(capability))
	}
	return ProviderDescriptor{
		Name:         provider.Name,
		Enabled:      provider.Enabled,
		Transport:    Transport(provider.Transport),
		Endpoint:     provider.Endpoint,
		Capabilities: capabilities,
	}
}
