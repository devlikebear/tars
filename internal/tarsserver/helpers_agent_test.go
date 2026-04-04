package tarsserver

import "testing"

func TestNewToolRegistryForAgentProfile_CronIncludesExplicitAllowedTools(t *testing.T) {
	profile := agentPromptProfileForLabel("cron:job_demo")
	registry := newToolRegistryForAgentProfile(t.TempDir(), profile, []string{"exec"})
	if _, ok := registry.Get("exec"); !ok {
		t.Fatalf("expected cron registry to include explicitly allowed exec tool")
	}
	if _, ok := registry.Get("read_file"); !ok {
		t.Fatalf("expected cron registry to retain baseline cron tools")
	}
}
