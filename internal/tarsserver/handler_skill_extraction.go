package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/rs/zerolog"
)

type skillExtractionListResponse struct {
	Count      int                         `json:"count"`
	Candidates []skill.ExtractionCandidate `json:"candidates"`
}

type skillExtractionReviewResponse struct {
	Candidate skill.ExtractionCandidate `json:"candidate"`
	Draft     skillCreatorDraftResponse `json:"draft,omitempty"`
	Saved     skillCreatorSaveResponse  `json:"saved,omitempty"`
}

func newSkillExtractionAPIHandler(workspaceDir string, store *session.Store, router llm.Router, logger zerolog.Logger, provider extensionsProvider) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/admin/skills/extractions", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		status, ok := parseExtractionCandidateStatus(r.URL.Query().Get("status"))
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
			return
		}
		items, err := skill.ListExtractionCandidates(workspaceDir, skill.ExtractionCandidateListOptions{Status: status})
		if err != nil {
			logger.Error().Err(err).Msg("list skill extraction candidates failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list skill extraction candidates failed"})
			return
		}
		writeJSON(w, http.StatusOK, skillExtractionListResponse{Count: len(items), Candidates: items})
	})

	mux.HandleFunc("/v1/admin/skills/extractions/extract", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req struct {
			SessionID     string `json:"session_id"`
			MaxCandidates int    `json:"max_candidates"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		sess, messages, err := skillExtractionSession(store, req.SessionID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		candidates := skill.DetectExtractionCandidates(sess, messages, skill.ExtractionOptions{
			Now:           time.Now().UTC(),
			MaxCandidates: req.MaxCandidates,
		})
		if llmCandidates := detectSkillExtractionCandidatesWithLLM(r.Context(), router, sess, messages, req.MaxCandidates, logger); len(llmCandidates) > 0 {
			candidates = llmCandidates
		}
		items, _, err := skill.AppendExtractionCandidatesIfNew(workspaceDir, candidates)
		if err != nil {
			logger.Error().Err(err).Str("session_id", strings.TrimSpace(req.SessionID)).Msg("append skill extraction candidates failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "append skill extraction candidates failed"})
			return
		}
		writeJSON(w, http.StatusOK, skillExtractionListResponse{Count: len(items), Candidates: items})
	})

	mux.HandleFunc("/v1/admin/skills/extractions/review", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req struct {
			ID     string                          `json:"id"`
			Action skill.ExtractionCandidateAction `json:"action"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		action := skill.ExtractionCandidateAction(strings.ToLower(strings.TrimSpace(string(req.Action))))
		if action == skill.ExtractionCandidateActionApprove {
			candidate, draft, saved, err := approveSkillExtractionCandidate(workspaceDir, strings.TrimSpace(req.ID))
			if err != nil {
				logger.Error().Err(err).Str("candidate_id", strings.TrimSpace(req.ID)).Msg("approve skill extraction candidate failed")
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if provider != nil {
				if reloadErr := provider.Reload(r.Context()); reloadErr != nil {
					logger.Warn().Err(reloadErr).Str("skill", draft.Name).Msg("reload extensions after skill approval failed")
				}
			}
			writeJSON(w, http.StatusOK, skillExtractionReviewResponse{Candidate: candidate, Draft: draft, Saved: saved})
			return
		}
		candidate, err := skill.ReviewExtractionCandidate(workspaceDir, strings.TrimSpace(req.ID), skill.ExtractionCandidateReview{Action: action})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, skillExtractionReviewResponse{Candidate: candidate})
	})

	return mux
}

func skillExtractionSession(store *session.Store, sessionID string) (session.Session, []session.Message, error) {
	if store == nil {
		return session.Session{}, nil, fmt.Errorf("session store is not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return session.Session{}, nil, fmt.Errorf("session_id is required")
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		return session.Session{}, nil, err
	}
	messages, err := session.ReadMessages(store.TranscriptPath(sessionID))
	if err != nil {
		return session.Session{}, nil, err
	}
	return sess, messages, nil
}

func approveSkillExtractionCandidate(workspaceDir string, candidateID string) (skill.ExtractionCandidate, skillCreatorDraftResponse, skillCreatorSaveResponse, error) {
	candidate, err := skill.FindExtractionCandidate(workspaceDir, candidateID)
	if err != nil {
		return skill.ExtractionCandidate{}, skillCreatorDraftResponse{}, skillCreatorSaveResponse{}, err
	}
	name := uniqueSkillDraftName(workspaceDir, candidate.Name)
	draft, err := buildSkillCreatorDraft(skillCreatorDraftRequest{
		Name:             name,
		Description:      firstNonEmptyString(candidate.Summary, candidate.Trigger),
		Category:         "session",
		Language:         "shell",
		Layout:           "single_file",
		UseCase:          firstNonEmptyString(candidate.UseCase, candidate.Summary),
		RecommendedTools: candidate.RecommendedTools,
	})
	if err != nil {
		return skill.ExtractionCandidate{}, skillCreatorDraftResponse{}, skillCreatorSaveResponse{}, err
	}
	draft.Files = addExtractionProvenanceToSkillDraft(draft.Files, candidate)
	saved, err := saveSkillCreatorDraft(workspaceDir, draft)
	if err != nil {
		return skill.ExtractionCandidate{}, skillCreatorDraftResponse{}, skillCreatorSaveResponse{}, err
	}
	reviewed, err := skill.ReviewExtractionCandidate(workspaceDir, candidate.ID, skill.ExtractionCandidateReview{
		Action:    skill.ExtractionCandidateActionApprove,
		DraftPath: saved.Path,
		DraftName: draft.Name,
	})
	if err != nil {
		return skill.ExtractionCandidate{}, skillCreatorDraftResponse{}, skillCreatorSaveResponse{}, err
	}
	return reviewed, draft, saved, nil
}

func addExtractionProvenanceToSkillDraft(files []skillCreatorFile, _ skill.ExtractionCandidate) []skillCreatorFile {
	return append([]skillCreatorFile(nil), files...)
}

func uniqueSkillDraftName(workspaceDir string, requested string) string {
	base := strings.TrimSpace(requested)
	if err := validateSkillCreatorName(base); err != nil {
		base = "session-skill"
	}
	for i := 0; i < 100; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s-%d", base, i+1)
		}
		if _, err := os.Stat(filepath.Join(workspaceDir, "skills", name)); os.IsNotExist(err) {
			return name
		}
	}
	return base + "-draft"
}

func parseExtractionCandidateStatus(raw string) (skill.ExtractionCandidateStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(skill.ExtractionCandidateStatusPending):
		return skill.ExtractionCandidateStatusPending, true
	case "all":
		return "", true
	case string(skill.ExtractionCandidateStatusApproved):
		return skill.ExtractionCandidateStatusApproved, true
	case string(skill.ExtractionCandidateStatusRejected):
		return skill.ExtractionCandidateStatusRejected, true
	default:
		return "", false
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func detectSkillExtractionCandidatesWithLLM(ctx context.Context, router llm.Router, sess session.Session, messages []session.Message, maxCandidates int, logger zerolog.Logger) []skill.ExtractionCandidate {
	if router == nil || len(messages) == 0 {
		return nil
	}
	client, _, err := router.ClientForTier(llm.TierLight)
	if err != nil || client == nil {
		return nil
	}
	prompt := renderSkillExtractionPrompt(sess, messages, maxCandidates)
	runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	text, err := client.Ask(runCtx, prompt)
	if err != nil {
		logger.Debug().Err(err).Str("session_id", sess.ID).Msg("skill extraction llm fallback")
		return nil
	}
	parsed := parseLLMSkillExtractionCandidates(sess, messages, text, maxCandidates)
	return parsed
}

func renderSkillExtractionPrompt(sess session.Session, messages []session.Message, maxCandidates int) string {
	if maxCandidates <= 0 || maxCandidates > 5 {
		maxCandidates = 5
	}
	var b strings.Builder
	b.WriteString("Extract reusable TARS skill candidates from this chat transcript.\n")
	b.WriteString("Return JSON only: {\"candidates\":[{\"name\":\"kebab-case\",\"title\":\"Title\",\"trigger\":\"when to use it\",\"summary\":\"one line\",\"use_case\":\"what the skill does\",\"recommended_tools\":[\"bash\"],\"message_range\":\"first..last\"}]}\n")
	b.WriteString(fmt.Sprintf("Maximum candidates: %d\n", maxCandidates))
	b.WriteString("Session: " + strings.TrimSpace(sess.ID) + " " + strings.TrimSpace(sess.Title) + "\n\n")
	for i, msg := range messages {
		content := strings.Join(strings.Fields(msg.Content), " ")
		if content == "" {
			continue
		}
		if len([]rune(content)) > 500 {
			content = string([]rune(content)[:497]) + "..."
		}
		id := strings.TrimSpace(msg.ID)
		if id == "" {
			id = fmt.Sprintf("%d", i)
		}
		b.WriteString(fmt.Sprintf("[%s] %s: %s\n", id, msg.Role, content))
	}
	return b.String()
}

func parseLLMSkillExtractionCandidates(sess session.Session, messages []session.Message, raw string, maxCandidates int) []skill.ExtractionCandidate {
	if maxCandidates <= 0 || maxCandidates > 5 {
		maxCandidates = 5
	}
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil
	}
	var payload struct {
		Candidates []struct {
			Name             string   `json:"name"`
			Title            string   `json:"title"`
			Trigger          string   `json:"trigger"`
			Summary          string   `json:"summary"`
			UseCase          string   `json:"use_case"`
			RecommendedTools []string `json:"recommended_tools"`
			MessageRange     string   `json:"message_range"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &payload); err != nil {
		return nil
	}
	now := time.Now().UTC()
	out := make([]skill.ExtractionCandidate, 0, len(payload.Candidates))
	for _, item := range payload.Candidates {
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Summary) == "" {
			continue
		}
		evidence := skillExtractionEvidenceFromMessages(messages, item.MessageRange)
		messageRange := strings.TrimSpace(item.MessageRange)
		if messageRange == "" {
			messageRange = skillExtractionEvidenceRange(evidence)
		}
		repeatedCount := len(evidence)
		if repeatedCount == 0 {
			repeatedCount = 1
		}
		out = append(out, skill.ExtractionCandidate{
			Status:           skill.ExtractionCandidateStatusPending,
			Name:             item.Name,
			Title:            item.Title,
			Trigger:          item.Trigger,
			Summary:          item.Summary,
			UseCase:          item.UseCase,
			RecommendedTools: item.RecommendedTools,
			SourceSession:    strings.TrimSpace(sess.ID),
			MessageRange:     messageRange,
			RepeatedCount:    repeatedCount,
			Evidence:         evidence,
			CreatedAt:        now,
			UpdatedAt:        now,
			Provenance: skill.ExtractionProvenance{
				Source:        "llm_session",
				SessionID:     strings.TrimSpace(sess.ID),
				MessageRange:  messageRange,
				SourceSummary: strings.TrimSpace(sess.Title),
				ExtractedAt:   now,
			},
		})
		if len(out) >= maxCandidates {
			break
		}
	}
	return out
}

func skillExtractionEvidenceFromMessages(messages []session.Message, rangeHint string) []skill.ExtractionEvidence {
	start, end := skillExtractionMessageRangeIndexes(messages, rangeHint)
	out := make([]skill.ExtractionEvidence, 0)
	if start >= 0 && end >= start {
		for i := start; i <= end && i < len(messages); i++ {
			if evidence, ok := skillExtractionEvidenceForMessage(messages[i], i); ok {
				out = append(out, evidence)
			}
			if len(out) >= 6 {
				return out
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	for i, msg := range messages {
		if evidence, ok := skillExtractionEvidenceForMessage(msg, i); ok {
			out = append(out, evidence)
		}
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func skillExtractionEvidenceForMessage(msg session.Message, index int) (skill.ExtractionEvidence, bool) {
	content := strings.TrimSpace(msg.Content)
	if content == "" || strings.EqualFold(msg.Role, "system") {
		return skill.ExtractionEvidence{}, false
	}
	return skill.ExtractionEvidence{
		MessageID: strings.TrimSpace(msg.ID),
		Index:     index,
		Role:      strings.TrimSpace(msg.Role),
		Snippet:   compactSkillExtractionSnippet(content, 180),
	}, true
}

func skillExtractionMessageRangeIndexes(messages []session.Message, rangeHint string) (int, int) {
	rangeHint = strings.TrimSpace(rangeHint)
	if rangeHint == "" {
		return -1, -1
	}
	parts := strings.Split(rangeHint, "..")
	start := skillExtractionMessageIndex(messages, parts[0])
	end := start
	if len(parts) > 1 {
		end = skillExtractionMessageIndex(messages, parts[len(parts)-1])
	}
	if start < 0 || end < 0 {
		return -1, -1
	}
	if end < start {
		start, end = end, start
	}
	return start, end
}

func skillExtractionMessageIndex(messages []session.Message, token string) int {
	token = strings.TrimSpace(token)
	if token == "" {
		return -1
	}
	for i, msg := range messages {
		if strings.TrimSpace(msg.ID) == token {
			return i
		}
	}
	if n, err := strconv.Atoi(token); err == nil && n >= 0 && n < len(messages) {
		return n
	}
	return -1
}

func skillExtractionEvidenceRange(evidence []skill.ExtractionEvidence) string {
	if len(evidence) == 0 {
		return ""
	}
	start := extractionEvidenceRef(evidence[0])
	end := extractionEvidenceRef(evidence[len(evidence)-1])
	if start == end {
		return start
	}
	return start + ".." + end
}

func extractionEvidenceRef(evidence skill.ExtractionEvidence) string {
	if strings.TrimSpace(evidence.MessageID) != "" {
		return strings.TrimSpace(evidence.MessageID)
	}
	return fmt.Sprintf("%d", evidence.Index)
}

func compactSkillExtractionSnippet(content string, max int) string {
	content = strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if max <= 0 || len([]rune(content)) <= max {
		return content
	}
	runes := []rune(content)
	return strings.TrimSpace(string(runes[:max-3])) + "..."
}
