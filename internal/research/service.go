package research

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Options struct {
	Now func() time.Time
}

type RunInput struct {
	Topic   string
	Summary string
	Body    string
}

type Report struct {
	Topic     string    `json:"topic"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

type Service struct {
	workspace string
	nowFn     func() time.Time
}

func NewService(workspaceDir string, opts Options) *Service {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	root := strings.TrimSpace(workspaceDir)
	if root == "" {
		root = "./workspace"
	}
	return &Service{workspace: root, nowFn: nowFn}
}

func (s *Service) Run(input RunInput) (Report, error) {
	if s == nil {
		return Report{}, fmt.Errorf("research service is nil")
	}
	topic := strings.TrimSpace(input.Topic)
	if topic == "" {
		return Report{}, fmt.Errorf("topic is required")
	}
	now := s.nowFn().UTC()
	reportsDir := filepath.Join(s.workspace, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return Report{}, err
	}
	slug := slugify(topic)
	name := fmt.Sprintf("%s-%s.md", now.Format("20060102-1504"), slug)
	reportPath := filepath.Join(reportsDir, name)
	body := strings.TrimSpace(input.Body)
	if body == "" {
		body = "- research findings pending"
	}
	content := buildReportMarkdown(now, topic, strings.TrimSpace(input.Summary), body)
	if err := os.WriteFile(reportPath, []byte(content), 0o644); err != nil {
		return Report{}, err
	}
	report := Report{Topic: topic, Path: reportPath, CreatedAt: now}
	if err := appendSummary(filepath.Join(reportsDir, "summary.jsonl"), report, strings.TrimSpace(input.Summary)); err != nil {
		return Report{}, err
	}
	return report, nil
}

func buildReportMarkdown(now time.Time, topic, summary, body string) string {
	var b strings.Builder
	b.WriteString("# Research Report\n\n")
	b.WriteString("- created_at: " + now.UTC().Format(time.RFC3339) + "\n")
	b.WriteString("- topic: " + strings.TrimSpace(topic) + "\n")
	if strings.TrimSpace(summary) != "" {
		b.WriteString("- summary: " + strings.TrimSpace(summary) + "\n")
	}
	b.WriteString("\n## Findings\n\n")
	b.WriteString(strings.TrimSpace(body))
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func appendSummary(path string, report Report, summary string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	payload := map[string]any{
		"timestamp": report.CreatedAt.Format(time.RFC3339),
		"topic":     report.Topic,
		"path":      report.Path,
		"summary":   summary,
	}
	line, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

func slugify(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return "report"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range v {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "report"
	}
	return out
}
