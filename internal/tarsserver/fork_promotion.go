package tarsserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

type forkPromotionListResponse struct {
	Session    session.Session                  `json:"session"`
	Parent     session.Session                  `json:"parent"`
	Candidates []session.ForkPromotionCandidate `json:"candidates"`
	Count      int                              `json:"count"`
}

type forkPromotionRequest struct {
	CandidateIDs []string `json:"candidate_ids"`
}

type forkPromotionResult struct {
	PromotedCount int                      `json:"promoted_count"`
	SkippedCount  int                      `json:"skipped_count"`
	Candidates    []memory.MemoryCandidate `json:"candidates"`
}

func handleForkPromotions(w http.ResponseWriter, r *http.Request, store *session.Store, sessionID string, logger zerolog.Logger) {
	switch r.Method {
	case http.MethodGet:
		payload, err := loadForkPromotionCandidates(store, sessionID)
		if err != nil {
			writeForkPromotionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPost:
		var req forkPromotionRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		selectedIDs := normalizedIDSet(req.CandidateIDs)
		if len(selectedIDs) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "candidate_ids is required"})
			return
		}
		payload, err := loadForkPromotionCandidates(store, sessionID)
		if err != nil {
			writeForkPromotionError(w, err)
			return
		}
		result, err := promoteForkCandidates(r.Context(), store.WorkspaceDir(), payload.Parent, payload.Candidates, selectedIDs)
		if err != nil {
			logger.Error().Err(err).Str("session_id", sessionID).Msg("promote fork insights failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "promote fork insights failed"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		requireMethod(w, r, http.MethodGet, http.MethodPost)
	}
}

func loadForkPromotionCandidates(store *session.Store, sessionID string) (forkPromotionListResponse, error) {
	if store == nil {
		return forkPromotionListResponse{}, forkPromotionError{status: http.StatusInternalServerError, message: "session store is not configured"}
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		return forkPromotionListResponse{}, forkPromotionError{status: http.StatusNotFound, message: "session not found"}
	}
	if strings.TrimSpace(sess.ParentSessionID) == "" {
		return forkPromotionListResponse{}, forkPromotionError{status: http.StatusBadRequest, message: "session is not a fork"}
	}
	parent, err := store.Get(sess.ParentSessionID)
	if err != nil {
		return forkPromotionListResponse{}, forkPromotionError{status: http.StatusNotFound, message: "parent session not found"}
	}
	messages, err := session.ReadMessages(store.TranscriptPath(sess.ID))
	if err != nil {
		return forkPromotionListResponse{}, fmt.Errorf("read fork transcript: %w", err)
	}
	candidates := session.DetectForkPromotionCandidates(sess, messages, session.ForkPromotionOptions{})
	return forkPromotionListResponse{
		Session:    sess,
		Parent:     parent,
		Candidates: candidates,
		Count:      len(candidates),
	}, nil
}

func promoteForkCandidates(
	ctx context.Context,
	workspaceDir string,
	parent session.Session,
	candidates []session.ForkPromotionCandidate,
	selectedIDs map[string]struct{},
) (forkPromotionResult, error) {
	result := forkPromotionResult{Candidates: []memory.MemoryCandidate{}}
	for _, candidate := range candidates {
		if _, ok := selectedIDs[candidate.ID]; !ok {
			continue
		}
		memoryCandidate := memoryCandidateFromForkPromotion(parent, candidate)
		queued, added, err := memory.AppendInboxCandidateIfNew(ctx, workspaceDir, nil, memoryCandidate)
		if err != nil {
			return forkPromotionResult{}, err
		}
		if added {
			result.PromotedCount++
		} else {
			result.SkippedCount++
		}
		result.Candidates = append(result.Candidates, queued)
	}
	return result, nil
}

func memoryCandidateFromForkPromotion(parent session.Session, candidate session.ForkPromotionCandidate) memory.MemoryCandidate {
	return memory.MemoryCandidate{
		Category:      candidate.Category,
		Summary:       candidate.Summary,
		Tags:          []string{"fork", "promotion", "parent:" + parent.ID, "message:" + candidate.MessageID},
		SourceSession: candidate.SessionID,
		Importance:    3,
		Auto:          false,
		CreatedAt:     candidate.CreatedAt,
		UpdatedAt:     candidate.CreatedAt,
		Provenance: memory.MemoryCandidateProvenance{
			Source:        "fork_promotion",
			SessionID:     candidate.SessionID,
			MessageRange:  fmt.Sprintf("message:%s@%d", candidate.MessageID, candidate.MessageIndex),
			SourceSummary: fmt.Sprintf("fork %s -> parent %s from %s", candidate.SessionID, parent.ID, candidate.ForkedFromMessageID),
			ExtractedAt:   candidate.CreatedAt,
		},
	}
}

func normalizedIDSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[value] = struct{}{}
	}
	return out
}

type forkPromotionError struct {
	status  int
	message string
}

func (e forkPromotionError) Error() string {
	return e.message
}

func writeForkPromotionError(w http.ResponseWriter, err error) {
	if typed, ok := err.(forkPromotionError); ok {
		writeJSON(w, typed.status, map[string]string{"error": typed.message})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load fork promotion candidates failed"})
}
