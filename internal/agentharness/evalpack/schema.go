// Package evalpack provides a versioned, deterministic evaluation harness for
// TARS agent-runtime behavior. The package deliberately separates execution
// success from expectation matching so a baseline can record known gaps without
// disguising them as successful agent work.
package evalpack

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const SchemaVersion = "1.0"

type Pack struct {
	SchemaVersion string     `json:"schema_version"`
	Scenarios     []Scenario `json:"scenarios"`
}

type Scenario struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Category      string            `json:"category"`
	Kind          string            `json:"kind"`
	Prompt        string            `json:"prompt"`
	SuccessToken  string            `json:"success_token"`
	LiveSupported bool              `json:"live_supported,omitempty"`
	Expected      ExpectedMetrics   `json:"expected"`
	Parameters    map[string]string `json:"parameters,omitempty"`
}

type ExpectedMetrics struct {
	TaskSuccess           bool `json:"task_success"`
	VerifierPass          bool `json:"verifier_pass"`
	RestartRecovered      bool `json:"restart_recovered"`
	DuplicateSideEffects  int  `json:"duplicate_side_effects"`
	OperatorInterventions int  `json:"operator_interventions"`
}

func LoadPack(path string) (Pack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Pack{}, fmt.Errorf("read agent harness pack %q: %w", path, err)
	}
	var pack Pack
	if err := json.Unmarshal(data, &pack); err != nil {
		return Pack{}, fmt.Errorf("decode agent harness pack %q: %w", path, err)
	}
	if err := ValidatePack(pack); err != nil {
		return Pack{}, fmt.Errorf("validate agent harness pack %q: %w", path, err)
	}
	return pack, nil
}

func ValidatePack(pack Pack) error {
	if strings.TrimSpace(pack.SchemaVersion) != SchemaVersion {
		return fmt.Errorf("schema_version must be %q", SchemaVersion)
	}
	if len(pack.Scenarios) == 0 {
		return fmt.Errorf("at least one scenario is required")
	}
	seen := make(map[string]struct{}, len(pack.Scenarios))
	for i, scenario := range pack.Scenarios {
		prefix := fmt.Sprintf("scenario[%d]", i)
		id := strings.TrimSpace(scenario.ID)
		if id == "" {
			return fmt.Errorf("%s id is required", prefix)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate scenario id %q", id)
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(scenario.Title) == "" {
			return fmt.Errorf("%s title is required", prefix)
		}
		if strings.TrimSpace(scenario.Category) == "" {
			return fmt.Errorf("%s category is required", prefix)
		}
		if strings.TrimSpace(scenario.Kind) == "" {
			return fmt.Errorf("%s kind is required", prefix)
		}
		if strings.TrimSpace(scenario.Prompt) == "" {
			return fmt.Errorf("%s prompt is required", prefix)
		}
		if strings.TrimSpace(scenario.SuccessToken) == "" {
			return fmt.Errorf("%s success_token is required", prefix)
		}
		if scenario.Expected.DuplicateSideEffects < 0 {
			return fmt.Errorf("%s expected duplicate_side_effects must be non-negative", prefix)
		}
		if scenario.Expected.OperatorInterventions < 0 {
			return fmt.Errorf("%s expected operator_interventions must be non-negative", prefix)
		}
	}
	return nil
}
