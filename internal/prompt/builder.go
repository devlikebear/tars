package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BuildOptions configures system prompt generation.
type BuildOptions struct {
	WorkspaceDir          string // path to workspace root
	SubAgent              bool   // if true, only inject AGENTS.md and TOOLS.md
	Query                 string
	ProjectID             string
	SessionID             string
	ForceRelevantMemory   bool
	StaticBudgetTokens    int
	RelevantBudgetTokens  int
}

// Build assembles a system prompt by reading workspace bootstrap files.
func Build(opts BuildOptions) string {
	var b strings.Builder

	b.WriteString("You are TARS, a personal AI assistant.\n")
	b.WriteString(fmt.Sprintf("Current time: %s\n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString("\n")

	remainingStaticTokens := opts.StaticBudgetTokens
	if remainingStaticTokens <= 0 {
		remainingStaticTokens = defaultStaticBudgetTokens
	}
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
		content = trimToBudget(content, section.maxChars, max(0, remainingStaticTokens-sectionHeaderTokenCost))
		if content == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s\n\n", section.name))
		b.WriteString(content)
		b.WriteString("\n\n")
		remainingStaticTokens -= estimateTokens(content) + sectionHeaderTokenCost
		if remainingStaticTokens <= 0 {
			break
		}
	}
	if !opts.SubAgent {
		appendRelevantMemory(&b, opts)
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

func trimToBudget(content string, maxChars int, maxTokens int) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	if maxChars > 0 && len(trimmed) > maxChars {
		trimmed = trimmed[:maxChars]
	}
	if maxTokens > 0 {
		maxCharsByTokens := maxTokens * 4
		if maxCharsByTokens <= 0 {
			return ""
		}
		if len(trimmed) > maxCharsByTokens {
			trimmed = trimmed[:maxCharsByTokens]
		}
	}
	return strings.TrimSpace(trimmed)
}

func estimateTokens(content string) int {
	if strings.TrimSpace(content) == "" {
		return 0
	}
	tokens := len(content) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
