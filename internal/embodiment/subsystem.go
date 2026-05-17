package embodiment

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

type Status struct {
	Enabled   bool
	Providers []ProviderDescriptor
}

type Subsystem struct {
	cfg            config.EmbodimentConfig
	logger         zerolog.Logger
	registry       *Registry
	providers      map[string]config.EmbodimentProviderConfig
	gate           *Gate
	cognition      *Cognition
	defaultSession string
	defaultAgent   string

	mu      sync.Mutex
	running bool
}

type Options struct {
	Runtime          AgentRuntime
	DefaultSessionID string
	DefaultAgent     string
	Now              func() time.Time
}

func New(cfg config.EmbodimentConfig, logger zerolog.Logger) *Subsystem {
	return NewWithOptions(cfg, logger, Options{})
}

func NewWithOptions(cfg config.EmbodimentConfig, logger zerolog.Logger, opts Options) *Subsystem {
	subsystem := &Subsystem{
		cfg:            cfg,
		logger:         logger,
		registry:       NewRegistry(),
		providers:      map[string]config.EmbodimentProviderConfig{},
		defaultSession: strings.TrimSpace(opts.DefaultSessionID),
		defaultAgent:   strings.TrimSpace(opts.DefaultAgent),
	}
	subsystem.gate = NewGate(defaultGateConfig(cfg, opts.Now))
	subsystem.cognition = NewCognition(opts.Runtime, CognitionConfig{
		DefaultSessionID: subsystem.defaultSession,
		DefaultAgent:     subsystem.defaultAgent,
	})
	for _, provider := range cfg.Providers {
		provider = config.NormalizeEmbodimentProviderForRuntime(provider)
		desc := descriptorFromConfig(provider)
		if err := subsystem.registry.Register(desc); err != nil {
			subsystem.logger.Warn().Err(err).Str("provider", strings.TrimSpace(provider.Name)).Msg("skip embodiment provider")
			continue
		}
		subsystem.providers[desc.Name] = provider
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

func (s *Subsystem) KnownProvider(provider string) bool {
	if s == nil || !s.cfg.Enabled {
		return false
	}
	desc, ok := s.registry.Get(provider)
	return ok && desc.Enabled
}

func (s *Subsystem) IngestPayload(ctx context.Context, provider string, payload map[string]any) (IngestResult, error) {
	percept, err := NormalizePercept(provider, payload, NormalizeOptions{
		KnownBody: s.KnownProvider(provider),
	})
	if err != nil {
		return IngestResult{}, err
	}
	return s.IngestPercept(ctx, percept)
}

func (s *Subsystem) IngestPercept(ctx context.Context, percept Percept) (IngestResult, error) {
	if s == nil || !s.cfg.Enabled {
		return IngestResult{Percept: percept, Decision: GateDecision{Mode: GateModeObservation, Reason: GateReasonDisabled}}, nil
	}
	desc, known := s.registry.Get(percept.Provider)
	if known && desc.Enabled {
		percept.IsSelfSensory = true
	}
	if providerCfg, ok := s.providers[normalizeName(percept.Provider)]; ok {
		if strings.TrimSpace(percept.SessionID) == "" {
			percept.SessionID = strings.TrimSpace(providerCfg.SessionID)
		}
	}
	if strings.TrimSpace(percept.SessionID) == "" {
		percept.SessionID = s.defaultSession
	}
	decision := s.gate.Decide(percept)
	cognitionResult, err := s.cognition.Trigger(ctx, percept, decision)
	if err != nil {
		return IngestResult{Percept: percept, Decision: decision}, err
	}
	return IngestResult{Percept: percept, Decision: decision, CognitionResult: cognitionResult}, nil
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

func defaultGateConfig(cfg config.EmbodimentConfig, now func() time.Time) GateConfig {
	gateCfg := GateConfig{
		MinTriggerInterval: 30 * time.Second,
		MaxTriggersPerHour: 60,
		Now:                now,
	}
	for _, provider := range cfg.Providers {
		if provider.MinTriggerInterval != "" {
			if parsed, err := time.ParseDuration(provider.MinTriggerInterval); err == nil && parsed >= 0 {
				gateCfg.MinTriggerInterval = parsed
			}
		}
		if provider.MaxTriggersPerHour > 0 {
			gateCfg.MaxTriggersPerHour = provider.MaxTriggersPerHour
		}
		if provider.TriggerObservations {
			gateCfg.TriggerObservations = true
		}
	}
	return gateCfg
}
