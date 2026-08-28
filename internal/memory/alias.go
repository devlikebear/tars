// Package memory is a compatibility shim.
//
// The implementation moved to pkg/memory so that the public API owns its own
// types and renders on pkg.go.dev; see #928. This package aliases that
// surface so no in-repo call site had to change in the same commit as the
// move.
//
// New in-repo code should import github.com/devlikebear/tars/pkg/memory
// directly.
package memory

import pkgmemory "github.com/devlikebear/tars/pkg/memory"

type (
	Backend                    = pkgmemory.Backend
	CompactionMemory           = pkgmemory.CompactionMemory
	DurableKind                = pkgmemory.DurableKind
	EmbedRequest               = pkgmemory.EmbedRequest
	Embedder                   = pkgmemory.Embedder
	Experience                 = pkgmemory.Experience
	FileBackend                = pkgmemory.FileBackend
	MemoryCandidate            = pkgmemory.MemoryCandidate
	MemoryCandidateAction      = pkgmemory.MemoryCandidateAction
	MemoryCandidateHint        = pkgmemory.MemoryCandidateHint
	MemoryCandidateListOptions = pkgmemory.MemoryCandidateListOptions
	MemoryCandidateProvenance  = pkgmemory.MemoryCandidateProvenance
	MemoryCandidateReview      = pkgmemory.MemoryCandidateReview
	MemoryCandidateStatus      = pkgmemory.MemoryCandidateStatus
	MemoryEntry                = pkgmemory.MemoryEntry
	MemoryNote                 = pkgmemory.MemoryNote
	SearchHit                  = pkgmemory.SearchHit
	SearchOptions              = pkgmemory.SearchOptions
	SearchRequest              = pkgmemory.SearchRequest
	Searcher                   = pkgmemory.Searcher
	SemanticConfig             = pkgmemory.SemanticConfig
	Service                    = pkgmemory.Service
	ServiceOptions             = pkgmemory.ServiceOptions
	WorkspaceBootstrapFileSpec = pkgmemory.WorkspaceBootstrapFileSpec
)

const (
	DefaultSemanticLimit          = pkgmemory.DefaultSemanticLimit
	DurableKindDaily              = pkgmemory.DurableKindDaily
	DurableKindMemory             = pkgmemory.DurableKindMemory
	MemoryCandidateActionApprove  = pkgmemory.MemoryCandidateActionApprove
	MemoryCandidateActionMerge    = pkgmemory.MemoryCandidateActionMerge
	MemoryCandidateActionReject   = pkgmemory.MemoryCandidateActionReject
	MemoryCandidateStatusApproved = pkgmemory.MemoryCandidateStatusApproved
	MemoryCandidateStatusMerged   = pkgmemory.MemoryCandidateStatusMerged
	MemoryCandidateStatusPending  = pkgmemory.MemoryCandidateStatusPending
	MemoryCandidateStatusRejected = pkgmemory.MemoryCandidateStatusRejected
)

var (
	AppendDailyLog                = pkgmemory.AppendDailyLog
	AppendExperience              = pkgmemory.AppendExperience
	AppendInboxCandidateIfNew     = pkgmemory.AppendInboxCandidateIfNew
	AppendMemoryNote              = pkgmemory.AppendMemoryNote
	EnsureWorkspace               = pkgmemory.EnsureWorkspace
	ErrSemanticUnavailable        = pkgmemory.ErrSemanticUnavailable
	IsSupportedEmbedProvider      = pkgmemory.IsSupportedEmbedProvider
	ListMemoryCandidates          = pkgmemory.ListMemoryCandidates
	ListMemoryNotesByPrefix       = pkgmemory.ListMemoryNotesByPrefix
	NewFileBackend                = pkgmemory.NewFileBackend
	NewService                    = pkgmemory.NewService
	NormalizeEmbedProvider        = pkgmemory.NormalizeEmbedProvider
	ReviewMemoryCandidate         = pkgmemory.ReviewMemoryCandidate
	SearchExperiences             = pkgmemory.SearchExperiences
	SupportedEmbedProviders       = pkgmemory.SupportedEmbedProviders
	ValidateSemanticConfig        = pkgmemory.ValidateSemanticConfig
	WorkspaceBootstrapFileSpecFor = pkgmemory.WorkspaceBootstrapFileSpecFor
	WorkspaceBootstrapFileSpecs   = pkgmemory.WorkspaceBootstrapFileSpecs
)
