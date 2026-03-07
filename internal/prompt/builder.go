package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/memory"
)

// BuildOptions configures system prompt generation.
type BuildOptions struct {
	WorkspaceDir string // path to workspace root
	SubAgent     bool   // if true, only inject AGENTS.md and TOOLS.md
}

// Build assembles a system prompt by reading workspace bootstrap files.
func Build(opts BuildOptions) string {
	var b strings.Builder

	b.WriteString("You are TARS, a personal AI assistant.\n")
	b.WriteString(fmt.Sprintf("Current time: %s\n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString("\n")

	for _, section := range bootstrapSections {
		if opts.SubAgent && !section.subAgent {
			continue
		}
		if !opts.SubAgent && section.subAgent {
			continue
		}
		content := readBootstrapSection(opts.WorkspaceDir, section)
		if content == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s\n\n", section.name))
		b.WriteString(content)
		b.WriteString("\n\n")
	}
	if !opts.SubAgent {
		appendRecentExperiences(&b, opts.WorkspaceDir)
	}

	return b.String()
}

func readBootstrapSection(workspaceDir string, section bootstrapSection) string {
	parts := make([]string, 0, len(section.files))
	for _, name := range section.files {
		content, err := readFileContent(filepath.Join(workspaceDir, name))
		if err != nil || strings.TrimSpace(content) == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(content))
	}
	if len(parts) == 0 {
		return ""
	}
	joined := strings.Join(parts, "\n\n")
	if section.maxChars > 0 && len(joined) > section.maxChars {
		joined = joined[:section.maxChars]
	}
	return joined
}

func appendRecentExperiences(b *strings.Builder, workspaceDir string) {
	if b == nil {
		return
	}
	rows, err := memory.SearchExperiences(workspaceDir, memory.SearchOptions{Limit: 8})
	if err != nil || len(rows) == 0 {
		return
	}
	b.WriteString("## Recent Experiences\n\n")
	for _, row := range rows {
		category := strings.TrimSpace(row.Category)
		summary := strings.TrimSpace(row.Summary)
		if summary == "" {
			continue
		}
		if category == "" {
			category = "memory"
		}
		b.WriteString(fmt.Sprintf("- [%s] %s\n", category, summary))
	}
	b.WriteString("\n")
}

func readFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}
