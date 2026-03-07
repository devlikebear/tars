package project

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestUpdatePayload_ToUpdateInput(t *testing.T) {
	var payload UpdatePayload
	raw := []byte(`{
		"project_id":" proj_demo ",
		"name":"Ops A",
		"type":"operations",
		"status":"paused",
		"git_repo":"https://example.com/acme/ops.git",
		"objective":"Keep service green",
		"instructions":"Check alerts first",
		"tools_allow":["read_file"," exec "],
		"tools_allow_groups":["memory"],
		"tools_allow_patterns":["^read"],
		"tools_deny":["write_file"],
		"tools_risk_max":"medium",
		"skills_allow":["deploy"],
		"mcp_servers":["filesystem"],
		"secrets_refs":["VAULT/prod/db"]
	}`)
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	got := payload.ToUpdateInput()
	want := UpdateInput{
		Name:               stringPtr("Ops A"),
		Type:               stringPtr("operations"),
		Status:             stringPtr("paused"),
		GitRepo:            stringPtr("https://example.com/acme/ops.git"),
		Objective:          stringPtr("Keep service green"),
		Instructions:       stringPtr("Check alerts first"),
		ToolsAllow:         []string{"read_file", " exec "},
		ToolsAllowGroups:   []string{"memory"},
		ToolsAllowPatterns: []string{"^read"},
		ToolsDeny:          []string{"write_file"},
		ToolsRiskMax:       stringPtr("medium"),
		SkillsAllow:        []string{"deploy"},
		MCPServers:         []string{"filesystem"},
		SecretsRefs:        []string{"VAULT/prod/db"},
	}
	if payload.ProjectID != " proj_demo " {
		t.Fatalf("expected project id to remain available for caller, got %q", payload.ProjectID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected update input:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestUpdatePayload_ToUpdateInput_PreservesOmittedOptionals(t *testing.T) {
	var payload UpdatePayload
	if err := json.Unmarshal([]byte(`{"project_id":"proj_demo"}`), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	got := payload.ToUpdateInput()

	if got.Name != nil || got.Type != nil || got.Status != nil || got.GitRepo != nil || got.Objective != nil || got.Instructions != nil || got.ToolsRiskMax != nil {
		t.Fatalf("expected omitted pointer fields to stay nil, got %+v", got)
	}
	if got.ToolsAllow != nil || got.ToolsAllowGroups != nil || got.ToolsAllowPatterns != nil || got.ToolsDeny != nil || got.SkillsAllow != nil || got.MCPServers != nil || got.SecretsRefs != nil {
		t.Fatalf("expected omitted slices to stay nil, got %+v", got)
	}
}
