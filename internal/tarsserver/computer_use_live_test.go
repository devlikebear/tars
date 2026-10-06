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
