//go:build integration

package tarsserver

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/computeruse"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/pkg/llm"
)

// Uses the existing configured LLM credentials and real cua-driver daemon.
// TARS_COMPUTER_USE_LIVE_CONFIG=<config> CUA_DRIVER_PATH=<driver> go test -tags integration ./internal/tarsserver -run TestLiveComputerUseLight -v
func TestLiveComputerUseLight(t *testing.T) {
	path := os.Getenv("TARS_COMPUTER_USE_LIVE_CONFIG")
	if path == "" {
		t.Skip("TARS_COMPUTER_USE_LIVE_CONFIG is required")
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.WorkspaceDir = t.TempDir()
	cfg.ToolsComputerUseEnabled = true
	cfg.ToolsComputerUseBackend = "llm"
	router, err := buildLLMRouter(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	found := false
	for _, tool := range buildOptionalChatTools(cfg, nil, router, nil) {
		if tool.Name != "computer_use" {
			continue
		}
		found = true
		out, err := tool.Execute(ctx, json.RawMessage(`{"goal":"Clear the calculator, then press 2, +, 3, = so its current result is 5. Finish only when 5 is visible.","app":"Calculator","max_steps":15}`))
		if err != nil {
			t.Fatal(err)
		}
		t.Log(out.Content[0].Text)
		var res computeruse.Result
		if err := json.Unmarshal([]byte(out.Content[0].Text), &res); err != nil {
			t.Fatal(err)
		}
		// The test goal explicitly authorizes clearing Calculator's current
		// number. Exercise the confirmation flow without approving any other
		// control or weakening the production risk gate.
		if res.Status == computeruse.StatusNeedsConfirmation {
			if res.ProposedAction == nil || res.ProposedAction.Op != "click" ||
				(res.ProposedAction.Target != "AXButton '모두 지우기'" && res.ProposedAction.Target != "AXButton 'All Clear'" && res.ProposedAction.Target != "AXButton 'Clear'") {
				t.Fatalf("unexpected confirmation: %+v", res.ProposedAction)
			}
			args, err := json.Marshal(map[string]any{"resume": res.Resume, "confirm": true})
			if err != nil {
				t.Fatal(err)
			}
			out, err = tool.Execute(ctx, args)
			if err != nil {
				t.Fatal(err)
			}
			t.Log(out.Content[0].Text)
			if err := json.Unmarshal([]byte(out.Content[0].Text), &res); err != nil {
				t.Fatal(err)
			}
		}
		if res.Status != computeruse.StatusDone {
			t.Fatalf("computer use did not finish: %s %s", res.Status, res.Reason)
		}
		if res.Usage.Backend != "llm" || res.Usage.InputTokens == 0 {
			t.Fatalf("wrong backend or missing metering: %+v", res.Usage)
		}

		driver := computeruse.NewLazyCuaDriver(cfg.ToolsComputerUseCuaDriverPath, 15*time.Second)
		window, err := driver.ResolveWindow(ctx, "Calculator")
		if err != nil {
			t.Fatal(err)
		}
		observed, err := driver.Snapshot(ctx, window, computeruse.SnapshotOpts{})
		if err != nil {
			t.Fatal(err)
		}
		display := false
		for _, text := range observed.Texts {
			if strings.HasPrefix(text, `AXStaticText = "5"`) {
				display = true
			}
		}
		t.Logf("independent final display observation: %v", observed.Texts)
		if !display {
			t.Fatal("model claimed done but independent Calculator display is not 5")
		}
	}
	if !found {
		t.Fatal("computer_use missing")
	}
}

// This test sends only the synthetic observations below, never reads the host
// GUI, and never executes a driver action. It checks configured provider/auth
// compatibility separately from OS permissions and real-screen export.
func TestConfiguredLightRecordedObservation(t *testing.T) {
	path := os.Getenv("TARS_COMPUTER_USE_MODEL_CONFIG")
	if path == "" {
		t.Skip("TARS_COMPUTER_USE_MODEL_CONFIG is required")
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.WorkspaceDir = t.TempDir()
	router, err := buildLLMRouter(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, resolution, err := router.ClientFor(llm.RoleComputerUse)
	if err != nil {
		t.Fatal(err)
	}
	backend := computeruse.NewLLMBackend(client, resolution.Model, nil)
	screen := computeruse.Snapshot{Window: computeruse.Window{App: "Synthetic Calculator"}, Elements: []computeruse.Element{
		{Index: 1, Role: "AXButton", Label: "2", Enabled: true},
		{Index: 2, Role: "AXButton", Label: "Clear", Enabled: true},
	}, Texts: []string{`AXStaticText = "0"`}}
	request := computeruse.Request{Goal: "Press 2 so the calculator display is 2. Finish only when the current display is 2."}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	state, shown := computeruse.RenderState(request, screen, nil, computeruse.RenderOptions{ExposeValues: true})
	d, spent, err := backend.Decide(ctx, state, computeruse.BuildQuestions(shown, nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("configured model=%s tier=%s decision=%+v usage=%+v", resolution.Model, resolution.Tier, d, spent)
	if d.Op != computeruse.OpClick || d.TargetIndex != 1 || d.Probabilistic || spent.InputTokens == 0 {
		t.Fatalf("incorrect synthetic action: %+v usage=%+v", d, spent)
	}
	screen.Texts = []string{`AXStaticText = "2"`}
	state, shown = computeruse.RenderState(request, screen, []computeruse.TraceStep{{Step: 1, Op: "click", Target: "AXButton '2'", Effect: "confirmed"}}, computeruse.RenderOptions{ExposeValues: true})
	d, spent, err = backend.Decide(ctx, state, computeruse.BuildQuestions(shown, nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("synthetic final observation decision=%+v usage=%+v", d, spent)
	if d.Op != computeruse.OpDone || d.Done != 1 {
		t.Fatalf("visible completion not recognized: %+v", d)
	}
}
