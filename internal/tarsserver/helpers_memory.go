package tarsserver

import (
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/memory"
)

func semanticMemoryConfigFromConfig(cfg config.Config) memory.SemanticConfig {
	return memory.SemanticConfig{
		Enabled:         cfg.MemorySemanticEnabled,
		EmbedProvider:   strings.TrimSpace(cfg.MemoryEmbedProvider),
		EmbedBaseURL:    strings.TrimSpace(cfg.MemoryEmbedBaseURL),
		EmbedAPIKey:     strings.TrimSpace(cfg.MemoryEmbedAPIKey),
		EmbedModel:      strings.TrimSpace(cfg.MemoryEmbedModel),
		EmbedDimensions: cfg.MemoryEmbedDimensions,
	}
}

func buildSemanticMemoryService(workspaceDir string, semanticCfg memory.SemanticConfig) *memory.Service {
	if !semanticCfg.Enabled {
		return nil
	}
	return memory.NewService(workspaceDir, memory.ServiceOptions{Config: semanticCfg})
}
