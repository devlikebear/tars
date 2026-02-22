package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultExperienceLimit = 8
	maxExperienceLimit     = 100
)

type Experience struct {
	Timestamp     time.Time `json:"timestamp"`
	Category      string    `json:"category"`
	Summary       string    `json:"summary"`
	Tags          []string  `json:"tags,omitempty"`
	SourceSession string    `json:"source_session,omitempty"`
	ProjectID     string    `json:"project_id,omitempty"`
	Importance    int       `json:"importance,omitempty"`
	Auto          bool      `json:"auto,omitempty"`
}

type SearchOptions struct {
	Query     string
	Category  string
	ProjectID string
	Limit     int
}

func AppendExperience(root string, exp Experience) error {
	if err := EnsureWorkspace(root); err != nil {
		return err
	}
	exp.Category = strings.TrimSpace(strings.ToLower(exp.Category))
	if exp.Category == "" {
		exp.Category = "fact"
	}
	exp.Summary = strings.TrimSpace(exp.Summary)
	if exp.Summary == "" {
		return fmt.Errorf("summary is required")
	}
	exp.SourceSession = strings.TrimSpace(exp.SourceSession)
	exp.ProjectID = strings.TrimSpace(exp.ProjectID)
	exp.Importance = normalizeImportance(exp.Importance)
	exp.Tags = normalizeStringList(exp.Tags)
	if exp.Timestamp.IsZero() {
		exp.Timestamp = time.Now().UTC()
	} else {
		exp.Timestamp = exp.Timestamp.UTC()
	}

	path := filepath.Join(root, "memory", "experiences.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open experiences log: %w", err)
	}
	defer file.Close()

	encoded, err := json.Marshal(exp)
	if err != nil {
		return fmt.Errorf("marshal experience: %w", err)
	}
	if _, err := file.WriteString(string(encoded) + "\n"); err != nil {
		return fmt.Errorf("append experience: %w", err)
	}
	return nil
}

func SearchExperiences(root string, opts SearchOptions) ([]Experience, error) {
	path := filepath.Join(root, "memory", "experiences.jsonl")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Experience{}, nil
		}
		return nil, fmt.Errorf("open experiences log: %w", err)
	}
	defer file.Close()

	query := strings.ToLower(strings.TrimSpace(opts.Query))
	category := strings.ToLower(strings.TrimSpace(opts.Category))
	projectID := strings.TrimSpace(opts.ProjectID)
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultExperienceLimit
	}
	if limit > maxExperienceLimit {
		limit = maxExperienceLimit
	}

	rows := make([]Experience, 0, limit)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item Experience
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		if !matchesExperience(item, query, category, projectID) {
			continue
		}
		rows = append(rows, normalizeExperience(item))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan experiences log: %w", err)
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Timestamp.After(rows[j].Timestamp)
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	if rows == nil {
		return []Experience{}, nil
	}
	return rows, nil
}

func normalizeExperience(exp Experience) Experience {
	exp.Timestamp = exp.Timestamp.UTC()
	exp.Category = strings.TrimSpace(strings.ToLower(exp.Category))
	exp.Summary = strings.TrimSpace(exp.Summary)
	exp.SourceSession = strings.TrimSpace(exp.SourceSession)
	exp.ProjectID = strings.TrimSpace(exp.ProjectID)
	exp.Tags = normalizeStringList(exp.Tags)
	exp.Importance = normalizeImportance(exp.Importance)
	if exp.Category == "" {
		exp.Category = "fact"
	}
	return exp
}

func normalizeImportance(v int) int {
	if v <= 0 {
		return 5
	}
	if v > 10 {
		return 10
	}
	return v
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func matchesExperience(item Experience, query, category, projectID string) bool {
	if category != "" && strings.ToLower(strings.TrimSpace(item.Category)) != category {
		return false
	}
	if projectID != "" && strings.TrimSpace(item.ProjectID) != projectID {
		return false
	}
	if query == "" {
		return true
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(item.Summary)), query) {
		return true
	}
	for _, tag := range item.Tags {
		if strings.Contains(strings.ToLower(strings.TrimSpace(tag)), query) {
			return true
		}
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(item.SourceSession)), query) {
		return true
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(item.ProjectID)), query) {
		return true
	}
	return false
}
