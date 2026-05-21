package memory

import (
	"context"
	"time"

	internal "github.com/devlikebear/tars/internal/memory"
)

type DurableKind = internal.DurableKind

const (
	DurableKindMemory = internal.DurableKindMemory
	DurableKindDaily  = internal.DurableKindDaily
)

var ErrSemanticUnavailable = internal.ErrSemanticUnavailable

type Backend = internal.Backend
type FileBackend = internal.FileBackend
type SemanticConfig = internal.SemanticConfig
type EmbedRequest = internal.EmbedRequest
type Embedder = internal.Embedder
type Searcher = internal.Searcher
type ServiceOptions = internal.ServiceOptions
type Service = internal.Service
type MemoryEntry = internal.MemoryEntry
type CompactionMemory = internal.CompactionMemory
type SearchRequest = internal.SearchRequest
type SearchHit = internal.SearchHit
type Experience = internal.Experience
type SearchOptions = internal.SearchOptions
type MemoryNote = internal.MemoryNote
type WorkspaceBootstrapFileSpec = internal.WorkspaceBootstrapFileSpec
type MemoryCandidateStatus = internal.MemoryCandidateStatus
type MemoryCandidateAction = internal.MemoryCandidateAction
type MemoryCandidate = internal.MemoryCandidate
type MemoryCandidateProvenance = internal.MemoryCandidateProvenance
type MemoryCandidateHint = internal.MemoryCandidateHint
type MemoryCandidateReview = internal.MemoryCandidateReview
type MemoryCandidateListOptions = internal.MemoryCandidateListOptions

const (
	MemoryCandidateStatusPending  = internal.MemoryCandidateStatusPending
	MemoryCandidateStatusApproved = internal.MemoryCandidateStatusApproved
	MemoryCandidateStatusRejected = internal.MemoryCandidateStatusRejected
	MemoryCandidateStatusMerged   = internal.MemoryCandidateStatusMerged
	MemoryCandidateActionApprove  = internal.MemoryCandidateActionApprove
	MemoryCandidateActionReject   = internal.MemoryCandidateActionReject
	MemoryCandidateActionMerge    = internal.MemoryCandidateActionMerge
)

func NewFileBackend(root string, semantic *Service) *FileBackend {
	return internal.NewFileBackend(root, semantic)
}

func NormalizeEmbedProvider(raw string) string { return internal.NormalizeEmbedProvider(raw) }

func SupportedEmbedProviders() []string { return internal.SupportedEmbedProviders() }

func IsSupportedEmbedProvider(raw string) bool { return internal.IsSupportedEmbedProvider(raw) }

func ValidateSemanticConfig(cfg SemanticConfig) error {
	return internal.ValidateSemanticConfig(cfg)
}

func NewService(root string, opts ServiceOptions) *Service {
	return internal.NewService(root, opts)
}

func EnsureWorkspace(root string) error { return internal.EnsureWorkspace(root) }

func WorkspaceBootstrapFileSpecs() []WorkspaceBootstrapFileSpec {
	return internal.WorkspaceBootstrapFileSpecs()
}

func WorkspaceBootstrapFileSpecFor(path string) (WorkspaceBootstrapFileSpec, bool) {
	return internal.WorkspaceBootstrapFileSpecFor(path)
}

func AppendDailyLog(root string, now time.Time, entry string) error {
	return internal.AppendDailyLog(root, now, entry)
}

func AppendMemoryNote(root string, now time.Time, entry string) error {
	return internal.AppendMemoryNote(root, now, entry)
}

func ListMemoryNotesByPrefix(root string, prefix string, limit int) ([]MemoryNote, error) {
	return internal.ListMemoryNotesByPrefix(root, prefix, limit)
}

func AppendExperience(root string, exp Experience) error {
	return internal.AppendExperience(root, exp)
}

func SearchExperiences(root string, opts SearchOptions) ([]Experience, error) {
	return internal.SearchExperiences(root, opts)
}

func AppendInboxCandidateIfNew(ctx context.Context, root string, backend Backend, candidate MemoryCandidate) (MemoryCandidate, bool, error) {
	return internal.AppendInboxCandidateIfNew(ctx, root, backend, candidate)
}

func ListMemoryCandidates(root string, opts MemoryCandidateListOptions) ([]MemoryCandidate, error) {
	return internal.ListMemoryCandidates(root, opts)
}

func ReviewMemoryCandidate(ctx context.Context, root string, backend Backend, id string, review MemoryCandidateReview) (MemoryCandidate, error) {
	return internal.ReviewMemoryCandidate(ctx, root, backend, id, review)
}
