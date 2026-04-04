package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	DefaultSemanticLimit   = 6
	taskTypeRetrievalDoc   = "RETRIEVAL_DOCUMENT"
	taskTypeRetrievalQuery = "RETRIEVAL_QUERY"
)

var ErrSemanticUnavailable = errors.New("semantic memory is unavailable")

type embedderFactory func(SemanticConfig, *http.Client) Embedder

var embedderFactories = map[string]embedderFactory{
	"gemini": func(cfg SemanticConfig, client *http.Client) Embedder {
		return newGeminiEmbedder(cfg, client)
	},
}

type SemanticConfig struct {
	Enabled         bool
	EmbedProvider   string
	EmbedBaseURL    string
	EmbedAPIKey     string
	EmbedModel      string
	EmbedDimensions int
}

func (cfg SemanticConfig) normalized() SemanticConfig {
	cfg.EmbedProvider = NormalizeEmbedProvider(cfg.EmbedProvider)
	cfg.EmbedBaseURL = strings.TrimSpace(cfg.EmbedBaseURL)
	cfg.EmbedAPIKey = strings.TrimSpace(cfg.EmbedAPIKey)
	cfg.EmbedModel = strings.TrimSpace(cfg.EmbedModel)
	if cfg.EmbedDimensions <= 0 {
		cfg.EmbedDimensions = 768
	}
	return cfg
}

func (cfg SemanticConfig) ready() bool {
	cfg = cfg.normalized()
	return cfg.Enabled &&
		cfg.EmbedProvider != "" &&
		cfg.EmbedBaseURL != "" &&
		cfg.EmbedAPIKey != "" &&
		cfg.EmbedModel != "" &&
		cfg.EmbedDimensions > 0
}

func NormalizeEmbedProvider(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func SupportedEmbedProviders() []string {
	providers := make([]string, 0, len(embedderFactories))
	for provider := range embedderFactories {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	return providers
}

func IsSupportedEmbedProvider(raw string) bool {
	_, ok := embedderFactories[NormalizeEmbedProvider(raw)]
	return ok
}

func ValidateSemanticConfig(cfg SemanticConfig) error {
	cfg = cfg.normalized()
	if !cfg.Enabled {
		return nil
	}
	if cfg.EmbedProvider == "" {
		return fmt.Errorf("semantic memory enabled but memory_embed_provider is empty")
	}
	if !IsSupportedEmbedProvider(cfg.EmbedProvider) {
		return fmt.Errorf(
			"semantic memory provider %q is not supported; supported providers: %s",
			cfg.EmbedProvider,
			strings.Join(SupportedEmbedProviders(), ", "),
		)
	}
	switch {
	case cfg.EmbedBaseURL == "":
		return fmt.Errorf("semantic memory enabled but memory_embed_base_url is empty")
	case cfg.EmbedAPIKey == "":
		return fmt.Errorf("semantic memory enabled but memory_embed_api_key is empty")
	case cfg.EmbedModel == "":
		return fmt.Errorf("semantic memory enabled but memory_embed_model is empty")
	case cfg.EmbedDimensions <= 0:
		return fmt.Errorf("semantic memory enabled but memory_embed_dimensions must be greater than zero")
	default:
		return nil
	}
}

type EmbedRequest struct {
	Text             string
	TaskType         string
	Title            string
	OutputDimensions int
}

type Embedder interface {
	Embed(ctx context.Context, req EmbedRequest) ([]float64, error)
}

type Searcher interface {
	Search(ctx context.Context, req SearchRequest) ([]SearchHit, error)
}

type ServiceOptions struct {
	Config     SemanticConfig
	Embedder   Embedder
	HTTPClient *http.Client
	Now        func() time.Time
}

type Service struct {
	root     string
	config   SemanticConfig
	embedder Embedder
	nowFn    func() time.Time
}

type MemoryEntry struct {
	ID                  string    `json:"id"`
	Kind                string    `json:"kind"`
	Scope               string    `json:"scope"`
	Category            string    `json:"category,omitempty"`
	ProjectID           string    `json:"project_id,omitempty"`
	SessionID           string    `json:"session_id,omitempty"`
	SourcePath          string    `json:"source_path,omitempty"`
	ContentHash         string    `json:"content_hash"`
	Abstract            string    `json:"abstract,omitempty"`
	Overview            string    `json:"overview,omitempty"`
	Body                string    `json:"body,omitempty"`
	Tags                []string  `json:"tags,omitempty"`
	Importance          int       `json:"importance,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	EmbeddingModel      string    `json:"embedding_model,omitempty"`
	EmbeddingDimensions int       `json:"embedding_dimensions,omitempty"`
	Embedding           []float64 `json:"embedding,omitempty"`
}

type CompactionMemory struct {
	Category   string
	Summary    string
	Tags       []string
	Importance int
}

type SearchRequest struct {
	Query     string
	ProjectID string
	SessionID string
	Limit     int
}

type SearchHit struct {
	Entry   MemoryEntry
	Score   float64
	Source  string
	Snippet string
	Date    time.Time
}

type indexState struct {
	EmbeddingModel      string                      `json:"embedding_model,omitempty"`
	EmbeddingDimensions int                         `json:"embedding_dimensions,omitempty"`
	Sources             map[string]indexedSourceRef `json:"sources,omitempty"`
}

type indexedSourceRef struct {
	ContentHash string    `json:"content_hash"`
	EntryID     string    `json:"entry_id"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

type projectDocInput struct {
	SourcePath string
	ProjectID  string
	SessionID  string
	Body       string
	UpdatedAt  time.Time
}

func NewService(root string, opts ServiceOptions) *Service {
	cfg := opts.Config.normalized()
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	embedder := opts.Embedder
	if embedder == nil && cfg.ready() {
		if factory, ok := embedderFactories[cfg.EmbedProvider]; ok {
			embedder = factory(cfg, opts.HTTPClient)
		}
	}
	return &Service{
		root:     strings.TrimSpace(root),
		config:   cfg,
		embedder: embedder,
		nowFn:    nowFn,
	}
}

func (s *Service) Enabled() bool {
	return s != nil && s.config.ready() && s.embedder != nil && strings.TrimSpace(s.root) != ""
}

func (s *Service) LoadEntries() ([]MemoryEntry, error) {
	if s == nil {
		return []MemoryEntry{}, nil
	}
	return loadEntries(s.entriesPath())
}

func (s *Service) IndexExperience(ctx context.Context, exp Experience) error {
	if !s.Enabled() {
		return nil
	}
	exp = normalizeExperience(exp)
	now := exp.Timestamp
	if now.IsZero() {
		now = s.nowFn().UTC()
	}
	entry := MemoryEntry{
		ID:          "experience:" + hashText(strings.Join([]string{exp.ProjectID, exp.SourceSession, exp.Category, exp.Summary, now.Format(time.RFC3339Nano)}, "|")),
		Kind:        "explicit_memory",
		Scope:       deriveScope(exp.ProjectID, exp.SourceSession),
		Category:    exp.Category,
		ProjectID:   exp.ProjectID,
		SessionID:   exp.SourceSession,
		ContentHash: hashText(exp.Summary),
		Abstract:    exp.Summary,
		Overview:    summarizeText(exp.Summary, 320),
		Body:        exp.Summary,
		Tags:        exp.Tags,
		Importance:  exp.Importance,
		CreatedAt:   now.UTC(),
		UpdatedAt:   now.UTC(),
	}
	return s.upsertEmbeddedEntry(ctx, entry, taskTypeRetrievalDoc)
}

func (s *Service) IndexCompactionSummary(ctx context.Context, sessionID, summary string, createdAt time.Time) error {
	if !s.Enabled() {
		return nil
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	if createdAt.IsZero() {
		createdAt = s.nowFn().UTC()
	}
	entry := MemoryEntry{
		ID:          "compaction-summary:" + hashText(sessionID+"|"+summary),
		Kind:        "compaction_summary",
		Scope:       "session",
		SessionID:   strings.TrimSpace(sessionID),
		ContentHash: hashText(summary),
		Abstract:    summarizeText(summary, 220),
		Overview:    summarizeText(summary, 1200),
		Body:        summary,
		Importance:  7,
		CreatedAt:   createdAt.UTC(),
		UpdatedAt:   createdAt.UTC(),
	}
	return s.upsertEmbeddedEntry(ctx, entry, taskTypeRetrievalDoc)
}

func (s *Service) IndexCompactionMemories(ctx context.Context, sessionID string, items []CompactionMemory, createdAt time.Time) error {
	if !s.Enabled() {
		return nil
	}
	if createdAt.IsZero() {
		createdAt = s.nowFn().UTC()
	}
	for _, item := range items {
		summary := strings.TrimSpace(item.Summary)
		if summary == "" {
			continue
		}
		entry := MemoryEntry{
			ID:          "compaction-memory:" + hashText(sessionID+"|"+item.Category+"|"+summary),
			Kind:        "compaction_memory",
			Scope:       "session",
			Category:    strings.TrimSpace(strings.ToLower(item.Category)),
			SessionID:   strings.TrimSpace(sessionID),
			ContentHash: hashText(summary),
			Abstract:    summarizeText(summary, 220),
			Overview:    summarizeText(summary, 320),
			Body:        summary,
			Tags:        normalizeStringList(item.Tags),
			Importance:  normalizeImportance(item.Importance),
			CreatedAt:   createdAt.UTC(),
			UpdatedAt:   createdAt.UTC(),
		}
		if err := s.upsertEmbeddedEntry(ctx, entry, taskTypeRetrievalDoc); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) EnsureProjectDocuments(_ context.Context, _, _ string) error {
	// Project documents are no longer indexed after the project package was removed.
	return nil
}

func (s *Service) Search(ctx context.Context, req SearchRequest) ([]SearchHit, error) {
	if !s.Enabled() {
		return nil, ErrSemanticUnavailable
	}
	if err := s.EnsureProjectDocuments(ctx, req.ProjectID, req.SessionID); err != nil {
		return nil, err
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return []SearchHit{}, nil
	}
	vector, err := s.embedder.Embed(ctx, EmbedRequest{
		Text:             query,
		TaskType:         taskTypeRetrievalQuery,
		OutputDimensions: s.config.EmbedDimensions,
	})
	if err != nil {
		return nil, err
	}
	entries, err := s.LoadEntries()
	if err != nil {
		return nil, err
	}
	terms := normalizeSemanticTerms(query)
	hits := make([]SearchHit, 0, len(entries))
	for _, entry := range entries {
		if entry.EmbeddingModel != s.config.EmbedModel || entry.EmbeddingDimensions != s.config.EmbedDimensions {
			continue
		}
		if len(entry.Embedding) == 0 {
			continue
		}
		score := cosineSimilarity(vector, entry.Embedding)
		score += lexicalBoost(entry, terms)
		if sameNormalized(entry.ProjectID, req.ProjectID) && strings.TrimSpace(req.ProjectID) != "" {
			score += 0.18
		}
		if sameNormalized(entry.SessionID, req.SessionID) && strings.TrimSpace(req.SessionID) != "" {
			score += 0.12
		}
		score += recencyBoost(entry.UpdatedAt)
		score += float64(normalizeImportance(entry.Importance)) / 200.0
		if score <= 0 {
			continue
		}
		snippet := chooseSnippet(entry)
		if snippet == "" {
			continue
		}
		hits = append(hits, SearchHit{
			Entry:   entry,
			Score:   score,
			Source:  entrySource(entry),
			Snippet: snippet,
			Date:    entry.UpdatedAt,
		})
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			if hits[i].Date.Equal(hits[j].Date) {
				return hits[i].Source < hits[j].Source
			}
			return hits[i].Date.After(hits[j].Date)
		}
		return hits[i].Score > hits[j].Score
	})

	limit := req.Limit
	if limit <= 0 {
		limit = DefaultSemanticLimit
	}
	seen := map[string]struct{}{}
	filtered := make([]SearchHit, 0, min(limit, len(hits)))
	for _, hit := range hits {
		key := strings.ToLower(strings.TrimSpace(hit.Entry.ContentHash))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(hit.Entry.Abstract))
		}
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, hit)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered, nil
}

func (s *Service) upsertEmbeddedEntry(ctx context.Context, entry MemoryEntry, taskType string) error {
	if !s.Enabled() {
		return nil
	}
	payload := strings.TrimSpace(entry.Body)
	if payload == "" {
		payload = strings.TrimSpace(entry.Abstract)
	}
	if payload == "" {
		return nil
	}
	vector, err := s.embedder.Embed(ctx, EmbedRequest{
		Text:             payload,
		TaskType:         taskType,
		Title:            entry.SourcePath,
		OutputDimensions: s.config.EmbedDimensions,
	})
	if err != nil {
		return err
	}
	entry.EmbeddingModel = s.config.EmbedModel
	entry.EmbeddingDimensions = s.config.EmbedDimensions
	entry.Embedding = vector

	entries, err := s.LoadEntries()
	if err != nil {
		return err
	}
	updated := false
	for idx := range entries {
		if entries[idx].ID != entry.ID {
			continue
		}
		entries[idx] = entry
		updated = true
		break
	}
	if !updated {
		entries = append(entries, entry)
	}
	return saveEntries(s.entriesPath(), entries)
}

func (s *Service) entriesPath() string {
	return filepath.Join(s.root, "memory", "index", "entries.jsonl")
}

func (s *Service) statePath() string {
	return filepath.Join(s.root, "memory", "index", "state.json")
}

func loadEntries(path string) ([]MemoryEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []MemoryEntry{}, nil
		}
		return nil, fmt.Errorf("read semantic entries: %w", err)
	}
	lines := strings.Split(string(raw), "\n")
	out := make([]MemoryEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry MemoryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

func saveEntries(path string, entries []MemoryEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create semantic index dir: %w", err)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].UpdatedAt.Equal(entries[j].UpdatedAt) {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].UpdatedAt.After(entries[j].UpdatedAt)
	})
	var b strings.Builder
	for _, entry := range entries {
		encoded, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal semantic entry: %w", err)
		}
		b.Write(encoded)
		b.WriteByte('\n')
	}
	return writeAtomicFile(path, []byte(b.String()))
}

func loadIndexState(path string) (indexState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return indexState{}, nil
		}
		return indexState{}, fmt.Errorf("read semantic state: %w", err)
	}
	var state indexState
	if err := json.Unmarshal(raw, &state); err != nil {
		return indexState{}, fmt.Errorf("decode semantic state: %w", err)
	}
	return state, nil
}

func saveIndexState(path string, state indexState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create semantic state dir: %w", err)
	}
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode semantic state: %w", err)
	}
	return writeAtomicFile(path, append(encoded, '\n'))
}

func writeAtomicFile(path string, content []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}


func readDoc(path string) (string, os.FileInfo, bool) {
	stat, err := os.Stat(path)
	if err != nil {
		return "", nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil, false
	}
	body := strings.TrimSpace(string(raw))
	if body == "" {
		return "", nil, false
	}
	return body, stat, true
}

func normalizeSemanticTerms(query string) []string {
	replacer := strings.NewReplacer(
		".", " ", ",", " ", "?", " ", "!", " ", ":", " ", ";", " ",
		"(", " ", ")", " ", "\"", " ", "'", " ", "\n", " ", "\t", " ",
	)
	cleaned := strings.ToLower(strings.TrimSpace(replacer.Replace(query)))
	if cleaned == "" {
		return nil
	}
	stopwords := map[string]struct{}{
		"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "to": {}, "of": {}, "in": {}, "on": {},
		"what": {}, "do": {}, "i": {}, "you": {}, "me": {}, "my": {}, "about": {}, "is": {}, "are": {},
		"did": {}, "was": {}, "were": {}, "that": {}, "this": {}, "it": {}, "remember": {},
		"prefer": {}, "preference": {}, "like": {}, "likes": {},
	}
	seen := map[string]struct{}{}
	terms := make([]string, 0, 8)
	for _, part := range strings.Fields(cleaned) {
		if len(part) < 2 {
			continue
		}
		if _, skip := stopwords[part]; skip {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		terms = append(terms, part)
	}
	return terms
}

func lexicalBoost(entry MemoryEntry, terms []string) float64 {
	if len(terms) == 0 {
		return 0
	}
	text := strings.ToLower(strings.Join([]string{entry.Abstract, entry.Overview, entry.Body}, " "))
	if strings.TrimSpace(text) == "" {
		return 0
	}
	score := 0.0
	matches := 0
	for _, term := range terms {
		if strings.Contains(text, term) {
			score += 0.06
			matches++
		}
	}
	if matches == len(terms) && matches > 1 {
		score += 0.05
	}
	return score
}

func cosineSimilarity(left, right []float64) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot float64
	var leftNorm float64
	var rightNorm float64
	for idx := range left {
		dot += left[idx] * right[idx]
		leftNorm += left[idx] * left[idx]
		rightNorm += right[idx] * right[idx]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func recencyBoost(ts time.Time) float64 {
	if ts.IsZero() {
		return 0
	}
	age := time.Since(ts.UTC())
	switch {
	case age <= 48*time.Hour:
		return 0.05
	case age <= 7*24*time.Hour:
		return 0.03
	case age <= 30*24*time.Hour:
		return 0.015
	default:
		return 0
	}
}

func chooseSnippet(entry MemoryEntry) string {
	switch {
	case strings.TrimSpace(entry.Abstract) != "":
		return strings.TrimSpace(entry.Abstract)
	case strings.TrimSpace(entry.Overview) != "":
		return strings.TrimSpace(entry.Overview)
	default:
		return strings.TrimSpace(entry.Body)
	}
}

func entrySource(entry MemoryEntry) string {
	if strings.TrimSpace(entry.SourcePath) != "" {
		return strings.TrimSpace(entry.SourcePath)
	}
	switch entry.Kind {
	case "explicit_memory":
		source := "experience"
		if strings.TrimSpace(entry.Category) != "" {
			source += ":" + strings.TrimSpace(entry.Category)
		}
		return source
	case "compaction_summary", "compaction_memory":
		if strings.TrimSpace(entry.SessionID) != "" {
			return "session:" + strings.TrimSpace(entry.SessionID)
		}
	}
	return "memory"
}

func deriveScope(projectID, sessionID string) string {
	switch {
	case strings.TrimSpace(projectID) != "":
		return "project"
	case strings.TrimSpace(sessionID) != "":
		return "session"
	default:
		return "global"
	}
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func summarizeText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit > 0 && len(value) > limit {
		return strings.TrimSpace(value[:limit])
	}
	return value
}

func firstMeaningfulParagraph(value string, limit int) string {
	for _, block := range strings.Split(value, "\n\n") {
		candidate := strings.TrimSpace(block)
		if candidate == "" || strings.HasPrefix(candidate, "#") {
			continue
		}
		return summarizeText(candidate, limit)
	}
	return summarizeText(value, limit)
}

func sameNormalized(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
