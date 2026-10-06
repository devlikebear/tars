package apptool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/computeruse"
)

// ComputerUseEngine is the part of *computeruse.Engine the tool drives.
type ComputerUseEngine interface {
	Run(ctx context.Context, req computeruse.Request) computeruse.Result
	Resume(ctx context.Context, token string, confirm bool) computeruse.Result
}

const computerUseMaxSteps = 50

// NewComputerUseTool is the single chat entry point to the GUI loop (#973).
// The model states a goal once; every step after that is decided by the
// configured light LLM or optional System One backend.
func NewComputerUseTool(engine ComputerUseEngine, enabled bool) Tool {
	return Tool{
		Name: "computer_use",
		Description: "Drive a desktop app's GUI toward a goal by reading its accessibility tree and clicking, typing and scrolling. " +
			"State the goal as one sentence describing the end state; the loop picks each step itself. " +
			"Text to type must be passed in inputs (name → text) and referred to by name in the goal; it cannot invent text. " +
			"The window's on-screen text is sent to the configured LLM provider by default, or the System One server when the Jev backend is selected; input values are not. " +
			"An action that looks hard to undo (delete, send, pay, change settings) stops with status needs_confirmation, a proposed_action and a resume token: " +
			"show the proposed action to the user, and only after they answer call again with resume and confirm (true runs it and continues, false cancels). " +
			"Other statuses: done, stuck, max_steps, cancelled, unavailable, error.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "goal":{"type":"string","description":"End state to reach, in one sentence. Required unless resume is set."},
    "app":{"type":"string","description":"App name or bundle id to drive. Omit to use the frontmost window."},
    "inputs":{"type":"object","additionalProperties":{"type":"string"},"description":"Named texts the loop may type, e.g. {\"query\":\"invoice 2026\"}."},
    "max_steps":{"type":"integer","minimum":1,"maximum":50},
    "resume":{"type":"string","description":"Resume token from a needs_confirmation result."},
    "confirm":{"type":"boolean","description":"With resume: true performs the proposed action, false cancels the run."}
  },
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if !enabled || engine == nil {
				return computerUseFailure(computeruse.StatusUnavailable, "computer_use is disabled", "set tools.computer_use.enabled: true"), nil
			}
			var input struct {
				Goal     string            `json:"goal,omitempty"`
				App      string            `json:"app,omitempty"`
				Inputs   map[string]string `json:"inputs,omitempty"`
				MaxSteps int               `json:"max_steps,omitempty"`
				Resume   string            `json:"resume,omitempty"`
				Confirm  *bool             `json:"confirm,omitempty"`
			}
			if len(params) > 0 {
				if err := json.Unmarshal(params, &input); err != nil {
					return computerUseFailure(computeruse.StatusError, fmt.Sprintf("invalid arguments: %v", err), ""), nil
				}
			}
			var res computeruse.Result
			if token := strings.TrimSpace(input.Resume); token != "" {
				// A missing confirm must not decide either way: true would act
				// without the user's answer, false would throw the run away.
				if input.Confirm == nil {
					return computerUseFailure(computeruse.StatusError, "confirm is required with resume", "pass confirm: true to perform the proposed action, false to cancel"), nil
				}
				res = engine.Resume(ctx, token, *input.Confirm)
			} else {
				goal := strings.TrimSpace(input.Goal)
				if goal == "" {
					return computerUseFailure(computeruse.StatusError, "goal or resume is required", ""), nil
				}
				if input.MaxSteps < 0 || input.MaxSteps > computerUseMaxSteps {
					return computerUseFailure(computeruse.StatusError, fmt.Sprintf("max_steps must be between 1 and %d", computerUseMaxSteps), ""), nil
				}
				res = engine.Run(ctx, computeruse.Request{
					Goal:     goal,
					App:      strings.TrimSpace(input.App),
					Inputs:   input.Inputs,
					MaxSteps: input.MaxSteps,
				})
			}
			isError := res.Status == computeruse.StatusUnavailable || res.Status == computeruse.StatusError
			return JSONTextResult(res, isError), nil
		},
	}
}

func computerUseFailure(status computeruse.Status, reason, hint string) Result {
	return JSONTextResult(computeruse.Result{Status: status, Reason: reason, Hint: hint, Trace: []computeruse.TraceStep{}}, true)
}
