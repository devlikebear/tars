package skill

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/devlikebear/tars/internal/atomicwrite"
	"github.com/devlikebear/tars/internal/session"
)

const (
	ExtractionCandidateStatusPending  ExtractionCandidateStatus = "pending"
	ExtractionCandidateStatusApproved ExtractionCandidateStatus = "approved"
	ExtractionCandidateStatusRejected ExtractionCandidateStatus = "rejected"

	ExtractionCandidateActionApprove ExtractionCandidateAction = "approve"
	ExtractionCandidateActionReject  ExtractionCandidateAction = "reject"
)

type ExtractionCandidateStatus string

type ExtractionCandidateAction string

type ExtractionCandidate struct {
	ID               string                    `json:"id"`
	Status           ExtractionCandidateStatus `json:"status"`
	Name             string                    `json:"name"`
	Title            string                    `json:"title"`
	Trigger          string                    `json:"trigger"`
	Summary          string                    `json:"summary"`
	UseCase          string                    `json:"use_case"`
	RecommendedTools []string                  `json:"recommended_tools,omitempty"`
	SourceSession    string                    `json:"source_session,omitempty"`
	MessageRange     string                    `json:"message_range,omitempty"`
	RepeatedCount    int                       `json:"repeated_count,omitempty"`
	Evidence         []ExtractionEvidence      `json:"evidence,omitempty"`
	Provenance       ExtractionProvenance      `json:"provenance"`
	CreatedAt        time.Time                 `json:"created_at"`
	UpdatedAt        time.Time                 `json:"updated_at"`
	ReviewedAt       time.Time                 `json:"reviewed_at,omitempty"`
	DraftPath        string                    `json:"draft_path,omitempty"`
	DraftName        string                    `json:"draft_name,omitempty"`
}

type ExtractionEvidence struct {
	MessageID string `json:"message_id,omitempty"`
	Index     int    `json:"index"`
	Role      string `json:"role"`
	Snippet   string `json:"snippet"`
}

type ExtractionProvenance struct {
	Source        string    `json:"source,omitempty"`
	SessionID     string    `json:"session_id,omitempty"`
	MessageRange  string    `json:"message_range,omitempty"`
	SourceSummary string    `json:"source_summary,omitempty"`
	ExtractedAt   time.Time `json:"extracted_at,omitempty"`
}

type ExtractionOptions struct {
	Now           time.Time
	MaxCandidates int
}

type ExtractionCandidateListOptions struct {
	Status ExtractionCandidateStatus
}

type ExtractionCandidateReview struct {
	Action    ExtractionCandidateAction `json:"action"`
	DraftPath string                    `json:"draft_path,omitempty"`
	DraftName string                    `json:"draft_name,omitempty"`
	Note      string                    `json:"note,omitempty"`
}

type extractionTopic struct {
	Name             string
	Title            string
	Trigger          string
	Summary          string
	UseCase          string
	RecommendedTools []string
	Keywords         []string
}

var extractionTopics = []extractionTopic{
	{
		Name:             "github-release-flow",
		Title:            "GitHub Release Flow",
		Trigger:          "Use when the user asks to ship an issue or PR through CI, merge, and release verification.",
		Summary:          "Capture the repeated GitHub PR, CI, squash merge, release, and Homebrew verification workflow.",
		UseCase:          "run the GitHub PR, CI, squash merge, and release verification flow",
		RecommendedTools: []string{"bash"},
		Keywords:         []string{"github", "pr", "pull request", "ci", "merge", "release", "homebrew", "issue"},
	},
	{
		Name:             "browser-smoke-check",
		Title:            "Browser Smoke Check",
		Trigger:          "Use when frontend work needs a real-browser smoke test with screenshots or console checks.",
		Summary:          "Capture the repeated browser validation workflow for local console changes.",
		UseCase:          "run a browser smoke test and report screenshots, console errors, and UI evidence",
		RecommendedTools: []string{"bash"},
		Keywords:         []string{"browser", "playwright", "screenshot", "console error", "localhost", "smoke"},
	},
	{
		Name:             "weekly-report-writer",
		Title:            "Weekly Report Writer",
		Trigger:          "Use when session history needs to become a weekly report or Notion summary.",
		Summary:          "Capture the repeated workflow for turning recent work into a concise weekly report.",
		UseCase:          "summarize recent work into a weekly report with evidence and next steps",
		RecommendedTools: []string{"bash", "memory_search"},
		Keywords:         []string{"weekly", "report", "notion", "summary", "recap"},
	},
	{
		Name:             "code-review-walkthrough",
		Title:            "Code Review Walkthrough",
		Trigger:          "Use when the user wants a grounded code review or architecture walkthrough from source.",
		Summary:          "Capture the repeated code review workflow with findings, file references, and verification.",
		UseCase:          "review code changes or walk through architecture with concrete file references",
		RecommendedTools: []string{"bash"},
		Keywords:         []string{"review", "code review", "architecture", "walkthrough", "finding", "검토", "리뷰"},
	},
}

var extractionSkillCuePattern = regexp.MustCompile(`(?i)\b(skill|workflow|repeatable|reusable|template|automation|자동화|반복|재사용|워크플로우|스킬)\b`)

func DetectExtractionCandidates(sess session.Session, messages []session.Message, opts ExtractionOptions) []ExtractionCandidate {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	limit := opts.MaxCandidates
	if limit <= 0 || limit > 5 {
		limit = 5
	}
	if len(messages) == 0 {
		return []ExtractionCandidate{}
	}

	candidates := make([]ExtractionCandidate, 0, limit)
	for _, topic := range extractionTopics {
		evidence := evidenceForTopic(messages, topic)
		if len(evidence) < 2 && !hasExplicitSkillCue(messages, topic) {
			continue
		}
		if len(evidence) == 0 {
			continue
		}
		candidate := normalizeExtractionCandidate(ExtractionCandidate{
			Status:           ExtractionCandidateStatusPending,
			Name:             topic.Name,
			Title:            topic.Title,
			Trigger:          topic.Trigger,
			Summary:          topic.Summary,
			UseCase:          topic.UseCase,
			RecommendedTools: topic.RecommendedTools,
			SourceSession:    strings.TrimSpace(sess.ID),
			MessageRange:     evidenceRange(evidence),
			RepeatedCount:    len(evidence),
			Evidence:         evidence,
			Provenance: ExtractionProvenance{
				Source:        "session",
				SessionID:     strings.TrimSpace(sess.ID),
				MessageRange:  evidenceRange(evidence),
				SourceSummary: strings.TrimSpace(sess.Title),
				ExtractedAt:   now,
			},
			CreatedAt: now,
			UpdatedAt: now,
		}, now)
		candidates = append(candidates, candidate)
		if len(candidates) >= limit {
			break
		}
	}
	if len(candidates) == 0 {
		if fallback, ok := fallbackExtractionCandidate(sess, messages, now); ok {
			candidates = append(candidates, fallback)
		}
	}
	return candidates
}

func AppendExtractionCandidatesIfNew(root string, candidates []ExtractionCandidate) ([]ExtractionCandidate, bool, error) {
	if err := os.MkdirAll(filepath.Dir(ExtractionInboxPath(root)), 0o755); err != nil {
		return nil, false, fmt.Errorf("create skill extraction inbox: %w", err)
	}
	existing, err := readExtractionCandidates(root)
	if err != nil {
		return nil, false, err
	}
	now := time.Now().UTC()
	added := make([]ExtractionCandidate, 0, len(candidates))
	changed := false
	for _, candidate := range candidates {
		candidate = normalizeExtractionCandidate(candidate, now)
		if candidate.Name == "" || candidate.Summary == "" {
			continue
		}
		if found, ok := matchingExtractionCandidate(existing, candidate); ok {
			added = append(added, found)
			continue
		}
		existing = append(existing, candidate)
		added = append(added, candidate)
		changed = true
	}
	if changed {
		if err := writeExtractionCandidates(root, existing); err != nil {
			return nil, false, err
		}
	}
	if added == nil {
		added = []ExtractionCandidate{}
	}
	return added, changed, nil
}

func ListExtractionCandidates(root string, opts ExtractionCandidateListOptions) ([]ExtractionCandidate, error) {
	items, err := readExtractionCandidates(root)
	if err != nil {
		return nil, err
	}
	status := normalizeExtractionStatus(opts.Status)
	out := make([]ExtractionCandidate, 0, len(items))
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
		return []ExtractionCandidate{}, nil
	}
	return out, nil
}

func FindExtractionCandidate(root string, id string) (ExtractionCandidate, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ExtractionCandidate{}, fmt.Errorf("candidate id is required")
	}
	items, err := readExtractionCandidates(root)
	if err != nil {
		return ExtractionCandidate{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return ExtractionCandidate{}, fmt.Errorf("skill extraction candidate not found: %s", id)
}

func ReviewExtractionCandidate(root string, id string, review ExtractionCandidateReview) (ExtractionCandidate, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ExtractionCandidate{}, fmt.Errorf("candidate id is required")
	}
	action := normalizeExtractionAction(review.Action)
	if action == "" {
		return ExtractionCandidate{}, fmt.Errorf("unknown skill extraction action: %s", review.Action)
	}
	items, err := readExtractionCandidates(root)
	if err != nil {
		return ExtractionCandidate{}, err
	}
	now := time.Now().UTC()
	for i := range items {
		if items[i].ID != id {
			continue
		}
		candidate := normalizeExtractionCandidate(items[i], now)
		switch action {
		case ExtractionCandidateActionApprove:
			candidate.Status = ExtractionCandidateStatusApproved
			candidate.DraftPath = strings.TrimSpace(review.DraftPath)
			candidate.DraftName = strings.TrimSpace(review.DraftName)
			if candidate.DraftName == "" {
				candidate.DraftName = candidate.Name
			}
		case ExtractionCandidateActionReject:
			candidate.Status = ExtractionCandidateStatusRejected
		}
		candidate.ReviewedAt = now
		candidate.UpdatedAt = now
		items[i] = candidate
		if err := writeExtractionCandidates(root, items); err != nil {
			return ExtractionCandidate{}, err
		}
		return candidate, nil
	}
	return ExtractionCandidate{}, fmt.Errorf("skill extraction candidate not found: %s", id)
}

func ExtractionInboxPath(root string) string {
	return filepath.Join(root, "_shared", "skill_extraction", "inbox.jsonl")
}

func readExtractionCandidates(root string) ([]ExtractionCandidate, error) {
	path := ExtractionInboxPath(root)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ExtractionCandidate{}, nil
		}
		return nil, fmt.Errorf("open skill extraction inbox: %w", err)
	}
	defer file.Close()
	var items []ExtractionCandidate
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item ExtractionCandidate
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		items = append(items, normalizeExtractionCandidate(item, time.Now().UTC()))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan skill extraction inbox: %w", err)
	}
	if items == nil {
		return []ExtractionCandidate{}, nil
	}
	return items, nil
}

func writeExtractionCandidates(root string, candidates []ExtractionCandidate) error {
	var b strings.Builder
	for _, candidate := range candidates {
		encoded, err := json.Marshal(candidate)
		if err != nil {
			return fmt.Errorf("marshal skill extraction candidate: %w", err)
		}
		b.WriteString(string(encoded))
		b.WriteByte('\n')
	}
	path := ExtractionInboxPath(root)
	if err := atomicwrite.Write(path, []byte(b.String())); err != nil {
		return err
	}
	_ = os.Chmod(path, 0o644)
	return nil
}

func evidenceForTopic(messages []session.Message, topic extractionTopic) []ExtractionEvidence {
	out := make([]ExtractionEvidence, 0)
	for i, msg := range messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" || strings.EqualFold(msg.Role, "system") {
			continue
		}
		score := 0
		lower := strings.ToLower(content)
		for _, keyword := range topic.Keywords {
			if strings.Contains(lower, strings.ToLower(keyword)) {
				score++
			}
		}
		if score == 0 {
			continue
		}
		out = append(out, ExtractionEvidence{
			MessageID: strings.TrimSpace(msg.ID),
			Index:     i,
			Role:      strings.TrimSpace(msg.Role),
			Snippet:   compactExtractionSnippet(content, 180),
		})
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func hasExplicitSkillCue(messages []session.Message, topic extractionTopic) bool {
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" || !extractionSkillCuePattern.MatchString(content) {
			continue
		}
		lower := strings.ToLower(content)
		for _, keyword := range topic.Keywords {
			if strings.Contains(lower, strings.ToLower(keyword)) {
				return true
			}
		}
	}
	return false
}

func fallbackExtractionCandidate(sess session.Session, messages []session.Message, now time.Time) (ExtractionCandidate, bool) {
	var evidence []ExtractionEvidence
	for i, msg := range messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" || strings.EqualFold(msg.Role, "system") {
			continue
		}
		if !extractionSkillCuePattern.MatchString(content) {
			continue
		}
		evidence = append(evidence, ExtractionEvidence{
			MessageID: strings.TrimSpace(msg.ID),
			Index:     i,
			Role:      strings.TrimSpace(msg.Role),
			Snippet:   compactExtractionSnippet(content, 180),
		})
	}
	if len(evidence) == 0 {
		return ExtractionCandidate{}, false
	}
	title := firstNonEmpty(strings.TrimSpace(sess.Title), "Session Workflow")
	name := slugFromText(title)
	if name == "" {
		name = "session-workflow"
	}
	candidate := normalizeExtractionCandidate(ExtractionCandidate{
		Status:           ExtractionCandidateStatusPending,
		Name:             name,
		Title:            title,
		Trigger:          "Use when this session's repeated workflow appears again.",
		Summary:          "Capture a reusable workflow inferred from this chat session.",
		UseCase:          "repeat the reusable workflow captured from this session",
		RecommendedTools: []string{"bash"},
		SourceSession:    strings.TrimSpace(sess.ID),
		MessageRange:     evidenceRange(evidence),
		RepeatedCount:    len(evidence),
		Evidence:         evidence,
		Provenance: ExtractionProvenance{
			Source:        "session",
			SessionID:     strings.TrimSpace(sess.ID),
			MessageRange:  evidenceRange(evidence),
			SourceSummary: strings.TrimSpace(sess.Title),
			ExtractedAt:   now,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}, now)
	return candidate, true
}

func normalizeExtractionCandidate(candidate ExtractionCandidate, now time.Time) ExtractionCandidate {
	candidate.Name = slugFromText(firstNonEmpty(candidate.Name, candidate.Title))
	candidate.Title = strings.TrimSpace(candidate.Title)
	if candidate.Title == "" {
		candidate.Title = titleFromSlug(candidate.Name)
	}
	candidate.Trigger = strings.TrimSpace(candidate.Trigger)
	candidate.Summary = strings.TrimSpace(candidate.Summary)
	candidate.UseCase = strings.TrimSpace(candidate.UseCase)
	if candidate.UseCase == "" {
		candidate.UseCase = candidate.Summary
	}
	candidate.RecommendedTools = normalizeExtractionStrings(candidate.RecommendedTools)
	if len(candidate.RecommendedTools) == 0 {
		candidate.RecommendedTools = []string{"bash"}
	}
	candidate.SourceSession = strings.TrimSpace(candidate.SourceSession)
	candidate.MessageRange = strings.TrimSpace(candidate.MessageRange)
	candidate.Status = normalizeExtractionStatus(candidate.Status)
	if candidate.Status == "" {
		candidate.Status = ExtractionCandidateStatusPending
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
	candidate.DraftPath = strings.TrimSpace(candidate.DraftPath)
	candidate.DraftName = slugFromText(candidate.DraftName)
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
	if candidate.MessageRange == "" {
		candidate.MessageRange = candidate.Provenance.MessageRange
	}
	candidate.ID = strings.TrimSpace(candidate.ID)
	if candidate.ID == "" {
		candidate.ID = stableExtractionCandidateID(candidate)
	}
	return candidate
}

func normalizeExtractionStatus(status ExtractionCandidateStatus) ExtractionCandidateStatus {
	switch ExtractionCandidateStatus(strings.ToLower(strings.TrimSpace(string(status)))) {
	case ExtractionCandidateStatusPending:
		return ExtractionCandidateStatusPending
	case ExtractionCandidateStatusApproved:
		return ExtractionCandidateStatusApproved
	case ExtractionCandidateStatusRejected:
		return ExtractionCandidateStatusRejected
	default:
		return ""
	}
}

func normalizeExtractionAction(action ExtractionCandidateAction) ExtractionCandidateAction {
	switch ExtractionCandidateAction(strings.ToLower(strings.TrimSpace(string(action)))) {
	case ExtractionCandidateActionApprove:
		return ExtractionCandidateActionApprove
	case ExtractionCandidateActionReject:
		return ExtractionCandidateActionReject
	default:
		return ""
	}
}

func matchingExtractionCandidate(existing []ExtractionCandidate, candidate ExtractionCandidate) (ExtractionCandidate, bool) {
	for _, item := range existing {
		if strings.TrimSpace(item.ID) != "" && item.ID == candidate.ID {
			return item, true
		}
		if strings.EqualFold(item.Name, candidate.Name) &&
			strings.TrimSpace(item.SourceSession) == strings.TrimSpace(candidate.SourceSession) &&
			strings.TrimSpace(item.MessageRange) == strings.TrimSpace(candidate.MessageRange) {
			return item, true
		}
	}
	return ExtractionCandidate{}, false
}

func stableExtractionCandidateID(candidate ExtractionCandidate) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(candidate.Name)),
		strings.TrimSpace(candidate.SourceSession),
		strings.TrimSpace(candidate.MessageRange),
		strings.ToLower(strings.TrimSpace(candidate.Summary)),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "skillcand_" + hex.EncodeToString(sum[:])[:16]
}

func evidenceRange(evidence []ExtractionEvidence) string {
	if len(evidence) == 0 {
		return ""
	}
	start := evidence[0].MessageID
	if start == "" {
		start = fmt.Sprintf("%d", evidence[0].Index)
	}
	end := evidence[len(evidence)-1].MessageID
	if end == "" {
		end = fmt.Sprintf("%d", evidence[len(evidence)-1].Index)
	}
	if start == end {
		return start
	}
	return start + ".." + end
}

func compactExtractionSnippet(content string, max int) string {
	content = strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if max <= 0 || len([]rune(content)) <= max {
		return content
	}
	runes := []rune(content)
	return strings.TrimSpace(string(runes[:max-3])) + "..."
}

func normalizeExtractionStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func slugFromText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 63 {
		slug = strings.Trim(slug[:63], "-")
	}
	return slug
}

func titleFromSlug(slug string) string {
	parts := strings.Split(strings.TrimSpace(slug), "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
