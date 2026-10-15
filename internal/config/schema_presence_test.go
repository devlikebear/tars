package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// presenceFieldKeys lists the fields built with a *WithPresence helper. A
// field added with one of those helpers must be listed here so the tests
// below cover it.
var presenceFieldKeys = map[string]bool{
	"session_auto_archive_days":        true,
	"session_auto_delete_days":         true,
	"tools_computer_use_enabled":       true,
	"tools_computer_use_expose_values": true,
}

func TestPresenceFieldListIsComplete(t *testing.T) {
	// A presence field is one whose merge drops a value set on a plain
	// Config: setting a non-zero value without the presence flag must not
	// survive merge. Any field behaving that way must be listed above.
	for _, field := range configInputFields {
		if field.yamlKey == "" {
			continue
		}
		cfg := Default()
		applyDefaults(&cfg)
		var probe Config
		field.merge(&probe, cfg)
		dropped := !reflect.DeepEqual(extractValue(field.yamlKey, probe), extractValue(field.yamlKey, cfg))
		if dropped && !presenceFieldKeys[field.yamlKey] {
			t.Errorf("%s drops its default on merge; add it to presenceFieldKeys", field.yamlKey)
		}
	}
}

// TestSchemaPresenceFieldsMatchRuntimeWhenUnset: with the key absent from the
// config file, the schema's default, file and effective values must equal
// what the runtime uses.
func TestSchemaPresenceFieldsMatchRuntimeWhenUnset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tars.config.yaml")
	if err := os.WriteFile(path, []byte("log_level: info\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fileCfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	fileValues := ConfigToMap(fileCfg)
	runtimeValues := ConfigToMap(runtimeCfg)
	defaults := map[string]any{}
	for _, field := range Schema() {
		defaults[field.Key] = field.DefaultValue
	}

	want := map[string]any{
		"session_auto_archive_days":        runtimeCfg.SessionAutoArchiveDays,
		"session_auto_delete_days":         runtimeCfg.SessionAutoDeleteDays,
		"tools_computer_use_enabled":       runtimeCfg.ToolsComputerUseEnabled,
		"tools_computer_use_expose_values": runtimeCfg.ToolsComputerUseExposeValues,
	}
	if len(want) != len(presenceFieldKeys) {
		t.Fatalf("want covers %d fields, presenceFieldKeys has %d", len(want), len(presenceFieldKeys))
	}
	if want["session_auto_archive_days"] != 7 || want["session_auto_delete_days"] != 30 || want["tools_computer_use_enabled"] != true {
		t.Fatalf("runtime defaults changed: %v", want)
	}
	for key, w := range want {
		for name, values := range map[string]map[string]any{"default": defaults, "file": fileValues, "runtime": runtimeValues} {
			if got := values[key]; !reflect.DeepEqual(got, w) {
				t.Errorf("%s %s value = %#v, want %#v", key, name, got, w)
			}
		}
	}
}

// TestSchemaPresenceFieldsKeepExplicitZero: saving 0/false through the
// console (PatchYAML) must turn the setting off and show it as off.
func TestSchemaPresenceFieldsKeepExplicitZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tars.config.yaml")
	if err := os.WriteFile(path, []byte("log_level: info\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	updates := map[string]any{
		"session_auto_archive_days":        0,
		"session_auto_delete_days":         0,
		"tools_computer_use_enabled":       false,
		"tools_computer_use_expose_values": false,
	}
	if err := PatchYAML(path, updates); err != nil {
		t.Fatal(err)
	}
	fileCfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtimeCfg.SessionAutoArchiveDays != 0 || runtimeCfg.SessionAutoDeleteDays != 0 || runtimeCfg.ToolsComputerUseEnabled || runtimeCfg.ToolsComputerUseExposeValues {
		t.Fatalf("runtime did not keep explicit zeros: %+v", runtimeCfg)
	}
	for name, values := range map[string]map[string]any{"file": ConfigToMap(fileCfg), "runtime": ConfigToMap(runtimeCfg)} {
		for key, w := range updates {
			if got := values[key]; !reflect.DeepEqual(got, w) {
				t.Errorf("%s %s value = %#v, want %#v", key, name, got, w)
			}
		}
	}
}
