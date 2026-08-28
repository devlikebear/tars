package memory

import (
	"context"
	"time"
)

type DurableKind string

const (
	DurableKindMemory DurableKind = "memory"
	DurableKindDaily  DurableKind = "daily"
)

// Backend is the narrow memory surface currently used by tools and reflection.
// It intentionally excludes session-store operations.
type Backend interface {
	LoadDurable(ctx context.Context, kind DurableKind, name string) (string, error)
	SaveDurable(ctx context.Context, kind DurableKind, name string, body string) error
	Search(ctx context.Context, req SearchRequest) ([]SearchHit, error)
	AppendMemoryNote(ctx context.Context, at time.Time, entry string) error
	AppendExperience(ctx context.Context, exp Experience) error
	SearchExperiences(ctx context.Context, opts SearchOptions) ([]Experience, error)
}
