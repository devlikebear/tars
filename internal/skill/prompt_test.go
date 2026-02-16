package skill

import (
	"strings"
	"testing"
)

func TestFormatAvailableSkills(t *testing.T) {
	text := FormatAvailableSkills([]Definition{
		{
			Name:          "deploy_check",
			Description:   "Validate deployment checklist",
			RuntimePath:   "_shared/skills_runtime/deploy_check/SKILL.md",
			UserInvocable: true,
		},
		{
			Name:          "silent_audit",
			Description:   "Internal audit helper",
			RuntimePath:   "_shared/skills_runtime/silent_audit/SKILL.md",
			UserInvocable: false,
		},
	})

	if !strings.Contains(text, "<available_skills>") {
		t.Fatalf("expected available_skills wrapper, got %q", text)
	}
	if !strings.Contains(text, "<name>deploy_check</name>") {
		t.Fatalf("expected skill name in xml, got %q", text)
	}
	if !strings.Contains(text, "<path>_shared/skills_runtime/deploy_check/SKILL.md</path>") {
		t.Fatalf("expected runtime path in xml, got %q", text)
	}
	if strings.Contains(text, "SKILL.md\n#") {
		t.Fatalf("skill body should not be embedded in available_skills")
	}
}
