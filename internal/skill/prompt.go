package skill

import (
	"strings"
)

func FormatAvailableSkills(skills []Definition) string {
	if len(skills) == 0 {
		return ""
	}
	sorted := append([]Definition(nil), skills...)
	sortSkillsByName(sorted)

	var b strings.Builder
	b.WriteString("<available_skills>\n")
	for _, skill := range sorted {
		b.WriteString("  <skill>\n")
		b.WriteString("    <name>")
		b.WriteString(escapeXML(skill.Name))
		b.WriteString("</name>\n")
		b.WriteString("    <description>")
		b.WriteString(escapeXML(strings.TrimSpace(skill.Description)))
		b.WriteString("</description>\n")
		b.WriteString("    <path>")
		b.WriteString(escapeXML(strings.TrimSpace(skill.RuntimePath)))
		b.WriteString("</path>\n")
		b.WriteString("    <user_invocable>")
		if skill.UserInvocable {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		b.WriteString("</user_invocable>\n")
		writeSkillPromptList(&b, "recommended_tools", skill.RecommendedTools)
		writeSkillPromptList(&b, "recommended_project_files", skill.RecommendedProjectFiles)
		writeSkillPromptList(&b, "wake_phases", skill.WakePhases)
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</available_skills>")
	return b.String()
}

func sortSkillsByName(skills []Definition) {
	for i := 0; i < len(skills)-1; i++ {
		for j := i + 1; j < len(skills); j++ {
			if strings.ToLower(skills[j].Name) < strings.ToLower(skills[i].Name) {
				skills[i], skills[j] = skills[j], skills[i]
			}
		}
	}
}

func escapeXML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}

func writeSkillPromptList(b *strings.Builder, key string, values []string) {
	if len(values) == 0 {
		return
	}
	b.WriteString("    <")
	b.WriteString(key)
	b.WriteString(">\n")
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		b.WriteString("      <item>")
		b.WriteString(escapeXML(trimmed))
		b.WriteString("</item>\n")
	}
	b.WriteString("    </")
	b.WriteString(key)
	b.WriteString(">\n")
}
