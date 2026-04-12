package tarsserver

import (
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/memory"
)

const memoryBackendFile = "file"

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

func normalizeMemoryBackend(raw string) string {
	if v := strings.ToLower(strings.TrimSpace(raw)); v != "" {
		return v
	}
	return memoryBackendFile
}

func validateMemoryBackend(raw string) error {
	if normalizeMemoryBackend(raw) != memoryBackendFile {
		return fmt.Errorf("memory backend %q is not supported; supported backends: %s", strings.TrimSpace(raw), memoryBackendFile)
	}
	return nil
}

func buildMemoryBackend(workspaceDir string, semanticCfg memory.SemanticConfig, backendName string) memory.Backend {
	if normalizeMemoryBackend(backendName) != memoryBackendFile {
		return nil
	}
	return memory.NewFileBackend(workspaceDir, buildSemanticMemoryService(workspaceDir, semanticCfg))
}
