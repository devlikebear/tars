package tarsserver

import (
	"context"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/localembed"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/rs/zerolog"
)

// localEmbedProvider is the memory_embed_provider value for the embedding
// sidecar TARS runs itself.
const localEmbedProvider = "local"

// newLocalEmbedManager returns the sidecar manager when semantic memory is on
// and uses provider local, and nil otherwise.
func newLocalEmbedManager(cfg config.Config) (*localembed.Manager, error) {
	if !cfg.MemorySemanticEnabled || memory.NormalizeEmbedProvider(cfg.MemoryEmbedProvider) != localEmbedProvider {
		return nil, nil
	}
	return localembed.New(localembed.Options{
		Dir:     localembed.DirFor(cfg.WorkspaceDir),
		Model:   cfg.MemoryEmbedModel,
		BaseURL: cfg.MemoryEmbedBaseURL,
	})
}

// startLocalEmbed starts the sidecar in the background; loading the model
// takes seconds and must not hold up the API. Until it answers — or when the
// model is not installed — embedding calls fail and recall falls back to
// keyword matches. shutdownRuntime stops it.
func startLocalEmbed(ctx context.Context, runtime *serveAPIRuntime, logger zerolog.Logger) {
	mgr, err := newLocalEmbedManager(runtime.cfg)
	if err != nil {
		logger.Warn().Err(err).Msg("local embedding sidecar misconfigured")
		return
	}
	if mgr == nil {
		return
	}
	runtime.localEmbed = mgr
	go func() {
		if err := mgr.Start(ctx); err != nil {
			logger.Warn().Err(err).Str("log", mgr.LogPath()).Msg("local embedding sidecar not running; memory recall uses keywords only")
			return
		}
		logger.Info().Str("model", runtime.cfg.MemoryEmbedModel).Msg("local embedding sidecar ready")
	}()
}
