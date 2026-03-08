package skill

import (
	"strings"
	"testing"
)

func TestFormatAvailableSkills(t *testing.T) {
	text := FormatAvailableSkills([]Definition{
		{
			Name:                    "deploy_check",
			Description:             "Validate deployment checklist",
			RuntimePath:             "_shared/skills_runtime/deploy_check/SKILL.md",
			UserInvocable:           true,
			RecommendedTools:        []string{"read_file", "write_file"},
			RecommendedProjectFiles: []string{"BRIEF.md", "STATE.md"},
			WakePhases:              []string{"plan", "draft"},
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
	if !strings.Contains(text, "<recommended_tools>") || !strings.Contains(text, "<item>read_file</item>") {
		t.Fatalf("expected recommended_tools metadata in xml, got %q", text)
	}
	if !strings.Contains(text, "<recommended_project_files>") || !strings.Contains(text, "<item>BRIEF.md</item>") {
		t.Fatalf("expected recommended_project_files metadata in xml, got %q", text)
	}
	if !strings.Contains(text, "<wake_phases>") || !strings.Contains(text, "<item>plan</item>") {
		t.Fatalf("expected wake_phases metadata in xml, got %q", text)
	}
}
