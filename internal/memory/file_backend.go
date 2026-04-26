package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	zlog "github.com/rs/zerolog/log"
)

type FileBackend struct {
	root     string
	semantic *Service
}

func NewFileBackend(root string, semantic *Service) *FileBackend {
	return &FileBackend{root: strings.TrimSpace(root), semantic: semantic}
}

var _ Backend = (*FileBackend)(nil)

func (b *FileBackend) LoadDurable(_ context.Context, kind DurableKind, name string) (string, error) {
	path, err := b.durablePath(kind, name)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (b *FileBackend) SaveDurable(_ context.Context, kind DurableKind, name string, body string) error {
	path, err := b.durablePath(kind, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func (b *FileBackend) Search(ctx context.Context, req SearchRequest) ([]SearchHit, error) {
	if b == nil || b.semantic == nil {
		return nil, ErrSemanticUnavailable
	}
	return b.semantic.Search(ctx, req)
}

func (b *FileBackend) AppendMemoryNote(_ context.Context, at time.Time, entry string) error {
	return AppendMemoryNote(b.root, at, entry)
}

func (b *FileBackend) AppendExperience(ctx context.Context, exp Experience) error {
	if err := AppendExperience(b.root, exp); err != nil {
		return err
	}
	if b != nil && b.semantic != nil {
		if err := b.semantic.IndexExperience(ctx, exp); err != nil {
			zlog.Logger.Warn().
				Err(err).
				Str("category", exp.Category).
				Str("source_session", exp.SourceSession).
				Time("timestamp", exp.Timestamp).
				Msg("memory: semantic IndexExperience failed; experience saved but not searchable")
		}
	}
	return nil
}

func (b *FileBackend) SearchExperiences(_ context.Context, opts SearchOptions) ([]Experience, error) {
	return SearchExperiences(b.root, opts)
}

func (b *FileBackend) durablePath(kind DurableKind, name string) (string, error) {
	if b == nil || strings.TrimSpace(b.root) == "" {
		return "", fmt.Errorf("memory backend is not configured")
	}
	if err := EnsureWorkspace(b.root); err != nil {
		return "", err
	}
	switch kind {
	case DurableKindMemory:
		return filepath.Join(b.root, "MEMORY.md"), nil
	case DurableKindDaily:
		name = strings.TrimSpace(name)
		if name == "" {
			return "", fmt.Errorf("daily durable name is required")
		}
		return filepath.Join(b.root, "memory", name+".md"), nil
	default:
		return "", fmt.Errorf("unsupported durable kind: %s", kind)
	}
}
