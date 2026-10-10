package tarsserver

import (
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/computeruse"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/jev"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/devlikebear/tars/pkg/llm"
)

// newComputerUseEngine builds the GUI loop behind the computer_use tool. It
// never fails: a missing decision backend or cua-driver binary leaves the
// tool registered and answering "unavailable" with a hint, and the binary is
// looked up per call so installing it needs no restart.
func newComputerUseEngine(cfg config.Config, router llm.Router, tracker *usage.Tracker) *computeruse.Engine {
	stepTimeout := time.Duration(cfg.ToolsComputerUseStepTimeoutSeconds) * time.Second
	engineCfg := computeruse.DefaultConfig()
	engineCfg.MaxSteps = cfg.ToolsComputerUseMaxSteps
	engineCfg.StepTimeout = stepTimeout
	engineCfg.TotalTimeout = time.Duration(cfg.ToolsComputerUseTotalTimeoutSeconds) * time.Second
	engineCfg.ExposeValues = cfg.ToolsComputerUseExposeValues

	driver := computeruse.NewLazyCuaDriver(cfg.ToolsComputerUseCuaDriverPath, stepTimeout)
	if cfg.ToolsComputerUseBackend == "" || cfg.ToolsComputerUseBackend == "llm" {
		if router == nil {
			return computeruse.NewEngineWithBackend(driver, nil, engineCfg)
		}
		client, resolution, err := router.ClientFor(llm.RoleComputerUse)
		if err != nil || !llm.SupportsDecisionOnly(resolution.Provider) {
			return computeruse.NewEngineWithBackend(driver, nil, engineCfg)
		}
		var cost computeruse.LLMCost
		if tracker != nil {
			cost = func(u llm.Usage) (float64, bool) {
				amount, known, _ := tracker.CallCost(resolution.Provider, resolution.Model, u)
				return amount, known
			}
		}
		return computeruse.NewEngineWithBackend(driver, computeruse.NewLLMBackend(client, resolution.Model, cost), engineCfg)
	}
	if cfg.ToolsComputerUseBackend != "jev" {
		return computeruse.NewEngineWithBackend(driver, nil, engineCfg)
	}
	base := strings.TrimSpace(cfg.Jev.BaseURL)
	if base == "" {
		// A nil Asker, not a typed nil: the engine reports "not configured"
		// before it touches the screen.
		return computeruse.NewEngine(driver, nil, engineCfg)
	}
	return computeruse.NewEngine(driver, jev.New(jev.Options{
		BaseURL: base,
		APIKey:  cfg.Jev.APIKey,
		Model:   cfg.Jev.Model,
		Timeout: time.Duration(cfg.Jev.TimeoutSeconds) * time.Second,
	}), engineCfg)
}
