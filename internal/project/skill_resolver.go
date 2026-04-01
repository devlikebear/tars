package project

// SkillContent holds the resolved name and body of a single skill.
type SkillContent struct {
	Name    string
	Content string
}

// SkillResolver resolves skill names into their content for injection into
// task prompts. Implementations live outside the project package to avoid
// importing the skill or extensions packages.
type SkillResolver interface {
	ResolveSkills(names []string) []SkillContent
}
