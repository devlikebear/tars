package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/memory"
)

const maxFileChars = 20000

// BuildOptions configures system prompt generation.
type BuildOptions struct {
	WorkspaceDir string // path to workspace root
	SubAgent     bool   // if true, only inject AGENTS.md and TOOLS.md
}

// bootstrapFile defines a workspace file to inject into the system prompt.
type bootstrapFile struct {
	name     string // filename (e.g., "IDENTITY.md")
	section  string // section header in prompt
	subAgent bool   // if true, included in sub-agent mode
}

var mainFiles = []bootstrapFile{
	{name: "IDENTITY.md", section: "Identity", subAgent: false},
	{name: "SOUL.md", section: "Persona", subAgent: false},
	{name: "USER.md", section: "User", subAgent: false},
	{name: "PROJECT.md", section: "Project", subAgent: false},
	{name: "AGENTS.md", section: "Agent Guidelines", subAgent: true},
	{name: "TOOLS.md", section: "Tools", subAgent: true},
	{name: "HEARTBEAT.md", section: "Heartbeat", subAgent: false},
	{name: "MEMORY.md", section: "Memory", subAgent: false},
}

// Build assembles a system prompt by reading workspace bootstrap files.
func Build(opts BuildOptions) string {
	var b strings.Builder

	b.WriteString("You are TARS, a personal AI assistant.\n")
	b.WriteString(fmt.Sprintf("Current time: %s\n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString("\n")

	for _, f := range mainFiles {
		if opts.SubAgent && !f.subAgent {
			continue
		}

		content, err := readFileContent(filepath.Join(opts.WorkspaceDir, f.name))
		if err != nil || content == "" {
			continue
		}

		if len(content) > maxFileChars {
			content = content[:maxFileChars]
		}

		b.WriteString(fmt.Sprintf("## %s\n\n", f.section))
		b.WriteString(content)
		b.WriteString("\n\n")
	}
	if !opts.SubAgent {
		appendRecentExperiences(&b, opts.WorkspaceDir)
	}

	return b.String()
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
