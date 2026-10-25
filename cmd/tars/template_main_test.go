package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/skillhub"
)

func TestTemplateCobra_SearchInstallListUpdateUninstall(t *testing.T) {
	tmpl := []byte("id: hotfix\nname: Hotfix\nstages:\n  - id: plan\n  - id: build\n")
	sum := sha256.Sum256(tmpl)
	mux := http.NewServeMux()
	mux.HandleFunc("/registry.json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(skillhub.RegistryIndex{Version: 3, FocusTemplates: []skillhub.FocusTemplateEntry{{
			Name: "hotfix", Description: "Small fixes, fast", Version: "0.1.0", Author: "devlikebear",
			Tags: []string{"development"}, Path: "focus-templates/hotfix",
			Files: skillhub.RegistryFiles{{Path: "template.yaml", SHA256: hex.EncodeToString(sum[:])}},
		}}})
	})
	mux.HandleFunc("/focus-templates/hotfix/template.yaml", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tmpl) })
	hub := httptest.NewServer(mux)
	defer hub.Close()
	factory := func(workspaceDir string) *skillhub.Installer {
		inst := skillhub.NewInstaller(workspaceDir)
		inst.Registry.RegistryURL = hub.URL + "/registry.json"
		inst.Registry.SkillBaseURL = hub.URL
		return inst
	}
	workspace := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		var stdout bytes.Buffer
		cmd := newTemplateCommandWithFactory(&stdout, &stdout, factory)
		cmd.SetOut(&stdout)
		cmd.SetErr(&stdout)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("tars template %v: %v\n%s", args, err, stdout.String())
		}
		return stdout.String()
	}
	ws := []string{"--workspace-dir", workspace}

	for _, want := range []string{"hotfix@0.1.0 (development)", "Small fixes, fast", "1 focus template(s) found."} {
		if out := run("search", "fix"); !strings.Contains(out, want) {
			t.Fatalf("search output missing %q:\n%s", want, out)
		}
	}
	if out := run("search", "nothing-matches"); !strings.Contains(out, "No focus templates found.") {
		t.Fatalf("empty search: %s", out)
	}
	if out := run("info", "hotfix"); !strings.Contains(out, "Author:      devlikebear") || !strings.Contains(out, "Files:       template.yaml") {
		t.Fatalf("info: %s", out)
	}
	if out := run("list", ws[0], ws[1]); !strings.Contains(out, "No hub focus templates installed.") {
		t.Fatalf("empty list: %s", out)
	}
	run(append([]string{"install", "hotfix"}, ws...)...)
	if _, err := os.Stat(filepath.Join(workspace, "focus-templates", "hotfix.yaml")); err != nil {
		t.Fatalf("template not installed: %v", err)
	}
	if out := run("list", ws[0], ws[1]); !strings.Contains(out, "hotfix@0.1.0  (tars-hub)  focus-templates/hotfix.yaml") {
		t.Fatalf("list: %s", out)
	}
	if out := run("update", ws[0], ws[1]); !strings.Contains(out, "hotfix (up to date)") {
		t.Fatalf("update: %s", out)
	}
	run(append([]string{"uninstall", "hotfix"}, ws...)...)
	if _, err := os.Stat(filepath.Join(workspace, "focus-templates", "hotfix.yaml")); !os.IsNotExist(err) {
		t.Fatalf("template still present: %v", err)
	}
}

func TestPrintPackEntryListsFocusTemplates(t *testing.T) {
	var out bytes.Buffer
	printPackEntry(&out, skillhub.PackEntry{Name: "fast", Version: "0.1.0", FocusTemplates: []string{"hotfix", "pc_maintenance"}})
	if !strings.Contains(out.String(), "Templates:   hotfix, pc_maintenance") {
		t.Fatalf("pack info: %s", out.String())
	}
}
