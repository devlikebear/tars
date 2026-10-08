package apptool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devlikebear/tars/internal/focuspipeline"
)

// FocusPlanEditor edits the approved plan of the turn's focus pipeline
// and returns the plan after the edit and what changed.
type FocusPlanEditor func(ctx context.Context, edit focuspipeline.PlanEdit) (focuspipeline.Plan, []string, error)

// NewFocusPlanEditTool lets a focus-mode turn change the plan the developer
// approved — its goal and its check lists — when the developer asks. The
// plan gate (G1) is otherwise the only place a plan changes, so a check
// that cannot pass in this environment kept a loop going to its limits.
func NewFocusPlanEditTool(edit FocusPlanEditor) Tool {
	return Tool{
		Name: "focus_plan_edit",
		Description: "Focus mode only: change this session's approved plan when the developer asks for it in the conversation — " +
			"the goal, the verification commands (verify, shell), the end-to-end goals (e2e, plain language for computer_use), " +
			"and the shell commands run before and after the end-to-end goals (e2e_setup builds and launches the changed app from the working folder, e2e_teardown stops it). " +
			"Pass only the fields that change; a list replaces the whole list and [] clears it. " +
			"Tasks, stages and limits cannot be changed. Never use it on your own to get past a failing check.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "goal":{"type":"string"},
    "verify":{"type":"array","items":{"type":"string"}},
    "e2e":{"type":"array","items":{"type":"string"}},
    "e2e_setup":{"type":"array","items":{"type":"string"}},
    "e2e_teardown":{"type":"array","items":{"type":"string"}}
  },
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if edit == nil {
				return JSONTextResult(map[string]any{"message": "focus_plan_edit needs a focus-mode session"}, true), nil
			}
			var input focuspipeline.PlanEdit
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			if input.Empty() {
				return JSONTextResult(map[string]any{"message": "name at least one of goal, verify, e2e, e2e_setup, e2e_teardown"}, true), nil
			}
			plan, changes, err := edit(ctx, input)
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			if changes == nil {
				changes = []string{}
			}
			return JSONTextResult(map[string]any{"changed": len(changes) > 0, "changes": changes, "plan": plan}, false), nil
		},
	}
}
