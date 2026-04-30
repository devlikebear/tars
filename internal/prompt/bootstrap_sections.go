package prompt

import "strings"

type bootstrapSection struct {
	name     string
	files    []string
	subAgent bool
	maxChars int
}

type FileImpact struct {
	Section         string `json:"section"`
	Role            string `json:"role"`
	MaxChars        int    `json:"max_chars"`
	Chars           int    `json:"chars"`
	EstimatedTokens int    `json:"estimated_tokens"`
	WillTruncate    bool   `json:"will_truncate"`
	TruncatedChars  int    `json:"truncated_chars,omitempty"`
}

const (
	userSectionMaxChars     = 6000
	identitySectionMaxChars = 8000
	defaultSectionMaxChars  = 12000
)

var bootstrapSections = []bootstrapSection{
	{name: "User", files: []string{"USER.md"}, maxChars: userSectionMaxChars},
	{name: "Identity", files: []string{"IDENTITY.md"}, maxChars: identitySectionMaxChars},
	{name: "Agent Guidelines", files: []string{"AGENTS.md"}, subAgent: true, maxChars: defaultSectionMaxChars},
	{name: "Tools", files: []string{"TOOLS.md"}, subAgent: true, maxChars: defaultSectionMaxChars},
}

func FileImpactFor(path string, content string) (FileImpact, bool) {
	trimmedPath := strings.TrimSpace(path)
	for _, section := range bootstrapSections {
		for _, file := range section.files {
			if file != trimmedPath {
				continue
			}
			trimmedContent := strings.TrimSpace(content)
			chars := len(trimmedContent)
			impact := FileImpact{
				Section:         section.name,
				Role:            promptFileRole(file),
				MaxChars:        section.maxChars,
				Chars:           chars,
				EstimatedTokens: estimateTokens(trimmedContent),
				WillTruncate:    section.maxChars > 0 && chars > section.maxChars,
			}
			if impact.WillTruncate {
				impact.TruncatedChars = chars - section.maxChars
			}
			return impact, true
		}
	}
	return FileImpact{}, false
}

func promptFileRole(path string) string {
	switch path {
	case "USER.md":
		return "User identity"
	case "IDENTITY.md":
		return "TARS persona"
	case "AGENTS.md":
		return "Sub-agent rules"
	case "TOOLS.md":
		return "Tool guidance"
	default:
		return "System prompt source"
	}
}
