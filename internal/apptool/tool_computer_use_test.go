package apptool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/devlikebear/tars/internal/computeruse"
)

type fakeComputerUseEngine struct {
	runs    []computeruse.Request
	resumes []string
	confirm []bool
	result  computeruse.Result
}

func (f *fakeComputerUseEngine) Run(_ context.Context, req computeruse.Request) computeruse.Result {
	f.runs = append(f.runs, req)
	return f.result
}

func (f *fakeComputerUseEngine) Resume(_ context.Context, token string, confirm bool) computeruse.Result {
	f.resumes = append(f.resumes, token)
	f.confirm = append(f.confirm, confirm)
	return f.result
}

func runComputerUse(t *testing.T, tl Tool, args string) (computeruse.Result, bool) {
	t.Helper()
	res, err := tl.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(res.Content))
	}
	var out computeruse.Result
	if err := json.Unmarshal([]byte(res.Content[0].Text), &out); err != nil {
		t.Fatalf("payload is not a computeruse.Result: %v\n%s", err, res.Content[0].Text)
	}
	return out, res.IsError
}

func TestNewComputerUseTool_Shape(t *testing.T) {
	assertToolShape(t, NewComputerUseTool(&fakeComputerUseEngine{}, true), "computer_use")
}

func TestComputerUseTool_DisabledOrUnwiredIsUnavailable(t *testing.T) {
	for name, tl := range map[string]Tool{
		"disabled":  NewComputerUseTool(&fakeComputerUseEngine{}, false),
		"no engine": NewComputerUseTool(nil, true),
	} {
		out, isErr := runComputerUse(t, tl, `{"goal":"open settings"}`)
		if !isErr || out.Status != computeruse.StatusUnavailable || out.Hint == "" {
			t.Errorf("%s: got %+v isError=%t, want unavailable error with a hint", name, out, isErr)
		}
	}
}

func TestComputerUseTool_RunPassesRequestThrough(t *testing.T) {
	eng := &fakeComputerUseEngine{result: computeruse.Result{Status: computeruse.StatusDone, Steps: 3, Trace: []computeruse.TraceStep{}}}
	out, isErr := runComputerUse(t, NewComputerUseTool(eng, true),
		`{"goal":"  make a folder named by the input  ","app":" Finder ","inputs":{"name":"Reports"},"max_steps":7}`)
	if isErr || out.Status != computeruse.StatusDone || out.Steps != 3 {
		t.Fatalf("got %+v isError=%t", out, isErr)
	}
	if len(eng.runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(eng.runs))
	}
	got := eng.runs[0]
	if got.Goal != "make a folder named by the input" || got.App != "Finder" || got.MaxSteps != 7 || got.Inputs["name"] != "Reports" {
		t.Fatalf("request = %+v", got)
	}
}

func TestComputerUseTool_RejectsBadArguments(t *testing.T) {
	cases := map[string]string{
		"no goal or resume":      `{}`,
		"blank goal":             `{"goal":"   "}`,
		"max_steps over the cap": `{"goal":"g","max_steps":51}`,
		"negative max_steps":     `{"goal":"g","max_steps":-1}`,
		"resume without confirm": `{"resume":"cu_abc"}`,
		"malformed":              `{"goal":`,
		"inputs not strings":     `{"goal":"g","inputs":{"n":1}}`,
	}
	for name, args := range cases {
		eng := &fakeComputerUseEngine{}
		out, isErr := runComputerUse(t, NewComputerUseTool(eng, true), args)
		if !isErr || out.Status != computeruse.StatusError || out.Reason == "" {
			t.Errorf("%s: got %+v isError=%t, want an error payload", name, out, isErr)
		}
		if len(eng.runs)+len(eng.resumes) != 0 {
			t.Errorf("%s: the engine was called", name)
		}
	}
}

func TestComputerUseTool_ResumeForwardsTheUsersAnswer(t *testing.T) {
	for _, confirm := range []bool{true, false} {
		eng := &fakeComputerUseEngine{result: computeruse.Result{Status: computeruse.StatusCancelled, Trace: []computeruse.TraceStep{}}}
		args, _ := json.Marshal(map[string]any{"resume": " cu_abc ", "confirm": confirm, "goal": "ignored"})
		if _, isErr := runComputerUse(t, NewComputerUseTool(eng, true), string(args)); isErr {
			t.Fatalf("confirm=%t: cancelled is not an error payload", confirm)
		}
		if len(eng.runs) != 0 || len(eng.resumes) != 1 || eng.resumes[0] != "cu_abc" || eng.confirm[0] != confirm {
			t.Fatalf("confirm=%t: runs=%v resumes=%v confirm=%v", confirm, eng.runs, eng.resumes, eng.confirm)
		}
	}
}

func TestComputerUseTool_EngineFailuresAreErrorPayloads(t *testing.T) {
	for status, wantErr := range map[computeruse.Status]bool{
		computeruse.StatusUnavailable:       true,
		computeruse.StatusError:             true,
		computeruse.StatusStuck:             false,
		computeruse.StatusMaxSteps:          false,
		computeruse.StatusNeedsConfirmation: false,
	} {
		eng := &fakeComputerUseEngine{result: computeruse.Result{Status: status, Trace: []computeruse.TraceStep{}}}
		out, isErr := runComputerUse(t, NewComputerUseTool(eng, true), `{"goal":"g"}`)
		if out.Status != status || isErr != wantErr {
			t.Errorf("%s: status=%s isError=%t, want isError=%t", status, out.Status, isErr, wantErr)
		}
	}
}
