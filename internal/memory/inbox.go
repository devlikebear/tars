package memory

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

const (
	MemoryCandidateStatusPending  MemoryCandidateStatus = "pending"
	MemoryCandidateStatusApproved MemoryCandidateStatus = "approved"
	MemoryCandidateStatusRejected MemoryCandidateStatus = "rejected"
	MemoryCandidateStatusMerged   MemoryCandidateStatus = "merged"

	MemoryCandidateActionApprove MemoryCandidateAction = "approve"
	MemoryCandidateActionReject  MemoryCandidateAction = "reject"
	MemoryCandidateActionMerge   MemoryCandidateAction = "merge"
)

type MemoryCandidateStatus string

type MemoryCandidateAction string

type MemoryCandidate struct {
	ID            string                    `json:"id"`
	Status        MemoryCandidateStatus     `json:"status"`
	Category      string                    `json:"category"`
	Summary       string                    `json:"summary"`
	Tags          []string                  `json:"tags,omitempty"`
	SourceSession string                    `json:"source_session,omitempty"`
	Importance    int                       `json:"importance,omitempty"`
	Auto          bool                      `json:"auto,omitempty"`
	CreatedAt     time.Time                 `json:"created_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
	ReviewedAt    time.Time                 `json:"reviewed_at,omitempty"`
	MergedInto    string                    `json:"merged_into,omitempty"`
	Provenance    MemoryCandidateProvenance `json:"provenance"`
	Similar       []MemoryCandidateHint     `json:"similar,omitempty"`
	Conflicts     []MemoryCandidateHint     `json:"conflicts,omitempty"`
}

type MemoryCandidateProvenance struct {
	Source        string    `json:"source,omitempty"`
	SessionID     string    `json:"session_id,omitempty"`
	MessageRange  string    `json:"message_range,omitempty"`
	SourceSummary string    `json:"source_summary,omitempty"`
	ExtractedAt   time.Time `json:"extracted_at,omitempty"`
}

type MemoryCandidateHint struct {
	Kind          string  `json:"kind"`
	Category      string  `json:"category"`
	Summary       string  `json:"summary"`
	SourceSession string  `json:"source_session,omitempty"`
	Score         float64 `json:"score,omitempty"`
	Reason        string  `json:"reason,omitempty"`
}

type MemoryCandidateReview struct {
	Action      MemoryCandidateAction `json:"action"`
	MergeTarget string                `json:"merge_target,omitempty"`
	Note        string                `json:"note,omitempty"`
}

type MemoryCandidateListOptions struct {
	Status MemoryCandidateStatus
}

func AppendInboxCandidateIfNew(ctx context.Context, root string, backend Backend, candidate MemoryCandidate) (MemoryCandidate, bool, error) {
	if err := EnsureWorkspace(root); err != nil {
		return MemoryCandidate{}, false, err
	}
	candidate = normalizeMemoryCandidate(candidate, time.Now().UTC())
	if candidate.Summary == "" {
		return MemoryCandidate{}, false, fmt.Errorf("summary is required")
	}
	candidates, err := readMemoryCandidates(root)
	if err != nil {
		return MemoryCandidate{}, false, err
	}
	for _, existing := range candidates {
		if sameCandidate(existing, candidate) {
			return existing, false, nil
		}
	}
	candidate.Similar, candidate.Conflicts = buildCandidateHints(ctx, backendForInbox(root, backend), candidate)

	path := memoryInboxPath(root)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return MemoryCandidate{}, false, fmt.Errorf("open memory inbox: %w", err)
	}
	defer file.Close()

	encoded, err := json.Marshal(candidate)
	if err != nil {
		return MemoryCandidate{}, false, fmt.Errorf("marshal memory candidate: %w", err)
	}
	if _, err := file.WriteString(string(encoded) + "\n"); err != nil {
		return MemoryCandidate{}, false, fmt.Errorf("append memory candidate: %w", err)
	}
	return candidate, true, nil
}

func ListMemoryCandidates(root string, opts MemoryCandidateListOptions) ([]MemoryCandidate, error) {
	items, err := readMemoryCandidates(root)
	if err != nil {
		return nil, err
	}
	status := normalizeCandidateStatus(opts.Status)
	out := make([]MemoryCandidate, 0, len(items))
	for _, item := range items {
		if status != "" && item.Status != status {
			continue
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if out == nil {
		return []MemoryCandidate{}, nil
	}
	return out, nil
}

func ReviewMemoryCandidate(ctx context.Context, root string, backend Backend, id string, review MemoryCandidateReview) (MemoryCandidate, error) {
	if err := EnsureWorkspace(root); err != nil {
		return MemoryCandidate{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return MemoryCandidate{}, fmt.Errorf("candidate id is required")
	}
	action := normalizeCandidateAction(review.Action)
	if action == "" {
		return MemoryCandidate{}, fmt.Errorf("unknown memory candidate action: %s", review.Action)
	}

	candidates, err := readMemoryCandidates(root)
	if err != nil {
		return MemoryCandidate{}, err
	}
	now := time.Now().UTC()
	for i := range candidates {
		if candidates[i].ID != id {
			continue
		}
		candidate := normalizeMemoryCandidate(candidates[i], now)
		switch action {
		case MemoryCandidateActionApprove:
			if candidate.Status != MemoryCandidateStatusApproved {
				if err := appendCandidateExperienceIfNew(ctx, backendForInbox(root, backend), candidate); err != nil {
					return MemoryCandidate{}, err
				}
			}
			candidate.Status = MemoryCandidateStatusApproved
		case MemoryCandidateActionReject:
			candidate.Status = MemoryCandidateStatusRejected
		case MemoryCandidateActionMerge:
			candidate.Status = MemoryCandidateStatusMerged
			candidate.MergedInto = strings.TrimSpace(review.MergeTarget)
			if candidate.MergedInto == "" && len(candidate.Similar) > 0 {
				candidate.MergedInto = strings.TrimSpace(candidate.Similar[0].Summary)
			}
		}
		candidate.ReviewedAt = now
		candidate.UpdatedAt = now
		candidates[i] = candidate
		if err := writeMemoryCandidates(root, candidates); err != nil {
			return MemoryCandidate{}, err
		}
		return candidate, nil
	}
	return MemoryCandidate{}, fmt.Errorf("memory candidate not found: %s", id)
}

func memoryInboxPath(root string) string {
	return filepath.Join(root, "memory", "inbox.jsonl")
}

func readMemoryCandidates(root string) ([]MemoryCandidate, error) {
	path := memoryInboxPath(root)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []MemoryCandidate{}, nil
		}
		return nil, fmt.Errorf("open memory inbox: %w", err)
	}
	defer file.Close()

	var items []MemoryCandidate
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item MemoryCandidate
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		items = append(items, normalizeMemoryCandidate(item, time.Now().UTC()))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan memory inbox: %w", err)
	}
	if items == nil {
		return []MemoryCandidate{}, nil
	}
	return items, nil
}

func writeMemoryCandidates(root string, candidates []MemoryCandidate) error {
	var b strings.Builder
	for _, candidate := range candidates {
		encoded, err := json.Marshal(candidate)
		if err != nil {
			return fmt.Errorf("marshal memory candidate: %w", err)
		}
		b.WriteString(string(encoded))
		b.WriteByte('\n')
	}
	path := memoryInboxPath(root)
	if err := atomicwrite.Write(path, []byte(b.String())); err != nil {
		return err
	}
	_ = os.Chmod(path, 0o644)
	return nil
}

func normalizeMemoryCandidate(candidate MemoryCandidate, now time.Time) MemoryCandidate {
	candidate.Category = strings.TrimSpace(strings.ToLower(candidate.Category))
	if candidate.Category == "" {
		candidate.Category = "fact"
	}
	candidate.Summary = strings.TrimSpace(candidate.Summary)
	candidate.SourceSession = strings.TrimSpace(candidate.SourceSession)
	candidate.Tags = normalizeStringList(candidate.Tags)
	candidate.Importance = normalizeImportance(candidate.Importance)
	candidate.Status = normalizeCandidateStatus(candidate.Status)
	if candidate.Status == "" {
		candidate.Status = MemoryCandidateStatusPending
	}
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = now
	} else {
		candidate.CreatedAt = candidate.CreatedAt.UTC()
	}
	if candidate.UpdatedAt.IsZero() {
		candidate.UpdatedAt = candidate.CreatedAt
	} else {
		candidate.UpdatedAt = candidate.UpdatedAt.UTC()
	}
	if !candidate.ReviewedAt.IsZero() {
		candidate.ReviewedAt = candidate.ReviewedAt.UTC()
	}
	candidate.MergedInto = strings.TrimSpace(candidate.MergedInto)
	candidate.Provenance.Source = strings.TrimSpace(candidate.Provenance.Source)
	candidate.Provenance.SessionID = strings.TrimSpace(candidate.Provenance.SessionID)
	candidate.Provenance.MessageRange = strings.TrimSpace(candidate.Provenance.MessageRange)
	candidate.Provenance.SourceSummary = strings.TrimSpace(candidate.Provenance.SourceSummary)
	if !candidate.Provenance.ExtractedAt.IsZero() {
		candidate.Provenance.ExtractedAt = candidate.Provenance.ExtractedAt.UTC()
	}
	if candidate.SourceSession == "" {
		candidate.SourceSession = candidate.Provenance.SessionID
	}
	if candidate.ID == "" {
		candidate.ID = stableCandidateID(candidate)
	} else {
		candidate.ID = strings.TrimSpace(candidate.ID)
	}
	return candidate
}

func normalizeCandidateStatus(status MemoryCandidateStatus) MemoryCandidateStatus {
	switch MemoryCandidateStatus(strings.ToLower(strings.TrimSpace(string(status)))) {
	case MemoryCandidateStatusPending:
		return MemoryCandidateStatusPending
	case MemoryCandidateStatusApproved:
		return MemoryCandidateStatusApproved
	case MemoryCandidateStatusRejected:
		return MemoryCandidateStatusRejected
	case MemoryCandidateStatusMerged:
		return MemoryCandidateStatusMerged
	default:
		return ""
	}
}

func normalizeCandidateAction(action MemoryCandidateAction) MemoryCandidateAction {
	switch MemoryCandidateAction(strings.ToLower(strings.TrimSpace(string(action)))) {
	case MemoryCandidateActionApprove:
		return MemoryCandidateActionApprove
	case MemoryCandidateActionReject:
		return MemoryCandidateActionReject
	case MemoryCandidateActionMerge:
		return MemoryCandidateActionMerge
	default:
		return ""
	}
}

func stableCandidateID(candidate MemoryCandidate) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(candidate.Category)),
		strings.ToLower(strings.TrimSpace(candidate.Summary)),
		strings.TrimSpace(candidate.SourceSession),
		strings.TrimSpace(candidate.Provenance.SessionID),
		strings.TrimSpace(candidate.Provenance.MessageRange),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "cand_" + hex.EncodeToString(sum[:])[:16]
}

func sameCandidate(a, b MemoryCandidate) bool {
	if strings.TrimSpace(a.ID) != "" && strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(a.Category), strings.TrimSpace(b.Category)) &&
		strings.EqualFold(strings.TrimSpace(a.Summary), strings.TrimSpace(b.Summary)) &&
		strings.TrimSpace(a.SourceSession) == strings.TrimSpace(b.SourceSession)
}

func backendForInbox(root string, backend Backend) Backend {
	if backend != nil {
		return backend
	}
	return NewFileBackend(root, nil)
}

func appendCandidateExperienceIfNew(ctx context.Context, backend Backend, candidate MemoryCandidate) error {
	existing, err := backend.SearchExperiences(ctx, SearchOptions{
		Query: candidate.Summary,
		Limit: 12,
	})
	if err != nil {
		return fmt.Errorf("search existing experiences: %w", err)
	}
	for _, item := range existing {
		if strings.EqualFold(strings.TrimSpace(item.Category), strings.TrimSpace(candidate.Category)) &&
			strings.EqualFold(strings.TrimSpace(item.Summary), strings.TrimSpace(candidate.Summary)) {
			return nil
		}
	}
	return backend.AppendExperience(ctx, Experience{
		Timestamp:     candidate.CreatedAt,
		Category:      candidate.Category,
		Summary:       candidate.Summary,
		Tags:          candidate.Tags,
		SourceSession: candidate.SourceSession,
		Importance:    candidate.Importance,
		Auto:          candidate.Auto,
	})
}

func buildCandidateHints(ctx context.Context, backend Backend, candidate MemoryCandidate) ([]MemoryCandidateHint, []MemoryCandidateHint) {
	if backend == nil || strings.TrimSpace(candidate.Summary) == "" {
		return nil, nil
	}
	existing, err := backend.SearchExperiences(ctx, SearchOptions{
		Category: candidate.Category,
		Limit:    24,
	})
	if err != nil {
		return nil, nil
	}
	var similar []MemoryCandidateHint
	var conflicts []MemoryCandidateHint
	candidateTokens := tokenSet(candidate.Summary)
	for _, exp := range existing {
		exp = normalizeExperience(exp)
		if strings.TrimSpace(exp.Summary) == "" {
			continue
		}
		score := jaccard(candidateTokens, tokenSet(exp.Summary))
		hint := MemoryCandidateHint{
			Category:      exp.Category,
			Summary:       exp.Summary,
			SourceSession: exp.SourceSession,
			Score:         score,
		}
		switch {
		case score >= 0.45 || containsEither(candidate.Summary, exp.Summary):
			hint.Kind = "similar"
			hint.Reason = "same category with overlapping wording"
			similar = append(similar, hint)
		case looksLikePreferenceConflict(candidate.Summary, exp.Summary, score):
			hint.Kind = "conflict"
			hint.Reason = "preference wording appears to disagree"
			conflicts = append(conflicts, hint)
		}
	}
	sort.SliceStable(similar, func(i, j int) bool {
		return similar[i].Score > similar[j].Score
	})
	sort.SliceStable(conflicts, func(i, j int) bool {
		return conflicts[i].Score > conflicts[j].Score
	})
	if len(similar) > 5 {
		similar = similar[:5]
	}
	if len(conflicts) > 5 {
		conflicts = conflicts[:5]
	}
	return similar, conflicts
}

func tokenSet(s string) map[string]struct{} {
	out := map[string]struct{}{}
	var b strings.Builder
	flush := func() {
		token := strings.ToLower(strings.TrimSpace(b.String()))
		b.Reset()
		if len([]rune(token)) < 3 {
			return
		}
		out[token] = struct{}{}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	for token := range a {
		if _, ok := b[token]; ok {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func containsEither(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	return a != "" && b != "" && (strings.Contains(a, b) || strings.Contains(b, a))
}

func looksLikePreferenceConflict(a, b string, score float64) bool {
	if score >= 0.45 {
		return false
	}
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	if !strings.Contains(a, "prefer") && !strings.Contains(b, "prefer") &&
		!strings.Contains(a, "선호") && !strings.Contains(b, "선호") {
		return false
	}
	opposites := [][2]string{
		{"concise", "detailed"},
		{"short", "detailed"},
		{"brief", "detailed"},
		{"korean", "english"},
		{"한국어", "영어"},
	}
	for _, pair := range opposites {
		if (strings.Contains(a, pair[0]) && strings.Contains(b, pair[1])) ||
			(strings.Contains(a, pair[1]) && strings.Contains(b, pair[0])) {
			return true
		}
	}
	return false
}
