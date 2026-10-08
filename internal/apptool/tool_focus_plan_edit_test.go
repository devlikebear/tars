package apptool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/focuspipeline"
)

func TestFocusPlanEditTool(t *testing.T) {
	var got focuspipeline.PlanEdit
	changes := []string{"e2e − check the app"}
	var failWith error
	tool := NewFocusPlanEditTool(func(_ context.Context, edit focuspipeline.PlanEdit) (focuspipeline.Plan, []string, error) {
		got = edit
		return focuspipeline.Plan{Goal: "g", Verify: []string{"make test"}}, changes, failWith
	})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"e2e":[]}`))
	if err != nil || result.IsError {
		t.Fatalf("result = %+v err = %v", result, err)
	}
	if got.E2E == nil || len(*got.E2E) != 0 || got.Verify != nil || got.Goal != nil {
		t.Fatalf("edit = %+v: only the named field, and [] clears it", got)
	}
	if text := result.Text(); !strings.Contains(text, `"changed":true`) || !strings.Contains(text, "check the app") {
		t.Fatalf("text = %s", text)
	}

	changes = nil
	if result, _ := tool.Execute(context.Background(), json.RawMessage(`{"goal":"g"}`)); result.IsError || !strings.Contains(result.Text(), `"changed":false`) || !strings.Contains(result.Text(), `"changes":[]`) {
		t.Fatalf("no-op edit: %+v", result)
	}

	for name, params := range map[string]string{"no field": `{}`, "bad json": `{"e2e":"x"}`} {
		if result, _ := tool.Execute(context.Background(), json.RawMessage(params)); !result.IsError {
			t.Fatalf("%s must be an error", name)
		}
	}
	failWith = errors.New("no approved plan to edit")
	if result, _ := tool.Execute(context.Background(), json.RawMessage(`{"e2e":[]}`)); !result.IsError || !strings.Contains(result.Text(), "no approved plan") {
		t.Fatalf("editor error: %+v", result)
	}
	if result, _ := NewFocusPlanEditTool(nil).Execute(context.Background(), json.RawMessage(`{"e2e":[]}`)); !result.IsError {
		t.Fatal("a tool without an editor must be an error")
	}
}
