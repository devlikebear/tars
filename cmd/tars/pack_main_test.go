package main

import (
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/skillhub"
)

func TestPrintPackInstallPlanShowsReviewedAction(t *testing.T) {
	plan := skillhub.PackInstallPlan{
		Pack: skillhub.PackEntry{
			Name:        "github-maintainer-pack",
			Version:     "0.1.0",
			Description: "GitHub maintainer workflow bundle",
		},
		Items: []skillhub.PackInstallItem{
			{Type: "plugin", Name: "project-swarm", Version: "0.7.0", Action: "install"},
			{Type: "mcp", Name: "filesystem", Version: "0.1.0", Action: "install"},
			{Type: "skill", Name: "project-start", Version: "0.6.0", Action: "install"},
		},
	}

	var out strings.Builder
	printPackInstallPlan(&out, plan)
	got := out.String()
	for _, want := range []string{
		"Pack: github-maintainer-pack@0.1.0",
		"Install plan:",
		"[install] plugin project-swarm@0.7.0",
		"[install] mcp filesystem@0.1.0",
		"[install] skill project-start@0.6.0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, got)
		}
	}
}

func TestConfirmPackInstallDefaultsToNo(t *testing.T) {
	var out strings.Builder
	ok, err := confirmPackInstall(strings.NewReader("\n"), &out)
	if err != nil {
		t.Fatalf("confirmPackInstall: %v", err)
	}
	if ok {
		t.Fatal("expected blank confirmation to cancel")
	}
	if !strings.Contains(out.String(), "Install this pack? [y/N]") {
		t.Fatalf("expected confirmation prompt, got %q", out.String())
	}
}

func TestConfirmPackInstallAcceptsYes(t *testing.T) {
	ok, err := confirmPackInstall(strings.NewReader("yes\n"), &strings.Builder{})
	if err != nil {
		t.Fatalf("confirmPackInstall: %v", err)
	}
	if !ok {
		t.Fatal("expected yes confirmation")
	}
}
