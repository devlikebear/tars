package prompt

type bootstrapSection struct {
	name     string
	files    []string
	subAgent bool
	maxChars int
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
