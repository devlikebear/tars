package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPatchRejectsInvalidWithoutChangingFile(t *testing.T) {
	for _, updates := range []map[string]any{{"unknown": true}, {"log_level": "invalid"}, {"pulse_enabled": "yes"}, {"agent_max_iterations": 1.5}, {"llm_providers": map[string]any{"bad": 42}}} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		original := []byte("log_level: info\n")
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
		if err := PatchYAML(path, updates); err == nil {
			t.Fatalf("accepted invalid input: %v", updates)
		}
		got, _ := os.ReadFile(path)
		if string(got) != string(original) {
			t.Fatal("changed file on failure")
		}
	}
}
func TestPatchMalformedSourcePreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("runtime: [\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := PatchYAML(path, map[string]any{"log_level": "debug"}); err == nil {
		t.Fatal("accepted malformed source")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Fatal("changed malformed source")
	}
}
func TestPatchPreservesPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("log_level: info\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := PatchYAML(path, map[string]any{"log_level": "debug"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != before.Mode().Perm() {
		t.Fatal("permissions changed")
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*"))
	if len(files) != 0 {
		t.Fatal("temporary files leaked")
	}
}
