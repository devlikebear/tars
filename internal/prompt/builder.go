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
	TotalBudgetTokens     int
}

// BuildResult captures prompt assembly output and budget usage.
type BuildResult struct {
	Prompt              string
	StaticTokens        int
	RelevantTokens      int
	RelevantMemoryCount int
	TotalTokens         int
}

// Build assembles a system prompt by reading workspace bootstrap files.
func Build(opts BuildOptions) string {
	return BuildResultFor(opts).Prompt
}

// BuildResultFor assembles a system prompt and returns budget usage details.
func BuildResultFor(opts BuildOptions) BuildResult {
	var b strings.Builder

	b.WriteString("You are TARS, a personal AI assistant.\n")
	b.WriteString(fmt.Sprintf("Current time: %s\n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString("\n")

	totalBudgetTokens := opts.TotalBudgetTokens
	if totalBudgetTokens <= 0 {
		totalBudgetTokens = defaultTotalBudgetTokens
	}
	totalTokens := estimateTokens(b.String())
	remainingTotalTokens := max(0, totalBudgetTokens-totalTokens)

	remainingStaticTokens := opts.StaticBudgetTokens
	if remainingStaticTokens <= 0 {
		remainingStaticTokens = defaultStaticBudgetTokens
	}
	staticTokens := 0
	for _, section := range bootstrapSections {
		if opts.SubAgent && !section.subAgent {
			continue
		}
		if !opts.SubAgent && section.subAgent {
			continue
		}
		if remainingStaticTokens <= 0 || remainingTotalTokens <= 0 {
			break
		}
		content := readBootstrapSection(opts.WorkspaceDir, section)
		if content == "" {
			continue
		}
		content = trimToBudget(content, section.maxChars, max(0, min(remainingStaticTokens, remainingTotalTokens)-sectionHeaderTokenCost))
		if content == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s\n\n", section.name))
		b.WriteString(content)
		b.WriteString("\n\n")
		sectionTokens := estimateTokens(content) + sectionHeaderTokenCost
		staticTokens += sectionTokens
		totalTokens += sectionTokens
		remainingStaticTokens -= sectionTokens
		remainingTotalTokens -= sectionTokens
	}
	relevantTokens := 0
	relevantCount := 0
	if !opts.SubAgent {
		relevantBudgetTokens := opts.RelevantBudgetTokens
		if relevantBudgetTokens <= 0 {
			relevantBudgetTokens = defaultRelevantBudgetTokens
		}
		relevantBudgetTokens = min(relevantBudgetTokens, remainingTotalTokens)
		section, count, usedTokens := buildRelevantMemorySection(opts, relevantBudgetTokens)
		if section != "" {
			b.WriteString(section)
			relevantTokens = usedTokens
			relevantCount = count
			totalTokens += usedTokens
		}
	}

	return BuildResult{
		Prompt:              b.String(),
		StaticTokens:        staticTokens,
		RelevantTokens:      relevantTokens,
		RelevantMemoryCount: relevantCount,
		TotalTokens:         totalTokens,
	}
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

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
