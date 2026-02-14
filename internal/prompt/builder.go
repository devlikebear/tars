package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

	return b.String()
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
