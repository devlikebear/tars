package tarsserver

import (
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/computeruse"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/jev"
)

// newComputerUseEngine builds the GUI loop behind the computer_use tool. It
// never fails: a missing System One server or cua-driver binary leaves the
// tool registered and answering "unavailable" with a hint, and the binary is
// looked up per call so installing it needs no restart.
func newComputerUseEngine(cfg config.Config) *computeruse.Engine {
	stepTimeout := time.Duration(cfg.ToolsComputerUseStepTimeoutSeconds) * time.Second
	engineCfg := computeruse.DefaultConfig()
	engineCfg.MaxSteps = cfg.ToolsComputerUseMaxSteps
	engineCfg.StepTimeout = stepTimeout
	engineCfg.TotalTimeout = time.Duration(cfg.ToolsComputerUseTotalTimeoutSeconds) * time.Second
	engineCfg.ExposeValues = cfg.ToolsComputerUseExposeValues

	driver := computeruse.NewLazyCuaDriver(cfg.ToolsComputerUseCuaDriverPath, stepTimeout)
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
