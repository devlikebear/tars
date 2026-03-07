package prompt

type bootstrapSection struct {
	name      string
	files     []string
	subAgent  bool
	maxChars  int
}

const (
	projectSectionMaxChars   = 12000
	userSectionMaxChars      = 6000
	identitySectionMaxChars  = 8000
	heartbeatSectionMaxChars = 4000
	defaultSectionMaxChars   = 12000
)

var bootstrapSections = []bootstrapSection{
	{name: "Project", files: []string{"PROJECT.md"}, maxChars: projectSectionMaxChars},
	{name: "User", files: []string{"USER.md"}, maxChars: userSectionMaxChars},
	{name: "Identity", files: []string{"IDENTITY.md", "SOUL.md"}, maxChars: identitySectionMaxChars},
	{name: "Heartbeat", files: []string{"HEARTBEAT.md"}, maxChars: heartbeatSectionMaxChars},
	{name: "Agent Guidelines", files: []string{"AGENTS.md"}, subAgent: true, maxChars: defaultSectionMaxChars},
	{name: "Tools", files: []string{"TOOLS.md"}, subAgent: true, maxChars: defaultSectionMaxChars},
}
