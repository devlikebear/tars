package tool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/project"
)

func TestProjectCreateTool_UsesWorkflowInputs(t *testing.T) {
	store := project.NewStore(filepath.Join(t.TempDir(), "workspace"), nil)
	tl := NewProjectCreateTool(store)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{
		"name":"Ops A",
		"type":"operations",
		"objective":"Keep service green",
		"workflow_profile":"research",
		"workflow_rules":[
			{"name":"require_sources","params":{"count":"3"}}
		],
		"instructions":"Check alerts first"
	}`))
	if err != nil {
		t.Fatalf("execute project_create: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error result: %s", result.Text())
	}

	var created project.Project
	if err := json.Unmarshal([]byte(result.Text()), &created); err != nil {
		t.Fatalf("decode tool result: %v", err)
	}
	if created.WorkflowProfile != "research" {
		t.Fatalf("expected workflow_profile=research, got %q", created.WorkflowProfile)
	}
	if len(created.WorkflowRules) != 1 || created.WorkflowRules[0].Name != "require_sources" {
		t.Fatalf("unexpected workflow_rules: %+v", created.WorkflowRules)
	}
}

func TestProjectUpdateTool_UsesSharedUpdatePayload(t *testing.T) {
	store := project.NewStore(filepath.Join(t.TempDir(), "workspace"), nil)
	created, err := store.Create(project.CreateInput{Name: "Ops A", Type: "operations"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	tl := NewProjectUpdateTool(store)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{
		"project_id":"`+created.ID+`",
		"objective":"Keep service green",
		"instructions":"Check alerts first",
		"tools_allow":["read_file","exec"],
		"tools_risk_max":"medium",
		"workflow_profile":"research",
		"workflow_rules":[
			{"name":"require_sources","params":{"count":"3"}}
		]
	}`))
	if err != nil {
		t.Fatalf("execute project_update: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error result: %s", result.Text())
	}

	var updated project.Project
	if err := json.Unmarshal([]byte(result.Text()), &updated); err != nil {
		t.Fatalf("decode tool result: %v", err)
	}
	if updated.Objective != "Keep service green" {
		t.Fatalf("expected updated objective, got %q", updated.Objective)
	}
	if !strings.Contains(updated.Body, "Check alerts first") {
		t.Fatalf("expected updated instructions in body, got %q", updated.Body)
	}
	if got := strings.Join(updated.ToolsAllow, ","); got != "read_file,exec" {
		t.Fatalf("unexpected tools_allow: %q", got)
	}
	if updated.ToolsRiskMax != "medium" {
		t.Fatalf("expected tools_risk_max=medium, got %q", updated.ToolsRiskMax)
	}
	if updated.WorkflowProfile != "research" {
		t.Fatalf("expected workflow_profile=research, got %q", updated.WorkflowProfile)
	}
	if len(updated.WorkflowRules) != 1 || updated.WorkflowRules[0].Name != "require_sources" {
		t.Fatalf("unexpected workflow_rules: %+v", updated.WorkflowRules)
	}
}

func TestProjectUpdateTool_SchemaMatchesSharedUpdatePayload(t *testing.T) {
	tl := NewProjectUpdateTool(nil)

	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(tl.Parameters, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}

	want := map[string]struct{}{}
	payloadType := reflect.TypeOf(project.UpdatePayload{})
	for i := 0; i < payloadType.NumField(); i++ {
		field := payloadType.Field(i)
		tag := strings.TrimSpace(strings.Split(field.Tag.Get("json"), ",")[0])
		if tag == "" || tag == "-" {
			continue
		}
		want[tag] = struct{}{}
	}
	if len(schema.Properties) != len(want) {
		t.Fatalf("expected %d schema properties, got %d", len(want), len(schema.Properties))
	}
	for key := range want {
		if _, ok := schema.Properties[key]; !ok {
			t.Fatalf("expected schema to include property %q", key)
		}
	}
}
