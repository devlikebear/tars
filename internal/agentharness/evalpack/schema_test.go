package evalpack

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPackAcceptsCanonicalScenarios(t *testing.T) {
	pack, err := LoadPack(filepath.Join("..", "..", "..", "testdata", "agent-harness", "scenarios.json"))
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	if pack.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", pack.SchemaVersion, SchemaVersion)
	}
	if len(pack.Scenarios) < 10 {
		t.Fatalf("scenario count = %d, want at least 10", len(pack.Scenarios))
	}

	seenKinds := map[string]bool{}
	for _, scenario := range pack.Scenarios {
		seenKinds[scenario.Kind] = true
	}
	for _, kind := range []string{
		"single_agent",
		"parallel_fanout",
		"dependency_chain",
		"restart_recovery",
		"approval_gate",
		"false_success",
		"skill_reuse",
		"duplicate_side_effect",
	} {
		if !seenKinds[kind] {
			t.Errorf("canonical pack is missing scenario kind %q", kind)
		}
	}
}

func TestValidatePackRejectsDuplicateIDs(t *testing.T) {
	pack := Pack{
		SchemaVersion: SchemaVersion,
		Scenarios: []Scenario{
			validScenario("duplicate"),
			validScenario("duplicate"),
		},
	}
	if err := ValidatePack(pack); err == nil {
		t.Fatal("expected duplicate scenario id error")
	}
}

func TestValidatePackRejectsInvalidExpectation(t *testing.T) {
	scenario := validScenario("bad-effects")
	scenario.Expected.DuplicateSideEffects = -1
	if err := ValidatePack(Pack{SchemaVersion: SchemaVersion, Scenarios: []Scenario{scenario}}); err == nil {
		t.Fatal("expected negative duplicate-side-effect error")
	}
}

func TestLoadPackReportsReadDecodeAndValidationErrors(t *testing.T) {
	if _, err := LoadPack(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected read error")
	}
	root := t.TempDir()
	invalidJSON := filepath.Join(root, "invalid.json")
	if err := os.WriteFile(invalidJSON, []byte("{"), 0o600); err != nil {
		t.Fatalf("write invalid JSON: %v", err)
	}
	if _, err := LoadPack(invalidJSON); err == nil {
		t.Fatal("expected decode error")
	}
	invalidPack := filepath.Join(root, "invalid-pack.json")
	if err := os.WriteFile(invalidPack, []byte(`{"schema_version":"0","scenarios":[]}`), 0o600); err != nil {
		t.Fatalf("write invalid pack: %v", err)
	}
	if _, err := LoadPack(invalidPack); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidatePackRequiredFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Pack)
	}{
		{name: "schema", mutate: func(pack *Pack) { pack.SchemaVersion = "0" }},
		{name: "scenarios", mutate: func(pack *Pack) { pack.Scenarios = nil }},
		{name: "id", mutate: func(pack *Pack) { pack.Scenarios[0].ID = "" }},
		{name: "title", mutate: func(pack *Pack) { pack.Scenarios[0].Title = "" }},
		{name: "category", mutate: func(pack *Pack) { pack.Scenarios[0].Category = "" }},
		{name: "kind", mutate: func(pack *Pack) { pack.Scenarios[0].Kind = "" }},
		{name: "prompt", mutate: func(pack *Pack) { pack.Scenarios[0].Prompt = "" }},
		{name: "success token", mutate: func(pack *Pack) { pack.Scenarios[0].SuccessToken = "" }},
		{name: "interventions", mutate: func(pack *Pack) { pack.Scenarios[0].Expected.OperatorInterventions = -1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pack := Pack{SchemaVersion: SchemaVersion, Scenarios: []Scenario{validScenario("required")}}
			tt.mutate(&pack)
			if err := ValidatePack(pack); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func validScenario(id string) Scenario {
	return Scenario{
		ID:           id,
		Title:        "Valid scenario",
		Category:     "correctness",
		Kind:         "single_agent",
		Prompt:       "Return HARNESS_OK",
		SuccessToken: "HARNESS_OK",
		Expected: ExpectedMetrics{
			TaskSuccess:  true,
			VerifierPass: true,
		},
	}
}
